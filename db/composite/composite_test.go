package composite

import "testing"

// 1万行。a は 100 種類、b は 10 種類。a=x AND b=y に当たるのは 10 件。
func fixture() []Row { return NewRows(10000, 100, 10) }

func eq(col string, v int) Cond { return Cond{Col: col, Op: "=", Val: v} }
func ge(col string, v int) Cond { return Cond{Col: col, Op: ">=", Val: v} }

// この章の中心その1。左端から連続して指定したぶんしか、位置決めに使えない。
func TestOnlyTheLeftmostPrefixIsUsable(t *testing.T) {
	rows := fixture()
	ix := Build(rows, "a", "b")
	full := ScanPages(rows)

	cases := []struct {
		name  string
		conds []Cond
	}{
		{"a = 5 AND b = 3", []Cond{eq("a", 5), eq("b", 3)}},
		{"a = 5", []Cond{eq("a", 5)}},
		{"b = 3", []Cond{eq("b", 3)}},
	}
	t.Logf("索引は (a, b)。表は %d 行 = %d ページ", len(rows), full)
	t.Logf("%-18s %8s %10s %10s %8s", "条件", "使えた列", "なめた件数", "索引ページ", "当たり")

	var got []Plan
	for _, c := range cases {
		p := ix.Lookup(c.conds)
		got = append(got, p)
		t.Logf("%-18s %8d %10d %10d %8d", c.name, p.Usable, p.Scanned, p.IndexReads, p.Rows)
	}

	// 2列とも使える → 10 件だけ見る。
	if got[0].Usable != 2 || got[0].Scanned != 10 {
		t.Fatalf("%+v", got[0])
	}
	// 左端だけ → その値の 100 件を見て、そこから絞る。
	if got[1].Usable != 1 || got[1].Scanned != 100 {
		t.Fatalf("%+v", got[1])
	}
	// 左端が無い → どこを開けばいいか分からないので、索引を全部なめる。
	if got[2].Usable != 0 || got[2].Scanned != len(rows) {
		t.Fatalf("%+v", got[2])
	}
	// なめる件数は 1000 倍違う。
	if got[2].Scanned < got[0].Scanned*100 {
		t.Errorf("差が出ていない: %d, %d", got[0].Scanned, got[2].Scanned)
	}
	// 順番を入れ替えた索引を張れば、b だけの条件も位置決めできる。
	rev := Build(rows, "b", "a")
	p := rev.Lookup([]Cond{eq("b", 3)})
	t.Logf("索引を (b, a) に張り替えると  使えた列 %d / なめた件数 %d", p.Usable, p.Scanned)
	if p.Usable != 1 || p.Scanned != 1000 {
		t.Fatalf("%+v", p)
	}
}

// この章の中心その2。範囲が来たら、そこで位置決めは打ち切りになる。
func TestRangeStopsThePrefix(t *testing.T) {
	rows := fixture()
	ab := Build(rows, "a", "b")

	// 等値 → 範囲: 両方使える。
	good := ab.Lookup([]Cond{eq("a", 5), ge("b", 8)})
	// 範囲 → 等値: a で打ち切られ、b はふるいにかけるだけになる。
	bad := ab.Lookup([]Cond{ge("a", 95), eq("b", 3)})

	t.Logf("a = 5  AND b >= 8   使えた列 %d / なめた %5d / 当たり %3d", good.Usable, good.Scanned, good.Rows)
	t.Logf("a >= 95 AND b = 3   使えた列 %d / なめた %5d / 当たり %3d", bad.Usable, bad.Scanned, bad.Rows)

	if good.Usable != 2 || bad.Usable != 1 {
		t.Fatalf("%+v %+v", good, bad)
	}
	// 打ち切られた側は、当たりの何倍もなめている。
	if bad.Scanned < bad.Rows*5 {
		t.Errorf("なめた %d / 当たり %d", bad.Scanned, bad.Rows)
	}
	// 同じ条件でも、列の順を入れ替えた索引なら両方使える。
	ba := Build(rows, "b", "a")
	fixed := ba.Lookup([]Cond{ge("a", 95), eq("b", 3)})
	t.Logf("索引を (b, a) にすると      使えた列 %d / なめた %5d / 当たり %3d",
		fixed.Usable, fixed.Scanned, fixed.Rows)
	if fixed.Usable != 2 || fixed.Scanned >= bad.Scanned {
		t.Fatalf("%+v", fixed)
	}
	if fixed.Rows != bad.Rows {
		t.Fatalf("当たりが変わった: %d, %d", fixed.Rows, bad.Rows)
	}
}

// この章の中心その3。索引はタダではない。1本増やすごとに書き込みが重くなる。
func TestEveryIndexCostsAWrite(t *testing.T) {
	const n = 10000
	for _, k := range []int{0, 1, 2, 4} {
		t.Logf("索引 %d 本   1行あたり %d ページ   %d 行入れて %d ページ",
			k, WriteCost(k), n, InsertPages(n, k))
	}
	// 索引ゼロなら本体だけ。4本張ると5倍書く。
	if WriteCost(0) != 1 || WriteCost(4) != 5 {
		t.Fatal("数え方が違う")
	}
	if InsertPages(n, 4) != InsertPages(n, 0)*5 {
		t.Fatal("比が合わない")
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	rows := NewRows(300, 10, 3)
	ix := Build(rows, "a", "b")

	// 条件が無ければ何も位置決めできず、全部なめる。
	p := ix.Lookup(nil)
	if p.Usable != 0 || p.Scanned != len(rows) || p.Rows != len(rows) {
		t.Fatalf("%+v", p)
	}
	// 索引に無い列だけを指定しても位置決めできない。
	if got := ix.Lookup([]Cond{eq("c", 1)}); got.Usable != 0 {
		t.Fatalf("%+v", got)
	}
	// 当たらない値でも索引は降りる。降りてみないと無いと分からない。
	miss := ix.Lookup([]Cond{eq("a", 999)})
	if miss.Rows != 0 || miss.IndexReads == 0 {
		t.Fatalf("%+v", miss)
	}
	// 1列だけの索引でも動く。
	one := Build(rows, "a")
	if got := one.Lookup([]Cond{eq("a", 1)}); got.Usable != 1 || got.Rows != 30 {
		t.Fatalf("%+v", got)
	}
	// 空でも落ちない。
	empty := Build(nil, "a")
	if empty.Height() != 1 || ScanPages(nil) != 0 {
		t.Fatal("空で崩れた")
	}
	if got := empty.Lookup([]Cond{eq("a", 1)}); got.Rows != 0 {
		t.Fatalf("%+v", got)
	}
}
