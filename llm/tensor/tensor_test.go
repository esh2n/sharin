package tensor

import (
	"math"
	"testing"
)

const eps = 1e-5

func almostEqual(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > eps {
			return false
		}
	}
	return true
}

func TestNewAndShape(t *testing.T) {
	x := New(2, 3)
	if x.Rows != 2 || x.Cols != 3 {
		t.Errorf("shape = (%d, %d), want (2, 3)", x.Rows, x.Cols)
	}
	if len(x.Data) != 6 {
		t.Errorf("data len = %d, want 6", len(x.Data))
	}
}

func TestAtSet(t *testing.T) {
	x := New(2, 2)
	x.Set(0, 1, 3.5)
	x.Set(1, 0, 7)
	if x.At(0, 1) != 3.5 || x.At(1, 0) != 7 {
		t.Errorf("At/Set がおかしい: %v", x.Data)
	}
}

func TestMatMul(t *testing.T) {
	// [1 2 3]   [1 2]     [22 28]
	// [4 5 6] × [3 4]  =  [49 64]
	//           [5 6]
	a := FromRows([][]float32{{1, 2, 3}, {4, 5, 6}})
	b := FromRows([][]float32{{1, 2}, {3, 4}, {5, 6}})
	got := MatMul(a, b)
	want := FromRows([][]float32{{22, 28}, {49, 64}})
	if !almostEqual(got.Data, want.Data) {
		t.Errorf("MatMul = %v, want %v", got.Data, want.Data)
	}
}

func TestMatMulShapeMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("形が合わない MatMul は panic すべき")
		}
	}()
	MatMul(New(2, 3), New(2, 2)) // 3 != 2
}

func TestAddBroadcastRow(t *testing.T) {
	// (2,3) に (1,3) のバイアスを各行へ足す(ブロードキャスト)。
	x := FromRows([][]float32{{1, 2, 3}, {4, 5, 6}})
	bias := FromRows([][]float32{{10, 20, 30}})
	got := AddRow(x, bias)
	want := FromRows([][]float32{{11, 22, 33}, {14, 25, 36}})
	if !almostEqual(got.Data, want.Data) {
		t.Errorf("AddRow = %v, want %v", got.Data, want.Data)
	}
}

func TestSoftmaxRows(t *testing.T) {
	// 各行が確率分布(合計1)になる。数値安定化(max 引き)も効いていること。
	x := FromRows([][]float32{{1, 1, 1, 1}, {1000, 1000, 1000, 1000}})
	got := SoftmaxRows(x)
	for r := 0; r < 2; r++ {
		sum := float32(0)
		for c := 0; c < 4; c++ {
			sum += got.At(r, c)
		}
		if math.Abs(float64(sum-1)) > eps {
			t.Errorf("行 %d の合計 = %v, want 1", r, sum)
		}
		if math.Abs(float64(got.At(r, 0)-0.25)) > eps {
			t.Errorf("一様分布になるべき: %v", got.At(r, 0))
		}
	}
}

func TestLayerNorm(t *testing.T) {
	// 各行を平均0・分散1に正規化する。
	x := FromRows([][]float32{{1, 2, 3, 4}})
	got := LayerNorm(x, 1e-5)
	// 平均が0付近になっていること。
	var mean float32
	for c := 0; c < 4; c++ {
		mean += got.At(0, c)
	}
	mean /= 4
	if math.Abs(float64(mean)) > eps {
		t.Errorf("正規化後の平均 = %v, want ~0", mean)
	}
}

func TestGELU(t *testing.T) {
	// GELU(0)=0, GELU は大きな正で恒等に近づき、大きな負で0に近づく。
	x := FromRows([][]float32{{0, 5, -5}})
	got := GELU(x)
	if math.Abs(float64(got.At(0, 0))) > eps {
		t.Errorf("GELU(0) = %v, want ~0", got.At(0, 0))
	}
	if got.At(0, 1) < 4.9 {
		t.Errorf("GELU(5) は 5 に近いべき: %v", got.At(0, 1))
	}
	if math.Abs(float64(got.At(0, 2))) > 0.1 {
		t.Errorf("GELU(-5) は 0 に近いべき: %v", got.At(0, 2))
	}
}
