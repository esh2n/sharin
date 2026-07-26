package os

import "sort"

// #region kernel

// Kind はスケジュール記録(トレース)1 行の種類。
type Kind string

const (
	KindRun   Kind = "run"   // タスクが CPU を使った
	KindYield Kind = "yield" // 自発的に CPU を手放した
	KindSleep Kind = "sleep" // ブロックに入った
	KindWake  Kind = "wake"  // 起床時刻が来て run queue に戻った
	KindExit  Kind = "exit"  // プログラムを実行し終えた
	KindIdle  Kind = "idle"  // 実行可能タスクが無く、CPU が空転した
)

// Event はスケジューラのトレース 1 行。At はその出来事が起きた時刻。
// N は run の tick 数 / sleep の長さ / idle の空転量に使う。
type Event struct {
	At   int
	Task string
	Kind Kind
	N    int
}

// Kernel は最小カーネル。誰が CPU を握るかを決めるスケジューラと、論理時計を持つ。
// プリエンプション(強制的な横取り)は無く、タスクが自発的に yield / sleep した
// ときにだけ制御が戻る = 協調スケジューリング。
type Kernel struct {
	clock    int
	ready    []*Task // run queue。先頭から取り出し末尾へ戻す = round-robin
	blocked  []*Task // sleep 中のタスク
	running  *Task
	switches int
	nextSeq  int
	trace    []Event
}

// NewKernel は空のカーネルを作る。
func NewKernel() *Kernel { return &Kernel{} }

// Spawn は新しいタスクを生成し、run queue の末尾に並べる(Ready)。
// プログラムは Run / Yield / Sleep の並びで与える。
func (k *Kernel) Spawn(name string, prog ...Op) *Task {
	t := &Task{Name: name, prog: prog, st: Ready, seq: k.nextSeq}
	k.nextSeq++
	k.ready = append(k.ready, t)
	return t
}

// #endregion kernel

// #region step

// wake は起床時刻を迎えた Blocked タスクを run queue に戻す。
// 同時に起きるタスクは、起床時刻 → 生成順で決定的に並べる。
func (k *Kernel) wake() {
	if len(k.blocked) == 0 {
		return
	}
	var woken, still []*Task
	for _, t := range k.blocked {
		if t.wake <= k.clock {
			woken = append(woken, t)
		} else {
			still = append(still, t)
		}
	}
	if len(woken) == 0 {
		return
	}
	sort.Slice(woken, func(i, j int) bool {
		if woken[i].wake != woken[j].wake {
			return woken[i].wake < woken[j].wake
		}
		return woken[i].seq < woken[j].seq
	})
	for _, t := range woken {
		t.st = Ready
		k.ready = append(k.ready, t)
		k.trace = append(k.trace, Event{At: k.clock, Task: t.Name, Kind: KindWake})
	}
	k.blocked = still
}

// Step は 1 回分のスケジュールを進める。run queue の先頭タスクを取り出し、
// yield / sleep / 完了 のいずれかで CPU を手放すまで走らせる(協調)。
// 進める余地が無ければ false を返す。
func (k *Kernel) Step() bool {
	k.wake()
	if len(k.ready) == 0 {
		// 実行可能タスクが無い。sleep 中のタスクがいれば、CPU は次の
		// 起床時刻まで空転(idle)して時計を進める——実機の HLT に当たる。
		if len(k.blocked) == 0 {
			return false // 全タスク完了
		}
		next := k.blocked[0].wake
		for _, t := range k.blocked {
			if t.wake < next {
				next = t.wake
			}
		}
		k.trace = append(k.trace, Event{At: k.clock, Task: "(idle)", Kind: KindIdle, N: next - k.clock})
		k.clock = next
		k.wake()
	}

	// run queue の先頭を dispatch(round-robin)。ここで文脈(pc)を復元する。
	t := k.ready[0]
	k.ready = k.ready[1:]
	t.st = Running
	k.running = t

	// yield / sleep / 完了 まで、保存された pc の続きから走らせる。
	for {
		if t.pc >= len(t.prog) {
			t.st = Done
			k.trace = append(k.trace, Event{At: k.clock, Task: t.Name, Kind: KindExit})
			k.switches++
			break
		}
		op := t.prog[t.pc]
		t.pc++ // 文脈を1つ進める。この pc が次回の復元点になる

		if op.Kind == OpRun {
			n := op.Arg
			if n < 1 {
				n = 1
			}
			k.trace = append(k.trace, Event{At: k.clock, Task: t.Name, Kind: KindRun, N: n})
			k.clock += n
			t.cpu += n
			continue // 協調: yield するまで CPU を握り続ける(横取りされない)
		}
		if op.Kind == OpYield {
			t.st = Ready
			k.ready = append(k.ready, t) // 末尾へ戻る = round-robin
			k.trace = append(k.trace, Event{At: k.clock, Task: t.Name, Kind: KindYield})
			k.switches++
			break
		}
		// OpSleep: 起床時刻を決めて blocked へ退避する。
		t.st = Blocked
		t.wake = k.clock + op.Arg
		k.blocked = append(k.blocked, t)
		k.trace = append(k.trace, Event{At: k.clock, Task: t.Name, Kind: KindSleep, N: op.Arg})
		k.switches++
		break
	}
	k.running = nil
	return true
}

// Run は全タスクが完了するまで Step を回し、スケジュール記録を返す。
func (k *Kernel) Run() []Event {
	for k.Step() {
	}
	return k.trace
}

// #endregion step

// #region views

// Clock は現在の論理時刻(消費した CPU tick + 空転 tick)を返す。
func (k *Kernel) Clock() int { return k.clock }

// Switches はタスクが CPU を手放した回数(文脈切り替えの回数)を返す。
func (k *Kernel) Switches() int { return k.switches }

// Trace はこれまでのスケジュール記録を返す。
func (k *Kernel) Trace() []Event { return k.trace }

// ReadyNames は run queue に並ぶタスク名を、先頭(次に走る)から順に返す。
func (k *Kernel) ReadyNames() []string {
	out := make([]string, len(k.ready))
	for i, t := range k.ready {
		out[i] = t.Name
	}
	return out
}

// BlockedNames は sleep 中のタスク名を起床時刻順に返す。
func (k *Kernel) BlockedNames() []string {
	ts := append([]*Task(nil), k.blocked...)
	sort.Slice(ts, func(i, j int) bool {
		if ts[i].wake != ts[j].wake {
			return ts[i].wake < ts[j].wake
		}
		return ts[i].seq < ts[j].seq
	})
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

// #endregion views
