package probe

import "testing"

// rd は素直な readiness の設定(毎周期、1 回で判定)。
func rd() Probe { return Probe{Period: 1, FailureThreshold: 1, SuccessThreshold: 1} }

// lv は liveness の設定。failure が n 回続いたら再起動する。
func lv(n int) Probe { return Probe{Period: 1, FailureThreshold: n, SuccessThreshold: 1} }

func run(s *Sim, ticks, rps int) *Sim {
	for i := 0; i < ticks; i++ {
		s.Tick(rps)
	}
	return s
}

// 温まった Pod と、これから起動する Pod を並べる。スケールアウトや
// 更新のときに必ずできる状況で、readiness の有無が効く場面でもある。
func warmAndCold() []Behavior {
	return []Behavior{{StartupTicks: 0}, {StartupTicks: 3}}
}

// readiness を使わないと、起動が終わっていない Pod にも振られて落ちる。
// 隣に処理できる Pod がいるのに、順番で回ってきたぶんが失われる。
func TestNoReadinessDropsDuringStartup(t *testing.T) {
	s := New(Config{}, warmAndCold()...)
	run(s, 6, 2)
	if s.Dropped != 3 {
		t.Fatalf("起動中の 3 周期に 1 件ずつ落ちるはずが %d 件\n%v", s.Dropped, s.Log)
	}
}

// readiness を入れると、起動が終わるまで転送先に入らない。
// 同じ状況で 1 件も落ちず、処理できる Pod だけが受ける。
func TestReadinessGatesTraffic(t *testing.T) {
	s := New(Config{Readiness: rd()}, warmAndCold()...)
	run(s, 6, 2)
	if !s.Safe() {
		t.Fatalf("readiness があれば落ちないはずが %d 件\n%v", s.Dropped, s.Log)
	}
	if s.Served != 12 {
		t.Fatalf("全件を温まった側が処理するはずが %d 件", s.Served)
	}
	// 起動が終われば自分で転送先に加わる。
	if !s.Pods()[1].Ready() {
		t.Fatal("起動後に readiness を通っていない")
	}
}

// readiness の失敗は再起動を起こさない。外すだけで、Pod は生き続け、
// 詰まりが解ければ自分で戻る。
func TestReadinessDoesNotRestart(t *testing.T) {
	s := New(Config{Readiness: rd()}, Behavior{}, Behavior{HangAt: 3, HangFor: 3})
	run(s, 12, 2)
	if s.Restarts() != 0 {
		t.Fatalf("readiness では再起動しないはずが %d 回", s.Restarts())
	}
	if !s.Safe() {
		t.Fatalf("外れている間は振られないので落ちないはずが %d 件\n%v", s.Dropped, s.Log)
	}
	if !s.Pods()[1].Ready() {
		t.Fatal("回復後に readiness へ戻っていない")
	}
}

// liveness の失敗は再起動を起こす。readiness と同じ検査でも扱いが違う。
func TestLivenessRestarts(t *testing.T) {
	s := New(Config{Liveness: lv(1)}, Behavior{HangAt: 3, HangFor: 3})
	run(s, 12, 0)
	if s.Restarts() == 0 {
		t.Fatal("liveness が落ちたら再起動するはず")
	}
}

// liveness を厳しくしすぎると、一時的に詰まっただけの Pod を殺し続ける。
// 自分で戻れる状態なのに、戻る前に再起動されてしまう。
func TestAggressiveLivenessRestartLoop(t *testing.T) {
	b := Behavior{StartupTicks: 4, HangAt: 6, HangFor: 3}
	tight := run(New(Config{Readiness: rd(), Liveness: lv(1)}, b), 24, 0)
	loose := run(New(Config{Readiness: rd(), Liveness: lv(5)}, b), 24, 0)
	if tight.Restarts() <= loose.Restarts() {
		t.Fatalf("厳しいほど再起動が増えるはず: tight=%d loose=%d", tight.Restarts(), loose.Restarts())
	}
	if loose.Restarts() != 0 {
		t.Fatalf("緩ければ自分で戻るので再起動しないはずが %d 回", loose.Restarts())
	}
}

