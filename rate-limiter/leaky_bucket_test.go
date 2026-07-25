package ratelimiter

import (
	"testing"
	"time"
)

func TestLeakyBucket(t *testing.T) {
	tests := []struct {
		name       string
		capacity   int
		leakPerSec float64
		steps      []step
	}{
		{
			name:       "容量を超えた分は溢れて拒否される",
			capacity:   3,
			leakPerSec: 1,
			steps: []step{
				{0, true},
				{0, true},
				{0, true},
				{0, false}, // バケツ満杯
			},
		},
		{
			name:       "漏れた分だけ新しく受け付けられる",
			capacity:   3,
			leakPerSec: 1,
			steps: []step{
				{0, true},
				{0, true},
				{0, true},
				{500 * time.Millisecond, false}, // 0.5しか漏れていない
				{500 * time.Millisecond, true},  // 1秒で1杯分空いた
				{0, false},
			},
		},
		{
			name:       "空のバケツはすぐ受け付けるがバーストは容量まで",
			capacity:   1,
			leakPerSec: 10,
			steps: []step{
				{time.Hour, true}, // どれだけ待っても
				{0, false},        // 容量1なら連続2発目は拒否
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newFakeClock()
			b, err := NewLeakyBucket(tt.capacity, tt.leakPerSec, clk.Now)
			if err != nil {
				t.Fatalf("NewLeakyBucket: %v", err)
			}
			runSteps(t, clk, b, tt.steps)
		})
	}
}

func TestNewLeakyBucketValidation(t *testing.T) {
	if _, err := NewLeakyBucket(0, 1, nil); err == nil {
		t.Error("capacity=0 でエラーになるべき")
	}
	if _, err := NewLeakyBucket(1, -1, nil); err == nil {
		t.Error("leakPerSec<=0 でエラーになるべき")
	}
}
