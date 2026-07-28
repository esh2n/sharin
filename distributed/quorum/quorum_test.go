package quorum

import "testing"

func five(cfg Config) *Cluster { return New(cfg, "a", "b", "c", "d", "e") }

const key = "x"

// 台数の引き算が重なりを作る。
func TestOverlapIsArithmetic(t *testing.T) {
	if !(Config{N: 3, R: 2, W: 2}).Overlaps() {
		t.Fatal("2+2 は 3 より大きい")
	}
	if (Config{N: 3, R: 1, W: 2}).Overlaps() {
		t.Fatal("1+2 は 3 を超えていない")
	}
	// 片方に寄せてもよい。読みを速くしたいなら W を全台にする。
	if !(Config{N: 3, R: 1, W: 3}).Overlaps() {
		t.Fatal("1+3 は 3 より大きい")
	}
}

// この章の中心その1。R + W > N なら、どの順で聞いても最新が混ざる。
func TestOverlapGuaranteesFreshRead(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2})
	home := c.Home(key)

	c.Put(key, "v1")
	// 1台落ちた状態で書く。W = 2 なので通る。落ちた台は v1 のまま。
	c.Kill(home[0])
	if w := c.Put(key, "v2"); !w.OK || w.Acks != 2 {
		t.Fatalf("2台で確定するはず: %+v", w)
	}
	c.Revive(home[0])
	if got := c.Stale(key); len(got) != 1 || got[0] != home[0] {
		t.Fatalf("古い台は1台のはず: %v", got)
	}

	// 聞く順がどう回っても、古い台だけで R 台を埋められない。
	for i := 0; i < 12; i++ {
		r := c.Get(key)
		if !r.OK || r.Value.Data != "v2" {
			t.Fatalf("%d 回目で古い値が返った: %+v", i, r)
		}
	}
}

// この章の中心その1の裏。重ならない設定にすると、同じ手順で古い値が返る。
func TestWithoutOverlapReadCanBeStale(t *testing.T) {
	c := five(Config{N: 3, R: 1, W: 1})
	home := c.Home(key)

	c.Put(key, "v1")
	c.Kill(home[1])
	c.Kill(home[2])
	if w := c.Put(key, "v2"); !w.OK || w.Acks != 1 {
		t.Fatalf("1台で確定してしまうはず: %+v", w)
	}
	c.Revive(home[1])
	c.Revive(home[2])

	stale := 0
	for i := 0; i < 12; i++ {
		if r := c.Get(key); r.Value.Data == "v1" {
			stale++
		}
	}
	if stale == 0 {
		t.Fatal("R + W が N を超えていないのに古い値が出ない")
	}
}

// 返事が足りなければ書けない。ただし、書けてしまった台の値は残る。
func TestWriteBelowWStillLeavesTraces(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 3})
	home := c.Home(key)
	c.Put(key, "v1")

	c.Kill(home[0])
	w := c.Put(key, "v2")
	if w.OK || w.Acks != 2 {
		t.Fatalf("3台そろわないので失敗するはず: %+v", w)
	}
	// 失敗と返しても、生きていた2台には新しい値が載っている。
	if v, _ := c.Node(home[1]).Get(key); v.Data != "v2" {
		t.Fatalf("書けた台に残っていない: %+v", v)
	}
}

// 読みも同じで、R 台そろわなければ読めない。
func TestReadBelowRFails(t *testing.T) {
	c := five(Config{N: 3, R: 3, W: 1})
	c.Put(key, "v1")
	c.Kill(c.Home(key)[0])

	r := c.Get(key)
	if r.OK || len(r.Asked) != 2 {
		t.Fatalf("3台そろわないので読めないはず: %+v", r)
	}
}

// この章の中心その2。重なりは「最新を持つ台が居る」までしか言わない。
// どれが最新かは版番号が決め、古い台は読んだついでに直す。
func TestReadRepairFixesTheStaleOne(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2, ReadRepair: true})
	home := c.Home(key)

	c.Put(key, "v1")
	c.Kill(home[0])
	c.Put(key, "v2")
	c.Revive(home[0])
	if len(c.Stale(key)) != 1 {
		t.Fatal("古い台が1台居るはず")
	}

	repaired := 0
	for i := 0; i < 3; i++ {
		repaired += len(c.Get(key).Repaired)
	}
	if repaired == 0 {
		t.Fatal("直した形跡が無い")
	}
	if got := c.Stale(key); len(got) != 0 {
		t.Fatalf("直っていない: %v", got)
	}
}

// 古い書き込みが後から届いても、値は戻らない。
func TestLateWriteDoesNotGoBackwards(t *testing.T) {
	old := Value{Data: "v1", Stamp: 1}
	newv := Value{Data: "v2", Stamp: 2}
	if Newer(newv, old) != newv || Newer(old, newv) != newv {
		t.Fatal("版番号が大きいほうが勝たない")
	}
	// 同じ版番号なら同じ書き込みなので、どちらを採っても変わらない。
	if Newer(newv, newv) != newv {
		t.Fatal("同じものをまとめて変わった")
	}

	n := &Node{Name: "a", data: map[string]Value{}}
	n.put(key, newv)
	n.put(key, old)
	if v, _ := n.Get(key); v != newv {
		t.Fatalf("古いもので上書きされた: %+v", v)
	}
}

