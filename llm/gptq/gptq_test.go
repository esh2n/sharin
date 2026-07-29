package gptq

import (
	"math"
	"testing"
)

// rnd は整数の線形合同法から [-1, 1) を作る。
// 浮動小数の漸化式にすると積和の融合で処理系ごとに結果が変わるので使わない。
type rnd struct{ s uint64 }

func newRnd(seed uint64) *rnd { return &rnd{s: seed | 1} }
func (r *rnd) next() float64 {
	r.s = r.s*6364136223846793005 + 1442695040888963407
	return float64(int64(r.s>>40)-(1<<23)) / (1 << 23)
}

// correlated は「よく似た動きをする入力」を作る。
// 共通の成分に、少しだけ個別の揺れを足す。
func correlated(d, n int, share float64) [][]float64 {
	r := newRnd(7)
	x := make([][]float64, d)
	for i := range x {
		x[i] = make([]float64, n)
	}
	for k := 0; k < n; k++ {
		base := r.next()
		for i := 0; i < d; i++ {
			x[i][k] = share*base + (1-share)*r.next()
		}
	}
	return x
}

func weights(d int) []float64 {
	r := newRnd(31)
	w := make([]float64, d)
	for i := range w {
		w[i] = r.next()
	}
	return w
}

// 逆行列がちゃんと逆行列になっている。
func TestInverse(t *testing.T) {
	x := correlated(8, 64, 0.6)
	h := Hessian(x, 0.01)
	hi := Inverse(h)

	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			s := 0.0
			for k := 0; k < 8; k++ {
				s += h[i][k] * hi[k][j]
			}
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(s-want) > 1e-9 {
				t.Fatalf("(%d,%d) = %g", i, j, s)
			}
		}
	}
	// 逆行列が立たない行列でも落ちない。
	zero := [][]float64{{0, 0}, {0, 0}}
	if got := Inverse(zero); len(got) != 2 {
		t.Fatal("形が違う")
	}
	// 下の行のほうが大きいときは入れ替えてから割る(桁落ちを避ける)。
	a := [][]float64{{1, 2}, {3, 4}}
	ai := Inverse(a)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			s := a[i][0]*ai[0][j] + a[i][1]*ai[1][j]
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(s-want) > 1e-12 {
				t.Fatalf("入れ替えを含む逆行列が違う (%d,%d) = %g", i, j, s)
			}
		}
	}
	// 正定値でない行列を分解しても落ちない(対角の下限で受ける)。
	if got := CholeskyUpper([][]float64{{0, 0}, {0, 0}}); got[0][0] <= 0 {
		t.Fatalf("対角が 0 以下: %v", got)
	}
}

// この章の中心。重みのずれは増えるのに、出力のずれは減る。
func TestErrorMovesFromOutputToWeights(t *testing.T) {
	d, n := 64, 256
	x := correlated(d, n, 0.9)
	w := weights(d)
	plan := Plan(x, 0.01)

	rtn := RoundToNearest(w, 3)
	fix := Quantize(w, plan, 3)

	// 出力のずれは減る。これが目的になる。
	oRTN := OutputError(w, rtn, x)
	oFix := OutputError(w, fix, x)
	if !(oFix < oRTN) {
		t.Fatalf("出力のずれが減らない: %g → %g", oRTN, oFix)
	}

	// 一方、重みそのもののずれは増える。最寄りに丸めるのをやめたのだから当然になる。
	wRTN := WeightError(w, rtn)
	wFix := WeightError(w, fix)
	if !(wFix > wRTN) {
		t.Fatalf("重みのずれが増えていない: %g → %g", wRTN, wFix)
	}
}

