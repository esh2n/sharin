package quant

import (
	"math"
	"testing"
)

// ゼロ点も丸めるので、両方の丸めが同じ向きに転ぶとコードが域を出る。
// だから最後に頭を押さえる。
func TestAsymmetricClampsAtTheTop(t *testing.T) {
	x := []float64{-2.0, 0.4}
	q := QuantizeAsymmetric(x, 2) // qmax = 3
	for _, c := range q.Codes {
		if c < 0 || c > 3 {
			t.Fatalf("コードが域を出た: %v", q.Codes)
		}
	}
	if clampInt(-5, -3, 3) != -3 || clampInt(5, -3, 3) != 3 || clampInt(1, -3, 3) != 1 {
		t.Fatal("頭と足を押さえていない")
	}

	// 並び順に関係なく、いちばん小さい値がコード 0 に写る。
	q8 := QuantizeAsymmetric([]float64{0.4, -2.0, 0.1}, 8)
	if q8.Codes[1] != 0 {
		t.Fatalf("最小がコード 0 になっていない: %v", q8.Codes)
	}
}

// レンジが 0 の入力でも 0 除算にならない。
func TestZeroRangeFallsBack(t *testing.T) {
	flat := QuantizeAsymmetric([]float64{0.5, 0.5, 0.5}, 8)
	for _, v := range flat.Dequantize() {
		if math.IsNaN(v) {
			t.Fatal("同じ値ばかりで NaN が出た")
		}
	}
	zeros := [][]float64{{0, 0}, {0, 0}}
	for _, m := range []*QuantizedMatrix{
		QuantizeMatrixPerTensor(zeros, 8),
		QuantizeMatrixPerChannel(zeros, 8),
	} {
		for r := range zeros {
			for _, v := range m.DequantizeRow(r) {
				if math.IsNaN(v) || v != 0 {
					t.Fatalf("全ゼロの行が壊れた: %g", v)
				}
			}
		}
	}
}
