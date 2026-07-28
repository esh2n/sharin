package gossip

import "testing"

func cfg() Config { return Config{Indirect: 2, SuspectFor: 3} }

func five(seed uint64) *Sim {
	return New(cfg(), seed, "a", "b", "c", "d", "e")
}

// まとめ方は3つの性質を満たす。だから届く順も回数も関係ない。
func TestMergeProperties(t *testing.T) {
	x := Member{Name: "a", State: Alive, Inc: 2}
	y := Member{Name: "a", State: Suspect, Inc: 1}
	z := Member{Name: "a", State: Dead, Inc: 2}

	if Merge(x, y) != Merge(y, x) {
		t.Fatal("順序で結果が変わる")
	}
	if Merge(Merge(x, y), z) != Merge(x, Merge(y, z)) {
		t.Fatal("括り方で結果が変わる")
	}
	if Merge(Merge(x, y), y) != Merge(x, y) {
		t.Fatal("二度まとめて変わった")
	}
	// 番号が大きいほうが勝つ。古い疑いは新しい主張に負ける。
	if Merge(x, y) != x {
		t.Fatalf("新しい番号が負けた: %+v", Merge(x, y))
	}
	// 番号が同じなら、悪い知らせが勝つ。
	if Merge(x, z) != z {
		t.Fatalf("同じ番号で悪い知らせが負けた: %+v", Merge(x, z))
	}
}

// この章の中心その1。1台では、相手の死と自分との断線を区別できない。
func TestIndirectProbeDistinguishesPartitionFromDeath(t *testing.T) {
	// a から b へだけ届かない。b は生きている。
	s := five(7)
	s.Block("a", "b")
	// a が b を選ぶまで何周期かかかるので、十分に回す。
	for i := 0; i < 20; i++ {
		s.Round()
	}

	if got := s.Node("a").View("b").State; got != Alive {
		t.Fatalf("届かないだけなのに %v にした", got)
	}
	found := false
	for _, l := range s.Log {
		if len(l) > 0 && contains(l, "からは届いた") {
			found = true
		}
	}
	if !found {
		t.Fatal("他の台に頼んだ形跡が無い")
	}

	// 頼む相手が居なければ、区別できないので疑うしかない。
	lone := New(Config{Indirect: 0, SuspectFor: 3}, 7, "a", "b")
	lone.Block("a", "b")
	lone.Round()
	if got := lone.Node("a").View("b").State; got != Suspect {
		t.Fatalf("頼れないのに疑わなかった: %v", got)
	}
}

// この章の中心その2。疑いという中間状態が、誤検知と検知遅れの間を取る。
func TestSuspectIsNotYetDead(t *testing.T) {
	s := five(3)
	s.Kill("b")

	// しばらくは疑いのまま。まだ死んだとは決めない。
	for i := 0; i < 12; i++ {
		s.Round()
	}
	if got, ok := s.Agreed("b"); !ok || got != Dead {
		t.Fatalf("最後には死んだと決まるはず: %v %v", got, ok)
	}

	// 疑いを経ずにいきなり死んだと決めていないことを、記録で確かめる。
	sawSuspect := false
	for _, l := range s.Log {
		if contains(l, "を疑い始めた") {
			sawSuspect = true
			break
		}
	}
	if !sawSuspect {
		t.Fatal("疑いを経ていない")
	}
}

// 疑われても、本人が反論すれば戻る。主張できるのは本人だけになる。
func TestOnlyTheOwnerCanRefute(t *testing.T) {
	s := five(11)
	s.Kill("b")
	for i := 0; i < 4; i++ {
		s.Round()
	}
	if s.Node("a").View("b").State == Alive {
		t.Skip("この seed ではまだ疑われていない")
	}

	before := s.Node("a").View("b")
	s.Revive("b")
	for i := 0; i < 10; i++ {
		s.Round()
	}
	after := s.Node("a").View("b")

	if after.Inc <= before.Inc {
		t.Fatalf("番号が上がっていない: %d → %d", before.Inc, after.Inc)
	}
	if after.State != Alive {
		t.Fatalf("反論が通っていない: %v", after.State)
	}
}

// 他人は番号を上げられない。だから疑いを勝手に取り消せない。
func TestOthersCannotClearSuspicion(t *testing.T) {
	suspected := Member{Name: "b", State: Suspect, Inc: 1}
	// 他人が「生きている」と言っても、番号が同じなら悪い知らせが勝つ。
	claim := Member{Name: "b", State: Alive, Inc: 1}
	if Merge(suspected, claim).State != Suspect {
		t.Fatal("同じ番号で疑いが消えた")
	}
	// 本人が番号を上げたときだけ通る。
	own := Member{Name: "b", State: Alive, Inc: 2}
	if Merge(suspected, own).State != Alive {
		t.Fatal("本人の主張が通らない")
	}
}

