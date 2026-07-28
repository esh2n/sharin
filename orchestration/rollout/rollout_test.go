package rollout

import "testing"

var (
	v1 = Release{Version: 1, StartupTicks: 2}
	v2 = Release{Version: 2, StartupTicks: 2}
	// 壊れた版。何周期経っても ready にならない。
	bad = Release{Version: 2, StartupTicks: 2, Broken: true}
)

func cfg(surge, unavail int) Config {
	return Config{Replicas: 4, MaxSurge: surge, MaxUnavailable: unavail}
}

// 素直な設定なら、全 Pod が新しい版に入れ替わって完了する。
func TestReplacesAllPods(t *testing.T) {
	r := New(cfg(1, 1), v1)
	r.Deploy(v2)
	r.Run(50)
	if !r.Done() {
		t.Fatalf("完了しないまま止まった\n%v", r.Log)
	}
	for _, p := range r.Pods() {
		if p.Version != 2 {
			t.Fatalf("古い版が残っている: %s", p.Name)
		}
	}
	if len(r.Pods()) != 4 {
		t.Fatalf("完了後は 4 個のはずが %d", len(r.Pods()))
	}
}

// maxUnavailable が 0 なら、入れ替え中も ready な数が目標を割らない。
// 多く作ってから消すので、容量は保たれる。
func TestMaxUnavailableZeroKeepsCapacity(t *testing.T) {
	r := New(cfg(1, 0), v1)
	r.Deploy(v2)
	r.Run(50)
	if !r.Done() {
		t.Fatalf("完了しないまま止まった\n%v", r.Log)
	}
	if r.MinAvailableSeen < 4 {
		t.Fatalf("容量が %d まで落ちた。maxUnavailable=0 なら 4 を保つはず", r.MinAvailableSeen)
	}
}

// maxSurge が 0 なら、多く作れないので先に消すしかない。
// 入れ替え中は容量が落ちる。落ちる幅は maxUnavailable が決める。
func TestMaxSurgeZeroReducesCapacity(t *testing.T) {
	r := New(cfg(0, 1), v1)
	r.Deploy(v2)
	r.Run(50)
	if !r.Done() {
		t.Fatalf("完了しないまま止まった\n%v", r.Log)
	}
	if r.MinAvailableSeen != 3 {
		t.Fatalf("maxUnavailable=1 なら 3 まで落ちるはずが %d", r.MinAvailableSeen)
	}
	if len(r.Pods()) != 4 {
		t.Fatalf("完了後は 4 個のはずが %d", len(r.Pods()))
	}
}

// 幅を広げるほど入れ替えは速く終わる。速さと安全は交換になっている。
func TestWiderWindowFinishesSooner(t *testing.T) {
	narrow := New(cfg(1, 0), v1)
	narrow.Deploy(v2)
	narrow.Run(50)

	wide := New(cfg(4, 2), v1)
	wide.Deploy(v2)
	wide.Run(50)

	if !narrow.Done() || !wide.Done() {
		t.Fatal("どちらも完了するはず")
	}
	if len(wide.History) >= len(narrow.History) {
		t.Fatalf("広いほど速いはず: wide=%d narrow=%d", len(wide.History), len(narrow.History))
	}
	if wide.MinAvailableSeen >= narrow.MinAvailableSeen {
		t.Fatalf("広いほど容量は落ちるはず: wide=%d narrow=%d", wide.MinAvailableSeen, narrow.MinAvailableSeen)
	}
}

// 壊れた版をデプロイしても、古い版は消えない。置き換えは途中で止まり、
// 全滅しない。ready を進行の条件にしていることが、そのまま安全弁になる。
func TestBrokenReleaseStallsInsteadOfWiping(t *testing.T) {
	r := New(cfg(1, 0), v1)
	r.Deploy(bad)
	r.Run(50)

	if r.Done() {
		t.Fatal("壊れた版で完了してはいけない")
	}
	if !r.Stalled() {
		t.Fatal("これ以上進めない状態のはず")
	}
	if r.Available() != 4 {
		t.Fatalf("maxUnavailable=0 なら容量は 4 のまま保たれるはずが %d", r.Available())
	}
	old := 0
	for _, p := range r.Pods() {
		if p.Version == 1 {
			old++
		}
	}
	if old != 4 {
		t.Fatalf("古い版が 4 個残るはずが %d\n%v", old, r.Log)
	}
}

