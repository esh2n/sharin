package circuitbreaker

import (
	"errors"
	"testing"
)

var errBoom = errors.New("boom")

func newTestBreaker() *Breaker {
	return New(Config{
		FailureThreshold: 3, // 連続 3 失敗で開く
		SuccessThreshold: 2, // half-open で連続 2 成功で閉じる
		OpenTimeout:      10,
	})
}

func fail(b *Breaker) error { return b.Call(func() error { return errBoom }) }
func ok(b *Breaker) error   { return b.Call(func() error { return nil }) }

func TestStartsClosedAndPasses(t *testing.T) {
	b := newTestBreaker()
	if b.State() != StateClosed {
		t.Fatalf("initial state = %v, want closed", b.State())
	}
	if err := ok(b); err != nil {
		t.Fatalf("closed breaker should pass call: %v", err)
	}
}

func TestOpensAfterThreshold(t *testing.T) {
	b := newTestBreaker()
	// 2 失敗ではまだ閉じたまま。
	fail(b)
	fail(b)
	if b.State() != StateClosed {
		t.Fatalf("state after 2 fails = %v, want closed", b.State())
	}
	// 3 失敗目で開く。
	if err := fail(b); !errors.Is(err, errBoom) {
		t.Fatalf("3rd fail should return underlying err, got %v", err)
	}
	if b.State() != StateOpen {
		t.Fatalf("state after 3 fails = %v, want open", b.State())
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	ok(b) // 連続失敗がリセットされる
	fail(b)
	fail(b)
	if b.State() != StateClosed {
		t.Fatalf("success should reset streak; state = %v", b.State())
	}
}

func TestOpenFailsFast(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	fail(b) // open

	called := false
	err := b.Call(func() error { called = true; return nil })
	if called {
		t.Fatal("open breaker must not call the function (fail fast)")
	}
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("open breaker should return ErrOpen, got %v", err)
	}
}

func TestHalfOpenAfterTimeout(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	fail(b) // open at t=0

	// タイムアウト未満では開いたまま。
	b.Advance(9)
	if err := ok(b); !errors.Is(err, ErrOpen) {
		t.Fatalf("before timeout should still be open, got %v", err)
	}

	// タイムアウト経過で half-open。試行が 1 つ通る。
	b.Advance(1) // t=10
	called := false
	if err := b.Call(func() error { called = true; return nil }); err != nil {
		t.Fatalf("half-open should allow a trial call: %v", err)
	}
	if !called {
		t.Fatal("half-open must let the trial call through")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %v, want half-open", b.State())
	}
}

func TestHalfOpenClosesOnSuccess(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	fail(b)
	b.Advance(10) // half-open eligible
	ok(b)         // 1st success → still half-open
	if b.State() != StateHalfOpen {
		t.Fatalf("after 1 success state = %v, want half-open", b.State())
	}
	ok(b) // 2nd success → close
	if b.State() != StateClosed {
		t.Fatalf("after 2 successes state = %v, want closed", b.State())
	}
}

func TestHalfOpenReopensOnFailure(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	fail(b)
	b.Advance(10)
	ok(b) // half-open, 1 success
	// half-open 中の失敗は即座に開き直す。
	if err := fail(b); !errors.Is(err, errBoom) {
		t.Fatalf("half-open failure should return underlying err, got %v", err)
	}
	if b.State() != StateOpen {
		t.Fatalf("half-open failure should reopen; state = %v", b.State())
	}
	// 再度タイムアウトを待たないと通らない。
	if err := ok(b); !errors.Is(err, ErrOpen) {
		t.Fatalf("reopened breaker should fail fast, got %v", err)
	}
}

func TestHalfOpenSingleProbe(t *testing.T) {
	b := newTestBreaker()
	fail(b)
	fail(b)
	fail(b)
	b.Advance(10) // eligible for half-open

	// half-open は同時に 1 本だけ試す。試行中はさらなる呼び出しを弾く。
	inProbe := false
	blocked := false
	err := b.Call(func() error {
		inProbe = true
		// 試行の最中に別の呼び出しが来ても通してはいけない。
		if e := b.Call(func() error { return nil }); errors.Is(e, ErrOpen) {
			blocked = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("probe should succeed: %v", err)
	}
	if !inProbe || !blocked {
		t.Fatalf("half-open should admit exactly one probe (inProbe=%v blocked=%v)", inProbe, blocked)
	}
}

func TestConfigValidation(t *testing.T) {
	// ゼロ値は安全側のデフォルトに落とす(壊れない)。
	b := New(Config{})
	if b.State() != StateClosed {
		t.Fatal("zero config should still start closed")
	}
	if err := ok(b); err != nil {
		t.Fatalf("zero config breaker should work: %v", err)
	}
}
