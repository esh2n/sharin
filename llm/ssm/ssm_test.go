package ssm

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-9 }

func seq(n int) []float64 {
	out := make([]float64, n)
	v := 0.3
	for i := range out {
		v = math.Mod(v*7.13+0.37, 1.0)
		out[i] = v*2 - 1
	}
	return out
}

func TestScanIsLinearRecurrence(t *testing.T) {
	// 状態更新 h_t = a·h_{t-1} + b·x_t、出力 y_t = c·h_t を手計算と照合。
	m := &SSM{A: 0.5, B: 1.0, C: 2.0}
	x := []float64{1, 0, 0}
	y := m.Scan(x)
	// h: 1, 0.5, 0.25 → y: 2, 1, 0.5
	want := []float64{2, 1, 0.5}
	for i := range want {
		if !approx(y[i], want[i]) {
			t.Fatalf("y = %v, want %v", y, want)
		}
	}
}

func TestScanLinearTimeManySteps(t *testing.T) {
	// 逐次スキャンは 1 ステップあたり定数計算。長さ n に線形。
	// attention の O(n²) と違い、状態は 1 個で足りることを ops カウントで示す。
	m := &SSM{A: 0.9, B: 0.1, C: 1.0}
	x := seq(1000)
	y, ops := m.ScanCounted(x)
	if len(y) != 1000 {
		t.Fatalf("len y = %d", len(y))
	}
	if ops != 1000 {
		t.Fatalf("ops = %d, want 1000 (linear)", ops)
	}
	// 対照: attention 相当の全ペア計算量。
	if AttentionOps(1000) != 1000*1000 {
		t.Fatalf("attention ops = %d", AttentionOps(1000))
	}
}

func TestDecayForgetsPast(t *testing.T) {
	// |A|<1 なら過去の入力の影響は指数的に薄れる。
	// t=0 のインパルスが t=50 でほぼ消えていることを確認。
	m := &SSM{A: 0.8, B: 1.0, C: 1.0}
	x := make([]float64, 60)
	x[0] = 1
	y := m.Scan(x)
	if y[0] != 1 {
		t.Fatalf("y[0] = %f", y[0])
	}
	if math.Abs(y[50]) > 1e-4 {
		t.Fatalf("impulse should decay: y[50] = %g", y[50])
	}
	// A=1 なら減衰せず状態が保持され続ける(累積和になる)。
	keep := &SSM{A: 1.0, B: 1.0, C: 1.0}
	ones := []float64{1, 1, 1, 1}
	if got := keep.Scan(ones); !approx(got[3], 4) {
		t.Fatalf("A=1 should accumulate: %v", got)
	}
}

func TestSelectiveGating(t *testing.T) {
	// 選択的 SSM: 入力ごとに「取り込むか無視するか」のゲートが変わる。
	// ゲートがほぼ 0 のトークンは状態を素通りさせ、ほぼ 1 のトークンは強く取り込む。
	m := NewSelective(0.9)
	// gate 列: 1 のところだけ入力を状態に入れる
	x := []float64{5, 5, 5}
	gate := []float64{1, 0, 0}
	y := m.ScanSelective(x, gate)
	// t0: h=0.1*5=0.5(A=0.9,B=0.1) 相当。t1,t2 はゲート0で入力遮断、状態は減衰のみ
	if y[1] >= y[0] {
		t.Fatalf("gated-off steps should not increase state: %v", y)
	}
	if y[2] >= y[1] {
		t.Fatalf("state should keep decaying while gated off: %v", y)
	}
	// 全ゲート開の同じ入力なら、状態はもっと大きく育つ。
	openGate := []float64{1, 1, 1}
	yo := m.ScanSelective(x, openGate)
	if yo[2] <= y[2] {
		t.Fatalf("open gate should accumulate more: %v vs %v", yo, y)
	}
}

func TestSelectiveGateClamped(t *testing.T) {
	// ゲートは [0,1] にクランプされる(範囲外入力を渡しても壊れない)。
	m := NewSelective(0.5)
	y := m.ScanSelective([]float64{1, 1}, []float64{5, -5})
	if math.IsNaN(y[0]) || math.IsNaN(y[1]) {
		t.Fatal("clamped gate should not produce NaN")
	}
}

func TestEmpty(t *testing.T) {
	m := &SSM{A: 0.5, B: 1, C: 1}
	if got := m.Scan(nil); len(got) != 0 {
		t.Fatalf("empty scan = %v", got)
	}
}
