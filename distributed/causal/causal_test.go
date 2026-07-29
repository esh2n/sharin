package causal

import (
	"testing"

	"github.com/esh2n/sharin/distributed/clock"
)

func eq(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// この章の中心その1。返信が投稿より先に届いても、先には渡さない。
func TestReplyWaitsForWhatItAnswers(t *testing.T) {
	s := New(Causal, "a", "b", "c")

	q := s.Broadcasts("a", "質問", "b") // c にはまだ届いていない
	r := s.Broadcasts("b", "回答", "a") // b は質問を見てから流した

	// c には回答が先に届く。
	if got := s.Deliver("c", r); len(got) != 0 {
		t.Fatalf("先に渡してしまった: %v", got)
	}
	if h := s.Node("c").Held(); len(h) != 1 || h[0].Body != "回答" {
		t.Fatalf("預かっていない: %v", h)
	}

	// 質問が届いた瞬間に、両方まとめて渡る。
	got := s.Deliver("c", q)
	if len(got) != 2 {
		t.Fatalf("まとめて渡していない: %d", len(got))
	}
	if !eq(s.Node("c").Order(), "質問", "回答") {
		t.Fatalf("順が違う: %v", s.Node("c").Order())
	}
	if len(s.Node("c").Held()) != 0 {
		t.Fatal("預かりが残っている")
	}
}

// 送り手ごとの順だけでは、別の相手をまたぐ原因と結果を守れない。
func TestFifoIsNotEnough(t *testing.T) {
	s := New(Fifo, "a", "b", "c")
	q := s.Broadcasts("a", "質問", "b")
	r := s.Broadcasts("b", "回答", "a")

	// 回答は b からの1件目なので、送り手ごとの順では通ってしまう。
	if got := s.Deliver("c", r); len(got) != 1 {
		t.Fatalf("送り手ごとの順なら渡るはず: %v", got)
	}
	s.Deliver("c", q)
	if !eq(s.Node("c").Order(), "回答", "質問") {
		t.Fatalf("逆転していない: %v", s.Node("c").Order())
	}

	// 同じ相手からの追い越しは、送り手ごとの順でも直る。
	s2 := New(Fifo, "a", "b")
	m1 := s2.Broadcast("a", "1件目")
	m2 := s2.Broadcast("a", "2件目")
	if got := s2.Deliver("b", m2); len(got) != 0 {
		t.Fatalf("同じ相手の追い越しは止まるはず: %v", got)
	}
	s2.Deliver("b", m1)
	if !eq(s2.Node("b").Order(), "1件目", "2件目") {
		t.Fatalf("並べ直せていない: %v", s2.Node("b").Order())
	}
}

// この章の中心その2。判定に要るのは数の大小だけになる。
func TestDeliveryConditionIsCounting(t *testing.T) {
	m := Message{From: "b", Seq: 1, Deps: clock.Vector{"a": 1, "b": 1}}

	// 送り手の次の1件で、送り手が見ていたものを自分も見ている。
	if !deliverable(m, clock.Vector{"a": 1}, Causal) {
		t.Fatal("渡せるはずが渡せない")
	}
	// a を見ていない。
	if deliverable(m, clock.Vector{}, Causal) {
		t.Fatal("原因を見ていないのに渡した")
	}
	// b の1件目をすでに渡している。
	if deliverable(m, clock.Vector{"a": 1, "b": 1}, Causal) {
		t.Fatal("二度渡そうとしている")
	}
	// b の1件目を飛ばして2件目は渡せない。
	m2 := Message{From: "b", Seq: 2, Deps: clock.Vector{"a": 1, "b": 2}}
	if deliverable(m2, clock.Vector{"a": 1}, Causal) {
		t.Fatal("飛ばして渡した")
	}
	// 送り手ごとの順なら、他人の分は見ない。
	if !deliverable(m, clock.Vector{}, Fifo) {
		t.Fatal("送り手ごとの順で止まった")
	}
}

// この章の中心その3。並行なものは順序を決めない。
func TestConcurrentMessagesMayLandInDifferentOrders(t *testing.T) {
	s := New(Causal, "a", "b", "c")
	base := s.Broadcasts("a", "お題", "b", "c")
	_ = base

	// a と c が、互いを見ないまま同時に流す。
	x := s.Broadcast("a", "犬")
	y := s.Broadcast("c", "猫")
	if got := Relation(x, y); got != clock.Concurrent {
		t.Fatalf("並行でない: %v", got)
	}

	// b には犬が先、c には猫が先(自分の分)に渡る。
	s.Deliver("b", x)
	s.Deliver("b", y)
	s.Deliver("c", x)

	if !eq(s.Node("b").Order(), "お題", "犬", "猫") {
		t.Fatalf("b の順: %v", s.Node("b").Order())
	}
	if !eq(s.Node("c").Order(), "お題", "猫", "犬") {
		t.Fatalf("c の順: %v", s.Node("c").Order())
	}
	// どちらも原因の順は満たしている。決まらないところは決めない。
	if len(s.Node("b").Held()) != 0 || len(s.Node("c").Held()) != 0 {
		t.Fatal("どちらも保留は無いはず")
	}
}

// 鎖になっていても、先頭が届いた瞬間に全部ほどける。
func TestChainUnblocksAtOnce(t *testing.T) {
	s := New(Causal, "a", "b", "c")
	m1 := s.Broadcasts("a", "1", "b")
	m2 := s.Broadcasts("b", "2", "a")
	m3 := s.Broadcasts("a", "3", "b")

	// c には逆順で届く。
	s.Deliver("c", m3)
	s.Deliver("c", m2)
	if n := len(s.Node("c").Held()); n != 2 {
		t.Fatalf("2件預かるはず: %d", n)
	}
	got := s.Deliver("c", m1)
	if len(got) != 3 {
		t.Fatalf("まとめて3件渡すはず: %d", len(got))
	}
	if !eq(s.Node("c").Order(), "1", "2", "3") {
		t.Fatalf("順が違う: %v", s.Node("c").Order())
	}
}

// 原因が来なければ、結果は永遠に渡らない。待つことの代償になる。
func TestMissingCauseBlocksForever(t *testing.T) {
	s := New(Causal, "a", "b", "c")
	s.Broadcasts("a", "質問", "b")
	r := s.Broadcasts("b", "回答", "a")

	for i := 0; i < 10; i++ {
		s.Deliver("c", r) // 何度届いても渡らない
	}
	if len(s.Node("c").Delivered) != 0 {
		t.Fatal("原因なしに渡った")
	}
	if len(s.Node("c").Held()) != 1 {
		t.Fatalf("同じものを何度も預かっている: %d", len(s.Node("c").Held()))
	}
}

// 二度届いても二度渡さない。
func TestDeliveredTwiceIsDeliveredOnce(t *testing.T) {
	s := New(Causal, "a", "b")
	m := s.Broadcasts("a", "1件目", "b")
	if got := s.Deliver("b", m); len(got) != 0 {
		t.Fatalf("二度渡した: %v", got)
	}
	if !eq(s.Node("b").Order(), "1件目") {
		t.Fatalf("記録が増えた: %v", s.Node("b").Order())
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	if Causal.String() != "原因の順" || Fifo.String() != "送り手ごとの順" {
		t.Fatal("名前が違う")
	}
	s := New(Causal, "a", "b")
	if len(s.Names()) != 2 || s.Mode() != Causal {
		t.Fatal("設定が返らない")
	}
	m := s.Broadcasts("a", "1件目", "b")
	if got := s.Node("b").Seen(); got["a"] != 1 {
		t.Fatalf("数え上げが進んでいない: %v", got)
	}
	// 出した本人にもその場で渡っている。
	if !eq(s.Node("a").Order(), "1件目") {
		t.Fatalf("本人に渡っていない: %v", s.Node("a").Order())
	}
	if Relation(m, m) != clock.Equal {
		t.Fatal("同じものが同じでない")
	}
	if len(s.Log) == 0 {
		t.Fatal("記録が無い")
	}
	if itoa(0) != "0" || itoa(12) != "12" {
		t.Fatal("itoa が違う")
	}
}
