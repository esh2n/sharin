// Package gpt は mini-GPT の forward pass を、tensor 編の行列演算だけで組んだもの。LLM 編の集大成。
//
// attention 編で作った1ヘッドの self-attention を、Transformer ブロックに仕立てる:
//
//	埋め込み(トークン→ベクトル) + 位置埋め込み
//	  ↓  (ブロックを NLayers 回繰り返す)
//	  ├─ マルチヘッド attention(複数ヘッドを並列に) + 残差
//	  └─ フィードフォワード(GELU 付き全結合) + 残差
//	  ↓
//	最後の LayerNorm → 語彙分布(logits)
//
// これで「トークン列 → 各位置での次トークンの logits」が計算できる。
// その logits を llm-sampling 編に渡せば、実際にテキストが生成される。
// ここまでの LLM 編がすべて合流する回。
package gpt

import (
	"math"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region config
// Config はモデルの形を決める。
type Config struct {
	VocabSize int // 語彙数
	DModel    int // 埋め込み次元(1トークンを表すベクトルの長さ)
	NHeads    int // attention ヘッド数(DModel を割り切ること)
	NLayers   int // Transformer ブロックの数
	DFF       int // フィードフォワードの中間次元
	MaxSeq    int // 扱える最大系列長(位置埋め込みのサイズ)
	Seed      uint64
}

// #endregion config

// #region model
type headWeights struct{ Wq, Wk, Wv *tensor.Tensor }

type block struct {
	heads  []headWeights
	Wo     *tensor.Tensor // マルチヘッドを束ねる出力射影
	W1, W2 *tensor.Tensor // フィードフォワード
	b1, b2 *tensor.Tensor
}

// Model は mini-GPT 本体。
type Model struct {
	cfg    Config
	tokEmb *tensor.Tensor // (VocabSize, DModel) トークン埋め込み表
	posEmb *tensor.Tensor // (MaxSeq, DModel) 位置埋め込み
	blocks []block
}

// New は Config からモデルを組み立てる(重みは決定的な擬似乱数で初期化)。
func New(cfg Config) *Model {
	if cfg.DModel%cfg.NHeads != 0 {
		panic("gpt: DModel must be divisible by NHeads")
	}
	if cfg.Seed == 0 {
		cfg.Seed = 1
	}
	rng := &rng{state: cfg.Seed}
	dHead := cfg.DModel / cfg.NHeads

	m := &Model{
		cfg:    cfg,
		tokEmb: randMat(cfg.VocabSize, cfg.DModel, rng),
		posEmb: randMat(cfg.MaxSeq, cfg.DModel, rng),
	}
	for l := 0; l < cfg.NLayers; l++ {
		b := block{
			Wo: randMat(cfg.DModel, cfg.DModel, rng),
			W1: randMat(cfg.DModel, cfg.DFF, rng),
			b1: randMat(1, cfg.DFF, rng),
			W2: randMat(cfg.DFF, cfg.DModel, rng),
			b2: randMat(1, cfg.DModel, rng),
		}
		for h := 0; h < cfg.NHeads; h++ {
			b.heads = append(b.heads, headWeights{
				Wq: randMat(cfg.DModel, dHead, rng),
				Wk: randMat(cfg.DModel, dHead, rng),
				Wv: randMat(cfg.DModel, dHead, rng),
			})
		}
		m.blocks = append(m.blocks, b)
	}
	return m
}

// #endregion model

// #region forward
// Forward はトークン列を受け、各位置での次トークンの logits (seq, VocabSize) を返す。
func (m *Model) Forward(tokens []int) *tensor.Tensor {
	seq := len(tokens)
	// 1. 埋め込み: 各トークンをベクトルにし、位置埋め込みを足す。
	//    位置を足すのは、attention 自体はトークンの順序を見ないから(順序情報の注入)。
	x := tensor.New(seq, m.cfg.DModel)
	for i, tok := range tokens {
		for c := 0; c < m.cfg.DModel; c++ {
			x.Set(i, c, m.tokEmb.At(tok, c)+m.posEmb.At(i, c))
		}
	}

	// 2. Transformer ブロックを重ねる。
	for i := range m.blocks {
		x = m.blocks[i].forward(x)
	}

	// 3. 最後の正規化 → 語彙への射影(埋め込み表を再利用 = weight tying)。
	x = tensor.LayerNorm(x, 1e-5)
	return tensor.MatMul(x, transpose(m.tokEmb)) // (seq, VocabSize)
}

// forward は1つの Transformer ブロック。pre-LN + 残差接続(GPT-2 方式)。
func (b *block) forward(x *tensor.Tensor) *tensor.Tensor {
	// マルチヘッド attention。入力を正規化してから attention、結果を元の x に足す(残差)。
	a := b.multiHead(tensor.LayerNorm(x, 1e-5))
	x = add(x, a)
	// フィードフォワード。同じく正規化 → FFN → 残差。
	f := b.feedForward(tensor.LayerNorm(x, 1e-5))
	x = add(x, f)
	return x
}

// multiHead は複数ヘッドの attention を並列に計算し、連結して Wo で射影する。
// 各ヘッドが別々の「注目のしかた」を学ぶ(ここでは別々の乱数重みを持つ)のが多頭の意味。
func (b *block) multiHead(x *tensor.Tensor) *tensor.Tensor {
	outs := make([]*tensor.Tensor, len(b.heads))
	for i, h := range b.heads {
		outs[i] = singleHead(x, h)
	}
	concat := concatCols(outs) // (seq, DModel)
	return tensor.MatMul(concat, b.Wo)
}

// singleHead は1ヘッドの因果 self-attention。attention 編と同じ softmax(QKᵀ/√d)·V。
func singleHead(x *tensor.Tensor, h headWeights) *tensor.Tensor {
	q := tensor.MatMul(x, h.Wq)
	k := tensor.MatMul(x, h.Wk)
	v := tensor.MatMul(x, h.Wv)

	scores := tensor.MatMul(q, transpose(k))
	scale := float32(1.0 / math.Sqrt(float64(h.Wq.Cols)))
	negInf := float32(math.Inf(-1))
	for i := 0; i < scores.Rows; i++ {
		for j := 0; j < scores.Cols; j++ {
			if j > i {
				scores.Set(i, j, negInf) // 因果マスク: 未来を見ない
			} else {
				scores.Set(i, j, scores.At(i, j)*scale)
			}
		}
	}
	weights := tensor.SoftmaxRows(scores)
	return tensor.MatMul(weights, v)
}

// feedForward は位置ごとの全結合 2層(GELU 付き)。attention が「混ぜる」のに対し、
// FFN は各トークンの中で非線形な変換をする。
func (b *block) feedForward(x *tensor.Tensor) *tensor.Tensor {
	h := tensor.AddRow(tensor.MatMul(x, b.W1), b.b1)
	h = tensor.GELU(h)
	return tensor.AddRow(tensor.MatMul(h, b.W2), b.b2)
}

// #endregion forward

// #region generate
// Generate はプロンプトに続けて n トークンを greedy(最大 logit)で生成する(自己回帰)。
// 1トークン生成するたびに、それを入力に足してまた forward する——これが「1語ずつ書く」動き。
func (m *Model) Generate(prompt []int, n int) []int {
	tokens := append([]int(nil), prompt...)
	for i := 0; i < n && len(tokens) < m.cfg.MaxSeq; i++ {
		logits := m.Forward(tokens)
		last := logits.Rows - 1 // 最後の位置の logits が「次のトークン」の予測
		next := argmaxRow(logits, last)
		tokens = append(tokens, next)
	}
	return tokens
}

func argmaxRow(t *tensor.Tensor, row int) int {
	best := 0
	for c := 1; c < t.Cols; c++ {
		if t.At(row, c) > t.At(row, best) {
			best = c
		}
	}
	return best
}

// #endregion generate

// #region helpers
func transpose(t *tensor.Tensor) *tensor.Tensor {
	out := tensor.New(t.Cols, t.Rows)
	for r := 0; r < t.Rows; r++ {
		for c := 0; c < t.Cols; c++ {
			out.Set(c, r, t.At(r, c))
		}
	}
	return out
}

// add は同じ形の2つの行列を要素ごとに足す(残差接続)。
func add(a, b *tensor.Tensor) *tensor.Tensor {
	out := tensor.New(a.Rows, a.Cols)
	for i := range a.Data {
		out.Data[i] = a.Data[i] + b.Data[i]
	}
	return out
}

// concatCols は複数の (seq, d) を横に連結して (seq, d*n) にする。
func concatCols(ts []*tensor.Tensor) *tensor.Tensor {
	rows := ts[0].Rows
	totalCols := 0
	for _, t := range ts {
		totalCols += t.Cols
	}
	out := tensor.New(rows, totalCols)
	for r := 0; r < rows; r++ {
		off := 0
		for _, t := range ts {
			for c := 0; c < t.Cols; c++ {
				out.Set(r, off+c, t.At(r, c))
			}
			off += t.Cols
		}
	}
	return out
}

// rng は決定的な線形合同法(学習の代わりに重みを埋める)。
type rng struct{ state uint64 }

func (r *rng) next() float32 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float32(int64(r.state>>40)%2000)/1000 - 1 // [-1, 1)
}

func randMat(rows, cols int, r *rng) *tensor.Tensor {
	m := tensor.New(rows, cols)
	scale := float32(0.02) // 小さな初期値(実物の初期化にならう)
	for i := range m.Data {
		m.Data[i] = r.next() * scale
	}
	return m
}

// #endregion helpers
