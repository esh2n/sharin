package moe

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/tensor"
)

func input(rows, cols int) *tensor.Tensor {
	x := tensor.New(rows, cols)
	v := float32(0.29)
	for i := range x.Data {
		v = float32(math.Mod(float64(v)*7.13+0.37, 1.0))
		x.Data[i] = v*2 - 1
	}
	return x
}

func TestNewValidates(t *testing.T) {
	bad := []Config{
		{DModel: 8, DHidden: 16, NExperts: 4, TopK: 0},
		{DModel: 8, DHidden: 16, NExperts: 4, TopK: 5},
		{DModel: 0, DHidden: 16, NExperts: 4, TopK: 2},
		{DModel: 8, DHidden: 0, NExperts: 4, TopK: 2},
	}
	for _, c := range bad {
		if _, err := New(c); err == nil {
			t.Errorf("config %+v should be rejected", c)
		}
	}
	if _, err := New(Config{DModel: 8, DHidden: 16, NExperts: 4, TopK: 2}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestRouteTopKWeights(t *testing.T) {
	m, _ := New(Config{DModel: 8, DHidden: 16, NExperts: 8, TopK: 2})
	x := input(1, 8)
	row := make([]float32, 8)
	copy(row, x.Data[:8])
	asg := m.Route(row)
	if len(asg) != 2 {
		t.Fatalf("assignments = %d, want 2", len(asg))
	}
	sum := float32(0)
	for _, a := range asg {
		if a.Expert < 0 || a.Expert >= 8 {
			t.Fatalf("expert out of range: %d", a.Expert)
		}
		if a.Weight <= 0 {
			t.Fatalf("weight must be positive: %f", a.Weight)
		}
		sum += a.Weight
	}
	if math.Abs(float64(sum-1)) > 1e-5 {
		t.Fatalf("weights should sum to 1, got %f", sum)
	}
	if asg[0].Expert == asg[1].Expert {
		t.Fatal("top-k experts must be distinct")
	}
}

func TestForwardSparsity(t *testing.T) {
	m, _ := New(Config{DModel: 8, DHidden: 16, NExperts: 8, TopK: 2})
	x := input(6, 8)
	out, stats := m.Forward(x)
	if out.Rows != 6 || out.Cols != 8 {
		t.Fatalf("shape = (%d,%d)", out.Rows, out.Cols)
	}
	// 各トークンはちょうど top-k 個の expert しか使わない。
	total := 0
	for _, n := range stats.TokensPerExpert {
		total += n
	}
	if total != 6*2 {
		t.Fatalf("total assignments = %d, want 12", total)
	}
}

func TestSingleExpertEqualsPlainFFN(t *testing.T) {
	// expert 1 個・top-1 なら、ルータは選択の余地がなく重み 1 で唯一の expert を通す。
	// つまり MoE は普通の FFN と完全に一致する。
	m, _ := New(Config{DModel: 8, DHidden: 16, NExperts: 1, TopK: 1})
	x := input(4, 8)
	out, _ := m.Forward(x)
	want := m.ExpertForward(0, x)
	for i := range out.Data {
		if math.Abs(float64(out.Data[i]-want.Data[i])) > 1e-6 {
			t.Fatal("single-expert MoE should equal its plain FFN")
		}
	}
}

func TestParamsAccounting(t *testing.T) {
	c := Config{DModel: 64, DHidden: 256, NExperts: 8, TopK: 2}
	total := c.TotalParams()
	active := c.ActiveParams()
	if total <= active {
		t.Fatalf("total %d should exceed active %d", total, active)
	}
	// expert 部分は 8 個中 2 個しか使わないので、総量はアクティブの約 4 倍。
	expertParams := 2 * 64 * 256
	wantTotal := 64*8 + 8*expertParams
	wantActive := 64*8 + 2*expertParams
	if total != wantTotal || active != wantActive {
		t.Fatalf("params = %d/%d, want %d/%d", total, active, wantTotal, wantActive)
	}
}

func TestLoadBalanceLoss(t *testing.T) {
	// 均等に散れば loss ≈ 1(最小)、1 つの expert に集中すれば loss ≈ N(最大)。
	uniform := &Stats{TokensPerExpert: []int{4, 4, 4, 4}}
	skewed := &Stats{TokensPerExpert: []int{16, 0, 0, 0}}
	lu := LoadBalanceLoss(uniform)
	ls := LoadBalanceLoss(skewed)
	if math.Abs(float64(lu-1)) > 1e-5 {
		t.Fatalf("uniform loss = %f, want 1", lu)
	}
	if math.Abs(float64(ls-4)) > 1e-5 {
		t.Fatalf("skewed loss = %f, want 4", ls)
	}
	if LoadBalanceLoss(&Stats{TokensPerExpert: []int{0, 0}}) != 0 {
		t.Fatal("empty stats should give 0")
	}
}
