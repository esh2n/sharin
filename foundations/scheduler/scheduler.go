package scheduler

// #region scheduler

// Kind はスケジュールのトレース1行の種類。
type Kind string

const (
	KindSpawn  Kind = "spawn"  // goroutine を生成し、どこかのキューに積んだ
	KindRun    Kind = "run"    // P が G を1量子(quantum)走らせた
	KindDone   Kind = "done"   // G が仕事を消化し切って終了した
	KindSteal  Kind = "steal"  // 暇な P が他の P から仕事を横取りした
	KindGlobal Kind = "global" // グローバルキューから仕事を引いた
	KindIdle   Kind = "idle"   // 走らせる G が無く、P が遊んだ
	KindSpill  Kind = "spill"  // ローカルキューが溢れ、半分をグローバルへ退避した
)

// Event はトレース1行。At はその出来事が起きた論理時刻(ラウンド開始時刻)。
type Event struct {
	At   int
	P    int    // 対象プロセッサ番号(スケジューラ全体の出来事は -1)
	Kind Kind
	G    string // 対象 goroutine(該当しなければ空)
	N    int    // run の tick / steal・global の本数
}

// Scheduler は複数の P を回す M:N スケジューラ。1 ラウンド = 全 P が1量子ずつ
// 同時に進む、というモデルで並行実行を決定的に表す。
type Scheduler struct {
	ps       []*P
	global   []*G // グローバル実行キュー(全 P が共有)
	quantum  int  // 1 回の実行で走らせる tick 数(協調的プリエンプションの粒度)
	localCap int  // ローカルキューの上限。超えると半分をグローバルへ退避
	clock    int
	nextGID  int
	all      []*G // 生成した全 G(検証・観察用)
	trace    []Event
}

// NewScheduler は numP 個のプロセッサを持つスケジューラを作る。quantum は
// 1 回の実行で走らせる tick 数(小さいほど頻繁に切り替わる)。
func NewScheduler(numP, quantum int) *Scheduler {
	if numP < 1 {
		numP = 1
	}
	if quantum < 1 {
		quantum = 1
	}
	ps := make([]*P, numP)
	for i := range ps {
		ps[i] = &P{ID: i}
	}
	return &Scheduler{ps: ps, quantum: quantum, localCap: 6, nextGID: 1}
}

// Go は goroutine を生成し、P0 のローカルキューに積む。1 つの goroutine から
// 生まれた仕事は同じ P に溜まりがち——これが偏りを生み、work-stealing が要る理由。
func (s *Scheduler) Go(name string, work int) *G { return s.GoOn(0, name, work) }

// GoOn は指定した P のローカルキューに goroutine を積む。ローカルが上限に達して
// いたら、半分をグローバルキューへ退避してから積む(Go の runqput のあふれ処理)。
func (s *Scheduler) GoOn(pid int, name string, work int) *G {
	if pid < 0 || pid >= len(s.ps) {
		pid = 0
	}
	if work < 1 {
		work = 1
	}
	g := &G{ID: s.nextGID, Name: name, work: work, st: Runnable}
	s.nextGID++
	s.all = append(s.all, g)

	p := s.ps[pid]
	if len(p.local) >= s.localCap {
		n := len(p.local) / 2
		s.global = append(s.global, p.local[:n]...)
		p.local = append([]*G(nil), p.local[n:]...)
		s.trace = append(s.trace, Event{At: s.clock, P: pid, Kind: KindSpill, N: n})
	}
	p.local = append(p.local, g)
	s.trace = append(s.trace, Event{At: s.clock, P: pid, Kind: KindSpawn, G: name})
	return g
}

// #endregion scheduler

// #region step

