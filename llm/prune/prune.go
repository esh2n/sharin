// Package prune は、重みの一部を 0 にして減らす(枝刈り)。
//
// [誤差を配る量子化](gptq)では、丸め先を選ぶのに入力の相関を使った。
// 同じ道具で別の問いにも答えられる。どれを 0 にするか、という問いだ。
//
// もともと順序は逆になる。ヘッセ行列を使って重みを1つ選んで消し、
// 残りを動かして補うという手は、枝刈りのために考えられたものだった
// (Optimal Brain Surgeon、1992年)。量子化に持ち込まれたのは30年後になる。
//
// 素朴には、絶対値の小さい重みから消せばよい。小さいのだから出力への
// 寄与も小さいだろう、という理屈だ。だがこれは近似でしかない。
// 同じ 0.01 でも、いつも大きく振れる入力に掛かっているなら出力を動かすし、
// ほとんど動かない入力に掛かっているなら消しても何も起きない。
//
// もう1つ、消す損は「消したあとどう補えるか」まで含めて測れる。
// 隣に似た動きをする入力があれば、そちらの重みを動かして肩代わりできる。
// 補えるなら、消す損は小さい。
//
// 実時間も乱数も使わない。入力は呼び出し側が渡す。
package prune

import (
	"math"
	"sort"

	"github.com/esh2n/sharin/llm/gptq"
)

// #region saliency

// Saliency は重み1つを 0 にしたときの損の見積もりを返す。
//
//	L_i = w_i² / (2 · [H⁻¹]_ii)
//
// 分子は重みの大きさで、分母が「補いやすさ」になる。
// [H⁻¹]_ii が大きいほど、その重みを消したときに残りで肩代わりしやすいので、
// 損が小さく出る。絶対値だけを見るのと違うのは、この分母のぶんになる。
func Saliency(w []float64, hinv [][]float64) []float64 {
	out := make([]float64, len(w))
	for i := range w {
		out[i] = w[i] * w[i] / (2 * hinv[i][i])
	}
	return out
}

// #endregion saliency

// #region select

// ByMagnitude は絶対値の小さい順に k 個を 0 にする。補正はしない。
func ByMagnitude(w []float64, k int) []float64 {
	score := make([]float64, len(w))
	for i, v := range w {
		score[i] = math.Abs(v)
	}
	out := append([]float64(nil), w...)
	for _, i := range smallest(score, k) {
		out[i] = 0
	}
	return out
}

// BySaliencyOnly は損の見積もりが小さい順に k 個を 0 にする。補正はしない。
//
// 選び方だけを変えて、補正の効果と切り分けるためのもの。
func BySaliencyOnly(w []float64, hinv [][]float64, k int) []float64 {
	out := append([]float64(nil), w...)
	for _, i := range smallest(Saliency(w, hinv), k) {
		out[i] = 0
	}
	return out
}

// smallest は score の小さい順に k 個の添字を返す。同点は添字の順で決める。
func smallest(score []float64, k int) []int {
	idx := make([]int, len(score))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return score[idx[a]] < score[idx[b]] })
	if k > len(idx) {
		k = len(idx)
	}
	return idx[:k]
}

// #endregion select

// #region obs

// BySaliency は損の見積もりが小さい順に1つずつ消し、消すたびに残りを動かす。
//
// 手順はこうなる。
//
//	① まだ生きている中から、損の見積もりがいちばん小さいものを選ぶ
//	② その重みを 0 にする
//	③ 残りを δ = -(w_q / [H⁻¹]_qq) · H⁻¹[:,q] だけ動かして補う
//	④ 消した1つを取り除いた形に H⁻¹ を作り直す
//
// ④ が要るのは、1つ消すたびに「残りで肩代わりできる度合い」が変わるからになる。
// 作り直しは掛け算と引き算だけで済む(逆行列を取り直さなくてよい)。
func BySaliency(w []float64, hinv [][]float64, k int) []float64 {
	d := len(w)
	cur := append([]float64(nil), w...)
	inv := clone(hinv)
	dead := make([]bool, d)

	for step := 0; step < k && step < d; step++ {
		q, best := -1, math.Inf(1)
		for i := 0; i < d; i++ {
			if dead[i] || inv[i][i] <= 0 {
				continue
			}
			if l := cur[i] * cur[i] / (2 * inv[i][i]); l < best {
				q, best = i, l
			}
		}
		if q < 0 {
			break
		}

		// ③ 残りを動かして補う。消したぶんの重みが、肩代わりできる先へ移る。
		f := cur[q] / inv[q][q]
		for j := 0; j < d; j++ {
			if dead[j] || j == q {
				continue
			}
			cur[j] -= f * inv[j][q]
		}
		cur[q] = 0
		dead[q] = true

		// ④ H⁻¹ から q を抜いた形に作り直す。
		p := inv[q][q]
		for i := 0; i < d; i++ {
			if dead[i] {
				continue
			}
			for j := 0; j < d; j++ {
				if dead[j] {
					continue
				}
				inv[i][j] -= inv[i][q] * inv[q][j] / p
			}
		}
	}
	return cur
}

// #endregion obs

// Plan は入力から H⁻¹ を作る。[誤差を配る量子化](gptq)の道具をそのまま使う。
func Plan(x [][]float64, damp float64) [][]float64 {
	return gptq.Inverse(gptq.Hessian(x, damp))
}

// Kept は 0 でない重みの数を返す。
func Kept(w []float64) int {
	n := 0
	for _, v := range w {
		if v != 0 {
			n++
		}
	}
	return n
}

// OutputError は元の重みと枝刈り後で、出力がどれだけずれたかを返す。
func OutputError(w, p []float64, x [][]float64) float64 {
	return gptq.OutputError(w, p, x)
}

func clone(a [][]float64) [][]float64 {
	out := make([][]float64, len(a))
	for i := range a {
		out[i] = append([]float64(nil), a[i]...)
	}
	return out
}
