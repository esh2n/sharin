package scheduler

import "testing"

// sum は int スライスの合計。
func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func maxMin(xs []int) (int, int) {
	mx, mn := xs[0], xs[0]
	for _, x := range xs {
		if x > mx {
			mx = x
		}
		if x < mn {
			mn = x
		}
	}
	return mx, mn
}

func TestAllGoroutinesComplete(t *testing.T) {
	s := NewScheduler(3, 2)
	works := []int{6, 4, 8, 2, 6, 4}
	var gs []*G
	for i, w := range works {
		gs = append(gs, s.Go(string(rune('A'+i)), w))
	}
	s.Run()

	if s.DoneCount() != len(works) {
		t.Fatalf("done: want %d got %d", len(works), s.DoneCount())
	}
	for _, g := range gs {
		if g.State() != Done {
			t.Fatalf("%s not done: %s", g.Name, g.State())
		}
		if g.Remaining() != 0 {
			t.Fatalf("%s remaining %d", g.Name, g.Remaining())
		}
	}
	// 仕事量は保存される: 全 P の実行 tick 合計 = 全 goroutine の work 合計。
	if got, want := sum(s.Rans()), sum(works); got != want {
		t.Fatalf("work conservation: ran %d, want %d", got, want)
	}
	if !s.drained() {
		t.Fatalf("queues should be drained after Run")
	}
}

func TestWorkStealingBalancesLoad(t *testing.T) {
	// 全部 P0 に積む(偏り)。spill しないよう localCap 内(6 本)に収め、
	// work-stealing だけで P1/P2 に仕事が渡ることを見る。無ければ P0 だけが働く。
	s := NewScheduler(3, 1)
	total := 0
	for i := 0; i < 6; i++ {
		w := 8
		s.Go(string(rune('A'+i)), w)
		total += w
	}
	s.Run()

	if s.DoneCount() != 6 {
		t.Fatalf("done: want 6 got %d", s.DoneCount())
	}
	// 3 つの P の実行量がそこそこ均されていること(偏りが緩和された)。
	mx, mn := maxMin(s.Rans())
	if mn == 0 {
		t.Fatalf("some P did no work — stealing failed: %v", s.Rans())
	}
	if mx-mn > total/2 {
		t.Fatalf("load not balanced: rans=%v (max-min=%d)", s.Rans(), mx-mn)
	}
	// 少なくとも1回は横取りが起きたはず(P0 に全部積んだので P1/P2 は盗む)。
	if sum(s.Steals()) == 0 {
		t.Fatalf("expected steals, got none")
	}
}

func TestGlobalQueueSpillAndDrain(t *testing.T) {
	// localCap=4 を超えて積むと、半分がグローバルへ退避する(spill)。
	s := NewScheduler(2, 2)
	for i := 0; i < 10; i++ {
		s.Go(string(rune('A'+i)), 2)
	}
	// この時点で P0 のローカルは上限付近、超過分はグローバルにいるはず。
	if s.GlobalLen() == 0 {
		t.Fatalf("expected spill to global queue, global empty")
	}
	sawSpill, sawGlobal := false, false
	s.Run()
	for _, e := range s.Trace() {
		if e.Kind == KindSpill {
			sawSpill = true
		}
		if e.Kind == KindGlobal {
			sawGlobal = true
		}
	}
	if !sawSpill {
		t.Fatalf("expected a spill event")
	}
	if !sawGlobal {
		t.Fatalf("expected a global-fetch event")
	}
	if s.DoneCount() != 10 || !s.drained() {
		t.Fatalf("not all drained: done=%d global=%d lens=%v", s.DoneCount(), s.GlobalLen(), s.QueueLens())
	}
}

func TestQuantumPreemptsLongGoroutine(t *testing.T) {
	// quantum=2 の P1 台。work=5 の G は 2,2,1 と3回に分けて走る。
	s := NewScheduler(1, 2)
	g := s.Go("long", 5)
	// 他にも1本入れて、量子境界で切り替わる(round-robin)ことを見る。
	h := s.Go("short", 2)
	s.Run()

	if g.State() != Done || h.State() != Done {
		t.Fatalf("both should finish: long=%s short=%s", g.State(), h.State())
	}
	// long への run イベントは複数回(量子で分割された)。
	runs := 0
	for _, e := range s.Trace() {
		if e.Kind == KindRun && e.G == "long" {
			runs++
		}
	}
	if runs < 2 {
		t.Fatalf("long goroutine should be preempted into multiple runs, got %d", runs)
	}
}

