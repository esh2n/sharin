package collect

import "testing"

// 間隔 60、1 回のコスト 1、同時 1 本。つまり 1 周で 60 対象まで。
var cfg = Config{Interval: 60, ScrapeCost: 1, Workers: 1}

// この章の中心その1。pull には 1 周の予算があり、超えたぶんは読めない。
func TestPullHasARoundBudget(t *testing.T) {
	t.Logf("間隔 %d / 1 回 %d / 同時 %d 本 → 上限 %d 対象",
		cfg.Interval, cfg.ScrapeCost, cfg.Workers, MaxTargets(cfg))
	t.Logf("%-8s %8s %8s %8s", "対象数", "読めた", "落とした", "1周の時間")

	for _, n := range []int{10, 60, 61, 120} {
		r := Pull(NewTargets(n, 10, 5), cfg)
		t.Logf("%-8d %8d %8d %8d", n, r.Scraped, r.Dropped, r.RoundCost)
	}

	// 上限ちょうどまでは全部読める。
	if r := Pull(NewTargets(60, 10, 5), cfg); r.Scraped != 60 || r.Dropped != 0 {
		t.Fatalf("60 対象: %+v", r)
	}
	// 1 つ超えると、超えたぶんが落ちる。
	if r := Pull(NewTargets(61, 10, 5), cfg); r.Dropped != 1 {
		t.Fatalf("61 対象: %+v", r)
	}
	// 倍にすると半分落ちる。
	if r := Pull(NewTargets(120, 10, 5), cfg); r.Scraped != 60 || r.Dropped != 60 {
		t.Fatalf("120 対象: %+v", r)
	}
	if Fits(60, cfg) != true || Fits(61, cfg) != false {
		t.Fatal("境界が合わない")
	}
}

// 同時に叩く本数を増やすと、上限がそのぶん伸びる。
func TestWorkersRaiseTheBudget(t *testing.T) {
	for _, w := range []int{1, 2, 4, 8} {
		c := Config{Interval: 60, ScrapeCost: 1, Workers: w}
		t.Logf("同時 %d 本 → 上限 %d 対象", w, MaxTargets(c))
	}
	if MaxTargets(Config{Interval: 60, ScrapeCost: 1, Workers: 4}) != 240 {
		t.Fatal("並列で伸びていない")
	}
	// 間隔を延ばしても同じだけ伸びる。ただし解像度が落ちる。
	if MaxTargets(Config{Interval: 240, ScrapeCost: 1, Workers: 1}) != 240 {
		t.Fatal("間隔で伸びていない")
	}
}

// この章の中心その2。落ちた対象について、pull は言えて push は言えない。
func TestOnlyPullCanTellThatATargetIsDown(t *testing.T) {
	targets := Down(NewTargets(10, 5, 5), 3, 7)

	pull := Pull(targets, cfg)
	push := Push(targets, cfg)

	t.Logf("pull: 読めた %d / 落ちていると判定 %v", pull.Scraped, pull.DownDetected)
	t.Logf("push: 届いた %d / 無音 %d / 落ちていると判定 %v",
		push.Received, push.Silent, push.DownDetected)

	// pull は叩いて失敗するので、どれが落ちたかを名指しできる。
	if len(pull.DownDetected) != 2 {
		t.Fatalf("pull が検出できていない: %+v", pull)
	}
	if pull.DownDetected[0] != "t3" || pull.DownDetected[1] != "t7" {
		t.Fatalf("名指しが違う: %v", pull.DownDetected)
	}
	// push は「来なかった」ことしか分からず、落ちたとは言えない。
	if len(push.DownDetected) != 0 {
		t.Fatalf("push が判定できてしまっている: %+v", push)
	}
	// 無音の数は分かる。だがそれが落ちたせいかは分からない。
	if push.Silent != 2 {
		t.Fatalf("無音の数が違う: %+v", push)
	}
}

// 差の正体は「叩くかどうか」ではなく「居るはずの一覧を持っているか」にある。
func TestTheRealDifferenceIsKnowingWhoShouldBeThere(t *testing.T) {
	targets := Down(NewTargets(10, 5, 5), 3, 7)
	known := AllIDs(targets)
	arrived := IDs(targets) // 生きているものだけが送ってくる

	missing := SilentButExpected(known, arrived)
	t.Logf("一覧を別に持てば、push でも名指しできる: %v", missing)

	if len(missing) != 2 || missing[0] != "t3" || missing[1] != "t7" {
		t.Fatalf("一覧があっても言えていない: %v", missing)
	}
	// 一覧が無ければ、同じ入力から何も言えない。
	if got := SilentButExpected(nil, arrived); len(got) != 0 {
		t.Fatalf("一覧なしで言えてしまっている: %v", got)
	}
}