// Step は1ラウンド進める: 全 P が「必要なら仕事を確保 → 1 量子走らせる」を
// index 順に1回ずつ行う。走らせる仕事がどこにも無ければ false(完了)。
func (s *Scheduler) Step() bool {
	if s.drained() {
		return false
	}
	at := s.clock
	for _, p := range s.ps {
		// ローカルが空なら、まずグローバル、次に他 P から横取りして仕事を確保する。
		if len(p.local) == 0 {
			if !s.fromGlobal(p, at) && !s.steal(p, at) {
				s.trace = append(s.trace, Event{At: at, P: p.ID, Kind: KindIdle})
				continue
			}
		}
		// ローカル先頭の G を1量子走らせる。
		g := p.local[0]
		p.local = p.local[1:]
		g.st = Running
		n := s.quantum
		if g.work < n {
			n = g.work
		}
		g.work -= n
		g.executed += n
		p.ran += n
		if g.work == 0 {
			g.st = Done
			s.trace = append(s.trace, Event{At: at, P: p.ID, Kind: KindRun, G: g.Name, N: n})
			s.trace = append(s.trace, Event{At: at, P: p.ID, Kind: KindDone, G: g.Name})
		} else {
			// 量子を使い切ったがまだ残る → ローカルキューの末尾へ戻す(協調的な切り替え)。
			g.st = Runnable
			p.local = append(p.local, g)
			s.trace = append(s.trace, Event{At: at, P: p.ID, Kind: KindRun, G: g.Name, N: n})
		}
	}
	s.clock += s.quantum // ラウンドぶんの時間が(並行に)経過した
	return true
}

// fromGlobal はグローバルキューから仕事を1バッチ引く。全 P で均されるよう、
// おおよそ「全体 / P 数 + 1」本を取る(1 つの P が総取りしないように)。
func (s *Scheduler) fromGlobal(p *P, at int) bool {
	if len(s.global) == 0 {
		return false
	}
	n := len(s.global)/len(s.ps) + 1
	if n > len(s.global) {
		n = len(s.global)
	}
	batch := s.global[:n]
	s.global = s.global[n:]
	p.local = append(p.local, batch...)
	s.trace = append(s.trace, Event{At: at, P: p.ID, Kind: KindGlobal, N: n})
	return true
}

// steal は最も混んでいる他の P からローカルキューの半分を横取りする。
// 決定的にするため、同数なら index の小さい P を選ぶ。半分盗めない(2 本未満の)
// P は対象にしない——盗むコストに見合わないから。
func (s *Scheduler) steal(thief *P, at int) bool {
	var victim *P
	for _, p := range s.ps {
		if p == thief || len(p.local) < 2 {
			continue
		}
		if victim == nil || len(p.local) > len(victim.local) {
			victim = p
		}
	}
	if victim == nil {
		return false
	}
	n := len(victim.local) / 2
	batch := victim.local[:n]
	victim.local = append([]*G(nil), victim.local[n:]...)
	thief.local = append(thief.local, batch...)
	thief.steals++
	s.trace = append(s.trace, Event{At: at, P: thief.ID, Kind: KindSteal, G: victim.tag(), N: n})
	return true
}

// drained は全ローカルキュー・グローバルキューが空(= 走らせる仕事が無い)かを返す。
func (s *Scheduler) drained() bool {
	if len(s.global) > 0 {
		return false
	}
	for _, p := range s.ps {
		if len(p.local) > 0 {
			return false
		}
	}
	return true
}

// Run は全 goroutine が終わるまでラウンドを回し、トレースを返す。
func (s *Scheduler) Run() []Event {
	for s.Step() {
	}
	return s.trace
}

// #endregion step

// #region views

// Clock は現在の論理時刻を返す。
func (s *Scheduler) Clock() int { return s.clock }

// Trace はスケジュール記録を返す。
func (s *Scheduler) Trace() []Event { return s.trace }

// GlobalLen はグローバルキューの長さを返す。
func (s *Scheduler) GlobalLen() int { return len(s.global) }

// Ps はプロセッサ一覧を返す(観察用)。
func (s *Scheduler) Ps() []*P { return s.ps }

// QueueLens は各 P のローカルキュー長を返す。
func (s *Scheduler) QueueLens() []int {
	out := make([]int, len(s.ps))
	for i, p := range s.ps {
		out[i] = len(p.local)
	}
	return out
}

// Rans は各 P が実行した総 tick を返す(値が揃っていれば負荷分散が効いた証拠)。
func (s *Scheduler) Rans() []int {
	out := make([]int, len(s.ps))
	for i, p := range s.ps {
		out[i] = p.ran
	}
	return out
}

// Steals は各 P の横取り成功回数を返す。
func (s *Scheduler) Steals() []int {
	out := make([]int, len(s.ps))
	for i, p := range s.ps {
		out[i] = p.steals
	}
	return out
}

// DoneCount は終了した goroutine の数を返す。
func (s *Scheduler) DoneCount() int {
	n := 0
	for _, g := range s.all {
		if g.st == Done {
			n++
		}
	}
	return n
}

// #endregion views