func TestDeterministic(t *testing.T) {
	// 同じ入力なら同じトレース(乱数を使っていない)。
	build := func() *Scheduler {
		s := NewScheduler(3, 2)
		for i := 0; i < 9; i++ {
			s.Go(string(rune('A'+i)), (i%3)+2)
		}
		return s
	}
	a := build()
	b := build()
	a.Run()
	b.Run()
	ta, tb := a.Trace(), b.Trace()
	if len(ta) != len(tb) {
		t.Fatalf("trace length differs: %d vs %d", len(ta), len(tb))
	}
	for i := range ta {
		if ta[i] != tb[i] {
			t.Fatalf("trace differs at %d: %+v vs %+v", i, ta[i], tb[i])
		}
	}
}

func TestGoOnDistributesAndSteals(t *testing.T) {
	// P1 に固めて積んで(spill しない 6 本)、P0/P2 が横取りすることを確認。
	s := NewScheduler(3, 1)
	for i := 0; i < 6; i++ {
		s.GoOn(1, string(rune('A'+i)), 3)
	}
	s.Run()
	if s.DoneCount() != 6 {
		t.Fatalf("done: want 6 got %d", s.DoneCount())
	}
	// P1 以外の少なくとも1つが盗んでいる。
	stole := s.Steals()
	if stole[0]+stole[2] == 0 {
		t.Fatalf("P0/P2 should have stolen from P1: %v", stole)
	}
}

func TestSingleProcessorNoSteal(t *testing.T) {
	// P が1台なら横取りは起きない(単純な逐次実行)。os 編の単一キューに相当。
	s := NewScheduler(1, 3)
	s.Go("x", 4)
	s.Go("y", 3)
	s.Run()
	if sum(s.Steals()) != 0 {
		t.Fatalf("single P should never steal: %v", s.Steals())
	}
	if s.DoneCount() != 2 {
		t.Fatalf("both should finish")
	}
}

func TestEmptySchedulerAndIdle(t *testing.T) {
	s := NewScheduler(2, 2)
	if s.Step() {
		t.Fatalf("empty scheduler Step: want false")
	}
	if tr := s.Run(); tr != nil {
		t.Fatalf("empty run trace: want nil, got %v", tr)
	}
	// 片方の P にだけ1本 → もう片方は idle になるラウンドがある。
	s.Go("solo", 1)
	s.Run()
	sawIdle := false
	for _, e := range s.Trace() {
		if e.Kind == KindIdle {
			sawIdle = true
		}
	}
	if !sawIdle {
		t.Fatalf("expected an idle P when only one goroutine exists")
	}
}

func TestClampsAndViews(t *testing.T) {
	// numP<1, quantum<1 はクランプされる。
	s := NewScheduler(0, 0)
	if len(s.Ps()) != 1 || s.quantum != 1 {
		t.Fatalf("clamp: nP=%d quantum=%d", len(s.Ps()), s.quantum)
	}
	// work<1 もクランプ。GoOn の範囲外 pid は P0 扱い。
	g := s.GoOn(99, "z", 0)
	if g.Remaining() != 1 {
		t.Fatalf("work clamp: want 1 got %d", g.Remaining())
	}
	if s.QueueLens()[0] != 1 || s.Ps()[0].QueueNames()[0] != "z" {
		t.Fatalf("GoOn out-of-range pid should land on P0")
	}
	s.Run()
	if g.Executed() != 1 || s.Clock() < 1 {
		t.Fatalf("executed=%d clock=%d", g.Executed(), s.Clock())
	}
}

func TestSingleProcessorSpillPullsFromGlobal(t *testing.T) {
	// P1台でも localCap を超えれば spill し、その P 自身が global から引き戻す。
	// fromGlobal のバッチ数クランプ(global が少ないとき)も通る。
	s := NewScheduler(1, 2)
	for i := 0; i < 9; i++ {
		s.GoOn(0, string(rune('A'+i)), 2)
	}
	if s.GlobalLen() == 0 {
		t.Fatalf("expected spill to global with 1 P")
	}
	s.Run()
	if s.DoneCount() != 9 || !s.drained() {
		t.Fatalf("not all done/drained: done=%d global=%d", s.DoneCount(), s.GlobalLen())
	}
	// P 個別アクセサ(デモが使う)を通す。
	p := s.Ps()[0]
	if p.Ran() != 18 || p.QueueLen() != 0 || p.Steals() != 0 {
		t.Fatalf("P accessors: ran=%d qlen=%d steals=%d", p.Ran(), p.QueueLen(), p.Steals())
	}
}

func TestStateStringAndAccessors(t *testing.T) {
	if Runnable.String() != "runnable" || Running.String() != "running" || Done.String() != "done" || State(9).String() != "?" {
		t.Fatalf("State.String mismatch")
	}
	p := &P{ID: 2}
	if p.tag() != "P2" {
		t.Fatalf("tag: %s", p.tag())
	}
}