// この章の中心その3。系列はラベルの掛け算で増える。
func TestCardinalityMultiplies(t *testing.T) {
	labels := []Label{
		{Name: "pod", Values: 30},
		{Name: "endpoint", Values: 20},
		{Name: "status", Values: 5},
	}
	t.Logf("%-28s %10s", "ラベル", "系列数")
	for i := 1; i <= len(labels); i++ {
		names := ""
		for _, l := range labels[:i] {
			if names != "" {
				names += " × "
			}
			names += l.Name
		}
		t.Logf("%-28s %10d", names, Cardinality(labels[:i]))
	}

	if Cardinality(labels) != 3000 {
		t.Fatalf("掛け算になっていない: %d", Cardinality(labels))
	}
	// ラベルを1つ足すと、その値の種類の数だけ倍になる。
	before := Cardinality(labels)
	after := Cardinality(append(labels, Label{Name: "version", Values: 4}))
	if after != before*4 {
		t.Fatalf("倍率が違う: %d → %d", before, after)
	}
	// user_id のような値域の広いラベルを1つ入れるだけで桁が変わる。
	huge := Cardinality(append(labels, Label{Name: "user_id", Values: 10000}))
	t.Logf("user_id(1万種)を足すと %d 系列", huge)
	if huge != 30_000_000 {
		t.Fatalf("%d", huge)
	}
	if Bytes(huge, 8) != 240_000_000 {
		t.Fatalf("%d", Bytes(huge, 8))
	}
}

// この章の中心その4。どこに置くかで、1つ落ちたときに失う範囲が変わる。
func TestPlacementDecidesTheBlastRadius(t *testing.T) {
	// 100 対象、ノードあたり 10 個、1 対象 50 系列。合計 5000 系列。
	targets := NewTargets(100, 10, 50)

	t.Logf("100 対象 / 10 ノード / 1 対象 50 系列 = 合計 5000 系列")
	t.Logf("%-10s %10s %14s %16s", "置き方", "収集器の数", "1つが抱える", "1つ落ちて失う")

	var layouts []Layout
	for _, p := range []Placement{Central, Sidecar, PerNode} {
		l := Place(targets, p)
		layouts = append(layouts, l)
		t.Logf("%-10s %10d %14d %16d", p, l.Collectors, l.MaxSeriesPerCollector, l.LostOnOneFailure)
	}

	central, sidecar, perNode := layouts[0], layouts[1], layouts[2]

	// 中央に1つ: 収集器は1つで済むが、落ちると全部失う。
	if central.Collectors != 1 || central.LostOnOneFailure != 5000 {
		t.Fatalf("%+v", central)
	}
	// サイドカー: 収集器は対象の数だけ要るが、失うのは1対象ぶん。
	if sidecar.Collectors != 100 || sidecar.LostOnOneFailure != 50 {
		t.Fatalf("%+v", sidecar)
	}
	// ノードごと: その中間。
	if perNode.Collectors != 10 || perNode.LostOnOneFailure != 500 {
		t.Fatalf("%+v", perNode)
	}
	// 収集器を増やすほど、失う範囲は小さくなる。両者は逆向きに動く。
	if !(central.Collectors < perNode.Collectors && perNode.Collectors < sidecar.Collectors) {
		t.Fatal("収集器の数が単調でない")
	}
	if !(central.LostOnOneFailure > perNode.LostOnOneFailure && perNode.LostOnOneFailure > sidecar.LostOnOneFailure) {
		t.Fatal("失う範囲が単調でない")
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 対象ゼロ。
	if r := Pull(nil, cfg); r.Scraped != 0 || r.Dropped != 0 || r.RoundCost != 0 {
		t.Fatalf("%+v", r)
	}
	if r := Push(nil, cfg); r.Received != 0 || r.Silent != 0 {
		t.Fatalf("%+v", r)
	}
	// Workers が 0 でも 1 本として扱う。
	if MaxTargets(Config{Interval: 60, ScrapeCost: 1, Workers: 0}) != 60 {
		t.Fatal("Workers=0 で崩れた")
	}
	// 全部落ちていれば、pull は全部を名指しし、系列は 0。
	all := Down(NewTargets(3, 3, 5), 0, 1, 2)
	r := Pull(all, cfg)
	if len(r.DownDetected) != 3 || r.Series != 0 || r.Scraped != 0 {
		t.Fatalf("%+v", r)
	}
	// ラベルが無ければ系列は 1(ラベルなしの1本)。
	if Cardinality(nil) != 1 {
		t.Fatal("空のラベルで 1 にならない")
	}
	// 対象ゼロの配置。
	if l := Place(nil, PerNode); l.Collectors != 0 || l.LostOnOneFailure != 0 {
		t.Fatalf("%+v", l)
	}
	if l := Place(nil, Central); l.Collectors != 1 || l.LostOnOneFailure != 0 {
		t.Fatalf("%+v", l)
	}
}
