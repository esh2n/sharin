package retry

import (
	"errors"
	"testing"
)

var errTransient = errors.New("transient")

func TestBackoffExponentialCapped(t *testing.T) {
	// 遅延は base·mult^n で増え、max で頭打ちになる(ジッターなし)。
	p := Policy{BaseDelay: 10, MaxDelay: 100, Multiplier: 2, Jitter: JitterNone}
	want := []int{10, 20, 40, 80, 100, 100} // 160 は max=100 に切られる
	for i, w := range want {
		if got := p.Backoff(i, nil); got != w {
			t.Fatalf("backoff(%d) = %d, want %d", i, got, w)
		}
	}
}

func TestFullJitterInRange(t *testing.T) {
	// full jitter は [0, base·mult^n] の一様乱数。上限を超えない。
	p := Policy{BaseDelay: 10, MaxDelay: 1000, Multiplier: 2, Jitter: JitterFull}
	r := NewRand(1)
	for attempt := 0; attempt < 6; attempt++ {
		cap := p.rawDelay(attempt)
		for i := 0; i < 50; i++ {
			d := p.Backoff(attempt, r)
			if d < 0 || d > cap {
				t.Fatalf("full jitter %d out of [0,%d]", d, cap)
			}
		}
	}
}

func TestEqualJitterInRange(t *testing.T) {
	// equal jitter は half + [0, half]。下限が上がり、上限は raw。
	p := Policy{BaseDelay: 100, MaxDelay: 10000, Multiplier: 2, Jitter: JitterEqual}
	r := NewRand(3)
	raw := p.rawDelay(2) // 400
	for i := 0; i < 100; i++ {
		d := p.Backoff(2, r)
		if d < raw/2 || d > raw {
			t.Fatalf("equal jitter %d out of [%d,%d]", d, raw/2, raw)
		}
	}
}

func TestRetrySucceedsEventually(t *testing.T) {
	// 3 回目で成功するなら、リトライで最終的に成功する。
	p := Policy{MaxAttempts: 5, BaseDelay: 1, MaxDelay: 100, Multiplier: 2, Jitter: JitterNone}
	calls := 0
	res := p.Do(func() error {
		calls++
		if calls < 3 {
			return errTransient
		}
		return nil
	}, nil)
	if res.Err != nil {
		t.Fatalf("should succeed, got %v", res.Err)
	}
	if res.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", res.Attempts)
	}
	// 待った合計は backoff(0)+backoff(1) = 1+2 = 3(成功したステップの前だけ待つ)。
	if res.TotalDelay != 3 {
		t.Fatalf("total delay = %d, want 3", res.TotalDelay)
	}
}

func TestRetryGivesUp(t *testing.T) {
	// ずっと失敗するなら MaxAttempts で諦め、最後のエラーを返す。
	p := Policy{MaxAttempts: 3, BaseDelay: 1, MaxDelay: 100, Multiplier: 2, Jitter: JitterNone}
	calls := 0
	res := p.Do(func() error { calls++; return errTransient }, nil)
	if !errors.Is(res.Err, errTransient) {
		t.Fatalf("should return last error, got %v", res.Err)
	}
	if res.Attempts != 3 || calls != 3 {
		t.Fatalf("attempts = %d calls = %d, want 3", res.Attempts, calls)
	}
	// 待ちは試行の間の 2 回だけ(最後の失敗後は待たない)。
	if res.TotalDelay != 1+2 {
		t.Fatalf("total delay = %d, want 3", res.TotalDelay)
	}
}

func TestPermanentErrorNotRetried(t *testing.T) {
	// Permanent でラップしたエラーは再試行せず即諦める。
	perm := Permanent(errors.New("bad request"))
	p := Policy{MaxAttempts: 5, BaseDelay: 1, Multiplier: 2, Jitter: JitterNone}
	calls := 0
	res := p.Do(func() error { calls++; return perm }, nil)
	if calls != 1 {
		t.Fatalf("permanent error should not retry; calls = %d", calls)
	}
	if res.Err == nil {
		t.Fatal("should surface the error")
	}
}

func TestJitterReducesCollision(t *testing.T) {
	// リトライ嵐の緩和: 多数のクライアントが同時に失敗した場合、
	// ジッターなしは全員が同じ時刻に再送(衝突)、full jitter は分散する。
	p := Policy{BaseDelay: 100, MaxDelay: 10000, Multiplier: 2, Jitter: JitterFull}
	r := NewRand(7)
	seen := map[int]int{}
	for client := 0; client < 100; client++ {
		d := p.Backoff(3, r) // 全員 attempt 3 で再送
		seen[d]++
	}
	// full jitter なら 100 クライアントが多数の異なる時刻に散る。
	if len(seen) < 50 {
		t.Fatalf("full jitter should spread retries, got %d distinct delays", len(seen))
	}
	// 対照: ジッターなしは全員同時刻。
	pNone := Policy{BaseDelay: 100, MaxDelay: 10000, Multiplier: 2, Jitter: JitterNone}
	if d := pNone.Backoff(3, nil); d != 800 {
		t.Fatalf("no jitter should be deterministic 800, got %d", d)
	}
}

func TestZeroConfigDefaults(t *testing.T) {
	// ゼロ値でも壊れない(1回は試す)。
	var p Policy
	res := p.Do(func() error { return nil }, nil)
	if res.Err != nil || res.Attempts != 1 {
		t.Fatalf("zero policy: err=%v attempts=%d", res.Err, res.Attempts)
	}
}
