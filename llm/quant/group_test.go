package quant

import (
	"math"
	"testing"
)

// noisy は決定的な擬似乱数で [-1, 1) の並びを作る。
//
// 整数の線形合同法から作るので、値は 2 のべき乗で割った厳密な浮動小数になる。
// 浮動小数の漸化式にすると、積和を融合する処理系としない処理系で結果が変わり、
// デモの数字と章の数字がずれる。
func noisy(n int) []float64 {
	out := make([]float64, n)
	var s uint64 = 7
	for i := range out {
		s = s*6364136223846793005 + 1442695040888963407
		out[i] = float64(int64(s>>40)-(1<<23)) / (1 << 23)
	}
	return out
}

// meanAbsErrExcept は、指定した添字を除いた平均絶対誤差。
func meanAbsErrExcept(a, b []float64, skip []int) float64 {
	out := map[int]bool{}
	for _, i := range skip {
		out[i] = true
	}
	sum, n := 0.0, 0
	for i := range a {
		if out[i] {
			continue
		}
		sum += math.Abs(a[i] - b[i])
		n++
	}
	return sum / float64(n)
}

func meanAbsErr(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += math.Abs(a[i] - b[i])
	}
	return s / float64(len(a))
}

// 区切りを細かくすると、大きい値の影響がその区切りの中で止まる。
func TestGroupwiseLimitsTheDamage(t *testing.T) {
	x := noisy(256)
	x[10] = 40 // 桁違いに大きい1つ

	whole := meanAbsErr(x, QuantizeSymmetric(x, 4).Dequantize())
	g64 := meanAbsErr(x, QuantizeGroupwise(x, 4, 64).Dequantize())
	g32 := meanAbsErr(x, QuantizeGroupwise(x, 4, 32).Dequantize())

	if !(g64 < whole) {
		t.Fatalf("区切っても改善しない: %g vs %g", g64, whole)
	}
	if !(g32 < g64) {
		t.Fatalf("細かくしても改善しない: %g vs %g", g32, g64)
	}
}

// 細かくするほど scale の上乗せが増える。縮めたぶんを scale が食い返す。
func TestSmallerGroupsCostMoreBits(t *testing.T) {
	g128 := QuantizeGroupwise(noisy(256), 4, 128)
	g64 := QuantizeGroupwise(noisy(256), 4, 64)
	g16 := QuantizeGroupwise(noisy(256), 4, 16)

	if got := g64.BitsPerValue(); math.Abs(got-4.25) > 1e-9 {
		t.Fatalf("4bit・64要素は 4.25 ビットのはず: %g", got)
	}
	if !(g128.BitsPerValue() < g64.BitsPerValue() && g64.BitsPerValue() < g16.BitsPerValue()) {
		t.Fatal("細かいほうが上乗せが大きいはず")
	}
	// 16要素まで刻むと 5 ビットになり、int4 と呼べなくなる。
	if got := g16.BitsPerValue(); got != 5 {
		t.Fatalf("4bit・16要素は 5 ビットのはず: %g", got)
	}
}

// 割り切れない長さでも、端数の区切りが自分の scale を持つ。
func TestRaggedLastGroup(t *testing.T) {
	x := noisy(100)
	g := QuantizeGroupwise(x, 8, 64)
	if len(g.Scales) != 2 {
		t.Fatalf("区切りは2つのはず: %d", len(g.Scales))
	}
	if err := meanAbsErr(x, g.Dequantize()); err > 0.01 {
		t.Fatalf("端数の区切りが壊れている: %g", err)
	}
	// 全ゼロの区切りでも 0 除算にならない。
	z := QuantizeGroupwise(make([]float64, 8), 8, 4)
	for _, v := range z.Dequantize() {
		if math.IsNaN(v) {
			t.Fatal("全ゼロで NaN が出た")
		}
	}
}