// この章の中心その3。緩めた quorum は、可用性と引き換えに重なりを手放す。
func TestSloppyQuorumTradesOverlapForAvailability(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2, Sloppy: true})
	home := c.Home(key)

	c.Put(key, "v1")
	c.Kill(home[0])
	c.Kill(home[1])

	// 担当が1台しか生きていないのに、代役ぶんを数えて書き込みは通る。
	w := c.Put(key, "v2")
	if !w.OK || len(w.Substitutes) != 2 {
		t.Fatalf("代役を立てて通るはず: %+v", w)
	}
	for _, s := range w.Substitutes {
		if s == home[0] || s == home[1] || s == home[2] {
			t.Fatalf("担当が代役になっている: %v", w.Substitutes)
		}
	}

	c.Revive(home[0])
	c.Revive(home[1])

	// R + W > N なのに、値が担当の上に無いので古い値が返りうる。
	stale := 0
	for i := 0; i < 6; i++ {
		if r := c.Get(key); r.Value.Data == "v1" {
			stale++
		}
	}
	if stale == 0 {
		t.Fatal("代役に預けたままなのに古い値が出ない")
	}

	// 受け渡しが済むと、担当の上にそろって重なりが戻る。
	if moved := c.Handoff(); moved != 2 {
		t.Fatalf("渡した数が違う: %d", moved)
	}
	if got := c.Stale(key); len(got) != 0 {
		t.Fatalf("担当にそろっていない: %v", got)
	}
	for i := 0; i < 6; i++ {
		if r := c.Get(key); r.Value.Data != "v2" {
			t.Fatalf("受け渡し後に古い値が返った: %+v", r)
		}
	}
}

// 担当が戻るまで、預かりぶんは渡さない。
func TestHintsWaitForTheOwner(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2, Sloppy: true})
	home := c.Home(key)
	c.Kill(home[0])
	w := c.Put(key, "v1")
	if len(w.Substitutes) != 1 {
		t.Fatalf("代役は1台のはず: %+v", w)
	}
	sub := w.Substitutes[0]

	if moved := c.Handoff(); moved != 0 {
		t.Fatalf("落ちている担当に渡した: %d", moved)
	}
	if hs := c.Node(sub).Hints(); len(hs) != 1 || hs[0].Owner != home[0] {
		t.Fatalf("預かりが残っていない: %+v", hs)
	}

	c.Revive(home[0])
	if moved := c.Handoff(); moved != 1 {
		t.Fatalf("戻った担当に渡していない: %d", moved)
	}
	if len(c.Node(sub).Hints()) != 0 {
		t.Fatal("渡したのに預かりが残っている")
	}
}

// 代役が居なければ、緩めても書き込みは通らない。
func TestSloppyNeedsSpareNodes(t *testing.T) {
	c := New(Config{N: 3, R: 2, W: 2, Sloppy: true}, "a", "b", "c")
	c.Kill(c.Home(key)[0])
	c.Kill(c.Home(key)[1])
	w := c.Put(key, "v1")
	if w.OK || len(w.Substitutes) != 0 {
		t.Fatalf("担当外の台が無いので代役は立たない: %+v", w)
	}
}

// 担当は key ごとに固定で、台数ぶん返る。
func TestHomeIsStablePerKey(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2})
	first := c.Home(key)
	if len(first) != 3 {
		t.Fatalf("担当は3台のはず: %v", first)
	}
	for i := 0; i < 5; i++ {
		got := c.Home(key)
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("担当が変わった: %v → %v", first, got)
			}
		}
	}
	// key が違えば担当も違いうる。
	same := true
	for _, k := range []string{"y", "z", "w", "v"} {
		h := c.Home(k)
		if h[0] != first[0] {
			same = false
		}
	}
	if same {
		t.Fatal("どの key も同じ担当になっている")
	}
	// 担当の数は台数を超えない。
	small := New(Config{N: 5, R: 1, W: 1}, "a", "b", "c")
	if len(small.Home(key)) != 3 {
		t.Fatalf("台数を超えた: %v", small.Home(key))
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	c := five(Config{N: 3, R: 2, W: 2})
	if len(c.Names()) != 5 || c.Config().N != 3 {
		t.Fatal("設定が返らない")
	}
	if r := c.Get(key); r.Found {
		t.Fatalf("まだ何も書いていない: %+v", r)
	}
	c.Put(key, "v1")
	if r := c.Get(key); !r.Found || r.Value.Stamp != 1 {
		t.Fatalf("版番号が違う: %+v", r)
	}
	if _, ok := c.Node("a").Get("none"); ok {
		t.Fatal("知らない key が返った")
	}
	c.Kill("a")
	if !c.IsDown("a") {
		t.Fatal("落ちていない")
	}
	c.Revive("a")
	if c.IsDown("a") {
		t.Fatal("戻っていない")
	}
	if len(c.Log) == 0 {
		t.Fatal("記録が無い")
	}
	if itoa(0) != "0" {
		t.Fatal("itoa が違う")
	}
}
