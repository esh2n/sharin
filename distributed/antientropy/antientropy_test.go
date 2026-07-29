package antientropy

import "testing"

// fill は同じ内容の2台を作る。
func fill(buckets, n int) (*Store, *Store) {
	a, b := NewStore(buckets), NewStore(buckets)
	for i := 0; i < n; i++ {
		k := "key" + itoa(i)
		a.Put(k, "v"+itoa(i), 1)
		b.Put(k, "v"+itoa(i), 1)
	}
	return a, b
}

// 中身が同じなら、根が一致する。比べるのは1回で済む。
func TestSameContentSameRoot(t *testing.T) {
	a, b := fill(1024, 1000)
	if a.Tree().Root() != b.Tree().Root() {
		t.Fatal("同じ中身なのに要約が違う")
	}
	res := Sync(a, b)
	if res.Compared != 1 || res.Sent != 0 {
		t.Fatalf("1回で終わるはず: 比較 %d 送信 %d", res.Compared, res.Sent)
	}
	// 置く順が違っても同じ要約になる(名前順に混ぜているため)。
	c := NewStore(1024)
	for i := 999; i >= 0; i-- {
		c.Put("key"+itoa(i), "v"+itoa(i), 1)
	}
	if c.Tree().Root() != a.Tree().Root() {
		t.Fatal("置く順で要約が変わった")
	}
}

