package preemption

import (
	"sort"
	"testing"

	"github.com/esh2n/sharin/orchestration/scheduler"
)

func cpu(m int) scheduler.Resources { return scheduler.Resources{CPU: m, Mem: m} }

func nodes(specs ...NodeSpec) []NodeSpec { return specs }

func names(ps []Pod) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
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

// 空きがあるなら誰も追い出さない。優先度は奪う理由にはならない。
func TestNoPreemptionWhenSpaceExists(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Place(Pod{Name: "batch", Priority: 10, Req: cpu(200)}, "node-a")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(300)})
	c.Run()

	if len(c.Evictions) != 0 {
		t.Fatalf("空きがあるのに追い出した: %v", c.Evictions)
	}
	if got := c.Placement()["api"]; got != "node-a" {
		t.Fatalf("api が置かれていない: %q", got)
	}
}

// 同じ優先度からは奪えない。等しい相手は「低い」ではない。
func TestCannotPreemptEqualPriority(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Place(Pod{Name: "other", Priority: 100, Req: cpu(900)}, "node-a")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(500)})
	c.Run()

	if len(c.Evictions) != 0 {
		t.Fatalf("同じ優先度から奪った: %v", c.Evictions)
	}
	if !eq(c.Pending(), []string{"api"}) {
		t.Fatalf("api が Pending になっていない: %v", c.Pending())
	}
}

// 待ち行列は優先度順。低いほうが先に来ても、高いほうが先に置かれる。
func TestQueueIsOrderedByPriority(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Submit(Pod{Name: "batch", Priority: 10, Req: cpu(600)})
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(600)})
	c.Run()

	if got := c.Placement()["api"]; got != "node-a" {
		t.Fatalf("後から来た高優先度が置かれていない: %q", got)
	}
	if !eq(c.Pending(), []string{"batch"}) {
		t.Fatalf("先に来た低優先度が置かれてしまった: %v", c.Pending())
	}
	if len(c.Evictions) != 0 {
		t.Fatal("行列の順序だけで済むはずが追い出しが起きた")
	}
}

// この章の中心。全部外してから戻すと、犠牲は 1 個で済む。
// 低いものから順に外す素朴なやり方だと 3 個になる場面を選んである。
func TestVictimsAreMinimizedByReprieve(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Place(Pod{Name: "small-a", Priority: 10, Req: cpu(100)}, "node-a")
	c.Place(Pod{Name: "small-b", Priority: 10, Req: cpu(100)}, "node-a")
	c.Place(Pod{Name: "big", Priority: 20, Req: cpu(600)}, "node-a")

	victims, ok := c.selectVictims(c.byName("node-a"), Pod{Name: "api", Priority: 100, Req: cpu(700)})
	if !ok {
		t.Fatal("全部外せば入るのに候補にならなかった")
	}
	if !eq(names(victims), []string{"big"}) {
		t.Fatalf("犠牲が最小になっていない: %v", names(victims))
	}
}

// 低いものを全部どけても入らないノードは、候補にならない。奪い損になる。
func TestNodeIsNotCandidateWhenEvenAllDoesNotFit(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Place(Pod{Name: "locked", Priority: 500, Req: cpu(800)}, "node-a")
	c.Place(Pod{Name: "batch", Priority: 10, Req: cpu(100)}, "node-a")

	if _, ok := c.selectVictims(c.byName("node-a"), Pod{Name: "api", Priority: 100, Req: cpu(500)}); ok {
		t.Fatal("どけても入らないのに候補になった")
	}
}

// 候補が複数あるとき、犠牲の優先度が低いほうを選ぶ。
func TestPicksNodeWithLowestVictimPriority(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	c.Place(Pod{Name: "batch", Priority: 50, Req: cpu(800)}, "node-a")
	c.Place(Pod{Name: "log", Priority: 10, Req: cpu(800)}, "node-b")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(900)})
	c.Run()

	if got := c.Placement()["api"]; got != "node-b" {
		t.Fatalf("犠牲の低いノードが選ばれていない: %q", got)
	}
	if len(c.Evictions) != 1 || c.Evictions[0].Victim != "log" {
		t.Fatalf("追い出しの相手が違う: %v", c.Evictions)
	}
}

