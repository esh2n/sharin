package total

import "testing"

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

// 係を置けば、届く順がばらばらでも全員が同じ順で渡す。
func TestSequencerMakesEveryoneAgree(t *testing.T) {
	s := NewSequencer("a", "b", "c")
	m1 := s.Assign("a", "1")
	m2 := s.Assign("b", "2")
	m3 := s.Assign("a", "3")

	// 台ごとに届く順を変える。
	s.Deliver("a", m1)
	s.Deliver("a", m2)
	s.Deliver("a", m3)

	s.Deliver("b", m3)
	s.Deliver("b", m1)
	s.Deliver("b", m2)

	s.Deliver("c", m2)
	s.Deliver("c", m3)
	s.Deliver("c", m1)

	for _, n := range s.Names() {
		if !eq(s.Node(n).Order(), "1", "2", "3") {
			t.Fatalf("%s の順: %v", n, s.Node(n).Order())
		}
	}
	if !Agreed(s.Node("a").Order(), s.Node("b").Order(), s.Node("c").Order()) {
		t.Fatal("そろっていない")
	}
}

// この章の中心その1。全順序にしても、原因の順は付いてこない。
func TestEveryoneCanAgreeOnTheWrongOrder(t *testing.T) {
	s := NewSequencer("a", "b", "c")

	// a が質問を出し、b がそれを見て回答を出す。
	// だが係には回答のほうが先に届いた。
	r := s.Assign("b", "回答")
	q := s.Assign("a", "質問")
	if r.Seq >= q.Seq {
		t.Fatalf("係に届いた順で番号が付くはず: %d %d", r.Seq, q.Seq)
	}

	for _, n := range s.Names() {
		s.Deliver(n, q)
		s.Deliver(n, r)
	}

	// 全員そろっている。そろっているが、答えが問いより先に来ている。
	if !Agreed(s.Node("a").Order(), s.Node("b").Order(), s.Node("c").Order()) {
		t.Fatal("そろっていない")
	}
	for _, n := range s.Names() {
		if !eq(s.Node(n).Order(), "回答", "質問") {
			t.Fatalf("%s の順: %v", n, s.Node(n).Order())
		}
	}
}

// 番号に穴が空くと、後ろは全部止まる。
func TestOneMissingNumberBlocksEverythingAfterIt(t *testing.T) {
	s := NewSequencer("a")
	m1 := s.Assign("a", "1")
	m2 := s.Assign("a", "2")
	m3 := s.Assign("a", "3")

	s.Deliver("a", m1)
	s.Deliver("a", m3) // 2 を飛ばして届く
	if !eq(s.Node("a").Order(), "1") {
		t.Fatalf("穴があるのに渡した: %v", s.Node("a").Order())
	}
	if len(s.Node("a").Held()) != 1 || s.Node("a").Next() != 2 {
		t.Fatalf("預かりと次番号: %v %d", s.Node("a").Held(), s.Node("a").Next())
	}

	// 穴が埋まると、預かりもまとめて渡る。
	if got := s.Deliver("a", m2); len(got) != 2 {
		t.Fatalf("まとめて渡していない: %d", len(got))
	}
	if !eq(s.Node("a").Order(), "1", "2", "3") {
		t.Fatalf("順: %v", s.Node("a").Order())
	}
	// 二度届いても二度渡さない。
	s.Deliver("a", m2)
	s.Deliver("a", m3)
	if len(s.Node("a").Delivered) != 3 {
		t.Fatalf("二度渡した: %v", s.Node("a").Order())
	}
	if got := s.Deliver("a", Numbered{Seq: 9, Body: "9"}); got != nil {
		t.Fatal("穴の先を渡した")
	}
	if got := s.Deliver("a", Numbered{Seq: 9, Body: "9"}); got != nil {
		t.Fatal("同じものを二度預かった")
	}
}

// 係を置かなくても、全員から聞けば同じ順に決まる。
func TestVotingMakesEveryoneAgree(t *testing.T) {
	s := NewVoting("a", "b", "c")
	x := s.Request("a", "A")
	y := s.Request("c", "C")

	// 互いに届ける。
	for _, n := range []string{"b", "c"} {
		s.Deliver(n, x)
	}
	for _, n := range []string{"a", "b"} {
		s.Deliver(n, y)
	}
	// まだ渡せない。b は c より後の時刻を a から聞いていない、など。
	for _, n := range s.Names() {
		if len(s.Node(n).Delivered) != 0 {
			t.Fatalf("%s が早く渡した: %v", n, s.Node(n).Order())
		}
	}

	// 全員が今の時刻を知らせ合うと、そろって渡せるようになる。
	for i := 0; i < 2; i++ {
		for _, from := range s.Names() {
			b := s.Beat(from)
			for _, to := range s.Names() {
				if to != from {
					s.Deliver(to, b)
				}
			}
		}
	}

	for _, n := range s.Names() {
		if !eq(s.Node(n).Order(), "A", "C") {
			t.Fatalf("%s の順: %v", n, s.Node(n).Order())
		}
	}
	if !Agreed(s.Node("a").Order(), s.Node("b").Order(), s.Node("c").Order()) {
		t.Fatal("そろっていない")
	}
}