// この章の中心その3。1台が出す問い合わせは、台数に関係なく一定になる。
func TestProbeCountDoesNotGrowWithSize(t *testing.T) {
	small := New(cfg(), 5, "a", "b", "c")
	big := New(cfg(), 5, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j")

	const rounds = 10
	for i := 0; i < rounds; i++ {
		small.Round()
		big.Round()
	}
	// 何も壊れていなければ、1周期に1台あたり1回。
	if small.Pings != 3*rounds {
		t.Fatalf("小さい系の回数が違う: %d", small.Pings)
	}
	if big.Pings != 10*rounds {
		t.Fatalf("大きい系の回数が違う: %d", big.Pings)
	}
	// 総数は台数に比例する。全員が全員を見張る形なら2乗になる。
	if big.Pings != 10*rounds || big.Pings >= 10*10*rounds {
		t.Fatalf("台数の2乗になっている: %d", big.Pings)
	}
}

// 知らせは全員に届く。直接話していない相手にも広まる。
func TestNewsSpreadsToEveryone(t *testing.T) {
	s := five(23)
	s.Kill("e")

	rounds := s.RunUntilAgreed("e", Dead, 40)
	if rounds < 0 {
		t.Fatal("全員の見立てがそろわない")
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		if got := s.Node(n).View("e").State; got != Dead {
			t.Fatalf("%s の見立てが %v", n, got)
		}
	}
}

// 疑いの猶予を長くすると、決めるまでが遅くなる。
func TestLongerSuspectDelaysTheDecision(t *testing.T) {
	quick := New(Config{Indirect: 2, SuspectFor: 1}, 31, "a", "b", "c", "d")
	slow := New(Config{Indirect: 2, SuspectFor: 8}, 31, "a", "b", "c", "d")
	quick.Kill("d")
	slow.Kill("d")

	q := quick.RunUntilAgreed("d", Dead, 60)
	sl := slow.RunUntilAgreed("d", Dead, 60)
	if q < 0 || sl < 0 {
		t.Fatalf("決まらない: %d %d", q, sl)
	}
	if q >= sl {
		t.Fatalf("猶予を延ばしても遅くならない: %d vs %d", q, sl)
	}
}

// 死んだと決めた相手には、もう問い合わせない。
func TestDeadIsNotProbed(t *testing.T) {
	s := New(cfg(), 41, "a", "b")
	s.Kill("b")
	for i := 0; i < 20; i++ {
		s.Round()
	}
	if s.Node("a").View("b").State != Dead {
		t.Fatal("死んだと決まっていない")
	}
	before := s.Pings
	for i := 0; i < 10; i++ {
		s.Round()
	}
	if s.Pings != before {
		t.Fatalf("死んだ相手に問い合わせている: %d → %d", before, s.Pings)
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	if Alive.String() != "生きている" || Suspect.String() != "疑わしい" || Dead.String() != "死んだ" {
		t.Fatal("状態の名前が違う")
	}
	s := five(2)
	if s.Tick() != 0 {
		t.Fatal("開始が 0 でない")
	}
	ms := s.Node("a").Members()
	if len(ms) != 5 || ms[0].Name != "a" {
		t.Fatalf("見立てが名前順でない: %v", ms)
	}
	if got, ok := s.Agreed("a"); !ok || got != Alive {
		t.Fatalf("最初は全員生きているはず: %v %v", got, ok)
	}
	s.Round()
	if s.Tick() != 1 {
		t.Fatal("周期が進んでいない")
	}
	// 全員落ちたら、そろえようがない。
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		s.Kill(n)
	}
	if _, ok := s.Agreed("a"); ok {
		t.Fatal("全員落ちているのに合意が出た")
	}
	if s.RunUntilAgreed("a", Dead, 3) >= 0 {
		t.Fatal("そろわないのに値が返った")
	}
	if itoa(0) != "0" || itoa(42) != "42" {
		t.Fatal("itoa が違う")
	}
	// 知らない名前を取り込むと、新しく覚える。
	n := s.Node("a")
	n.apply(Member{Name: "z", State: Alive})
	if n.View("z").Name != "z" {
		t.Fatal("知らない相手を覚えていない")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