// 保護がある側を避けて、優先度の高いほうを犠牲にする。
// そして追い出された Pod が行列に戻り、次はその Pod が下を追い出す。
// 選択肢が無くなれば、保護は破られる。
func TestBudgetShiftsTheVictimAndCascades(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), map[string]int{"log": 1})
	c.Place(Pod{Name: "batch", App: "batch", Priority: 50, Req: cpu(800)}, "node-a")
	c.Place(Pod{Name: "log", App: "log", Priority: 10, Req: cpu(800)}, "node-b")
	c.Submit(Pod{Name: "api", App: "api", Priority: 100, Req: cpu(900)})
	c.Run()

	if len(c.Evictions) != 2 {
		t.Fatalf("玉突きが 2 段になっていない: %v", c.Evictions)
	}
	// 1 段目。保護のある log ではなく、優先度の高い batch のほうが犠牲になる。
	if c.Evictions[0].Victim != "batch" || c.Evictions[0].By != "api" {
		t.Fatalf("1 段目が違う: %+v", c.Evictions[0])
	}
	if c.Evictions[0].Violates {
		t.Fatal("保護を破らずに済む選択をしたはずが、破ったことになっている")
	}
	// 2 段目。追い出された batch が、行き場を求めて log を追い出す。
	// ここでは他に選択肢が無いので、保護を破って進む。
	if c.Evictions[1].Victim != "log" || c.Evictions[1].By != "batch" {
		t.Fatalf("2 段目が違う: %+v", c.Evictions[1])
	}
	if !c.Evictions[1].Violates {
		t.Fatal("保護を破ったことが記録されていない")
	}
	if !eq(c.Pending(), []string{"log"}) {
		t.Fatalf("最後に押し出されたのは log のはず: %v", c.Pending())
	}
	if got := c.Placement()["api"]; got != "node-a" {
		t.Fatalf("api の配置が違う: %q", got)
	}
	if got := c.Placement()["batch"]; got != "node-b" {
		t.Fatalf("batch の配置が違う: %q", got)
	}
}

// 保護が無ければ、素直に優先度のいちばん低いほうが犠牲になる。
// 上のテストとの差が、保護だけであることを示す。
func TestWithoutBudgetTheLowestGoes(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	c.Place(Pod{Name: "batch", App: "batch", Priority: 50, Req: cpu(800)}, "node-a")
	c.Place(Pod{Name: "log", App: "log", Priority: 10, Req: cpu(800)}, "node-b")
	c.Submit(Pod{Name: "api", App: "api", Priority: 100, Req: cpu(900)})
	c.Run()

	if len(c.Evictions) != 1 || c.Evictions[0].Victim != "log" {
		t.Fatalf("保護が無いのに玉突きが起きた: %v", c.Evictions)
	}
	if !eq(c.Pending(), []string{"log"}) {
		t.Fatalf("log が Pending になっていない: %v", c.Pending())
	}
}

// 犠牲の数が少ないほうを選ぶ(優先度も保護も同じとき)。
func TestPicksFewerVictims(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	c.Place(Pod{Name: "a1", Priority: 10, Req: cpu(450)}, "node-a")
	c.Place(Pod{Name: "a2", Priority: 10, Req: cpu(450)}, "node-a")
	c.Place(Pod{Name: "b1", Priority: 10, Req: cpu(900)}, "node-b")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(900)})
	c.Run()

	if got := c.Placement()["api"]; got != "node-b" {
		t.Fatalf("犠牲の少ないノードが選ばれていない: %q", got)
	}
	if len(c.Evictions) != 1 {
		t.Fatalf("犠牲が最小になっていない: %v", c.Evictions)
	}
}

// 追い出された Pod は、行き場があれば普通に置き直される。
func TestVictimIsRescheduledWhenRoomExists(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(500)}, // api は入らないが batch は入る
	), nil)
	c.Place(Pod{Name: "batch", Priority: 10, Req: cpu(400)}, "node-a")
	c.Place(Pod{Name: "keep", Priority: 900, Req: cpu(300)}, "node-a")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(650)})
	c.Run()

	if len(c.Evictions) != 1 || c.Evictions[0].Victim != "batch" {
		t.Fatalf("追い出しが想定と違う: %v", c.Evictions)
	}
	if got := c.Placement()["batch"]; got != "node-b" {
		t.Fatalf("追い出された Pod が置き直されていない: %q", got)
	}
	if len(c.Pending()) != 0 {
		t.Fatalf("Pending が残った: %v", c.Pending())
	}
}

