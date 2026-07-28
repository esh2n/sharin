package clock

import "testing"

// scenario は3ノードの筋書きを1つ作る。
//
//	a: a1(local) → a2(send m1) ................ a3(local)
//	b: b1(local) ............ b2(recv m1) → b3(send m2)
//	c: c1(local) ................................ c2(recv m2)
//
// a1・a2 と b1 は互いを知らない。c1 も誰も知らない。
func scenario() (*Sim, map[string]Event) {
	s := New("a", "b", "c")
	ev := map[string]Event{}

	ev["a1"] = s.Local("a", "a で更新")
	e, m1 := s.Send("a", "b へ知らせる")
	ev["a2"] = e
	ev["b1"] = s.Local("b", "b で更新")
	ev["c1"] = s.Local("c", "c で更新")
	ev["b2"] = s.Recv("b", m1, "a の知らせを受けた")
	e, m2 := s.Send("b", "c へ知らせる")
	ev["b3"] = e
	ev["a3"] = s.Local("a", "a でまた更新")
	ev["c2"] = s.Recv("c", m2, "b の知らせを受けた")
	return s, ev
}

// 原因は結果より小さい。これが論理時計の唯一の約束になる。
func TestCauseHasSmallerLamport(t *testing.T) {
	_, ev := scenario()
	pairs := [][2]string{
		{"a1", "a2"}, // 同じノードの前後
		{"a2", "b2"}, // 送信と受信
		{"b1", "b2"}, // 同じノードの前後
		{"a1", "b2"}, // 送信を挟んだ間接的な因果
		{"b3", "c2"}, // 送信と受信
		{"a2", "c2"}, // 2段を挟んだ因果
	}
	for _, p := range pairs {
		x, y := ev[p[0]], ev[p[1]]
		if !(x.Lamport < y.Lamport) {
			t.Errorf("%s(%d)は%s(%d)より小さいはず", p[0], x.Lamport, p[1], y.Lamport)
		}
	}
}

// この章の中心。約束は片道しかない。
// 小さいからといって原因とは限らない。
func TestSmallerDoesNotMeanCause(t *testing.T) {
	s, ev := scenario()

	b1, a2 := ev["b1"], ev["a2"]
	// b1 の数のほうが小さい。並べれば b1 が先に見える。
	if !(b1.Lamport < a2.Lamport) {
		t.Fatalf("この筋書きでは b1(%d) < a2(%d) のはず", b1.Lamport, a2.Lamport)
	}
	// だが実際には互いを知らない。
	if got := Compare(b1.Vector, a2.Vector); got != Concurrent {
		t.Fatalf("b1 と a2 は同時のはずが %v", got)
	}

	byLamport, byVector := s.Relation(b1, a2)
	if byLamport != "前" || byVector != Concurrent {
		t.Fatalf("2つの時計の答えが割れていない: %s / %v", byLamport, byVector)
	}
}

// ベクタなら、同時を同時として出せる。
func TestVectorFindsConcurrency(t *testing.T) {
	_, ev := scenario()
	cases := []struct {
		x, y string
		want Ord
	}{
		{"a1", "a2", Before},
		{"a2", "b2", Before},
		{"b2", "a2", After},
		{"a1", "b1", Concurrent},
		{"a2", "b1", Concurrent},
		{"c1", "a1", Concurrent},
		{"c1", "b1", Concurrent},
		{"a3", "b3", Concurrent}, // a は b の知らせをまだ知らない
		{"a2", "c2", Before},     // 2段を挟んでも因果は残る
	}
	for _, c := range cases {
		if got := Compare(ev[c.x].Vector, ev[c.y].Vector); got != c.want {
			t.Errorf("%s と %s: %v(want %v)", c.x, c.y, got, c.want)
		}
	}
}

// 同じベクタどうしは「同じ」になる。
func TestEqualVectors(t *testing.T) {
	v := Vector{"a": 2, "b": 1}
	if got := Compare(v, v.Clone()); got != Equal {
		t.Fatalf("同じベクタが %v", got)
	}
	// 写しを変えても元は変わらない。
	c := v.Clone()
	c["a"] = 9
	if v["a"] != 2 {
		t.Fatal("写しを変えたら元も変わった")
	}
	// 空のベクタは、何かを持つベクタより前になる。
	if got := Compare(Vector{}, Vector{"a": 1}); got != Before {
		t.Fatalf("空との比較が %v", got)
	}
}

// 同時に起きた組をすべて挙げられる。これが次の章の入口になる。
func TestListsAllConcurrentPairs(t *testing.T) {
	s, _ := scenario()
	pairs := s.Concurrent()
	if len(pairs) == 0 {
		t.Fatal("同時の組が1つも出ない")
	}
	for _, p := range pairs {
		if Compare(p[0].Vector, p[1].Vector) != Concurrent {
			t.Fatalf("同時でない組が入っている: %s と %s", p[0].Label, p[1].Label)
		}
		if p[0].ID >= p[1].ID {
			t.Fatal("番号の小さい順になっていない")
		}
	}
}

// Lamport の値で並べれば必ず一列になる。だがその列に因果の意味は無い。
func TestLamportGivesATotalOrder(t *testing.T) {
	s, _ := scenario()
	sorted := s.ByLamport()

	if len(sorted) != len(s.Events) {
		t.Fatal("数が合わない")
	}
	for i := 1; i < len(sorted); i++ {
		x, y := sorted[i-1], sorted[i]
		if !LamportLess(x.Lamport, x.Node, y.Lamport, y.Node) {
			t.Fatalf("並んでいない: %s と %s", x.Label, y.Label)
		}
	}
	// 一列に並んだのに、その中に同時の組が含まれている。
	found := false
	for i := 1; i < len(sorted); i++ {
		if Compare(sorted[i-1].Vector, sorted[i].Vector) == Concurrent {
			found = true
		}
	}
	if !found {
		t.Fatal("隣り合った同時の組が無い。対照になっていない")
	}
}

