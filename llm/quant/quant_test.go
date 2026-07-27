package quant

import (
	"math"
	"testing"
)

func maxAbsErr(a, b []float64) float64 {
	m := 0.0
	for i := range a {
		if d := math.Abs(a[i] - b[i]); d > m {
			m = d
		}
	}
	return m
}

func TestSymmetricRoundTrip(t *testing.T) {
	// 対称量子化 → 復元で、誤差が量子化ステップの半分以内に収まる。
	x := []float64{-1.0, -0.3, 0, 0.25, 0.8, 1.0}
	q := QuantizeSymmetric(x, 8)
	back := q.Dequantize()
	step := q.Scale
	if err := maxAbsErr(x, back); err > step/2+1e-9 {
		t.Fatalf("error %g exceeds half-step %g", err, step/2)
	}
	// 量子化コードは int8 の範囲に収まる。
	for _, c := range q.Codes {
		if c < -127 || c > 127 {
			t.Fatalf("code %d out of int8 range", c)
		}
	}
}

func TestMoreBitsLessError(t *testing.T) {
	// ビット数を増やすほど量子化誤差は小さくなる。
	x := make([]float64, 256)
	v := 0.11
	for i := range x {
		v = math.Mod(v*7.13+0.37, 1.0)
		x[i] = v*2 - 1
	}
	e4 := maxAbsErr(x, QuantizeSymmetric(x, 4).Dequantize())
	e8 := maxAbsErr(x, QuantizeSymmetric(x, 8).Dequantize())
	if !(e8 < e4) {
		t.Fatalf("8-bit error %g should be < 4-bit error %g", e8, e4)
	}
	// おおむねビットを 4 増やすと誤差は 1 桁以上下がる。
	if e4/e8 < 8 {
		t.Fatalf("expected large gap, got e4/e8 = %g", e4/e8)
	}
}

func TestZeroPreserved(t *testing.T) {
	// 対称量子化は 0 を厳密に 0 に写す(コード 0)。バイアス項などで効く性質。
	x := []float64{-0.5, 0, 0.5}
	q := QuantizeSymmetric(x, 8)
	if q.Codes[1] != 0 || q.Dequantize()[1] != 0 {
		t.Fatalf("zero not preserved: code=%d val=%g", q.Codes[1], q.Dequantize()[1])
	}
}

func TestAsymmetricUsesFullRange(t *testing.T) {
	// 非対称量子化: 偏った分布 [0.2, 1.0] でも全コード域を使い、
	// 対称量子化より誤差が小さくなる。
	x := []float64{0.2, 0.35, 0.5, 0.7, 0.85, 1.0}
	asym := QuantizeAsymmetric(x, 8)
	sym := QuantizeSymmetric(x, 8)
	eAsym := maxAbsErr(x, asym.Dequantize())
	eSym := maxAbsErr(x, sym.Dequantize())
	if eAsym >= eSym {
		t.Fatalf("asymmetric %g should beat symmetric %g on skewed data", eAsym, eSym)
	}
	if r := asym.Dequantize(); maxAbsErr(x, r) > (1.0-0.2)/254+1e-6 {
		t.Fatalf("asym error too large: %g", maxAbsErr(x, r))
	}
}

func TestPerChannelBeatsPerTensor(t *testing.T) {
	// 行ごとにスケールが大きく違う行列では、per-channel が per-tensor に勝つ。
	// 行0は微小レンジ、行1は巨大レンジ。per-tensor は大きい方に引っ張られ、
	// 小さい行の分解能が潰れる。
	rows := [][]float64{
		{0.01, -0.02, 0.015, -0.008},
		{50, -80, 30, -100},
	}
	pt := QuantizeMatrixPerTensor(rows, 8)
	pc := QuantizeMatrixPerChannel(rows, 8)
	// 小さい行(行0)の誤差を比べる。
	ePT := maxAbsErr(rows[0], pt.DequantizeRow(0))
	ePC := maxAbsErr(rows[0], pc.DequantizeRow(0))
	if ePC >= ePT {
		t.Fatalf("per-channel row0 error %g should beat per-tensor %g", ePC, ePT)
	}
}

func TestMemoryBytes(t *testing.T) {
	// メモリ削減の会計: fp32 は 4 byte/要素、int8 は 1 byte/要素 + scale。
	if got := MemoryBytes(1_000_000, 32); got != 4_000_000 {
		t.Fatalf("fp32 bytes = %d", got)
	}
	if got := MemoryBytes(1_000_000, 8); got != 1_000_000 {
		t.Fatalf("int8 bytes = %d", got)
	}
	// 4-bit は 0.5 byte/要素。
	if got := MemoryBytes(1_000_000, 4); got != 500_000 {
		t.Fatalf("int4 bytes = %d", got)
	}
	if CompressionRatio(4) != 8 {
		t.Fatalf("4-bit vs fp32 ratio = %g", CompressionRatio(4))
	}
}

func TestEmpty(t *testing.T) {
	q := QuantizeSymmetric(nil, 8)
	if len(q.Dequantize()) != 0 {
		t.Fatal("empty should round-trip to empty")
	}
	// 全ゼロ入力は scale 0 にならず 1 にフォールバックし NaN を出さない。
	z := QuantizeSymmetric([]float64{0, 0, 0}, 8)
	for _, v := range z.Dequantize() {
		if math.IsNaN(v) {
			t.Fatal("all-zero input produced NaN")
		}
	}
}
