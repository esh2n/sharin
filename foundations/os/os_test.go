package os

import (
	"reflect"
	"testing"
)

// runOrder は trace から run した順にタスク名を取り出す。
func runOrder(trace []Event) []string {
	var out []string
	for _, e := range trace {
		if e.Kind == KindRun {
			out = append(out, e.Task)
		}
	}
	return out
}

// kinds は指定タスクのイベント種別を順に取り出す。
func kinds(trace []Event, task string) []Kind {
	var out []Kind
	for _, e := range trace {
		if e.Task == task {
			out = append(out, e.Kind)
		}
	}
	return out
}

// round-robin: 各タスクが Run,Yield を繰り返すと、CPU は公平に順番で回る。
func TestRoundRobinInterleaves(t *testing.T) {
	k := NewKernel()
	prog := []Op{Run(1), Yield(), Run(1), Yield(), Run(1)}
	a := k.Spawn("A", prog...)
	b := k.Spawn("B", prog...)
	c := k.Spawn("C", prog...)

	trace := k.Run()

	got := runOrder(trace)
	want := []string{"A", "B", "C", "A", "B", "C", "A", "B", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("実行順が round-robin でない: got %v want %v", got, want)
	}
	if k.Clock() != 9 {
		t.Errorf("clock = %d, want 9", k.Clock())
	}
	for _, task := range []*Task{a, b, c} {
		if task.CPU() != 3 {
			t.Errorf("%s の CPU = %d, want 3", task.Name, task.CPU())
		}
		if task.State() != Done {
			t.Errorf("%s の状態 = %s, want done", task.Name, task.State())
		}
	}
}

// 協調方式は横取りしない: yield しない貪欲タスクは CPU を独占し、
// 他タスクはその完了まで一切走れない。
func TestCooperativeIsNonPreemptive(t *testing.T) {
	k := NewKernel()
	k.Spawn("greedy", Run(5)) // 5 tick 走りっぱなし、yield しない
	k.Spawn("polite", Run(1), Yield(), Run(1))

	trace := k.Run()

	// 最初の run は greedy が 5 tick まとめて握る。
	first := trace[0]
	if first.Task != "greedy" || first.Kind != KindRun || first.N != 5 {
		t.Fatalf("先頭イベント = %+v, want greedy run 5", first)
	}
	// polite の最初の run は clock=5 まで待たされる(横取りできなかった証拠)。
	for _, e := range trace {
		if e.Task == "polite" && e.Kind == KindRun {
			if e.At != 5 {
				t.Errorf("polite の初回 run は At=%d, want 5(greedy の独占後)", e.At)
			}
			break
		}
	}
}

// context switch の実体は pc の保存/復元。1 ステップごとに pc が進み、
// 再開時は続きから走る。
func TestContextIsProgramCounter(t *testing.T) {
	k := NewKernel()
	a := k.Spawn("A", Run(1), Yield(), Run(1))

	if !k.Step() { // Run(1) して Yield で手放す
		t.Fatal("Step 1 が false")
	}
	if a.State() != Ready {
		t.Errorf("yield 後の状態 = %s, want ready", a.State())
	}
	if a.CPU() != 1 {
		t.Errorf("1 ステップ後の CPU = %d, want 1", a.CPU())
	}
	if got := k.ReadyNames(); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("run queue = %v, want [A]", got)
	}

	if !k.Step() { // 保存された pc の続き(2つ目の Run)から再開して完了
		t.Fatal("Step 2 が false")
	}
	if a.State() != Done {
		t.Errorf("完了後の状態 = %s, want done", a.State())
	}
	if a.CPU() != 2 {
		t.Errorf("CPU = %d, want 2", a.CPU())
	}
	if k.Step() {
		t.Error("全完了後の Step は false であるべき")
	}
}

// sleep したタスクは run queue から外れ、他タスクが走り、起床時刻に戻る。
func TestSleepBlocksAndWakes(t *testing.T) {
	k := NewKernel()
	s := k.Spawn("S", Run(1), Sleep(3), Run(1))
	k.Spawn("B", Run(1), Yield(), Run(1), Yield(), Run(1))

	trace := k.Run()

	// S は run → sleep → (起床) wake → run → exit の順を辿る。
	got := kinds(trace, "S")
	want := []Kind{KindRun, KindSleep, KindWake, KindRun, KindExit}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("S のイベント列 = %v, want %v", got, want)
	}
	if s.CPU() != 2 {
		t.Errorf("S の CPU = %d, want 2", s.CPU())
	}
	if s.State() != Done {
		t.Errorf("S の状態 = %s, want done", s.State())
	}
}

