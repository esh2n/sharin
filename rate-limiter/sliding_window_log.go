package ratelimiter

import (
	"errors"
	"sync"
	"time"
)

var _ Limiter = (*SlidingWindowLog)(nil)

// #region slidingwindowlog
// SlidingWindowLog は通したリクエストの時刻をすべて記録し、
// 「直近 window の間に limit 件未満か」を毎回正確に判定する。
// 境界バーストが起きない代わりに、リミットぶんの時刻を保持するメモリを食う。
type SlidingWindowLog struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	log    []time.Time // 通したリクエストの時刻(昇順)
	now    func() time.Time
}

// NewSlidingWindowLog は新しい SlidingWindowLog を返す。now が nil なら time.Now を使う。
func NewSlidingWindowLog(limit int, window time.Duration, now func() time.Time) (*SlidingWindowLog, error) {
	if limit <= 0 {
		return nil, errors.New("ratelimiter: limit must be positive")
	}
	if window <= 0 {
		return nil, errors.New("ratelimiter: window must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &SlidingWindowLog{
		limit:  limit,
		window: window,
		log:    make([]time.Time, 0, limit),
		now:    now,
	}, nil
}

// Allow は窓の外に出た古い記録を捨ててから、残り件数で判定する。
func (l *SlidingWindowLog) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	t := l.now()
	cutoff := t.Add(-l.window)

	// 先頭(最も古い)から、窓の外に出たぶんだけ切り詰める。
	evict := 0
	for evict < len(l.log) && !l.log[evict].After(cutoff) {
		evict++
	}
	l.log = append(l.log[:0], l.log[evict:]...)

	if len(l.log) >= l.limit {
		return false
	}
	l.log = append(l.log, t)
	return true
}

// #endregion slidingwindowlog
