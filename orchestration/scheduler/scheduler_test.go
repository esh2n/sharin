package scheduler

import "testing"

// small は 500m/512Mi を要求する Pod を作る。
func small(name string) Pod { return Pod{Name: name, Req: Resources{CPU: 500, Mem: 512}} }

// nodes は同容量のノードを 3 台作る(2000m/2048Mi)。
func nodes() []*Node {
	return []*Node{
		NewNode("node-a", Resources{CPU: 2000, Mem: 2048}),
		NewNode("node-b", Resources{CPU: 2000, Mem: 2048}),
		NewNode("node-c", Resources{CPU: 2000, Mem: 2048}),
	}
}

// 空きが足りないノードは filter で落ちる。要求は予約であり、
// すでに載っている Pod のぶんだけ空きが減る。
func TestFilterRejectsInsufficientResources(t *testing.T) {
	ns := []*Node{
		NewNode("tiny", Resources{CPU: 100, Mem: 128}),
		NewNode("big", Resources{CPU: 2000, Mem: 2048}),
	}
	feasible, verdicts := Filter(small("p"), ns)
	if len(feasible) != 1 || feasible[0].Name != "big" {
		t.Fatalf("big だけが候補に残るはず: %v", names(feasible))
	}
	if verdicts[0].Fits || verdicts[0].Why == "" {
		t.Fatalf("tiny には落ちた理由が付くはず: %+v", verdicts[0])
	}
}

// 汚れ(taint)を許容しない Pod は、そのノードに置けない。
// 許容する Pod だけが置ける。
func TestFilterTaintToleration(t *testing.T) {
	ns := []*Node{NewNode("gpu", Resources{CPU: 2000, Mem: 2048}).Taint("hardware", "gpu")}

	plain := small("plain")
	if feasible, _ := Filter(plain, ns); len(feasible) != 0 {
		t.Fatalf("許容していない Pod は置けないはず: %v", names(feasible))
	}

	tolerant := small("tolerant")
	tolerant.Tolerations = []Taint{{Key: "hardware", Value: "gpu"}}
	if feasible, _ := Filter(tolerant, ns); len(feasible) != 1 {
		t.Fatalf("許容する Pod は置けるはず: %v", names(feasible))
	}
}

// Spread は空いているノードを選ぶので、Pod が均等に散る。
func TestSpreadDistributes(t *testing.T) {
	ns := nodes()
	s := New(Spread)
	for _, p := range []Pod{small("p1"), small("p2"), small("p3")} {
		if r := s.Schedule(p, ns); !r.Scheduled() {
			t.Fatalf("%s が置けなかった", p.Name)
		}
	}
	for _, n := range ns {
		if len(n.Pods()) != 1 {
			t.Fatalf("%s に %d 個。均等に 1 個ずつのはず", n.Name, len(n.Pods()))
		}
	}
}

// BinPack は使用率が高いノードを選ぶので、1 台に詰まっていく。
// 同じ filter を通っているので、置ける場所は Spread と変わらない。
func TestBinPackConcentrates(t *testing.T) {
	ns := nodes()
	s := New(BinPack)
	for _, p := range []Pod{small("p1"), small("p2"), small("p3")} {
		if r := s.Schedule(p, ns); !r.Scheduled() {
			t.Fatalf("%s が置けなかった", p.Name)
		}
	}
	if len(ns[0].Pods()) != 3 {
		t.Fatalf("node-a に 3 個詰まるはず: %v", ns[0].Pods())
	}
	if len(ns[1].Pods()) != 0 || len(ns[2].Pods()) != 0 {
		t.Fatalf("他のノードは空のはず: %v %v", ns[1].Pods(), ns[2].Pods())
	}
}

