// Package attention は self-attention(自己注意)を1ヘッド、tensor 編の行列演算だけで組む。
//
// attention は Transformer の心臓。「各トークンが、他のどのトークンにどれだけ注目するか」を
// 計算し、注目度に応じて情報を混ぜ合わせる。仕組みは3つの行列 Q, K, V に集約される:
//
//	Q(query): 「私は何を探している?」   各トークンが出す問い合わせ
//	K(key):   「私は何を持っている?」   各トークンが出す見出し
//	V(value): 「私の中身はこれ」        各トークンが出す実体
//
// トークン i の Q と、全トークンの K の内積で「i が誰にどれだけ注目するか」の重みを出し、
// その重みで V を混ぜる。式にすると softmax(Q·Kᵀ / √d)·V —— まさに行列積の塊。
package attention

import (
	"math"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region head
// Head は1つの attention ヘッド。Wq, Wk, Wv は入力を Q, K, V に変換する重み行列。
// (実物はこれらを学習で得るが、ここでは仕組みを見るため乱数 or 恒等で初期化する。)
type Head struct {
	Wq, Wk, Wv *tensor.Tensor
	dHead      int
}

// NewHead は dModel 次元の入力を dHead 次元の Q/K/V に写すヘッドを作る。
// 重みは決定的な擬似乱数で埋める(テストの再現性のため)。
func NewHead(dModel, dHead int) *Head {
	return &Head{
		Wq:    randMatrix(dModel, dHead, 1),
		Wk:    randMatrix(dModel, dHead, 2),
		Wv:    randMatrix(dModel, dHead, 3),
		dHead: dHead,
	}
}

// NewHeadIdentity は Wq=Wk=Wv=単位行列のヘッド(スケーリング等の検証用)。
func NewHeadIdentity(d int) *Head {
	return &Head{Wq: identity(d), Wk: identity(d), Wv: identity(d), dHead: d}
}

// #endregion head

// #region attention
// rawScores は Q·Kᵀ / √dHead を返す(softmax 前のスコア = 注目の強さの生値)。
// √dHead で割るのは、次元が大きいと内積が大きくなりすぎて softmax が尖りすぎるのを防ぐため。
func (h *Head) rawScores(x *tensor.Tensor) *tensor.Tensor {
	q := tensor.MatMul(x, h.Wq) // (seq, dHead)
	k := tensor.MatMul(x, h.Wk) // (seq, dHead)
	kt := transpose(k)          // (dHead, seq)
	scores := tensor.MatMul(q, kt)

	scale := float32(1.0 / math.Sqrt(float64(h.dHead)))
	for i := range scores.Data {
		scores.Data[i] *= scale
	}
	return scores
}

// forwardWithWeights は attention の出力と、注目の重み(softmax後)を返す。
// causal=true なら因果マスクをかけ、各トークンが未来を見ないようにする。
func (h *Head) forwardWithWeights(x *tensor.Tensor, causal bool) (out, weights *tensor.Tensor) {
	scores := h.rawScores(x)

	if causal {
		// 上三角(j > i = 未来)を -Inf にする。softmax で 0 になり、注目が消える。
		negInf := float32(math.Inf(-1))
		for i := 0; i < scores.Rows; i++ {
			for j := i + 1; j < scores.Cols; j++ {
				scores.Set(i, j, negInf)
			}
		}
	}

	weights = tensor.SoftmaxRows(scores) // 各行を確率分布に
	v := tensor.MatMul(x, h.Wv)          // (seq, dHead)
	out = tensor.MatMul(weights, v)      // 重み付きで V を混ぜる
	return out, weights
}

// Forward は attention の出力だけを返す。
func (h *Head) Forward(x *tensor.Tensor, causal bool) *tensor.Tensor {
	out, _ := h.forwardWithWeights(x, causal)
	return out
}

// #endregion attention

// #region helpers
// transpose は行列を転置する。
func transpose(t *tensor.Tensor) *tensor.Tensor {
	out := tensor.New(t.Cols, t.Rows)
	for r := 0; r < t.Rows; r++ {
		for c := 0; c < t.Cols; c++ {
			out.Set(c, r, t.At(r, c))
		}
	}
	return out
}

func identity(d int) *tensor.Tensor {
	m := tensor.New(d, d)
	for i := 0; i < d; i++ {
		m.Set(i, i, 1)
	}
	return m
}

// randMatrix は seed から決定的に埋めた小さな重み行列を返す(学習の代わり)。
func randMatrix(rows, cols int, seed uint64) *tensor.Tensor {
	m := tensor.New(rows, cols)
	s := seed*2862933555777941757 + 3037000493
	for i := range m.Data {
		s = s*6364136223846793005 + 1442695040888963407
		// [-0.5, 0.5) の小さな値に落とす。
		m.Data[i] = float32(int64(s>>40)%1000)/1000 - 0.5
	}
	return m
}

// #endregion helpers
