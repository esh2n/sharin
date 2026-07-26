package dlock

import "testing"

func TestMutualExclusion(t *testing.T) {
	s := New()
	tok, ok := s.Acquire("A", 10)
	if !ok || tok != 1 {
		t.Fatalf("A acquire: ok=%v tok=%d", ok, tok)
	}
	// 保持中は他が取れない。
	if _, ok := s.Acquire("B", 10); ok {
		t.Fatalf("B should not acquire while A holds")
	}
	h, ht := s.Holder()
	if h != "A" || ht != 1 {
		t.Fatalf("holder: %s token %d", h, ht)
	}
}

func TestLeaseExpiresAndReacquire(t *testing.T) {
	s := New()
	s.Acquire("A", 10)
	if _, ok := s.Acquire("B", 10); ok {
		t.Fatalf("B should not acquire before expiry")
	}
	s.Tick(10) // リース失効(clock 0→10, expiry=10, clock>=expiry)
	if h, _ := s.Holder(); h != "" {
		t.Fatalf("lock should be free after expiry, holder=%s", h)
	}
	tok, ok := s.Acquire("B", 10)
	if !ok {
		t.Fatalf("B should acquire after A's lease expired")
	}
	if tok != 2 {
		t.Fatalf("fencing token should advance to 2, got %d", tok)
	}
}

func TestFencingTokensMonotonic(t *testing.T) {
	s := New()
	var toks []int64
	for i := 0; i < 3; i++ {
		tok, ok := s.Acquire("C", 5)
		if !ok {
			t.Fatalf("acquire %d failed", i)
		}
		toks = append(toks, tok)
		s.Release("C")
	}
	for i := 1; i < len(toks); i++ {
		if toks[i] <= toks[i-1] {
			t.Fatalf("tokens not monotonic: %v", toks)
		}
	}
}

func TestRenewExtendsLease(t *testing.T) {
	s := New()
	s.Acquire("A", 10)
	s.Tick(8)
	if !s.Renew("A", 10) { // expiry を 18 へ延長
		t.Fatalf("A should renew its own lease")
	}
	s.Tick(8) // clock=16 < 18: まだ有効
	if h, _ := s.Holder(); h != "A" {
		t.Fatalf("lease should still be held after renew, holder=%s", h)
	}
	if _, ok := s.Acquire("B", 10); ok {
		t.Fatalf("B should not acquire while A's renewed lease is valid")
	}
	// 持ち主でない者の Renew は失敗。
	if s.Renew("B", 10) {
		t.Fatalf("non-holder renew should fail")
	}
}

func TestReleaseFreesLock(t *testing.T) {
	s := New()
	s.Acquire("A", 100)
	s.Release("B") // 持ち主でない → 何も起きない
	if h, _ := s.Holder(); h != "A" {
		t.Fatalf("release by non-holder should be no-op, holder=%s", h)
	}
	s.Release("A")
	if h, _ := s.Holder(); h != "" {
		t.Fatalf("lock should be free after release, holder=%s", h)
	}
}

// これが本題: リース失効による二重取得でも、フェンシングトークンが古い持ち主の
// 書き込みを弾いて破壊を防ぐ。
func TestFencingPreventsStaleWrite(t *testing.T) {
	s := New()
	res := NewResource("row-42")

	// A がロックを取り、トークン 1 で書き込む。
	tokA, _ := s.Acquire("A", 10)
	if !res.Write(tokA, "A-value") {
		t.Fatalf("A's write should succeed")
	}

	// A が GC で一時停止している間にリースが失効する。
	s.Tick(10)

	// B がロックを取り(トークン 2)、書き込む。
	tokB, ok := s.Acquire("B", 10)
	if !ok || tokB != 2 {
		t.Fatalf("B acquire: ok=%v tok=%d", ok, tokB)
	}
	if !res.Write(tokB, "B-value") {
		t.Fatalf("B's write should succeed")
	}

	// A が再開。まだ自分が持っているつもりで、古いトークン 1 で書き込む。
	if res.Write(tokA, "A-STALE") {
		t.Fatalf("A's stale write should be fenced off (rejected)")
	}
	if res.Data() != "B-value" {
		t.Fatalf("resource corrupted: got %q, want B-value", res.Data())
	}
	if res.Rejected != 1 {
		t.Fatalf("expected 1 fenced write, got %d", res.Rejected)
	}
}

// 対比: フェンシングが無ければ、古い持ち主の書き込みが新しい値を破壊する。
func TestWithoutFencingCorrupts(t *testing.T) {
	s := New()
	res := NewUnfencedResource("row-42")

	tokA, _ := s.Acquire("A", 10)
	res.Write(tokA, "A-value")
	s.Tick(10)
	tokB, _ := s.Acquire("B", 10)
	res.Write(tokB, "B-value")

	// フェンシング無し → A の出遅れた書き込みが通ってしまう(破壊)。
	if !res.Write(tokA, "A-STALE") {
		t.Fatalf("unfenced resource should accept any write")
	}
	if res.Data() != "A-STALE" {
		t.Fatalf("without fencing, stale write should corrupt: got %q", res.Data())
	}
}

func TestResourceAccessorsAndClamp(t *testing.T) {
	s := New()
	// lease<1 は 1 にクランプ。
	tok, _ := s.Acquire("A", 0)
	if s.Expiry() != s.Now()+1 {
		t.Fatalf("lease clamp: expiry=%d now=%d", s.Expiry(), s.Now())
	}
	// Renew の lease<1 もクランプ。
	s.Renew("A", 0)
	res := NewResource("r")
	res.Write(tok, "v")
	if res.MaxToken() != tok || !res.Fenced() || res.Data() != "v" {
		t.Fatalf("resource accessors wrong: max=%d fenced=%v data=%q", res.MaxToken(), res.Fenced(), res.Data())
	}
	// Tick(0) は無視。
	before := s.Now()
	s.Tick(0)
	if s.Now() != before {
		t.Fatalf("Tick(0) should not advance")
	}
}

func TestSameHolderWritesRepeatedly(t *testing.T) {
	// 同じトークンでの複数回書き込みは受理される(>= 判定)。
	s := New()
	tok, _ := s.Acquire("A", 100)
	res := NewResource("r")
	if !res.Write(tok, "v1") || !res.Write(tok, "v2") {
		t.Fatalf("same-token writes should both succeed")
	}
	if res.Data() != "v2" {
		t.Fatalf("data should be latest: %q", res.Data())
	}
}
