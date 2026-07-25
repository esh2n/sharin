package sampling

import (
	"math"
	"testing"
)

const eps = 1e-9

func almostEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > eps {
			return false
		}
	}
	return true
}

func TestSoftmax(t *testing.T) {
	tests := []struct {
		name   string
		logits []float64
		want   []float64
	}{
		{
			name:   "同じlogitsなら一様分布",
			logits: []float64{1, 1, 1, 1},
			want:   []float64{0.25, 0.25, 0.25, 0.25},
		},
		{
			name:   "差が大きいほど偏る",
			logits: []float64{math.Log(3), math.Log(1)},
			want:   []float64{0.75, 0.25},
		},
		{
			name:   "-Infのトークンは確率0",
			logits: []float64{0, math.Inf(-1)},
			want:   []float64{1, 0},
		},
		{
			// 素朴に exp(1000) を計算すると overflow して NaN になる。
			// max を引く数値安定化がされていればこのテストが通る。
			name:   "巨大なlogitsでもoverflowしない",
			logits: []float64{1000, 1000},
			want:   []float64{0.5, 0.5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Softmax(tt.logits)
			if err != nil {
				t.Fatalf("Softmax: %v", err)
			}
			if !almostEqual(got, tt.want) {
				t.Errorf("Softmax(%v) = %v, want %v", tt.logits, got, tt.want)
			}
		})
	}

	if _, err := Softmax(nil); err == nil {
		t.Error("空のlogitsはエラーになるべき")
	}
}

func TestGreedy(t *testing.T) {
	got, err := Greedy([]float64{0.1, 2.5, 1.3})
	if err != nil || got != 1 {
		t.Errorf("Greedy = %d, %v; want 1, nil", got, err)
	}
	if _, err := Greedy(nil); err == nil {
		t.Error("空のlogitsはエラーになるべき")
	}
}

func TestSample(t *testing.T) {
	probs := []float64{0.2, 0.5, 0.3}
	tests := []struct {
		r    float64
		want int
	}{
		{0.0, 0},
		{0.19, 0},
		{0.2, 1},
		{0.69, 1},
		{0.7, 2},
		{0.999, 2},
	}
	for _, tt := range tests {
		got, err := Sample(probs, func() float64 { return tt.r })
		if err != nil {
			t.Fatalf("Sample(r=%v): %v", tt.r, err)
		}
		if got != tt.want {
			t.Errorf("Sample(r=%v) = %d, want %d", tt.r, got, tt.want)
		}
	}

	if _, err := Sample(nil, func() float64 { return 0 }); err == nil {
		t.Error("空のprobsはエラーになるべき")
	}
	if _, err := Sample(probs, nil); err == nil {
		t.Error("rng=nilはエラーになるべき")
	}
}
