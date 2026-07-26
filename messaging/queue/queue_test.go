package queue

import "testing"

// collect は処理されたメッセージを順に記録するだけの handle を返す。
func collect(out *[]Message) func(Message) {
	return func(m Message) { *out = append(*out, m) }
}

func mustConsumer(t *testing.T, b *Broker, s Semantics, h func(Message)) *Consumer {
	t.Helper()
	c, err := NewConsumer(b, s, h)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	return c
}

func TestFIFOOrder(t *testing.T) {
	b := NewBroker()
	b.Publish("k1", "a")
	b.Publish("k2", "b")
	b.Publish("k3", "c")

	var got []Message
	c := mustConsumer(t, b, AtLeastOnce, collect(&got))
	if n := c.Poll(10); n != 3 {
		t.Fatalf("Poll = %d, want 3", n)
	}
	want := []string{"a", "b", "c"}
	for i, m := range got {
		if m.Body != want[i] || m.Offset != i {
			t.Fatalf("got[%d] = {off:%d body:%q}, want off:%d body:%q", i, m.Offset, m.Body, i, want[i])
		}
	}
}

func TestBrokerDoesNotDeleteAndCursorsAreIndependent(t *testing.T) {
	b := NewBroker()
	for _, s := range []string{"a", "b", "c"} {
		b.Publish("k-"+s, s)
	}
	var g1, g2 []Message
	c1 := mustConsumer(t, b, AtLeastOnce, collect(&g1))
	c2 := mustConsumer(t, b, AtLeastOnce, collect(&g2))
	c1.Poll(10)
	c2.Poll(10)
	if len(g1) != 3 || len(g2) != 3 {
		t.Fatalf("both consumers should see all 3: g1=%d g2=%d", len(g1), len(g2))
	}
	// ブローカはメッセージを消していない。
	if b.Len() != 3 {
		t.Fatalf("broker Len = %d, want 3", b.Len())
	}
}

func TestPollRespectsMaxAndAdvances(t *testing.T) {
	b := NewBroker()
	for i := 0; i < 5; i++ {
		b.Publish("k", "m")
	}
	var got []Message
	c := mustConsumer(t, b, AtLeastOnce, collect(&got))
	if n := c.Poll(2); n != 2 || c.Committed() != 2 {
		t.Fatalf("first Poll(2): n=%d committed=%d", n, c.Committed())
	}
	if n := c.Poll(2); n != 2 || c.Committed() != 4 {
		t.Fatalf("second Poll(2): n=%d committed=%d", n, c.Committed())
	}
	if n := c.Poll(2); n != 1 || c.Committed() != 5 {
		t.Fatalf("third Poll(2): n=%d committed=%d", n, c.Committed())
	}
	if n := c.Poll(2); n != 0 {
		t.Fatalf("Poll on drained log = %d, want 0", n)
	}
}

// at-least-once: 確定前クラッシュ → 再配送され、同じメッセージが2回処理される。
func TestAtLeastOnceRedeliversOnCrash(t *testing.T) {
	b := NewBroker()
	b.Publish("k1", "a")
	b.Publish("k2", "b")

	var got []Message
	c := mustConsumer(t, b, AtLeastOnce, collect(&got))

	if n := c.PollCrash(2); n != 2 {
		t.Fatalf("PollCrash processed %d, want 2", n)
	}
	if c.Committed() != 0 {
		t.Fatalf("after crash committed = %d, want 0 (not committed)", c.Committed())
	}
	// 再起動して読み直す → 同じ2件が再配送される。
	if n := c.Poll(10); n != 2 {
		t.Fatalf("redelivery Poll = %d, want 2", n)
	}
	if len(got) != 4 {
		t.Fatalf("handle called %d times, want 4 (2 processed twice)", len(got))
	}
}

// at-most-once: 確定は先に済むので、処理前クラッシュでメッセージが失われる。
func TestAtMostOnceLosesOnCrash(t *testing.T) {
	b := NewBroker()
	b.Publish("k1", "a")
	b.Publish("k2", "b")

	var got []Message
	c := mustConsumer(t, b, AtMostOnce, collect(&got))

	if n := c.PollCrash(2); n != 0 {
		t.Fatalf("PollCrash processed %d, want 0 (crashed before handling)", n)
	}
	if c.Committed() != 2 {
		t.Fatalf("after crash committed = %d, want 2 (committed before handling)", c.Committed())
	}
	// 再起動しても、確定済みなので再配送されない → 取りこぼし。
	if n := c.Poll(10); n != 0 {
		t.Fatalf("Poll after loss = %d, want 0", n)
	}
	if len(got) != 0 {
		t.Fatalf("handle called %d times, want 0 (messages lost)", len(got))
	}
}

// at-least-once + 冪等 = 実質1回。再配送があっても副作用は1回きり。
func TestIdempotentGivesEffectivelyOnce(t *testing.T) {
	b := NewBroker()
	b.Publish("order-1", "charge")
	b.Publish("order-2", "charge")

	sink := NewIdempotentSink()
	c := mustConsumer(t, b, AtLeastOnce, sink.Handle)

	c.PollCrash(2) // 処理したが確定前にクラッシュ
	c.Poll(10)     // 再配送(同じ2件がもう一度届く)

	if len(sink.Delivered) != 2 {
		t.Fatalf("effectively-once: Delivered = %d, want 2", len(sink.Delivered))
	}
	if sink.Duplicates != 2 {
		t.Fatalf("expected 2 duplicates caught, got %d", sink.Duplicates)
	}
	// 副作用(Delivered)は各注文1回きり。
	keys := map[string]int{}
	for _, m := range sink.Delivered {
		keys[m.Key]++
	}
	for k, n := range keys {
		if n != 1 {
			t.Fatalf("key %s applied %d times, want 1", k, n)
		}
	}
}

// クラッシュが無ければ at-most-once でも全件1回配送される(確定が先なだけ)。
func TestAtMostOnceHappyPath(t *testing.T) {
	b := NewBroker()
	b.Publish("k1", "a")
	b.Publish("k2", "b")
	var got []Message
	c := mustConsumer(t, b, AtMostOnce, collect(&got))
	if n := c.Poll(10); n != 2 || c.Committed() != 2 {
		t.Fatalf("Poll n=%d committed=%d, want 2/2", n, c.Committed())
	}
	if len(got) != 2 {
		t.Fatalf("delivered %d, want 2", len(got))
	}
}

func TestSemanticsString(t *testing.T) {
	if AtMostOnce.String() != "at-most-once" || AtLeastOnce.String() != "at-least-once" {
		t.Fatalf("Semantics.String mismatch")
	}
	if Semantics(99).String() != "unknown" {
		t.Fatalf("unknown Semantics.String = %q", Semantics(99).String())
	}
}

func TestNewConsumerValidation(t *testing.T) {
	if _, err := NewConsumer(nil, AtLeastOnce, func(Message) {}); err == nil {
		t.Error("nil broker should error")
	}
	if _, err := NewConsumer(NewBroker(), AtLeastOnce, nil); err == nil {
		t.Error("nil handle should error")
	}
}
