package layers

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/tensor"
)

func approx(a, b, tol float32) bool { return float32(math.Abs(float64(a-b))) <= tol }

func TestRMSNormUnitRMS(t *testing.T) {
	// 正規化後の各行は RMS(二乗平均平方根)がほぼ 1 になる。
	x := tensor.FromRows([][]float32{{1, 2, 3, 4}, {-5, 0, 5, 10}})
	got := RMSNorm(x, 1e-5)
	for r := 0; r < got.Rows; r++ {
		sum := float32(0)
		for c := 0; c < got.Cols; c++ {
			sum += got.At(r, c) * got.At(r, c)
		}
		rms := float32(math.Sqrt(float64(sum / float32(got.Cols))))
		if !approx(rms, 1, 1e-3) {
			t.Fatalf("row %d rms = %f", r, rms)
		}
	}
}

func TestRMSNormScaleInvariantButNotShiftInvariant(t *testing.T) {
	x := tensor.FromRows([][]float32{{0.5, -1.5, 2.0, 1.0}})
	base := RMSNorm(x, 1e-6)

	// スケール不変: 2 倍しても出力は同じ(分母も 2 倍になるので打ち消す)。
	scaled := tensor.FromRows([][]float32{{1.0, -3.0, 4.0, 2.0}})
	if got := RMSNorm(scaled, 1e-6); !rowsEqual(got, base, 1e-4) {
		t.Fatal("RMSNorm should be scale-invariant")
	}

	// シフト非不変: 平均を引かないので、+2 すると出力が変わる。
	shifted := tensor.FromRows([][]float32{{2.5, 0.5, 4.0, 3.0}})
	if got := RMSNorm(shifted, 1e-6); rowsEqual(got, base, 1e-4) {
		t.Fatal("RMSNorm must NOT be shift-invariant (it does not center)")
	}

	// 対照: LayerNorm は平均を引くのでシフトしても出力が変わらない。
	lnBase := tensor.LayerNorm(x, 1e-6)
	lnShift := tensor.LayerNorm(shifted, 1e-6)
	if !rowsEqual(lnBase, lnShift, 1e-4) {
		t.Fatal("LayerNorm should be shift-invariant")
	}
}

func rowsEqual(a, b *tensor.Tensor, tol float32) bool {
	if a.Rows != b.Rows || a.Cols != b.Cols {
		return false
	}
	for i := range a.Data {
		if !approx(a.Data[i], b.Data[i], tol) {
			return false
		}
	}
	return true
}

func TestSiLU(t *testing.T) {
	x := tensor.FromRows([][]float32{{0, 1, 6, -6}})
	got := SiLU(x)
	if got.At(0, 0) != 0 {
		t.Fatalf("silu(0) = %f", got.At(0, 0))
	}
	// silu(1) = 1·σ(1) ≈ 0.7311
	if !approx(got.At(0, 1), 0.7311, 1e-3) {
		t.Fatalf("silu(1) = %f", got.At(0, 1))
	}
	// 大きい正の値はほぼ素通し、大きい負の値はほぼ 0。
	if !approx(got.At(0, 2), 6, 0.02) || !approx(got.At(0, 3), 0, 0.02) {
		t.Fatalf("silu(±6) = %f, %f", got.At(0, 2), got.At(0, 3))
	}
}

func TestSwiGLUGate(t *testing.T) {
	// ゲート側(W1)が強い負を出すと SiLU がほぼ 0 になり、
	// 値側(W3)が何を出していても中間チャネルが閉じる。
	// dModel=1, dHidden=1: gate = x·(-10), value = x·(5), out = silu(gate)*value·W2
	f := &SwiGLUFFN{
		W1: tensor.FromRows([][]float32{{-10}}),
		W3: tensor.FromRows([][]float32{{5}}),
		W2: tensor.FromRows([][]float32{{1}}),
	}
	x := tensor.FromRows([][]float32{{1}})
	if got := f.Forward(x).At(0, 0); !approx(got, 0, 0.01) {
		t.Fatalf("closed gate should output ~0, got %f", got)
	}
	// ゲートが強い正なら値側がそのまま通る(silu(10)≈10 → 10*5 = 50)。
	open := &SwiGLUFFN{
		W1: tensor.FromRows([][]float32{{10}}),
		W3: tensor.FromRows([][]float32{{5}}),
		W2: tensor.FromRows([][]float32{{1}}),
	}
	if got := open.Forward(x).At(0, 0); !approx(got, 50, 0.1) {
		t.Fatalf("open gate should pass value, got %f", got)
	}
}

func TestSwiGLUShapeAndDeterminism(t *testing.T) {
	a := NewSwiGLUFFN(8, 16)
	b := NewSwiGLUFFN(8, 16)
	x := tensor.New(3, 8)
	for i := range x.Data {
		x.Data[i] = float32(i%7) * 0.3
	}
	oa := a.Forward(x)
	ob := b.Forward(x)
	if oa.Rows != 3 || oa.Cols != 8 {
		t.Fatalf("shape = (%d,%d)", oa.Rows, oa.Cols)
	}
	if !rowsEqual(oa, ob, 0) {
		t.Fatal("same config should give identical outputs")
	}
}