// 数が同じときはノード名で決める。必ずどちらかが先になる。
func TestTiesAreBrokenByName(t *testing.T) {
	if !LamportLess(3, "a", 3, "b") {
		t.Fatal("同じ数で名前順になっていない")
	}
	if LamportLess(3, "b", 3, "a") {
		t.Fatal("逆向きにも先になっている")
	}
	if !LamportLess(2, "z", 3, "a") {
		t.Fatal("数の比較が名前に負けている")
	}
}

// 受け取ると、相手の知っていたことを自分も知ることになる。
func TestRecvMergesKnowledge(t *testing.T) {
	a := NewVClock("a")
	b := NewVClock("b")

	a.Local()
	a.Local()
	sent := a.Send() // {a:3}
	b.Local()        // {b:1}

	got := b.Recv(sent)
	if got["a"] != 3 {
		t.Fatalf("相手のぶんを取り込んでいない: %v", got)
	}
	if got["b"] != 2 {
		t.Fatalf("自分のぶんが進んでいない: %v", got)
	}
	// 受け取ったあとは、送信より後になる。
	if Compare(sent, got) != Before {
		t.Fatal("受信が送信より後になっていない")
	}
	// 古い知らせを受け取っても、後戻りしない。
	old := Vector{"a": 1}
	after := b.Recv(old)
	if after["a"] != 3 {
		t.Fatalf("古い知らせで巻き戻った: %v", after)
	}
}

// Lamport 側も、受け取ると追いつく。
func TestLamportCatchesUp(t *testing.T) {
	a := NewLamport("a")
	b := NewLamport("b")
	for i := 0; i < 5; i++ {
		a.Local()
	}
	sent := a.Send() // 6

	if b.Now() != 0 {
		t.Fatal("b は何もしていない")
	}
	if got := b.Recv(sent); got != 7 {
		t.Fatalf("追いついていない: %d", got)
	}
	// 自分のほうが進んでいれば、そのまま続ける。
	c := NewLamport("c")
	for i := 0; i < 10; i++ {
		c.Local()
	}
	if got := c.Recv(2); got != 11 {
		t.Fatalf("遅れた知らせで巻き戻った: %d", got)
	}
	if a.ID != "a" || a.Now() != 6 {
		t.Fatal("状態が違う")
	}
}

// ベクタの大きさは、実際に動いたノードの数だけ増える。これが代償になる。
//
// 数えるものが1つでなくノードの数だけあるので、送るたびにその全部を運ぶことになる。
// Lamport のほうは、ノードが何台あっても数1つのまま変わらない。
func TestVectorGrowsWithParticipants(t *testing.T) {
	// 2台だけが動く筋書き。
	small := New("a", "b")
	_, m := small.Send("a", "y")
	small.Recv("b", m, "z")
	smallLast := small.Events[len(small.Events)-1]

	// 5台が順に受け渡す筋書き。
	big := New("a", "b", "c", "d", "e")
	_, msg := big.Send("a", "y")
	var last Event
	for _, n := range []string{"b", "c", "d", "e"} {
		last = big.Recv(n, msg, "受けた")
		_, msg = big.Send(n, "次へ")
	}

	if len(last.Vector) <= len(smallLast.Vector) {
		t.Fatalf("動いたノードが増えてもベクタが大きくならない: %v vs %v", last.Vector, smallLast.Vector)
	}
	if len(last.Vector) != 5 {
		t.Fatalf("5台ぶんになっていない: %v", last.Vector)
	}

	// 何もしていないノードは、ベクタに現れない。
	quiet := New("a", "b", "c", "d", "e")
	quiet.Local("a", "x")
	if got := len(quiet.Events[0].Vector); got != 1 {
		t.Fatalf("動いていないノードが数えられている: %v", quiet.Events[0].Vector)
	}
}

// 表示まわり。
func TestNamesAndKeys(t *testing.T) {
	if Equal.String() != "同じ" || Before.String() != "前" ||
		After.String() != "後" || Concurrent.String() != "同時" {
		t.Fatal("関係の名前が違う")
	}
	v := Vector{"c": 1, "a": 2, "b": 3}
	got := v.Keys()
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("名前順でない: %v", got)
	}
	s := New("z", "a")
	if s.Nodes[0] != "a" {
		t.Fatalf("ノードが名前順でない: %v", s.Nodes)
	}
	e := s.Local("a", "x")
	if e.ID != 1 || e.Node != "a" || e.Label != "x" {
		t.Fatalf("記録が違う: %+v", e)
	}
	if s.vec["a"].ID != "a" {
		t.Fatal("時計の持ち主が違う")
	}
	b, o := s.Relation(e, e)
	if b != "同じ数" || o != Equal {
		t.Fatalf("自分どうしの関係が違う: %s / %v", b, o)
	}
	later := s.Local("a", "y")
	if b, o := s.Relation(later, e); b != "後" || o != After {
		t.Fatalf("後ろ向きの関係が違う: %s / %v", b, o)
	}
	if b, _ := s.Relation(e, later); b != "前" {
		t.Fatal("前向きの関係が違う")
	}
}
