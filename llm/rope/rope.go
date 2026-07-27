// Package rope は Transformer の位置エンコーディングを実装する。
//
// attention はトークンの順序を見ないので、位置情報を外から注入する必要がある。
// 古典は「位置ごとのベクトルを埋め込みに足す」絶対位置方式(Sinusoidal)。
// 現在の主流は RoPE(rotary position embedding)で、Q と K を位置に応じて
// 回転させる。回転どうしの内積は角度の差だけで決まるため、attention スコアが
// 相対位置のみに依存するようになる。
package rope

import (
	"errors"
	"math"
)

// #region sinusoidal

// Sinusoidal は "Attention Is All You Need" の絶対位置エンコーディングを返す。
// 各位置 pos の偶数次元に sin(pos/10000^(2i/d))、奇数次元に cos を置き、
// これをトークン埋め込みに足す。mini-GPT の学習型位置埋め込みの学習不要版。
func Sinusoidal(seqLen, dim int) [][]float64 {
	pe := make([][]float64, seqLen)
	for pos := 0; pos < seqLen; pos++ {
		row := make([]float64, dim)
		for i := 0; i+1 < dim; i += 2 {
			angle := float64(pos) / math.Pow(10000, float64(i)/float64(dim))
			row[i] = math.Sin(angle)
			row[i+1] = math.Cos(angle)
		}
		pe[pos] = row
	}
	return pe
}

// #endregion sinusoidal

// #region rope

// RoPE は次元をペア (x0,x1), (x2,x3), ... に分け、ペア i を
// 角度 pos·freqs[i] だけ回転させる。freqs は先頭ペアほど高周波
// (1 位置で大きく回る = 近距離を細かく見る)、末尾ほど低周波
// (ゆっくり回る = 遠距離の大まかな位置を持つ)。
type RoPE struct {
	dim   int
	freqs []float64
}

// New は dim 次元(偶数)用の RoPE を作る。freqs[i] = 10000^(-2i/dim)。
func New(dim int) (*RoPE, error) {
	if dim <= 0 || dim%2 != 0 {
		return nil, errors.New("rope: dim must be positive and even")
	}
	freqs := make([]float64, dim/2)
	for i := range freqs {
		freqs[i] = math.Pow(10000, -2*float64(i)/float64(dim))
	}
	return &RoPE{dim: dim, freqs: freqs}, nil
}

// Apply はベクトル x を位置 pos の回転にかけた新しいベクトルを返す。
// 回転は直交変換なので長さを変えず、同じ pos なら常に同じ回転になる。
// attention では Q と K にだけ適用する(V は回さない)。
func (r *RoPE) Apply(x []float64, pos int) []float64 {
	return r.applyAt(x, float64(pos))
}

func (r *RoPE) applyAt(x []float64, pos float64) []float64 {
	out := make([]float64, len(x))
	copy(out, x)
	for i := 0; i*2+1 < len(x) && i < len(r.freqs); i++ {
		theta := pos * r.freqs[i]
		sin, cos := math.Sin(theta), math.Cos(theta)
		a, b := x[i*2], x[i*2+1]
		out[i*2] = a*cos - b*sin
		out[i*2+1] = a*sin + b*cos
	}
	return out
}

// #endregion rope

// #region interp

// ApplyInterpolated は位置補間(position interpolation)つきの適用。
// 位置を factor で割ってから回すことで、学習時より長い系列でも
// 角度が学習済みのレンジに収まる。長コンテキスト化の最も素朴な形。
func (r *RoPE) ApplyInterpolated(x []float64, pos int, factor float64) []float64 {
	return r.applyAt(x, float64(pos)/factor)
}

// #endregion interp
