package ratelimiter

import (
	"errors"
	"math"
	"sync"
	"time"
)

var _ Limiter = (*LeakyBucket)(nil)

// #region leakybucket
// LeakyBucket は底から leakPerSec ずつ水が漏れるバケツ。
// リクエストは水1杯ぶんで、溢れなければ受け付ける。
// token bucket と対称的な実装だが「出ていく速度が一定」という見方をする。
type LeakyBucket struct {
	mu         sync.Mutex
	capacity   float64
	leakPerSec float64
	water      float64   // 現在の水位
	last       time.Time // 最後に水位を計算した時刻
	now        func() time.Time
}

// NewLeakyBucket は空のバケツを返す。now が nil なら time.Now を使う。
func NewLeakyBucket(capacity int, leakPerSec float64, now func() time.Time) (*LeakyBucket, error) {
	if capacity <= 0 {
		return nil, errors.New("ratelimiter: capacity must be positive")
	}
	if leakPerSec <= 0 {
		return nil, errors.New("ratelimiter: leakPerSec must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &LeakyBucket{
		capacity:   float64(capacity),
		leakPerSec: leakPerSec,
		last:       now(),
		now:        now,
	}, nil
}

// Allow は前回からの経過時間ぶん水を漏らしてから、水1杯の追加を試みる。
func (b *LeakyBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := b.now()
	elapsed := t.Sub(b.last).Seconds()
	b.water = math.Max(0, b.water-elapsed*b.leakPerSec)
	b.last = t

	if b.water+1 > b.capacity {
		return false
	}
	b.water++
	return true
}

// #endregion leakybucket