// 起動が liveness の猶予より遅いと、起動を終える前に殺されて無限に立ち上がらない。
func TestSlowStartupKilledByLiveness(t *testing.T) {
	s := New(Config{Liveness: Probe{InitialDelay: 2, Period: 1, FailureThreshold: 2, SuccessThreshold: 1}},
		Behavior{StartupTicks: 10})
	run(s, 30, 0)
	if s.Restarts() < 3 {
		t.Fatalf("起動が終わる前に殺され続けるはずが %d 回", s.Restarts())
	}
	if s.Pods()[0].Healthy() {
		t.Fatal("一度も起動を終えられないはず")
	}
}

// 起動用の検査を足すと、それが通るまで liveness は動かない。
// 同じ遅い起動でも、殺されずに立ち上がる。
func TestStartupProbeProtectsSlowStart(t *testing.T) {
	cfg := Config{
		Startup:   Probe{Period: 1, FailureThreshold: 30, SuccessThreshold: 1},
		Liveness:  Probe{Period: 1, FailureThreshold: 2, SuccessThreshold: 1},
		Readiness: rd(),
	}
	s := New(cfg, Behavior{}, Behavior{StartupTicks: 10})
	run(s, 30, 2)
	if s.Restarts() != 0 {
		t.Fatalf("起動検査があれば殺されないはずが %d 回\n%v", s.Restarts(), s.Log)
	}
	if !s.Pods()[1].Ready() {
		t.Fatal("起動後に readiness を通っていない")
	}
	if !s.Safe() {
		t.Fatalf("readiness があるので落ちないはずが %d 件", s.Dropped)
	}
}

// 1 回の失敗では判定を変えない。連続回数がたまたまの失敗を吸収する。
func TestThresholdAbsorbsSingleFailure(t *testing.T) {
	g := &gate{passing: true}
	p := Probe{Period: 1, FailureThreshold: 3, SuccessThreshold: 1}
	if g.record(false, p) || !g.passing {
		t.Fatal("1 回の失敗で落ちてはいけない")
	}
	if g.record(true, p); !g.passing {
		t.Fatal("間に成功が入れば数え直すはず")
	}
	g.record(false, p)
	g.record(false, p)
	if !g.record(false, p) || g.passing {
		t.Fatal("3 回続けば落ちるはず")
	}
}

// 検査は設定した間隔でしか走らない。間隔を空けるほど気づくのが遅れる。
func TestPeriodAndInitialDelay(t *testing.T) {
	p := Probe{InitialDelay: 3, Period: 2}
	for _, age := range []int{0, 1, 2, 4, 6} {
		if p.due(age) != (age >= 3 && (age-3)%2 == 0) {
			t.Fatalf("age=%d の判定が違う", age)
		}
	}
	if (Probe{}).enabled() {
		t.Fatal("Period 0 は使わない設定のはず")
	}
}

// 全 Pod が readiness から外れると転送先が空になり、来た分は全部落ちる。
func TestAllUnreadyDropsAll(t *testing.T) {
	s := New(Config{Readiness: rd()}, Behavior{StartupTicks: 5}, Behavior{StartupTicks: 5})
	run(s, 3, 2)
	if s.Dropped != 6 {
		t.Fatalf("両方まだ起動中なので 6 件落ちるはずが %d", s.Dropped)
	}
	if len(s.Endpoints()) != 0 {
		t.Fatalf("転送先は空のはず: %v", s.Endpoints())
	}
}

// 再起動すると経過も検査の状態も初期化される。起動からやり直しになる。
func TestRestartResetsState(t *testing.T) {
	s := New(Config{Readiness: rd(), Liveness: lv(1)}, Behavior{StartupTicks: 2, HangAt: 5, HangFor: 0})
	run(s, 10, 0)
	p := s.Pods()[0]
	if p.Restarts == 0 {
		t.Fatal("再起動していない")
	}
	if p.Age() >= 10 {
		t.Fatalf("再起動で経過が戻るはずが %d", p.Age())
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(7) != "7" || itoa(4096) != "4096" {
		t.Fatal("itoa が違う")
	}
}
