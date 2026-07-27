// Package gqa は attention のヘッド構成の変種 MHA / MQA / GQA を 1 つの実装で表す。
//
// マルチヘッド attention(MHA)はヘッドごとに独立の Q/K/V を持つ。だが生成時に
// キャッシュする K/V のメモリはヘッド数に比例して膨らみ、長文生成のボトルネックに
// なる。そこで K/V だけをヘッド間で共有するのが MQA(全ヘッドで 1 組)と
// GQA(グループごとに 1 組)。Q ヘッド数はそのままなので表現力の低下は小さく、
// KV キャッシュは NKVHeads/NHeads 倍まで縮む。
// 3 方式の違いは「Q ヘッドがどの K/V を引くか」の対応表だけで、
// attention の計算そのものは変わらない。
package gqa

import (
	"errors"
	"math"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region config

// Config は attention のヘッド構成。
// NKVHeads = NHeads で MHA、= 1 で MQA、その間が GQA になる。
type Config struct {
	DModel   int // 埋め込み次元
	NHeads   int // Q ヘッド数
	NKVHeads int // K/V ヘッド数(NHeads の約数)
}

// KVCacheFloats は系列長 seqLen まで生成したときにキャッシュする float 数。
// K と V の 2 本 × 系列長 × KV ヘッド数 × ヘッド次元。NHeads は現れない。
// これが「K/V を共有するとキャッシュが縮む」ことの式そのもの。
func (c Config) KVCacheFloats(seqLen int) int {
	return 2 * seqLen * c.NKVHeads * (c.DModel / c.NHeads)
}

// #endregion config

// Attention は GQA 一般形の causal self-attention。
type Attention struct {
	cfg   Config
	dHead int
	wq    []*tensor.Tensor // Q ヘッドごとの射影 (DModel, dHead)
	wk    []*tensor.Tensor // KV ヘッドごとの射影
	wv    []*tensor.Tensor
}

// New は構成を検証して Attention を作る。重みは決定的な擬似乱数で埋める。
func New(cfg Config) (*Attention, error) {
	if cfg.DModel <= 0 || cfg.NHeads <= 0 || cfg.NKVHeads <= 0 {
		return nil, errors.New("gqa: sizes must be positive")
	}
	if cfg.DModel%cfg.NHeads != 0 {
		return nil, errors.New("gqa: DModel must be divisible by NHeads")
	}
	if cfg.NKVHeads > cfg.NHeads || cfg.NHeads%cfg.NKVHeads != 0 {
		return nil, errors.New("gqa: NHeads must be a multiple of NKVHeads")
	}
	a := &Attention{cfg: cfg, dHead: cfg.DModel / cfg.NHeads}
	for h := 0; h < cfg.NHeads; h++ {
		a.wq = append(a.wq, randMatrix(cfg.DModel, a.dHead, uint64(100+h)))
	}
	for h := 0; h < cfg.NKVHeads; h++ {
		a.wk = append(a.wk, randMatrix(cfg.DModel, a.dHead, uint64(200+h)))
		a.wv = append(a.wv, randMatrix(cfg.DModel, a.dHead, uint64(300+h)))
	}
	return a, nil
}

// #region forward

// KVHeadFor は Q ヘッド h が共有する KV ヘッドの番号を返す。
// MHA なら h 自身、MQA なら常に 0、GQA ならグループ番号。
func (a *Attention) KVHeadFor(h int) int {
	return h / (a.cfg.NHeads / a.cfg.NKVHeads)
}

// Forward は causal self-attention を計算し (seq, DModel) を返す。
// K/V は KV ヘッド数ぶんしか作らず、各 Q ヘッドは対応表で共有先を引く。
func (a *Attention) Forward(x *tensor.Tensor) *tensor.Tensor {
	// K/V は KV ヘッドごとに 1 回だけ射影する(ここが共有の実体)。
	ks := make([]*tensor.Tensor, a.cfg.NKVHeads)
	vs := make([]*tensor.Tensor, a.cfg.NKVHeads)
	for h := 0; h < a.cfg.NKVHeads; h++ {
		ks[h] = tensor.MatMul(x, a.wk[h])
		vs[h] = tensor.MatMul(x, a.wv[h])
	}

	out := tensor.New(x.Rows, a.cfg.DModel)
	for h := 0; h < a.cfg.NHeads; h++ {
		q := tensor.MatMul(x, a.wq[h])
		kv := a.KVHeadFor(h)
		head := causalAttend(q, ks[kv], vs[kv], a.dHead)
		// ヘッド出力を (seq, DModel) の担当区画に連結する。
		for r := 0; r < head.Rows; r++ {
			for c := 0; c < head.Cols; c++ {
				out.Set(r, h*a.dHead+c, head.At(r, c))
			}
		}
	}
	return out
}

// causalAttend は softmax(Q·Kᵀ/√d + 因果マスク)·V。attention 編と同じ計算。
func causalAttend(q, k, v *tensor.Tensor, dHead int) *tensor.Tensor {
	scores := tensor.MatMul(q, transpose(k))
	scale := float32(1.0 / math.Sqrt(float64(dHead)))
	negInf := float32(math.Inf(-1))
	for i := 0; i < scores.Rows; i++ {
		for j := 0; j < scores.Cols; j++ {
			if j > i {
				scores.Set(i, j, negInf)
			} else {
				scores.Set(i, j, scores.At(i, j)*scale)
			}
		}
	}
	return tensor.MatMul(tensor.SoftmaxRows(scores), v)
}

// #endregion forward

// SetUniformKV は全 KV ヘッドの重みを KV ヘッド 0 のものに揃える(テスト用)。
// この状態では K/V の共有数を変えても出力が変わらないことが確かめられる。
func (a *Attention) SetUniformKV() {
	base := randMatrix(a.cfg.DModel, a.dHead, 200)
	baseV := randMatrix(a.cfg.DModel, a.dHead, 300)
	for h := range a.wk {
		a.wk[h] = base
		a.wv[h] = baseV
	}
}

func transpose(t *tensor.Tensor) *tensor.Tensor {
	out := tensor.New(t.Cols, t.Rows)
	for r := 0; r < t.Rows; r++ {
		for c := 0; c < t.Cols; c++ {
			out.Set(c, r, t.At(r, c))
		}
	}
	return out
}

// randMatrix は seed から決定的に [-0.5, 0.5) の値を敷いた行列を作る。
func randMatrix(rows, cols int, seed uint64) *tensor.Tensor {
	out := tensor.New(rows, cols)
	s := seed*6364136223846793005 + 1442695040888963407
	for i := range out.Data {
		s = s*6364136223846793005 + 1442695040888963407
		out.Data[i] = float32(s>>40)/float32(1<<24) - 0.5
	}
	return out
}
