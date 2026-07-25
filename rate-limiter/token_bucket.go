package ratelimiter

import (
	"errors"
	"math"
	"sync"
	"time"
)

var _ Limiter = (*TokenBucket)(nil)

// #region tokenbucket
// TokenBucket は容量 capacity のバケツにトークンを ratePerSec 個/秒で補充し、
// リクエストごとに1トークン消費するリミッター。
// バケツに溜まっている分だけバーストを許すのが特徴。
type TokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	ratePerSec float64
	tokens     float64   // 現在のトークン残量
	last       time.Time // 最後に残量を計算した時刻
	now        func() time.Time
}

// NewTokenBucket は満タン状態のバケツを返す。now が nil なら time.Now を使う。
func NewTokenBucket(capacity int, ratePerSec float64, now func() time.Time) (*TokenBucket, error) {
	if capacity <= 0 {
		return nil, errors.New("ratelimiter: capacity must be positive")
	}
	if ratePerSec <= 0 {
		return nil, errors.New("ratelimiter: ratePerSec must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &TokenBucket{
		capacity:   float64(capacity),
		ratePerSec: ratePerSec,
		tokens:     float64(capacity),
		last:       now(),
		now:        now,
	}, nil
}

// Allow は前回からの経過時間ぶんトークンを補充してから、1トークン消費を試みる。
// タイマーで定期補充するのではなく、呼ばれた瞬間に補充量を計算する(lazy refill)。
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := b.now()
	elapsed := t.Sub(b.last).Seconds()
	b.tokens = math.Min(b.capacity, b.tokens+elapsed*b.ratePerSec)
	b.last = t

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// #endregion tokenbucket