// どこにも置けなければ配置しない。Pod は Pending のまま残り、
// 落ちた理由が全ノードぶん残る。
func TestUnschedulableStaysPending(t *testing.T) {
	ns := []*Node{NewNode("tiny", Resources{CPU: 100, Mem: 128})}
	r := New(Spread).Schedule(small("huge"), ns)
	if r.Scheduled() {
		t.Fatalf("置けないはずが %s に置かれた", r.Node)
	}
	if len(r.Verdicts) != 1 || r.Verdicts[0].Why == "" {
		t.Fatalf("理由が残るはず: %+v", r.Verdicts)
	}
	if len(ns[0].Pods()) != 0 {
		t.Fatalf("置けなかったのにノードが変わった: %v", ns[0].Pods())
	}
}

// 配置すると要求のぶんだけ使用量が予約され、空きが減る。
// 実測でなく要求で確保するので、使っていなくても空きは戻らない。
func TestBindReservesRequest(t *testing.T) {
	n := NewNode("n", Resources{CPU: 1000, Mem: 1024})
	New(Spread).Schedule(small("p"), []*Node{n})
	if n.Used() != (Resources{CPU: 500, Mem: 512}) {
		t.Fatalf("要求のぶん予約されるはず: %+v", n.Used())
	}
	if n.Free() != (Resources{CPU: 500, Mem: 512}) {
		t.Fatalf("空きが減るはず: %+v", n.Free())
	}
}

// 容量ぴったりまで詰めたら、次の Pod は置けなくなる。
func TestFillsToCapacity(t *testing.T) {
	ns := []*Node{NewNode("n", Resources{CPU: 1000, Mem: 1024})}
	s := New(BinPack)
	rs := s.ScheduleAll([]Pod{small("p1"), small("p2"), small("p3")}, ns)
	if !rs[0].Scheduled() || !rs[1].Scheduled() {
		t.Fatalf("2 個までは入るはず: %+v", rs)
	}
	if rs[2].Scheduled() {
		t.Fatalf("3 個目は入らないはず: %+v", rs[2])
	}
}

// 同点のときはノード名の辞書順で決めるので、結果は毎回同じになる。
func TestDeterministicTieBreak(t *testing.T) {
	for i := 0; i < 20; i++ {
		ns := nodes()
		r := New(Spread).Schedule(small("p"), ns)
		if r.Node != "node-a" {
			t.Fatalf("同点は名前順で node-a のはずが %s", r.Node)
		}
	}
}

// 点数は「置いた後の使用率」で決まる。Spread と BinPack は
// 同じ状況に正反対の点をつける。
func TestScoreOpposites(t *testing.T) {
	n := NewNode("n", Resources{CPU: 1000, Mem: 1000})
	p := Pod{Name: "p", Req: Resources{CPU: 250, Mem: 250}}
	if got := Score(p, n, BinPack); got != 25 {
		t.Fatalf("BinPack は使用率そのもの(25)のはずが %d", got)
	}
	if got := Score(p, n, Spread); got != 75 {
		t.Fatalf("Spread は残りの空き(75)のはずが %d", got)
	}
}

// 容量 0 のノードは満杯とみなし、そもそも filter で落ちる。
func TestZeroCapacityNode(t *testing.T) {
	ns := []*Node{NewNode("empty", Resources{})}
	if r := New(Spread).Schedule(small("p"), ns); r.Scheduled() {
		t.Fatalf("容量 0 に置けてしまった")
	}
	if got := Score(small("p"), ns[0], BinPack); got != 100 {
		t.Fatalf("容量 0 は満杯(100)扱いのはずが %d", got)
	}
}

// 順に処理するので、先に置いた Pod が後の判断に効く。
func TestScheduleAllOrderMatters(t *testing.T) {
	ns := []*Node{NewNode("n", Resources{CPU: 1000, Mem: 1024})}
	big := Pod{Name: "big", Req: Resources{CPU: 900, Mem: 900}}
	rs := New(BinPack).ScheduleAll([]Pod{big, small("small")}, ns)
	if !rs[0].Scheduled() || rs[1].Scheduled() {
		t.Fatalf("先に大きいのが入ると小さいのが入らないはず: %+v", rs)
	}
}

func names(ns []*Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Name
	}
	return out
}
