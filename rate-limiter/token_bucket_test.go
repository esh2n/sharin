package ratelimiter

import (
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	tests := []struct {
		name       string
		capacity   int
		ratePerSec float64
		steps      []step
	}{
		{
			name:       "初期状態は満タンで容量分のバーストを許す",
			capacity:   3,
			ratePerSec: 1,
			steps: []step{
				{0, true},
				{0, true},
				{0, true},
				{0, false}, // 4発目は空
			},
		},
		{
			name:       "経過時間に比例して補充される",
			capacity:   3,
			ratePerSec: 1,
			steps: []step{
				{0, true},
				{0, true},
				{0, true},
				{500 * time.Millisecond, false}, // 0.5トークンでは足りない
				{500 * time.Millisecond, true},  // 合計1秒で1トークン
				{0, false},
			},
		},
		{
			name:       "放置してもトークンは容量を超えない",
			capacity:   2,
			ratePerSec: 10,
			steps: []step{
				{10 * time.Second, true}, // 100トークン相当待っても
				{0, true},                // 使えるのは容量の2発まで
				{0, false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newFakeClock()
			b, err := NewTokenBucket(tt.capacity, tt.ratePerSec, clk.Now)
			if err != nil {
				t.Fatalf("NewTokenBucket: %v", err)
			}
			runSteps(t, clk, b, tt.steps)
		})
	}
}

func TestNewTokenBucketValidation(t *testing.T) {
	if _, err := NewTokenBucket(0, 1, nil); err == nil {
		t.Error("capacity=0 でエラーになるべき")
	}
	if _, err := NewTokenBucket(1, 0, nil); err == nil {
		t.Error("ratePerSec=0 でエラーになるべき")
	}
	if _, err := NewTokenBucket(1, 1, nil); err != nil {
		t.Errorf("now=nil は time.Now にフォールバックすべき: %v", err)
	}
}