// この章の中心。1件違うだけで根は変わるが、送るのは1件で済む。
func TestOneDifferenceCostsLogNotAll(t *testing.T) {
	a, b := fill(1024, 1000)
	b.Put("key500", "あたらしい", 2)

	if a.Tree().Root() == b.Tree().Root() {
		t.Fatal("1件違うのに要約が同じ")
	}

	res := Sync(a, b)
	if len(res.Buckets) != 1 {
		t.Fatalf("違う区画は1つのはず: %v", res.Buckets)
	}
	// 比べる数は木の高さで決まる。根1つと、各段で子2つずつ。
	depth := a.Tree().Depth()
	if res.Compared != 1+2*depth {
		t.Fatalf("比較 %d(高さ %d)", res.Compared, depth)
	}
	if res.Sent != 1 {
		t.Fatalf("送信 %d 件", res.Sent)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "key500" {
		t.Fatalf("直した key: %v", res.Updated)
	}
	if v, _ := a.Get("key500"); v.Data != "あたらしい" {
		t.Fatalf("直っていない: %+v", v)
	}
	// 直したあとは一致するので、次は1回で終わる。
	if again := Sync(a, b); again.Compared != 1 || again.Sent != 0 {
		t.Fatalf("直したのに差が残っている: %+v", again)
	}
}

// 対照。木を使わないと、違いが1件でも全件送ることになる。
func TestCompareAllSendsEverything(t *testing.T) {
	a, b := fill(1024, 1000)
	b.Put("key500", "あたらしい", 2)
	a.Put("key700", "こちらが新しい", 2) // 逆向きの差も混ぜる

	res := CompareAll(a, b)
	if res.Compared != 1000 || res.Sent != 1000 {
		t.Fatalf("全件のはず: 比較 %d 送信 %d", res.Compared, res.Sent)
	}
	if len(res.Updated) != 2 {
		t.Fatalf("直したのは2件のはず: %v", res.Updated)
	}
	if v, _ := b.Get("key700"); v.Data != "こちらが新しい" {
		t.Fatalf("逆向きに直っていない: %+v", v)
	}
}

// 違いが増えても、比べる数は件数ではなく違いの数で伸びる。
func TestCostGrowsWithDifferencesNotWithSize(t *testing.T) {
	one, oneB := fill(1024, 1000)
	oneB.Put("key1", "x", 2)

	many, manyB := fill(1024, 1000)
	for i := 0; i < 16; i++ {
		manyB.Put("key"+itoa(i*61), "x", 2)
	}

	r1 := Sync(one, oneB)
	r16 := Sync(many, manyB)
	if r16.Compared <= r1.Compared {
		t.Fatalf("違いが増えたのに比較が増えない: %d %d", r1.Compared, r16.Compared)
	}
	// それでも全件(1000)には遠い。
	if r16.Compared >= 1000 {
		t.Fatalf("全件比較と変わらない: %d", r16.Compared)
	}

	// 件数を10倍にしても、違いが1件なら比較は木の高さぶんしか増えない。
	big, bigB := fill(1024, 10000)
	bigB.Put("key1", "x", 2)
	rBig := Sync(big, bigB)
	if rBig.Compared != r1.Compared {
		t.Fatalf("件数で比較が変わった: %d → %d", r1.Compared, rBig.Compared)
	}
}

// 片方にしか無い key も、違いとして見つかる。
func TestMissingKeyIsFound(t *testing.T) {
	a, b := fill(64, 100)
	b.Put("あたらしいkey", "v", 1)

	res := Sync(a, b)
	if len(res.Buckets) != 1 {
		t.Fatalf("区画: %v", res.Buckets)
	}
	if _, ok := a.Get("あたらしいkey"); !ok {
		t.Fatal("足りない key が来ていない")
	}
	// どちら向きにも直る。
	a.Put("もうひとつ", "v", 1)
	Sync(a, b)
	if _, ok := b.Get("もうひとつ"); !ok {
		t.Fatal("逆向きに直っていない")
	}
}

// 木は在処しか言わない。どちらが新しいかは版番号が決める。
func TestTreeOnlySaysWhereNotWhich(t *testing.T) {
	a, b := NewStore(16), NewStore(16)
	a.Put("k", "古い", 1)
	b.Put("k", "新しい", 2)

	// 木は「違う」までしか言わない。
	buckets, _ := Diff(a.Tree(), b.Tree())
	if len(buckets) != 1 {
		t.Fatalf("区画: %v", buckets)
	}

	Sync(a, b)
	for _, s := range []*Store{a, b} {
		if v, _ := s.Get("k"); v.Data != "新しい" {
			t.Fatalf("版番号で決まっていない: %+v", v)
		}
	}

	// 古いものを後から置いても戻らない。
	a.Put("k", "古い", 1)
	if v, _ := a.Get("k"); v.Data != "新しい" {
		t.Fatalf("戻ってしまった: %+v", v)
	}
}

// 区画の切り方が違うと、突き合わせようがない。
func TestDifferentBucketCountCannotBeCompared(t *testing.T) {
	a := NewStore(16)
	b := NewStore(32)
	a.Put("k", "v", 1)
	b.Put("k", "v", 1)

	buckets, compared := Diff(a.Tree(), b.Tree())
	if buckets != nil || compared != 0 {
		t.Fatalf("比べられないはず: %v %d", buckets, compared)
	}
	if res := Sync(a, b); res.Compared != 0 {
		t.Fatalf("比べてしまった: %+v", res)
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	s := NewStore(0)
	if s.Buckets() != 1 {
		t.Fatalf("区画は最低1つ: %d", s.Buckets())
	}
	if s.Tree().Depth() != 0 {
		t.Fatal("葉が1つなら高さ 0")
	}
	empty := NewStore(8)
	if empty.Tree().Root() != 0 {
		t.Fatal("空の台の要約は 0")
	}
	if NewStore(8).Tree().Root() != empty.Tree().Root() {
		t.Fatal("空どうしが一致しない")
	}
	s.Put("k", "v", 3)
	if v, ok := s.Get("k"); !ok || v.Stamp != 3 {
		t.Fatalf("置けていない: %+v", v)
	}
	if s.Bucket("k") != 0 {
		t.Fatal("区画が1つなら 0")
	}
	if len(s.Keys()) != 1 {
		t.Fatal("key が返らない")
	}
	if itoa(0) != "0" || itoa(-12) != "-12" || itoa(305) != "305" {
		t.Fatal("itoa が違う")
	}
	if mix(1, 2) == mix(2, 1) {
		t.Fatal("左右を入れ替えても同じになっている")
	}
}
