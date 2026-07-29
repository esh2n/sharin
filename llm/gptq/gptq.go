// Package gptq は、丸めた誤差を捨てずに、まだ丸めていない重みへ配る量子化を実装する。
//
// [量子化](quant)の章では、各重みを最寄りの格子点に丸めた。1つずつ独立に丸めて、
// 出た誤差はそのまま捨てる。素朴だが、誤差は消えてなくなるわけではない。
//
// 層の出力は内積になる。y = Σ wᵢxᵢ なので、効いてくるのは重み1つずつの誤差ではなく、
// 入力を掛けて足し合わせたあとの誤差になる。個々の丸めが半格子以内でも、
// 同じ向きに揃えば積み上がる。
//
// だったら、丸めて出た誤差を捨てずに、まだ丸めていない重みへ肩代わりさせればよい。
// 前から順に1つずつ確定させ、確定するたびに残りを動かす。動かす量は
// 入力どうしの相関から決まる。よく似た動きをする2つの入力なら、片方の重みを
// 減らして他方を増やしても出力はほとんど変わるので、肩代わりが効く。
//
// 相関はヘッセ行列 H = X Xᵀ に入っている。その逆行列をコレスキー分解すると、
// i 行目がそのまま i 段目の配り方になる。分解1回で全段ぶんが手に入るので、
// なぞるのは1回で済む。相関を無視して対角だけにすると配る先が消え、
// 素朴な丸めに戻る。
//
// 実時間も乱数も使わない。入力は整数の線形合同法から作るので、結果は必ず再現する。
package gptq

import (
	"math"

	"github.com/esh2n/sharin/llm/quant"
)

// #region hessian

// Hessian は入力からヘッセ行列 H = X Xᵀ / n を作る。
//
// x は列が1回ぶんの入力。H[i][j] は入力 i と入力 j がどれくらい一緒に動くかで、
// ここが大きいほど、重み i の誤差を重み j に肩代わりさせやすい。
//
// damp は対角に足す下駄。相関が強すぎると逆行列が立たなくなるので、
// 対角の平均に対する割合で少しだけ持ち上げる(実物も同じことをする)。
func Hessian(x [][]float64, damp float64) [][]float64 {
	d := len(x)
	n := len(x[0])
	h := make([][]float64, d)
	for i := range h {
		h[i] = make([]float64, d)
		for j := 0; j < d; j++ {
			s := 0.0
			for k := 0; k < n; k++ {
				s += x[i][k] * x[j][k]
			}
			h[i][j] = s / float64(n)
		}
	}
	mean := 0.0
	for i := 0; i < d; i++ {
		mean += h[i][i]
	}
	mean /= float64(d)
	for i := 0; i < d; i++ {
		h[i][i] += damp * mean
	}
	return h
}

// Diagonal は非対角を落として対角だけ残す。
//
// 相関を無視したことになる。逆行列も対角だけになるので、配る先が消える。
func Diagonal(h [][]float64) [][]float64 {
	out := make([][]float64, len(h))
	for i := range h {
		out[i] = make([]float64, len(h))
		out[i][i] = h[i][i]
	}
	return out
}

// #endregion hessian

// #region cholesky

// CholeskyUpper は対称正定値な行列 A を A = Uᵀ U と分解して、上三角 U を返す。
//
// これが GPTQ の肝になる。順に確定させるたびに、残りの重みについての
// ヘッセ行列を作り直す必要があるのだが、素直にやると毎段で逆行列が要る。
// H⁻¹ をコレスキー分解しておくと、その i 行目がそのまま i 段目の配り方になる。
// 分解1回で全段ぶんが手に入るので、なぞるのは1回で済む。
func CholeskyUpper(a [][]float64) [][]float64 {
	d := len(a)
	l := make([][]float64, d)
	for i := range l {
		l[i] = make([]float64, d)
	}
	for i := 0; i < d; i++ {
		for j := 0; j <= i; j++ {
			s := a[i][j]
			for k := 0; k < j; k++ {
				s -= l[i][k] * l[j][k]
			}
			if i == j {
				if s <= 0 {
					s = 1e-12 // 数値のゆらぎで負に落ちたときの下限
				}
				l[i][i] = math.Sqrt(s)
				continue
			}
			l[i][j] = s / l[j][j]
		}
	}
	u := make([][]float64, d)
	for i := range u {
		u[i] = make([]float64, d)
		for j := 0; j < d; j++ {
			u[i][j] = l[j][i]
		}
	}
	return u
}

