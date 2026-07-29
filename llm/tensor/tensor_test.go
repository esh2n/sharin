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

// 並べ方を変えても結果は変わらない。変わるのは速さだけになる。
func TestLoopOrderDoesNotChangeTheResult(t *testing.T) {
	a := New(17, 23)
	b := New(23, 19)
	// 整数の線形合同法から作る。浮動小数の漸化式は不動点に落ちて、
	// 全要素がほぼ同じ値になってしまう。
	var s uint64 = 7
	fill := func(t *Tensor) {
		for i := range t.Data {
			s = s*6364136223846793005 + 1442695040888963407
			t.Data[i] = float32(int64(s>>40)-(1<<23)) / (1 << 23)
		}
	}
	fill(a)
	fill(b)

	x := MatMul(a, b)
	y := MatMulByDot(a, b)
	for i := range x.Data {
		if math.Abs(float64(x.Data[i]-y.Data[i])) > 1e-5 {
			t.Fatalf("%d 番目が違う: %g vs %g", i, x.Data[i], y.Data[i])
		}
	}

	// 形が合わなければどちらも止まる。
	defer func() {
		if recover() == nil {
			t.Fatal("形が合わないのに通った")
		}
	}()
	MatMulByDot(a, a)
}

// 非線形が無ければ、何層重ねても1つの行列積に潰れる。
func TestWithoutNonlinearityLayersCollapse(t *testing.T) {
	x := FromRows([][]float32{{1, 2, 3}, {-1, 0.5, 2}})
	w1 := FromRows([][]float32{{0.1, -0.2}, {0.3, 0.4}, {-0.5, 0.6}})
	w2 := FromRows([][]float32{{2, -1, 0.5}, {0.25, 1.5, -2}})

	two := MatMul(MatMul(x, w1), w2) // 2層ぶん通す
	one := MatMul(x, MatMul(w1, w2)) // 重みを先に1つにまとめる

	for i := range two.Data {
		if math.Abs(float64(two.Data[i]-one.Data[i])) > 1e-5 {
			t.Fatalf("潰れていない: %g vs %g", two.Data[i], one.Data[i])
		}
	}

	// 間に非線形を挟むと、もう1つの行列では書けない。
	bent := MatMul(GELU(MatMul(x, w1)), w2)
	same := true
	for i := range bent.Data {
		if math.Abs(float64(bent.Data[i]-one.Data[i])) > 1e-5 {
			same = false
		}
	}
	if same {
		t.Fatal("非線形を挟んでも同じになっている")
	}
}

func benchMats(n int) (*Tensor, *Tensor) {
	a, b := New(n, n), New(n, n)
	var s uint64 = 7
	for i := range a.Data {
		s = s*6364136223846793005 + 1442695040888963407
		v := float32(int64(s>>40)-(1<<23)) / (1 << 23)
		a.Data[i] = v
		b.Data[i] = -v
	}
	return a, b
}

func BenchmarkMatMul(bch *testing.B) {
	a, b := benchMats(256)
	bch.ResetTimer()
	for i := 0; i < bch.N; i++ {
		MatMul(a, b)
	}
}

func BenchmarkMatMulByDot(bch *testing.B) {
	a, b := benchMats(256)
	bch.ResetTimer()
	for i := 0; i < bch.N; i++ {
		MatMulByDot(a, b)
	}
}
