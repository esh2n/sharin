// Package tensor は LLM を Go で自作するための最小の行列演算ライブラリ。
//
// numpy に逃げず、matmul・softmax・layernorm・GELU を自分で書く——それ自体が
// 車輪の再発明の主旨。Transformer の forward pass は、結局この数本の関数の組み合わせ。
// 「attention は行列積の塊」を実感するための土台。
//
// 表現は単純に「float32 のスライス + 行数・列数」の2次元行列に絞る(実物は N 次元)。
package tensor

import "math"

// #region core
// Tensor は行優先(row-major)で並べた2次元行列。
// Data[r*Cols + c] が (r, c) 成分。
type Tensor struct {
	Rows, Cols int
	Data       []float32
}

// New はゼロ埋めの (rows, cols) 行列を作る。
func New(rows, cols int) *Tensor {
	return &Tensor{Rows: rows, Cols: cols, Data: make([]float32, rows*cols)}
}

// FromRows は2次元スライスから行列を作る。
func FromRows(rows [][]float32) *Tensor {
	r := len(rows)
	c := len(rows[0])
	t := New(r, c)
	for i := range rows {
		copy(t.Data[i*c:], rows[i])
	}
	return t
}

// At は (r, c) 成分を返す。
func (t *Tensor) At(r, c int) float32 { return t.Data[r*t.Cols+c] }

// Set は (r, c) 成分を書く。
func (t *Tensor) Set(r, c int, v float32) { t.Data[r*t.Cols+c] = v }

// #endregion core

// #region matmul
// MatMul は行列積 a·b。a が (m, k)、b が (k, n) なら結果は (m, n)。
// Transformer の計算のほとんどはこれ。attention も全結合層も、突き詰めれば MatMul。
func MatMul(a, b *Tensor) *Tensor {
	if a.Cols != b.Rows {
		panic("tensor: MatMul shape mismatch")
	}
	m, k, n := a.Rows, a.Cols, b.Cols
	out := New(m, n)
	for i := 0; i < m; i++ {
		for p := 0; p < k; p++ {
			aip := a.At(i, p)
			for j := 0; j < n; j++ {
				out.Data[i*n+j] += aip * b.At(p, j)
			}
		}
	}
	return out
}

// AddRow は各行に同じベクトル(バイアス)を足す(ブロードキャスト)。
// bias は (1, Cols)。全結合層の +b がこれ。
func AddRow(x, bias *Tensor) *Tensor {
	out := New(x.Rows, x.Cols)
	for r := 0; r < x.Rows; r++ {
		for c := 0; c < x.Cols; c++ {
			out.Set(r, c, x.At(r, c)+bias.At(0, c))
		}
	}
	return out
}

// #endregion matmul

// #region softmax
// SoftmaxRows は各行を確率分布に変換する(行ごとに独立)。
// llm-sampling 編の softmax と同じ。max 引きで overflow を防ぐ。
// attention で「どのトークンにどれだけ注目するか」を確率にするのに使う。
func SoftmaxRows(x *Tensor) *Tensor {
	out := New(x.Rows, x.Cols)
	for r := 0; r < x.Rows; r++ {
		maxv := float32(math.Inf(-1))
		for c := 0; c < x.Cols; c++ {
			if x.At(r, c) > maxv {
				maxv = x.At(r, c)
			}
		}
		var sum float32
		for c := 0; c < x.Cols; c++ {
			e := float32(math.Exp(float64(x.At(r, c) - maxv)))
			out.Set(r, c, e)
			sum += e
		}
		for c := 0; c < x.Cols; c++ {
			out.Set(r, c, out.At(r, c)/sum)
		}
	}
	return out
}

// #endregion softmax

// #region norm
// LayerNorm は各行を平均0・分散1に正規化する(行ごと)。
// Transformer の各層の入口で使い、値の大きさを揃えて学習・推論を安定させる。
// (実物はこの後に学習された gain/bias を掛けるが、ここでは正規化のみ。)
func LayerNorm(x *Tensor, eps float32) *Tensor {
	out := New(x.Rows, x.Cols)
	n := float32(x.Cols)
	for r := 0; r < x.Rows; r++ {
		var mean float32
		for c := 0; c < x.Cols; c++ {
			mean += x.At(r, c)
		}
		mean /= n
		var variance float32
		for c := 0; c < x.Cols; c++ {
			d := x.At(r, c) - mean
			variance += d * d
		}
		variance /= n
		std := float32(math.Sqrt(float64(variance) + float64(eps)))
		for c := 0; c < x.Cols; c++ {
			out.Set(r, c, (x.At(r, c)-mean)/std)
		}
	}
	return out
}

// GELU は活性化関数。ReLU の滑らかな版で、GPT や BERT が使う。
// tanh 近似式: 0.5x(1 + tanh(√(2/π)(x + 0.044715x³)))。
func GELU(x *Tensor) *Tensor {
	out := New(x.Rows, x.Cols)
	const c = 0.7978845608 // √(2/π)
	for i, v := range x.Data {
		v64 := float64(v)
		inner := c * (v64 + 0.044715*v64*v64*v64)
		out.Data[i] = float32(0.5 * v64 * (1 + math.Tanh(inner)))
	}
	return out
}

// #endregion norm