// この章の中心その2。全員から聞くので、1台が黙ると全体が止まる。
func TestOneSilentNodeStopsEveryone(t *testing.T) {
	s := NewVoting("a", "b", "c")
	x := s.Request("a", "A")
	s.Deliver("b", x)
	s.Deliver("c", x)

	// b と c は動くが、a 自身も含めて誰も渡せない。
	for i := 0; i < 5; i++ {
		for _, from := range []string{"b", "c"} {
			m := s.Beat(from)
			for _, to := range s.Names() {
				if to != from {
					s.Deliver(to, m)
				}
			}
		}
	}
	// b と c は、a より後の時刻を互いから聞いたので渡せる。
	for _, n := range []string{"b", "c"} {
		if len(s.Node(n).Delivered) != 1 {
			t.Fatalf("%s が渡せていない: %v", n, s.Node(n).Order())
		}
	}

	// ここで c が黙る。要求は受け取るが、返事をしない。
	y := s.Request("b", "B")
	s.Deliver("a", y)
	s.Deliver("c", y)
	for i := 0; i < 5; i++ {
		m := s.Beat("a")
		s.Deliver("b", m)
	}
	if len(s.Node("b").Delivered) != 1 {
		t.Fatalf("黙っている台が居るのに渡した: %v", s.Node("b").Order())
	}
	if len(s.Node("b").Queue()) != 1 {
		t.Fatalf("待ち行列: %v", s.Node("b").Queue())
	}

	// c が一言でも知らせれば動く。
	m := s.Beat("c")
	s.Deliver("b", m)
	if len(s.Node("b").Delivered) != 2 {
		t.Fatalf("知らせても渡さない: %v", s.Node("b").Order())
	}
}

// 同じ時刻の要求は、名前で決める。どの台で並べても同じ順になる。
func TestSameStampIsBrokenByName(t *testing.T) {
	s := NewVoting("a", "b", "c")
	x := s.Request("c", "C") // 時刻 1
	y := s.Request("a", "A") // 時刻 1
	if x.T != y.T {
		t.Fatalf("同時刻のはず: %d %d", x.T, y.T)
	}

	s.Deliver("b", x)
	s.Deliver("b", y)
	// 並べた結果は名前の順。
	q := s.Node("b").Queue()
	if len(q) != 2 || q[0].From != "a" {
		t.Fatalf("名前で決まっていない: %v", q)
	}
}

// 二度届いても二度並ばない。
func TestSameRequestQueuedOnce(t *testing.T) {
	s := NewVoting("a", "b", "c")
	x := s.Request("a", "A")
	s.Deliver("b", x)
	s.Deliver("b", x)
	if len(s.Node("b").Queue()) != 1 {
		t.Fatalf("二度並んだ: %v", s.Node("b").Queue())
	}
	// 台が2つしか無いと、相手から聞いた時点で条件がそろうのですぐ渡る。
	two := NewVoting("a", "b")
	if got := two.Deliver("b", two.Request("a", "A")); len(got) != 1 {
		t.Fatalf("2台なら即渡るはず: %v", got)
	}
}

// そろっているかの判定そのもの。
func TestAgreed(t *testing.T) {
	if !Agreed([]string{"1", "2"}, []string{"1", "2", "3"}) {
		t.Fatal("進み具合が違うだけならそろっている")
	}
	if !Agreed([]string{"1", "2", "3"}, []string{"1", "2"}) {
		t.Fatal("どちらが進んでいても同じ")
	}
	if Agreed([]string{"1", "2"}, []string{"2", "1"}) {
		t.Fatal("順が違うのにそろっていると言った")
	}
	if !Agreed([]string{"1"}) {
		t.Fatal("1つならそろっている")
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	s := NewSequencer("a", "b")
	if len(s.Names()) != 2 || s.Node("a").Next() != 1 {
		t.Fatal("最初の番号が 1 でない")
	}
	m := s.Assign("a", "1")
	s.Deliver("a", m)
	if len(s.Log) == 0 {
		t.Fatal("記録が無い")
	}

	v := NewVoting("a", "b")
	if len(v.Names()) != 2 {
		t.Fatal("台数が違う")
	}
	r := v.Request("a", "A")
	if h := v.Node("a").Heard(); h["a"] != r.T {
		t.Fatalf("自分の時刻を覚えていない: %v", h)
	}
	if len(v.Log) == 0 {
		t.Fatal("記録が無い")
	}
	if itoa(0) != "0" || itoa(105) != "105" {
		t.Fatal("itoa が違う")
	}
}
