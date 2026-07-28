package quant

import (
	"math"
	"testing"
)

// この章の中心。ごく一部の桁違いな値が、全体の格子を潰す。
func TestOutliersDestroyTheGrid(t *testing.T) {
	// 活性を模した並び。大多数は ±1 に収まり、3つだけ桁が違う。
	marks := []int{7, 200, 431}
	x := noisy(512)
	for _, i := range marks {
		x[i] = 25
	}

	plain := QuantizeSymmetric(x, 8)
	grouped := QuantizeGroupwise(x, 8, 64)
	mixed := QuantizeWithOutliers(x, 8, 6.0)

	// 外れ値そのものの誤差は比べない。潰された大多数の側を見る。
	ePlain := meanAbsErrExcept(x, plain.Dequantize(), marks)
	eGroup := meanAbsErrExcept(x, grouped.Dequantize(), marks)
	eMixed := meanAbsErrExcept(x, mixed.Dequantize(), marks)

	// 区切ると被害はその区切りの中で止まるが、消えはしない。
	if !(eGroup < ePlain) {
		t.Fatalf("区切りが効いていない: %g vs %g", eGroup, ePlain)
	}
	// 抜いてしまえば、残りは自分のレンジで格子を使える。
	if !(eMixed < eGroup) {
		t.Fatalf("分離が効いていない: %g vs %g", eMixed, eGroup)
	}
	// 全体で1つの scale にすると、桁の差そのまま誤差が膨らむ。
	if ePlain/eMixed < 20 {
		t.Fatalf("差が小さすぎる: %g", ePlain/eMixed)
	}

	// 成り立つのは、外れ値がごく一部だからになる。
	if r := mixed.OutlierRatio(); r > 0.01 {
		t.Fatalf("外れ値が多すぎる: %g", r)
	}
	// 外れ値そのものは元の値のまま返る。
	if got := mixed.Dequantize()[7]; got != 25 {
		t.Fatalf("外れ値が丸められた: %g", got)
	}
	if r := (&Mixed{Base: QuantizeSymmetric(nil, 8)}).OutlierRatio(); r != 0 {
		t.Fatalf("空で 0 にならない: %g", r)
	}
}

// 効きの大きい列だけ引き伸ばすと、その列の丸め誤差が小さくなる。
func TestSalientChannelsGetAFinerGrid(t *testing.T) {
	n := 256
	w := noisy(n)
	act := make([]float64, n) // 各列の活性の大きさ
	for i := range act {
		act[i] = 0.5
	}
	salient := []int{3, 40, 77}
	for _, i := range salient {
		w[i] = 0.05 // 重みは小さいが
		act[i] = 20 // 活性が大きいので出力への効きは大きい
	}

	// 出力への効きで重みづけした誤差。これが小さいほど出力が保たれる。
	weighted := func(back []float64) float64 {
		s := 0.0
		for i := range w {
			s += act[i] * math.Abs(w[i]-back[i])
		}
		return s
	}

	plain := weighted(QuantizeSymmetric(w, 4).Dequantize())

	s := make([]float64, n)
	for i := range s {
		s[i] = 1
	}
	for _, i := range salient {
		s[i] = 4
	}
	scaled := weighted(QuantizeScaled(w, s, 4).Dequantize())

	if !(scaled < plain) {
		t.Fatalf("引き伸ばしが効いていない: %g vs %g", scaled, plain)
	}
	// 引き伸ばした列だけを見ると、誤差はおおよそ 1/s になる。
	before := QuantizeSymmetric(w, 4).Dequantize()
	after := QuantizeScaled(w, s, 4).Dequantize()
	for _, i := range salient {
		eb := math.Abs(w[i] - before[i])
		ea := math.Abs(w[i] - after[i])
		if ea > eb/2 {
			t.Fatalf("列 %d の誤差が縮んでいない: %g → %g", i, eb, ea)
		}
	}
}
