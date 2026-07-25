package ratelimiter

import (
	"errors"
	"sync"
	"time"
)

var _ Limiter = (*FixedWindow)(nil)

// #region fixedwindow
// FixedWindow は時間軸を window 幅で固定に区切り、
// 各区間のリクエスト数を limit までに制限する。
// 実装は最も単純だが、区間の境界をまたぐバーストを止められない(本文参照)。
type FixedWindow struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	start  time.Time // 現在の区間の開始時刻
	count  int
	now    func() time.Time
}

// NewFixedWindow は新しい FixedWindow を返す。now が nil なら time.Now を使う。
func NewFixedWindow(limit int, window time.Duration, now func() time.Time) (*FixedWindow, error) {
	if limit <= 0 {
		return nil, errors.New("ratelimiter: limit must be positive")
	}
	if window <= 0 {
		return nil, errors.New("ratelimiter: window must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &FixedWindow{
		limit:  limit,
		window: window,
		now:    now,
	}, nil
}

// Allow は現在時刻が属する区間を求め、区間が変わっていればカウントをリセットする。
func (w *FixedWindow) Allow() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	start := w.now().Truncate(w.window)
	if !start.Equal(w.start) {
		w.start = start
		w.count = 0
	}

	if w.count >= w.limit {
		return false
	}
	w.count++
	return true
}

// #endregion fixedwindow