// Plan は入力から「各段でどう配るか」を作る。H⁻¹ のコレスキー分解になる。
func Plan(x [][]float64, damp float64) [][]float64 {
	return CholeskyUpper(Inverse(Hessian(x, damp)))
}

// #endregion cholesky

// Inverse は掃き出し法で逆行列を作る。
func Inverse(a [][]float64) [][]float64 {
	d := len(a)
	m := make([][]float64, d)
	for i := range m {
		m[i] = make([]float64, 2*d)
		copy(m[i], a[i])
		m[i][d+i] = 1
	}
	for c := 0; c < d; c++ {
		// 絶対値がいちばん大きい行を持ってくる(桁落ちを避ける)
		p := c
		for r := c + 1; r < d; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		m[c], m[p] = m[p], m[c]
		pivot := m[c][c]
		if pivot == 0 {
			continue
		}
		for k := 0; k < 2*d; k++ {
			m[c][k] /= pivot
		}
		for r := 0; r < d; r++ {
			if r == c || m[r][c] == 0 {
				continue
			}
			f := m[r][c]
			for k := 0; k < 2*d; k++ {
				m[r][k] -= f * m[c][k]
			}
		}
	}
	out := make([][]float64, d)
	for i := range out {
		out[i] = append([]float64(nil), m[i][d:]...)
	}
	return out
}

// #region quantize

// RoundToNearest は素朴な丸め。1つずつ独立に最寄りの格子点へ寄せ、誤差は捨てる。
func RoundToNearest(w []float64, bits int) []float64 {
	scale := scaleOf(w, bits)
	qmax := (1 << (bits - 1)) - 1
	out := make([]float64, len(w))
	for i, v := range w {
		out[i] = clamp(math.Round(v/scale), qmax) * scale
	}
	return out
}

// Quantize は前から順に1つずつ確定させ、出た誤差を残りへ配る。
//
// plan は Plan が作った配り方(H⁻¹ のコレスキー分解)。手順は3行で書ける。
//
//	① 重み i を丸めて確定させる
//	② 出た誤差を plan[i][i] で割る(その重みが出力に効く度合いで割り戻す)
//	③ まだ確定していない j へ、plan[i][j] に比例して配る
//
// 配る先は相関が決める。相関を無視した plan なら ③ の中身が 0 になり、
// 素朴な丸めと同じ結果になる。
//
// scale は元の重みから先に決めて動かさない。配ったせいで格子まで変わると、
// 何を比べているのか分からなくなる。
func Quantize(w []float64, plan [][]float64, bits int) []float64 {
	scale := scaleOf(w, bits)
	qmax := (1 << (bits - 1)) - 1

	work := append([]float64(nil), w...)
	out := make([]float64, len(w))
	for i := range work {
		q := clamp(math.Round(work[i]/scale), qmax) * scale
		out[i] = q

		err := (work[i] - q) / plan[i][i]
		for j := i + 1; j < len(work); j++ {
			work[j] -= err * plan[i][j]
		}
	}
	return out
}

// #endregion quantize

// #region error

// Outputs は入力 x に対する層の出力 Xᵀw を返す。
func Outputs(w []float64, x [][]float64) []float64 {
	n := len(x[0])
	out := make([]float64, n)
	for k := 0; k < n; k++ {
		s := 0.0
		for i := range w {
			s += w[i] * x[i][k]
		}
		out[k] = s
	}
	return out
}

// OutputError は元の重みと丸めた重みで、出力がどれだけずれたかを返す(二乗平均平方根)。
//
// 重みそのもののずれではなく、出力のずれを見るのがこの章の要点になる。
func OutputError(w, q []float64, x [][]float64) float64 {
	a := Outputs(w, x)
	b := Outputs(q, x)
	s := 0.0
	for k := range a {
		d := a[k] - b[k]
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)))
}

// WeightError は重みそのもののずれ(二乗平均平方根)。
func WeightError(w, q []float64) float64 {
	s := 0.0
	for i := range w {
		d := w[i] - q[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(w)))
}

// #endregion error

// scaleOf は [量子化](quant)の対称量子化と同じ格子間隔を使う。
func scaleOf(w []float64, bits int) float64 {
	return quant.QuantizeSymmetric(w, bits).Scale
}

func clamp(v float64, qmax int) float64 {
	if v > float64(qmax) {
		return float64(qmax)
	}
	if v < float64(-qmax) {
		return float64(-qmax)
	}
	return v
}
