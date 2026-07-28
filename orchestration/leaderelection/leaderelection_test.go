package leaderelection

import "testing"

func run(s *Sim, n int) {
	for i := 0; i < n; i++ {
		s.Tick()
	}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 誰も持っていないところから始めれば、いちばん先に動いた者が持ち主になる。
func TestFirstCandidateTakesTheLease(t *testing.T) {
	s := New(Default(), "c2", "c1", "c3")
	s.Tick()

	if s.Holder() != "c1" {
		t.Fatalf("名前順の先頭が持ち主になっていない: %q", s.Holder())
	}
	if !eq(s.Believers(), []string{"c1"}) {
		t.Fatalf("持ち主だと思っているのが1人でない: %v", s.Believers())
	}
}

// 持ち主が更新し続けている限り、他は奪わない。
func TestOthersDoNotStealWhileRenewing(t *testing.T) {
	s := New(Default(), "c1", "c2", "c3")
	run(s, 60)

	if s.Holder() != "c1" {
		t.Fatalf("持ち主が変わった: %q", s.Holder())
	}
	if s.Overlap != 0 || s.Vacant != 0 {
		t.Fatalf("何も起きていないのに重なり %d、空位 %d", s.Overlap, s.Vacant)
	}
}

// この章の中心。降りるほうが先なら、2人が同時に持ち主だと思う時刻はできない。
// 代わりに、誰も持ち主でない時間ができる。
func TestSafeConfigNeverOverlaps(t *testing.T) {
	cfg := Default() // 15 / 10 / 2
	if !cfg.Safe() {
		t.Fatal("既定の設定が安全でない")
	}
	s := New(cfg, "c1", "c2")
	run(s, 7)
	s.Partition("c1")
	run(s, 30)

	if s.Overlap != 0 {
		t.Fatalf("安全な設定で重なった: %d", s.Overlap)
	}
	if s.Vacant == 0 {
		t.Fatal("降りてから奪われるまでの空位が無い")
	}
	if s.Holder() != "c2" {
		t.Fatalf("最終的に c2 が持ち主になっていない: %q", s.Holder())
	}
	if !eq(s.Believers(), []string{"c2"}) {
		t.Fatalf("持ち主だと思っているのが c2 だけでない: %v", s.Believers())
	}
}

// 大小を逆にすると、逆転した幅ぶんだけ重なる。
func TestUnsafeConfigOverlapsByTheInversion(t *testing.T) {
	cfg := Config{LeaseDuration: 15, RenewDeadline: 20, RetryPeriod: 2}
	if cfg.Safe() {
		t.Fatal("危険な設定が安全と判定された")
	}
	s := New(cfg, "c1", "c2")
	run(s, 7)
	s.Partition("c1")
	run(s, 30)

	if s.Overlap == 0 {
		t.Fatal("逆転しているのに重なりが出ていない")
	}
	if s.Overlap != 4 {
		t.Fatalf("重なりの幅が想定と違う: %d", s.Overlap)
	}
	if s.DoubleActs != s.Overlap {
		t.Fatalf("2人重なりでの二重操作が幅と一致しない: %d", s.DoubleActs)
	}
	if s.Vacant != 0 {
		t.Fatalf("重なる設定で空位が出た: %d", s.Vacant)
	}
}

// 安全な設定と危険な設定の差が、重なりと空位の取引になっている。
func TestSafetyIsTradedForVacancy(t *testing.T) {
	safe := New(Default(), "c1", "c2")
	risky := New(Config{LeaseDuration: 15, RenewDeadline: 20, RetryPeriod: 2}, "c1", "c2")
	for _, s := range []*Sim{safe, risky} {
		run(s, 7)
		s.Partition("c1")
		run(s, 30)
	}

	if safe.Overlap != 0 || safe.Vacant == 0 {
		t.Fatalf("安全側の性質が違う: 重なり %d 空位 %d", safe.Overlap, safe.Vacant)
	}
	if risky.Overlap == 0 || risky.Vacant != 0 {
		t.Fatalf("危険側の性質が違う: 重なり %d 空位 %d", risky.Overlap, risky.Vacant)
	}
}

// 切り離しが解ければ、古い持ち主はすぐ気づいて降りる。
func TestHealedOldHolderStepsDown(t *testing.T) {
	cfg := Config{LeaseDuration: 15, RenewDeadline: 20, RetryPeriod: 2}
	s := New(cfg, "c1", "c2")
	run(s, 7)
	s.Partition("c1")
	run(s, 16) // c2 が奪ったあと、c1 はまだ自分を持ち主だと思っている

	if len(s.Believers()) != 2 {
		t.Fatalf("重なりの状態を作れていない: %v", s.Believers())
	}
	before := s.Overlap
	s.Heal("c1")
	s.Tick()

	if !eq(s.Believers(), []string{"c2"}) {
		t.Fatalf("届くようになっても降りていない: %v", s.Believers())
	}
	if s.Overlap != before {
		t.Fatalf("降りた時刻まで重なりに数えている: %d → %d", before, s.Overlap)
	}
}

// 観測が先で判断が後。長く切り離されていた候補が復帰しても、盗まない。
// 順序が逆だと、古い観測のまま「期限切れだ」と判断して奪ってしまう。
func TestObserveBeforeJudgePreventsStaleSteal(t *testing.T) {
	s := New(Default(), "c1", "c2")
	run(s, 7)
	s.Partition("c2")
	run(s, 40) // c2 の観測は 40 以上古くなる。LeaseDuration は 15
	s.Heal("c2")
	s.Tick()

	if s.Holder() != "c1" {
		t.Fatalf("生きている持ち主から盗んだ: %q", s.Holder())
	}
	if !eq(s.Believers(), []string{"c1"}) {
		t.Fatalf("持ち主が1人でない: %v", s.Believers())
	}
	if s.Overlap != 0 || s.Vacant != 0 {
		t.Fatalf("持ち主は一度も途切れていないはず: 重なり %d 空位 %d", s.Overlap, s.Vacant)
	}
}

// 持ち主が切り離されている間、残りのうち1人だけが奪う。
func TestOnlyOneSuccessorTakesOver(t *testing.T) {
	s := New(Default(), "c1", "c2", "c3")
	run(s, 7)
	s.Partition("c1")
	run(s, 30)

	if len(s.Believers()) != 1 {
		t.Fatalf("後継が1人でない: %v", s.Believers())
	}
	if s.Overlap != 0 {
		t.Fatalf("後継の取り合いで重なった: %d", s.Overlap)
	}
}

// 設定の大小の検査。
func TestSafe(t *testing.T) {
	cases := []struct {
		cfg  Config
		want bool
	}{
		{Config{LeaseDuration: 15, RenewDeadline: 10, RetryPeriod: 2}, true},
		{Config{LeaseDuration: 15, RenewDeadline: 15, RetryPeriod: 2}, false}, // 同じでも駄目
		{Config{LeaseDuration: 15, RenewDeadline: 20, RetryPeriod: 2}, false},
		{Config{LeaseDuration: 15, RenewDeadline: 10, RetryPeriod: 10}, false}, // 試行が猶予以上
	}
	for _, c := range cases {
		if got := c.cfg.Safe(); got != c.want {
			t.Errorf("Safe(%+v) = %v, want %v", c.cfg, got, c.want)
		}
	}
}

// 知らない名前への操作は何も起こさない。二度呼んでも一度しか記録しない。
func TestPartitionAndHealAreGuarded(t *testing.T) {
	s := New(Default(), "c1")
	s.Tick()
	s.Partition("nobody")
	s.Heal("nobody")
	s.Heal("c1") // 切り離されていないので何もしない
	n := len(s.Log)
	s.Partition("c1")
	s.Partition("c1") // 二度目は記録しない
	if len(s.Log) != n+1 {
		t.Fatalf("記録が想定と違う: %v", s.Log[n:])
	}
	if s.Now() != 1 {
		t.Fatalf("時刻が進んでいる: %d", s.Now())
	}
}

func TestItoa(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, "0"}, {9, "9"}, {15, "15"}} {
		if got := itoa(c.n); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
