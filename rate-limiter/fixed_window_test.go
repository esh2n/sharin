package ratelimiter

import (
	"testing"
	"time"
)

func TestFixedWindow(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		window time.Duration
		steps  []step
	}{
		{
			name:   "ウィンドウ内は limit 回まで",
			limit:  3,
			window: time.Second,
			steps: []step{
				{0, true},
				{0, true},
				{0, true},
				{0, false},
			},
		},
		{
			name:   "ウィンドウが切り替わるとカウントはリセット",
			limit:  2,
			window: time.Second,
			steps: []step{
				{0, true},
				{0, true},
				{0, false},
				{time.Second, true}, // 次のウィンドウ
				{0, true},
				{0, false},
			},
		},
		{
			// この方式の弱点をテストとして固定する:
			// 境界をまたぐと窓幅より短い時間に limit の2倍が通ってしまう。
			name:   "境界バースト: 0.35秒間に limit の2倍が通る(仕様上の弱点)",
			limit:  3,
			window: time.Second,
			steps: []step{
				{700 * time.Millisecond, true}, // t=0.70 窓[0,1)
				{0, true},
				{0, true},
				{350 * time.Millisecond, true}, // t=1.05 窓[1,2) でリセット済み
				{0, true},
				{0, true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newFakeClock()
			w, err := NewFixedWindow(tt.limit, tt.window, clk.Now)
			if err != nil {
				t.Fatalf("NewFixedWindow: %v", err)
			}
			runSteps(t, clk, w, tt.steps)
		})
	}
}

func TestNewFixedWindowValidation(t *testing.T) {
	if _, err := NewFixedWindow(0, time.Second, nil); err == nil {
		t.Error("limit=0 でエラーになるべき")
	}
	if _, err := NewFixedWindow(1, 0, nil); err == nil {
		t.Error("window=0 でエラーになるべき")
	}
}
