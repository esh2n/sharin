package ratelimiter

import (
	"testing"
	"time"
)

// fakeClock は手動で進める時計。テストから now() として注入する。
type fakeClock struct {
	t time.Time
}

// newFakeClock はウィンドウ境界の計算が決定的になるよう、
// 秒単位に揃った固定時刻から始まる時計を返す。
func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

// step は「時計を advance 進めてから Allow を1回呼ぶと want が返る」を表す。
type step struct {
	advance time.Duration
	want    bool
}

func runSteps(t *testing.T, clk *fakeClock, l Limiter, steps []step) {
	t.Helper()
	for i, s := range steps {
		clk.Advance(s.advance)
		if got := l.Allow(); got != s.want {
			t.Fatalf("step %d (t=%s): Allow() = %v, want %v",
				i, clk.Now().Format("15:04:05.000"), got, s.want)
		}
	}
}
