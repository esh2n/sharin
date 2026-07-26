package pubsub

import "testing"

func bodies(ms []Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Body
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// ファンアウト: 購読者全員が全メッセージを受け取る(1件を1人が奪うキューとは違う)。
func TestFanoutAllSubscribersGetAll(t *testing.T) {
	b := NewBroker()
	a := b.Subscribe("news", FromBeginning)
	c := b.Subscribe("news", FromBeginning)
	b.Publish("news", "m1")
	b.Publish("news", "m2")

	eq(t, bodies(a.Poll(10)), []string{"m1", "m2"})
	eq(t, bodies(c.Poll(10)), []string{"m1", "m2"})
}

// 独立カーソル: 遅い購読者がいても他は止まらない。
func TestIndependentCursors(t *testing.T) {
	b := NewBroker()
	slow := b.Subscribe("news", FromBeginning)
	fast := b.Subscribe("news", FromBeginning)
	b.Publish("news", "m1")
	b.Publish("news", "m2")
	b.Publish("news", "m3")

	// fast は全部読む。slow は読まない。
	eq(t, bodies(fast.Poll(10)), []string{"m1", "m2", "m3"})
	if slow.Backlog() != 3 {
		t.Fatalf("slow backlog = %d, want 3", slow.Backlog())
	}
	if fast.Backlog() != 0 {
		t.Fatalf("fast backlog = %d, want 0", fast.Backlog())
	}
	// slow が後から読んでも、取りこぼしていない。
	eq(t, bodies(slow.Poll(10)), []string{"m1", "m2", "m3"})
}

// FromNow は購読時点より前を飛ばす。
func TestFromNowSkipsBacklog(t *testing.T) {
	b := NewBroker()
	b.Publish("news", "old1")
	b.Publish("news", "old2")
	s := b.Subscribe("news", FromNow)
	if s.Cursor() != 2 {
		t.Fatalf("FromNow cursor = %d, want 2", s.Cursor())
	}
	b.Publish("news", "new1")
	eq(t, bodies(s.Poll(10)), []string{"new1"})
}

// FromBeginning は過去ぶんも再生する。
func TestFromBeginningReplays(t *testing.T) {
	b := NewBroker()
	b.Publish("news", "old1")
	b.Publish("news", "old2")
	s := b.Subscribe("news", FromBeginning)
	eq(t, bodies(s.Poll(10)), []string{"old1", "old2"})
}

// トピックは互いに独立。
func TestTopicsAreIsolated(t *testing.T) {
	b := NewBroker()
	sports := b.Subscribe("sports", FromBeginning)
	b.Publish("news", "n1")
	b.Publish("sports", "s1")

	eq(t, bodies(sports.Poll(10)), []string{"s1"})
	if b.NumTopics() != 2 {
		t.Fatalf("NumTopics = %d, want 2", b.NumTopics())
	}
}

func TestPollRespectsMaxAndDrains(t *testing.T) {
	b := NewBroker()
	s := b.Subscribe("t", FromBeginning)
	for i := 0; i < 5; i++ {
		b.Publish("t", "m")
	}
	if got := s.Poll(2); len(got) != 2 {
		t.Fatalf("Poll(2) = %d, want 2", len(got))
	}
	if got := s.Poll(2); len(got) != 2 {
		t.Fatalf("Poll(2) = %d, want 2", len(got))
	}
	if got := s.Poll(2); len(got) != 1 {
		t.Fatalf("Poll(2) = %d, want 1", len(got))
	}
	if got := s.Poll(2); got != nil {
		t.Fatalf("Poll on drained = %v, want nil", got)
	}
}

func TestLenAndUnknownTopic(t *testing.T) {
	b := NewBroker()
	if b.Len("nope") != 0 {
		t.Fatal("unknown topic Len should be 0")
	}
	s := b.Subscribe("nope", FromBeginning)
	if got := s.Poll(10); got != nil {
		t.Fatalf("Poll on empty topic = %v, want nil", got)
	}
	b.Publish("nope", "x")
	if b.Len("nope") != 1 {
		t.Fatalf("Len = %d, want 1", b.Len("nope"))
	}
}