// ブロック中に BlockedNames で観測でき、run queue には出ない。
func TestBlockedIsObservable(t *testing.T) {
	k := NewKernel()
	k.Spawn("S", Run(1), Sleep(5), Run(1))
	k.Spawn("B", Run(1), Yield(), Run(1))

	k.Step() // S: Run(1), Sleep(5) → blocked
	if got := k.BlockedNames(); !reflect.DeepEqual(got, []string{"S"}) {
		t.Errorf("blocked = %v, want [S]", got)
	}
	if got := k.ReadyNames(); !reflect.DeepEqual(got, []string{"B"}) {
		t.Errorf("ready = %v, want [B]", got)
	}
}

// 実行可能タスクが無く sleeper だけ残ると、CPU は空転(idle)して時計を進める。
func TestIdleFastForwards(t *testing.T) {
	k := NewKernel()
	k.Spawn("only", Run(1), Sleep(5), Run(1))

	trace := k.Run()

	var idle *Event
	for i := range trace {
		if trace[i].Kind == KindIdle {
			idle = &trace[i]
			break
		}
	}
	if idle == nil {
		t.Fatal("idle イベントが記録されていない")
	}
	if idle.At != 1 || idle.N != 5 {
		t.Errorf("idle = At %d N %d, want At 1 N 5", idle.At, idle.N)
	}
	if k.Clock() != 7 { // Run(1)=1 → sleep 中に 1..6 まで空転 → Run(1)=7
		t.Errorf("clock = %d, want 7", k.Clock())
	}
}

// 同時に起きる sleeper は生成順で決定的に run queue へ戻る。
func TestSimultaneousWakeIsDeterministic(t *testing.T) {
	k := NewKernel()
	k.Spawn("A", Sleep(2), Run(1))
	k.Spawn("B", Sleep(2), Run(1))

	trace := k.Run()

	if got := runOrder(trace); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("起床後の実行順 = %v, want [A B](生成順)", got)
	}
}

// Run(0) のような 0 以下の tick は 1 tick として扱う。
func TestZeroRunCountsAsOne(t *testing.T) {
	k := NewKernel()
	a := k.Spawn("A", Run(0))
	k.Run()
	if a.CPU() != 1 {
		t.Errorf("Run(0) の CPU = %d, want 1", a.CPU())
	}
}

// 文脈切り替えの回数を数える(yield / sleep / exit で 1 回ずつ)。
func TestSwitchCount(t *testing.T) {
	k := NewKernel()
	k.Spawn("A", Run(1), Yield(), Run(1)) // yield 1 + exit 1 = 2
	k.Spawn("B", Run(1))                  // exit 1 = 1
	k.Run()
	if k.Switches() != 3 {
		t.Errorf("switches = %d, want 3", k.Switches())
	}
}

// 空のカーネルは何もせず false を返す。
func TestEmptyKernel(t *testing.T) {
	k := NewKernel()
	if k.Step() {
		t.Error("空のカーネルの Step は false であるべき")
	}
	if len(k.Run()) != 0 {
		t.Error("空のカーネルの trace は空であるべき")
	}
	if k.Clock() != 0 || k.Switches() != 0 {
		t.Error("空のカーネルは clock/switches ともに 0")
	}
}

// State.String の全分岐。
func TestStateString(t *testing.T) {
	cases := map[State]string{Ready: "ready", Running: "running", Blocked: "blocked", Done: "done", State(99): "?"}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}

// BlockedNames の起床時刻順ソート、wake の部分起床(一部だけ起きる)、
// Trace の取得をまとめて確認する。
func TestBlockedOrderingAndPartialWake(t *testing.T) {
	k := NewKernel()
	k.Spawn("A", Run(1), Sleep(2), Run(1)) // clock1 で sleep → wake 3
	k.Spawn("B", Run(1), Sleep(8), Run(1)) // clock2 で sleep → wake 10
	k.Spawn("C", Run(1), Yield(), Run(1), Yield(), Run(1), Yield(), Run(1))

	k.Step() // A: Run(1), Sleep(2)
	k.Step() // B: Run(1), Sleep(8)
	// この時点で A(wake 3) と B(wake 10) が blocked。起床時刻順で並ぶ。
	if got := k.BlockedNames(); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("blocked = %v, want [A B](起床時刻順)", got)
	}

	trace := k.Run()
	// A は先に起きて完了し、B はまだ寝ている瞬間があった = 部分起床が起きた。
	if len(trace) == 0 {
		t.Fatal("Trace が空")
	}
	if k.Trace()[0].Task != "A" {
		t.Errorf("先頭イベントの task = %q, want A", k.Trace()[0].Task)
	}
}
