// Package layers は Transformer ブロックの中の細かい改良、RMSNorm と SwiGLU を
// 実装する。
//
// mini-GPT は LayerNorm と GELU を使った(tensor 編)。実物のオープンモデルは
// ほぼ共通して RMSNorm と SwiGLU に置き換えている。どちらも「小さいが層数ぶん
// 積み重なる」改良で、RMSNorm は平均を引く工程を丸ごと省いて速くし、
// SwiGLU は FFN にゲート(通す量を入力ごとに決める係数)を足して質を上げる。
package layers

import (
	"math"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region rmsnorm

// RMSNorm は各行を RMS(二乗平均平方根)で割るだけの正規化。
// LayerNorm から「平均を引く」工程を省いた簡略版で、平均の計算と減算が
// 消えるぶん速い。スケールには不変(2 倍しても出力が同じ)だが、
// 平均を引かないのでシフトには不変でない。実務では品質差がほぼ出ないことが
// 分かり、Llama 以降のオープンモデルの標準になった。
func RMSNorm(x *tensor.Tensor, eps float32) *tensor.Tensor {
	out := tensor.New(x.Rows, x.Cols)
	for r := 0; r < x.Rows; r++ {
		sum := float32(0)
		for c := 0; c < x.Cols; c++ {
			v := x.At(r, c)
			sum += v * v
		}
		inv := 1 / float32(math.Sqrt(float64(sum/float32(x.Cols)+eps)))
		for c := 0; c < x.Cols; c++ {
			out.Set(r, c, x.At(r, c)*inv)
		}
	}
	return out
}

// #endregion rmsnorm

// #region swiglu

// SiLU は x·σ(x)。大きい正はほぼ素通し、大きい負はほぼ 0 で、
// その間が滑らかにつながる(GELU とよく似た形)。
func SiLU(x *tensor.Tensor) *tensor.Tensor {
	out := tensor.New(x.Rows, x.Cols)
	for i, v := range x.Data {
		sigmoid := 1 / (1 + float32(math.Exp(float64(-v))))
		out.Data[i] = v * sigmoid
	}
	return out
}

// SwiGLUFFN はゲート付きの FFN。通常の FFN が
//
//	act(x·W1)·W2
//
// なのに対し、SwiGLU は入力から「値」と「ゲート」を別々に作り、掛け合わせる:
//
//	( SiLU(x·W1) ⊙ x·W3 )·W2
//
// SiLU(x·W1) がゲートで、チャネルごとに「値をどれだけ通すか」を入力に応じて
// 決める。行列が 1 枚増えるぶん、同じパラメータ数なら中間次元を約 2/3 に
// 狭めて釣り合いを取るのが実物の流儀。
type SwiGLUFFN struct {
	W1 *tensor.Tensor // ゲート側 (dModel, dHidden)
	W3 *tensor.Tensor // 値側 (dModel, dHidden)
	W2 *tensor.Tensor // 出力射影 (dHidden, dModel)
}

// NewSwiGLUFFN は決定的な擬似乱数重みで FFN を作る。
func NewSwiGLUFFN(dModel, dHidden int) *SwiGLUFFN {
	return &SwiGLUFFN{
		W1: randMatrix(dModel, dHidden, 11),
		W3: randMatrix(dModel, dHidden, 13),
		W2: randMatrix(dHidden, dModel, 19),
	}
}

// Forward は SwiGLU FFN を適用する。
func (f *SwiGLUFFN) Forward(x *tensor.Tensor) *tensor.Tensor {
	gate := SiLU(tensor.MatMul(x, f.W1))
	value := tensor.MatMul(x, f.W3)
	h := tensor.New(gate.Rows, gate.Cols)
	for i := range h.Data {
		h.Data[i] = gate.Data[i] * value.Data[i]
	}
	return tensor.MatMul(h, f.W2)
}

// #endregion swiglu

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