// 保護は数を見る。同じ App の Pod が余っていれば、1 つ消しても破らない。
func TestBudgetCountsReplicas(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), map[string]int{"log": 1})
	c.Place(Pod{Name: "log-1", App: "log", Priority: 10, Req: cpu(800)}, "node-a")
	c.Place(Pod{Name: "log-2", App: "log", Priority: 10, Req: cpu(100)}, "node-b")

	if got := c.violations([]Pod{{Name: "log-1", App: "log"}}); got != 0 {
		t.Fatalf("2 個あるうち 1 個消すのは破りではない: %d", got)
	}
	if got := c.violations([]Pod{{Name: "log-1", App: "log"}, {Name: "log-2", App: "log"}}); got != 1 {
		t.Fatalf("2 個とも消せば破り 1 件のはず: %d", got)
	}
	if got := c.violations([]Pod{{Name: "x", App: "unmanaged"}}); got != 0 {
		t.Fatalf("保護の無い App で破りが数えられた: %d", got)
	}
}

// 置き場所も奪う相手も無ければ Pending のまま残る。
func TestUnschedulableStaysPending(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(500)}), nil)
	c.Submit(Pod{Name: "huge", Priority: 1000, Req: cpu(900)})
	c.Run()

	if !eq(c.Pending(), []string{"huge"}) {
		t.Fatalf("Pending になっていない: %v", c.Pending())
	}
	if len(c.Evictions) != 0 {
		t.Fatal("空のノードから追い出した")
	}
}

// 存在しないノードへの Place は何も起こさない。
func TestPlaceOnUnknownNodeIsIgnored(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(500)}), nil)
	c.Place(Pod{Name: "ghost", Priority: 10, Req: cpu(100)}, "node-z")
	if len(c.Placement()) != 0 {
		t.Fatalf("知らないノードに置かれた: %v", c.Placement())
	}
	if c.PodsOn("node-z") != nil {
		t.Fatal("知らないノードの一覧が返った")
	}
}

// 観測用の API。
func TestAccessors(t *testing.T) {
	c := New(nodes(NodeSpec{Name: "node-a", Cap: cpu(1000)}), nil)
	c.Place(Pod{Name: "b", Priority: 10, Req: cpu(100)}, "node-a")
	c.Place(Pod{Name: "a", Priority: 10, Req: cpu(100)}, "node-a")
	if !eq(c.PodsOn("node-a"), []string{"a", "b"}) {
		t.Fatalf("PodsOn が名前順で返っていない: %v", c.PodsOn("node-a"))
	}
	if c.byName("node-z") != nil {
		t.Fatal("知らないノードが引けた")
	}
	if len(c.Log) != 0 {
		t.Fatal("何もしていないのにログがある")
	}
}

// 玉突きが循環しても、上限で必ず止まる。
func TestRunTerminates(t *testing.T) {
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	for i, r := range []int{900, 800, 700, 600} {
		c.Place(Pod{Name: "p" + itoa(i), Priority: 10 + i, Req: cpu(r)}, "node-a")
	}
	for i := 0; i < 6; i++ {
		c.Submit(Pod{Name: "q" + itoa(i), Priority: 100, Req: cpu(900)})
	}
	c.Run() // 止まればよい(無限ループしないこと)
	if len(c.Placement())+len(c.Pending()) == 0 {
		t.Fatal("何も処理されていない")
	}
}

// 迷ったときの決め方が決定的であること。空きが同じなら名前順、
// 犠牲がまったく同じ形ならノード名順で決める。
func TestTiesAreBrokenDeterministically(t *testing.T) {
	// 空きが大きいほうへ。
	c := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	c.Place(Pod{Name: "sitting", Priority: 10, Req: cpu(500)}, "node-a")
	c.Submit(Pod{Name: "api", Priority: 100, Req: cpu(100)})
	c.Run()
	if got := c.Placement()["api"]; got != "node-b" {
		t.Fatalf("空きの大きいノードが選ばれていない: %q", got)
	}

	// 空きが同じなら名前順。
	d := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	d.Submit(Pod{Name: "api", Priority: 100, Req: cpu(100)})
	d.Run()
	if got := d.Placement()["api"]; got != "node-a" {
		t.Fatalf("同じ空きで名前順になっていない: %q", got)
	}

	// 犠牲の形がまったく同じなら、ノード名順。
	e := New(nodes(
		NodeSpec{Name: "node-a", Cap: cpu(1000)},
		NodeSpec{Name: "node-b", Cap: cpu(1000)},
	), nil)
	e.Place(Pod{Name: "x", Priority: 10, Req: cpu(800)}, "node-a")
	e.Place(Pod{Name: "y", Priority: 10, Req: cpu(800)}, "node-b")
	e.Submit(Pod{Name: "api", Priority: 100, Req: cpu(900)})
	e.Run()
	if got := e.Placement()["api"]; got != "node-a" {
		t.Fatalf("同形の犠牲で名前順になっていない: %q", got)
	}
}

func TestItoa(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, "0"}, {5, "5"}, {120, "120"}} {
		if got := itoa(c.n); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
