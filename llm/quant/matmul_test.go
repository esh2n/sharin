package quant

import (
	"math"
	"testing"
)

// scale は総和の外に括り出せる。だから中は整数の積和だけで済む。
func TestScaleFactorsOutOfTheSum(t *testing.T) {
	a := noisy(128)
	b := noisy(128)
	for i := range b {
		b[i] = -b[i] * 0.3
	}
	qa := QuantizeSymmetric(a, 8)
	qb := QuantizeSymmetric(b, 8)

	// 復元してから掛けた結果と、整数のまま掛けて最後に scale を掛けた結果。
	da, db := qa.Dequantize(), qb.Dequantize()
	want := 0.0
	for i := range da {
		want += da[i] * db[i]
	}
	got := Dot(qa, qb)

	if math.Abs(got-want) > 1e-9*math.Max(1, math.Abs(want)) {
		t.Fatalf("括り出すと結果が変わった: %g vs %g", got, want)
	}
	// 途中は整数のまま。浮動小数の掛け算は最後の1回だけになる。
	if DotCodes(qa, qb) == 0 {
		t.Fatal("整数の積和が 0 になっている")
	}
}

// 非対称量子化でもゼロ点を引けば同じ形になる。
func TestZeroPointComesOutToo(t *testing.T) {
	a := []float64{0.2, 0.5, 0.9, 1.0}
	qa := QuantizeAsymmetric(a, 8)
	qb := QuantizeAsymmetric(a, 8)

	da := qa.Dequantize()
	want := 0.0
	for i := range da {
		want += da[i] * da[i]
	}
	if got := Dot(qa, qb); math.Abs(got-want) > 1e-9 {
		t.Fatalf("ゼロ点が抜けている: %g vs %g", got, want)
	}
}

// 整数の貯め先は int32 で足りる。桁あふれは勘定で確かめられる。
func TestAccumulatorFitsInInt32(t *testing.T) {
	const n = 4096
	worst := &Quantized{Codes: make([]int, n), Scale: 1}
	for i := range worst.Codes {
		worst.Codes[i] = 127
	}
	acc := DotCodes(worst, worst)
	if acc != n*127*127 {
		t.Fatalf("勘定が合わない: %d", acc)
	}
	if acc >= math.MaxInt32 {
		t.Fatalf("int32 に収まらない: %d", acc)
	}
	// 127×127 = 16129。int32 の上限まで 13 万項ほど余裕がある。
	if math.MaxInt32/(127*127) < 100_000 {
		t.Fatal("余裕の見積もりが違う")
	}
}