// maxUnavailable を大きくすると、壊れた版でも止まるまでに被害が出る。
// 幅の設定が、そのまま被害の上限になっている。
func TestMaxUnavailableBoundsTheDamage(t *testing.T) {
	safe := New(cfg(1, 0), v1)
	safe.Deploy(bad)
	safe.Run(50)

	risky := New(cfg(1, 2), v1)
	risky.Deploy(bad)
	risky.Run(50)

	if safe.MinAvailableSeen != 4 {
		t.Fatalf("maxUnavailable=0 は 4 を保つはずが %d", safe.MinAvailableSeen)
	}
	if risky.MinAvailableSeen != 2 {
		t.Fatalf("maxUnavailable=2 は 2 まで落ちるはずが %d", risky.MinAvailableSeen)
	}
	if !risky.Stalled() {
		t.Fatal("下限まで落ちたらそこで止まるはず")
	}
}

// 止まった後で元の版に戻せば、同じ仕組みで元に戻る。
// ロールバックは特別な操作でなく、目標をもう一度書き換えるだけ。
func TestRollbackUsesTheSameMechanism(t *testing.T) {
	r := New(cfg(1, 1), v1)
	r.Deploy(bad)
	r.Run(50)
	if !r.Stalled() {
		t.Fatal("壊れた版で止まっているはず")
	}

	r.Deploy(v1) // 元の版へ戻す
	r.Run(50)
	if !r.Done() {
		t.Fatalf("戻せば完了するはず\n%v", r.Log)
	}
	for _, p := range r.Pods() {
		if p.Version != 1 {
			t.Fatalf("v1 に戻っていない: %s は v%d", p.Name, p.Version)
		}
	}
}

// 幅がどちらも 0 だと、作ることも消すこともできず1歩も進めない。
func TestBothZeroCannotProgress(t *testing.T) {
	c := cfg(0, 0)
	if !c.Deadlocked() {
		t.Fatal("どちらも 0 は進めない設定のはず")
	}
	r := New(c, v1)
	r.Deploy(v2)
	r.Run(20)
	if r.Done() {
		t.Fatal("進めないはずが完了した")
	}
	for _, p := range r.Pods() {
		if p.Version != 1 {
			t.Fatal("1つも入れ替わらないはず")
		}
	}
}

// 起動が遅い版ほど、入れ替えに時間がかかる。ready を待つから。
func TestSlowStartupTakesLonger(t *testing.T) {
	fast := New(cfg(1, 0), v1)
	fast.Deploy(Release{Version: 2, StartupTicks: 1})
	fast.Run(60)

	slow := New(cfg(1, 0), v1)
	slow.Deploy(Release{Version: 2, StartupTicks: 6})
	slow.Run(60)

	if !fast.Done() || !slow.Done() {
		t.Fatal("どちらも完了するはず")
	}
	if len(slow.History) <= len(fast.History) {
		t.Fatalf("起動が遅いほど長くかかるはず: slow=%d fast=%d", len(slow.History), len(fast.History))
	}
}

// 目標と同じ版を Deploy し直しても何も起こらない(冪等)。
func TestDeploySameVersionIsNoop(t *testing.T) {
	r := New(cfg(1, 1), v1)
	r.Deploy(v1)
	r.Run(10)
	if !r.Done() {
		t.Fatal("すでに目標に達しているはず")
	}
	if len(r.History) != 0 {
		t.Fatalf("何も起こらないはずが %d 周期動いた", len(r.History))
	}
}

func TestItoaAndMinAvailable(t *testing.T) {
	if itoa(0) != "0" || itoa(41) != "41" {
		t.Fatal("itoa が違う")
	}
	if (Config{Replicas: 2, MaxUnavailable: 5}).minAvailable() != 0 {
		t.Fatal("下限は 0 未満にならないはず")
	}
}
