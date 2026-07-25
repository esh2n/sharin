package ratelimiter

import (
	"testing"
	"time"
)

func TestSlidingWindowLog(t *testing.T) {
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
			// fixed window の境界バーストと同じシナリオ。
			// こちらは「直近1秒間」を常に見るので2倍バーストは通らない。
			name:   "境界バーストを防ぐ(fixed window との対比)",
			limit:  3,
			window: time.Second,
			steps: []step{
				{700 * time.Millisecond, true}, // t=0.70
				{0, true},
				{0, true},
				{350 * time.Millisecond, false}, // t=1.05 直近1秒に3件残っている
			},
		},
		{
			name:   "窓から古い記録が抜ければまた通る",
			limit:  3,
			window: time.Second,
			steps: []step{
				{700 * time.Millisecond, true},  // t=0.70
				{0, true},                       //
				{0, true},                       //
				{350 * time.Millisecond, false}, // t=1.05
				{700 * time.Millisecond, true},  // t=1.75 t=0.70 の3件は窓外
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newFakeClock()
			l, err := NewSlidingWindowLog(tt.limit, tt.window, clk.Now)
			if err != nil {
				t.Fatalf("NewSlidingWindowLog: %v", err)
			}
			runSteps(t, clk, l, tt.steps)
		})
	}
}

func TestNewSlidingWindowLogValidation(t *testing.T) {
	if _, err := NewSlidingWindowLog(0, time.Second, nil); err == nil {
		t.Error("limit=0 でエラーになるべき")
	}
	if _, err := NewSlidingWindowLog(1, 0, nil); err == nil {
		t.Error("window=0 でエラーになるべき")
	}
}
