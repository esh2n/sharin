package congestion

import (
	"math"
	"testing"
)

// TestSlowStartDoubles はスロースタートで cwnd が倍々に増え、ssthresh で
// 輻輳回避に切り替わることを確かめる。
func TestSlowStartDoubles(t *testing.T) {
	c := New(8)
	// 1 → 2 → 4 → 8(ここで ssthresh に到達し輻輳回避へ)。
	want := []float64{2, 4, 8}
	for i, w := range want {
		c.OnRoundACKed()
		if c.Cwnd != w {
			t.Fatalf("round %d: cwnd=%v want %v", i, c.Cwnd, w)
		}
	}
	if c.State != CongestionAvoidance {
		t.Fatalf("should be in congestion avoidance, got %v", c.State)
	}
	// 以後は 1 ずつ加算。
	c.OnRoundACKed()
	if c.Cwnd != 9 {
		t.Fatalf("additive increase: cwnd=%v want 9", c.Cwnd)
	}
}

// TestLossHalves は損失で ssthresh と cwnd が半分に切られることを確かめる(乗算減少)。
func TestLossHalves(t *testing.T) {
	c := New(64)
	c.Cwnd = 40
	c.OnLoss()
	if c.Ssthresh != 20 || c.Cwnd != 20 {
		t.Fatalf("after loss: cwnd=%v ssthresh=%v want 20/20", c.Cwnd, c.Ssthresh)
	}
	if c.State != CongestionAvoidance {
		t.Fatal("loss (fast retransmit) should enter congestion avoidance")
	}
}

// TestTimeoutResets はタイムアウトで cwnd が 1 に戻りスロースタートに戻ることを確かめる。
func TestTimeoutResets(t *testing.T) {
	c := New(64)
	c.Cwnd = 40
	c.OnTimeout()
	if c.Cwnd != 1 || c.Ssthresh != 20 {
		t.Fatalf("after timeout: cwnd=%v ssthresh=%v want 1/20", c.Cwnd, c.Ssthresh)
	}
	if c.State != SlowStart {
		t.Fatal("timeout should restart slow start")
	}
}

// TestSawtooth はこの章の主眼その 1。cwnd が容量を探って増減を繰り返す
// のこぎり波になり、容量を大きく超え続けないことを固定する。
func TestSawtooth(t *testing.T) {
	capacity := 32.0
	hist := Simulate(16, capacity, 40)

	rose, fell := false, false
	for i := 1; i < len(hist); i++ {
		if hist[i] > hist[i-1] {
			rose = true
		}
		if hist[i] < hist[i-1] {
			fell = true // 損失で切られた
		}
	}
	if !rose || !fell {
		t.Fatalf("expected sawtooth (rise and fall): rose=%v fell=%v", rose, fell)
	}
	// 容量を大きく超え続けない(超えたら次で半減)。
	for i, v := range hist {
		if v > capacity*2 {
			t.Fatalf("cwnd %v at round %d far exceeds capacity %v", v, i, capacity)
		}
	}
}

// TestAIMDConvergesToFairness はこの章の主眼その 2。不公平な取り分から始めても、
// AIMD で 2 接続の取り分が等しい方へ収束することを固定する。
func TestAIMDConvergesToFairness(t *testing.T) {
	capacity := 40.0
	// 大きく偏った初期状態(片方 30、片方 2)。
	histA, histB := SimulateFairness(capacity, 30, 2, 200)

	initialGap := math.Abs(30 - 2)
	a, b := histA[len(histA)-1], histB[len(histB)-1]
	finalGap := math.Abs(a - b)

	// 取り分の差が大きく縮む。
	if finalGap >= initialGap/4 {
		t.Fatalf("gap should shrink a lot: initial=%v final=%v", initialGap, finalGap)
	}
	// 最終的にほぼ等しい(比が 1 に近い)。
	ratio := a / b
	if ratio < 0.7 || ratio > 1.3 {
		t.Fatalf("final shares should be near-equal, ratio=%v (a=%v b=%v)", ratio, a, b)
	}
}

func TestSsthreshFloor(t *testing.T) {
	c := New(1)
	c.Cwnd = 1
	c.OnLoss()
	if c.Ssthresh < 1 || c.Cwnd < 1 {
		t.Fatalf("cwnd/ssthresh must not drop below 1: cwnd=%v ssthresh=%v", c.Cwnd, c.Ssthresh)
	}
}

func TestStateString(t *testing.T) {
	if SlowStart.String() != "slow-start" || CongestionAvoidance.String() != "congestion-avoidance" {
		t.Fatal("unexpected state strings")
	}
}
