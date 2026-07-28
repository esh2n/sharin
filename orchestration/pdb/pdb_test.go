package pdb

import "testing"

func web() map[string]string { return map[string]string{"app": "web"} }

// build は web な Pod を n 個(node-a と node-b に交互)置き、
// 下限 min の宣言を1つ付けたクラスタを作る。
func build(n, min, startup int) *Cluster {
	c := New(Config{StartupTicks: startup, ReplaceNode: "node-c"})
	for i := 1; i <= n; i++ {
		node := "node-a"
		if i%2 == 0 {
			node = "node-b"
		}
		c.AddPod("web-"+itoa(i), node, web(), true)
	}
	c.AddBudget("web-pdb", min, web())
	return c
}

// 下限に余裕があれば退避できる。
func TestEvictAllowedWithSlack(t *testing.T) {
	c := build(3, 2, 0)
	if !c.Evict("web-1") {
		t.Fatalf("3 個あって下限 2 なら1つは退避できるはず\n%v", c.Log)
	}
	if c.Evicted != 1 || c.Denied != 0 {
		t.Fatalf("evicted=%d denied=%d", c.Evicted, c.Denied)
	}
}

// 下限を割る退避は断られる。作り直し中の Pod は頭数に入らないので、
// 立ち上がるまで次の退避は通らない。
func TestEvictDeniedWhenAtFloor(t *testing.T) {
	c := build(3, 2, 3) // 作り直しに 3 周期かかる
	if !c.Evict("web-1") {
		t.Fatal("1つ目は通るはず")
	}
	if c.Evict("web-2") {
		t.Fatalf("ready なのは 2 個。下限 2 を割るので断られるはず\n%v", c.Log)
	}
	if c.Denied != 1 {
		t.Fatalf("断られた回数が %d", c.Denied)
	}

	// 代わりが立ち上がれば、また1つ退避できる。
	for i := 0; i < 3; i++ {
		c.Tick()
	}
	if !c.Evict("web-2") {
		t.Fatalf("立ち上がった後は通るはず\n%v", c.Log)
	}
}

// 下限とレプリカ数が同じだと、1つも退避できない。
// 更新も集約もノードの入れ替えも、すべて止まる。
func TestFloorEqualToReplicasBlocksEverything(t *testing.T) {
	c := build(3, 3, 1)
	for _, name := range []string{"web-1", "web-2", "web-3"} {
		if c.Evict(name) {
			t.Fatalf("%s が退避できてしまった", name)
		}
	}
	if c.Denied != 3 {
		t.Fatalf("3 回とも断られるはずが %d", c.Denied)
	}
	if _, remaining := c.Drain("node-a"); len(remaining) == 0 {
		t.Fatal("drain も1歩も進まないはず")
	}
}

// 宣言が守るのは自発的な退避だけ。落ちる分は止められない。
func TestCrashIgnoresBudget(t *testing.T) {
	c := build(3, 3, 1) // 1つも退避できない厳しい設定
	c.Crash("web-1")
	c.Crash("web-2")
	if c.Crashed != 2 {
		t.Fatalf("落ちる分は宣言に関係なく消えるはずが %d", c.Crashed)
	}
	if c.Denied != 0 {
		t.Fatal("Crash は宣言を参照しないはず")
	}
	if got := c.Available(web()); got != 1 {
		t.Fatalf("下限 3 を宣言していても 1 個まで減るはずが %d", got)
	}
}

// drain は退避の連続。断られたぶんはノードに残るので、何度も呼ぶことになる。
func TestDrainProgressesOverTime(t *testing.T) {
	c := build(4, 3, 2) // node-a に web-1, web-3
	ev, remaining := c.Drain("node-a")
	if ev != 1 || len(remaining) != 1 {
		t.Fatalf("1つ退避して1つ残るはず: ev=%d remaining=%v\n%v", ev, remaining, c.Log)
	}
	for i := 0; i < 2; i++ {
		c.Tick()
	}
	ev2, remaining2 := c.Drain("node-a")
	if ev2 != 1 || len(remaining2) != 0 {
		t.Fatalf("立ち上がった後に残りを退避できるはず: ev=%d remaining=%v\n%v", ev2, remaining2, c.Log)
	}
}

// 条件に合わない Pod は、その宣言の制約を受けない。
func TestBudgetOnlyAppliesToMatchingPods(t *testing.T) {
	c := build(2, 2, 0)
	c.AddPod("db-1", "node-a", map[string]string{"app": "db"}, true)
	if !c.Evict("db-1") {
		t.Fatal("別のラベルの Pod は web の宣言に縛られないはず")
	}
	if c.Evict("web-1") {
		t.Fatal("web は下限ちょうどなので断られるはず")
	}
}

// ready でない Pod の退避は、頭数を減らさないので通る。
func TestEvictingUnreadyPodIsAllowed(t *testing.T) {
	c := build(2, 2, 0)
	c.AddPod("web-broken", "node-a", web(), false)
	if !c.Evict("web-broken") {
		t.Fatalf("ready でない Pod を消しても頭数は減らないので通るはず\n%v", c.Log)
	}
}

// 宣言が無ければ、退避は常に通る。
func TestNoBudgetAllowsEverything(t *testing.T) {
	c := New(Config{})
	c.AddPod("web-1", "node-a", web(), true)
	if !c.Evict("web-1") {
		t.Fatal("宣言が無ければ通るはず")
	}
	if len(c.Pods()) != 0 {
		t.Fatal("代わりの設定が無いので作り直されないはず")
	}
}

// 存在しない Pod への操作は何もしない。
func TestUnknownPodIsNoop(t *testing.T) {
	c := build(2, 1, 0)
	if c.Evict("nosuch") {
		t.Fatal("存在しない Pod は退避できないはず")
	}
	c.Crash("nosuch")
	if c.Crashed != 0 || len(c.Pods()) != 2 {
		t.Fatal("何も起きないはず")
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(3) != "3" || itoa(1024) != "1024" {
		t.Fatal("itoa が違う")
	}
}
