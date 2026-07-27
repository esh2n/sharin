package metrics

import (
	"math"
	"testing"
)

func TestCounterIncAdd(t *testing.T) {
	var c Counter
	c.Inc()
	c.Inc()
	c.Add(5)
	if c.Value() != 7 {
		t.Fatalf("got %v want 7", c.Value())
	}
}

func TestCounterRejectsNegative(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on negative Add")
		}
	}()
	var c Counter
	c.Add(-1)
}

func TestGaugeUpDown(t *testing.T) {
	var g Gauge
	g.Set(10)
	g.Add(5)
	g.Sub(3)
	if g.Value() != 12 {
		t.Fatalf("got %v want 12", g.Value())
	}
}

func TestHistogramObserveAndCount(t *testing.T) {
	h := NewHistogram([]float64{10, 50, 100})
	for _, x := range []float64{5, 8, 30, 30, 70, 200} {
		h.Observe(x)
	}
	// バケット: (-∞,10]=2, (10,50]=2, (50,100]=1, (100,+∞)=1
	_, counts := h.Buckets()
	want := []uint64{2, 2, 1, 1}
	for i := range want {
		if counts[i] != want[i] {
			t.Fatalf("bucket %d: got %d want %d", i, counts[i], want[i])
		}
	}
	if h.Count() != 6 {
		t.Fatalf("count: got %d want 6", h.Count())
	}
	if h.Sum() != 343 {
		t.Fatalf("sum: got %v want 343", h.Sum())
	}
}

// TestMeanHidesTail はこの章の主眼。速い大多数と遅い少数が混じると、
// 平均は低いのに p99 は高い。平均だけ見ているとテールを見逃す。
func TestMeanHidesTail(t *testing.T) {
	h := NewHistogram([]float64{10, 20, 50, 100, 200, 500, 1000})
	// 980 件が速い(約 4ms)、20 件(2%)だけ遅い(約 800ms)。
	for i := 0; i < 980; i++ {
		h.Observe(4)
	}
	for i := 0; i < 20; i++ {
		h.Observe(800)
	}
	mean := h.Mean()
	p99 := h.Quantile(0.99)
	// 平均は 20ms 前後(速い方に引っ張られてテールが埋もれる)。
	if mean > 25 {
		t.Fatalf("mean %v unexpectedly high", mean)
	}
	// p99 は遅い側にあり、平均よりずっと大きい。
	if p99 <= mean*10 {
		t.Fatalf("expected p99 (%v) >> mean (%v)", p99, mean)
	}
	t.Logf("mean=%.1f p99=%.1f", mean, p99)
}

func TestQuantileInterpolates(t *testing.T) {
	h := NewHistogram([]float64{100})
	// (0,100] に 100 件を均等とみなす。q=0.5 は約 50。
	for i := 0; i < 100; i++ {
		h.Observe(50)
	}
	// 全部が (0,100] バケット。q=0.5 → 0 + 100*0.5 = 50。
	if got := h.Quantile(0.5); math.Abs(got-50) > 1e-9 {
		t.Fatalf("q0.5: got %v want 50", got)
	}
	// q=0.99 → 99。
	if got := h.Quantile(0.99); math.Abs(got-99) > 1e-9 {
		t.Fatalf("q0.99: got %v want 99", got)
	}
}

func TestQuantileEdges(t *testing.T) {
	empty := NewHistogram([]float64{10})
	if empty.Quantile(0.5) != 0 {
		t.Fatal("empty quantile should be 0")
	}
	if empty.Mean() != 0 {
		t.Fatal("empty mean should be 0")
	}
	h := NewHistogram([]float64{10, 100})
	h.Observe(5)
	// q<=0 は 0、q>=1 は +Inf バケット→最大有限上限で頭打ち。
	if h.Quantile(0) != 0 {
		t.Fatalf("q0 got %v", h.Quantile(0))
	}
	// 全観測が最初のバケットなので q=1 でもそこに収まる。
	if got := h.Quantile(1); got != 10 {
		t.Fatalf("q1 got %v want 10", got)
	}
}

func TestQuantileInfBucketClamps(t *testing.T) {
	h := NewHistogram([]float64{10, 100})
	h.Observe(5)
	h.Observe(9999) // +Inf バケット
	// p99 は +Inf バケットに入るが上限が無いので最大有限上限 100 で頭打ち。
	if got := h.Quantile(0.99); got != 100 {
		t.Fatalf("got %v want 100 (clamped)", got)
	}
}

// TestMergeCombinesDistributions はヒストグラムの合算性を固定する。
// 2 台のバケットを足すと、全体の分布として p99 が出せる。
func TestMergeCombinesDistributions(t *testing.T) {
	bounds := []float64{10, 50, 100, 500}
	a := NewHistogram(bounds)
	b := NewHistogram(bounds)
	// a は速い台、b は遅い台。
	for i := 0; i < 100; i++ {
		a.Observe(5)
	}
	for i := 0; i < 100; i++ {
		b.Observe(300)
	}
	a.Merge(b)
	if a.Count() != 200 {
		t.Fatalf("merged count: got %d want 200", a.Count())
	}
	// 合算後、中央値は速い側、p99 は遅い側。
	if med := a.Quantile(0.5); med > 50 {
		t.Fatalf("median %v should be in fast side", med)
	}
	if p99 := a.Quantile(0.99); p99 < 100 {
		t.Fatalf("p99 %v should be in slow side", p99)
	}
}

func TestMergeMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on bounds mismatch")
		}
	}()
	a := NewHistogram([]float64{10})
	b := NewHistogram([]float64{10, 20})
	a.Merge(b)
}