// 配る先は入力の相関が決める。相関を無視すると素朴な丸めに戻る。
func TestWithoutCorrelationItIsJustRounding(t *testing.T) {
	d, n := 32, 128
	x := correlated(d, n, 0.8)
	w := weights(d)

	diag := CholeskyUpper(Inverse(Diagonal(Hessian(x, 0.01))))
	fix := Quantize(w, diag, 3)
	rtn := RoundToNearest(w, 3)

	for i := range w {
		if fix[i] != rtn[i] {
			t.Fatalf("%d 番目が違う: %g vs %g", i, fix[i], rtn[i])
		}
	}
}

// この章の中心その2。得の大きさは、入力の相関がそのまま決める。
func TestGainComesFromCorrelation(t *testing.T) {
	d, n := 64, 256
	w := weights(d)

	gain := func(share float64) float64 {
		x := correlated(d, n, share)
		plan := Plan(x, 0.01)
		return OutputError(w, RoundToNearest(w, 3), x) / OutputError(w, Quantize(w, plan, 3), x)
	}
	// 共通成分を増やすほど、肩代わりできる余地が増える。
	g0, g5, g7, g9 := gain(0), gain(0.5), gain(0.7), gain(0.9)
	if !(g5 < g7 && g7 < g9) {
		t.Fatalf("相関が強いほど得が大きいはず: %g %g %g", g5, g7, g9)
	}
	if g9 < 5 {
		t.Fatalf("強い相関で得が小さい: %g", g9)
	}
	// 相関がまったく無ければ、配っても得が無い。
	if math.Abs(g0-1) > 0.05 {
		t.Fatalf("相関が無いのに変わった: %g", g0)
	}
}

// どのビット数でも配る意味はあるが、得の大きさは単調には動かない。
func TestGainHoldsAcrossBitWidths(t *testing.T) {
	d, n := 64, 256
	x := correlated(d, n, 0.9)
	w := weights(d)
	plan := Plan(x, 0.01)

	for _, bits := range []int{2, 3, 4, 8} {
		rtn := OutputError(w, RoundToNearest(w, bits), x)
		fix := OutputError(w, Quantize(w, plan, bits), x)
		if !(fix < rtn) {
			t.Fatalf("%dbit で悪化した: %g → %g", bits, rtn, fix)
		}
	}
}

// 先頭の重みには誰も配っていないので、素朴な丸めと同じところへ行く。
func TestFirstWeightIsUntouched(t *testing.T) {
	d := 64
	x := correlated(d, 256, 0.9)
	w := weights(d)
	plan := Plan(x, 0.01)

	fix := Quantize(w, plan, 3)
	rtn := RoundToNearest(w, 3)
	if fix[0] != rtn[0] {
		t.Fatalf("先頭が動いた: %g vs %g", fix[0], rtn[0])
	}

	// 配られたせいで、途中には別の格子点へ行くものが出る。
	moved := 0
	for i := range w {
		if fix[i] != rtn[i] {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("まったく配られていない")
	}
	// それでも動くのは一部で、大半は同じところへ行く。
	if moved > d/4 {
		t.Fatalf("動きすぎ: %d / %d", moved, d)
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	x := correlated(4, 16, 0.5)
	h := Hessian(x, 0.1)
	if len(h) != 4 || len(h[0]) != 4 {
		t.Fatal("形が違う")
	}
	// 対角は下駄のぶんだけ持ち上がっている。
	raw := Hessian(x, 0)
	if !(h[0][0] > raw[0][0]) {
		t.Fatal("下駄が乗っていない")
	}
	if d := Diagonal(h); d[0][1] != 0 || d[0][0] != h[0][0] {
		t.Fatal("対角だけになっていない")
	}
	w := weights(4)
	if len(Outputs(w, x)) != 16 {
		t.Fatal("出力の数が違う")
	}
	if WeightError(w, w) != 0 || OutputError(w, w, x) != 0 {
		t.Fatal("同じものでずれが出た")
	}
	// 頭と足を押さえている。
	if clamp(99, 3) != 3 || clamp(-99, 3) != -3 || clamp(1, 3) != 1 {
		t.Fatal("clamp が違う")
	}
}
