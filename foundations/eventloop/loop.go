package eventloop

import (
	"sort"
	"strconv"
	"strings"
)

// #region loop

// Phase はトレース 1 行の種類。イベントループの 1 周がどの段階かを表す。
type Phase string

const (
	PhasePoll     Phase = "poll"     // epoll_wait が ready 集合を返した
	PhaseDispatch Phase = "dispatch" // ある FD のハンドラを呼んだ
	PhaseIdle     Phase = "idle"     // ready が無く、次の到着まで眠った(epoll_wait のブロック)
)

// Step はトレース 1 行。At はその周が起きた論理時刻。
type Step struct {
	At    int
	Phase Phase
	FD    int // dispatch のとき対象 FD 番号。それ以外は -1
	Note  string
}

// worldEvent は「外の世界」で起きること(ネットワーク到着 / 送信バッファ解放)。
// 実機では非同期に起きるが、ここでは tick で予定して決定化する。
type worldEvent struct {
	at  int
	seq int // 同時刻の適用順を決定的にする
	fn  func()
}

// Loop は 1 本のスレッドで多数の FD をさばくイベントループ(リアクタ)。
// poller に「どれか準備できたか」を尋ね、準備できた FD のハンドラだけを呼ぶ。
// これを繰り返すのがループの全て——決してどれか 1 本の接続で立ち止まらない。
type Loop struct {
	poller   *Poller
	fds      map[int]*FD
	handlers map[int]func(*FD, Interest)
	nextID   int
	clock    int
	world    []worldEvent
	worldSeq int
	trace    []Step
}

// NewLoop は空のイベントループを作る。
func NewLoop() *Loop {
	return &Loop{
		poller:   NewPoller(),
		fds:      map[int]*FD{},
		handlers: map[int]func(*FD, Interest){},
		nextID:   1,
	}
}

// Open は新しい接続(FD)を作る。sendBuf は送信バッファの初期容量
// (相手が受け取る前に書ける最大バイト数)。
func (l *Loop) Open(name string, sendBuf int) *FD {
	f := &FD{id: l.nextID, name: name, out: sendBuf}
	l.nextID++
	l.fds[f.id] = f
	return f
}

// Register は FD をイベントループに登録する。in で待つイベントを、handler で
// ready 時の処理を渡す。handler はノンブロッキングに Read/Write し、決して
// ブロックしてはならない(ブロックした瞬間、他の全接続が巻き添えで止まる)。
func (l *Loop) Register(f *FD, in Interest, handler func(*FD, Interest)) {
	l.poller.Add(f, in)
	l.handlers[f.id] = handler
}

// Watch は登録済み FD の関心を差し替える(送りたいデータができたら Writable を
// 足す/送り終えたら外す)。
func (l *Loop) Watch(f *FD, in Interest) { l.poller.Modify(f.id, in) }

// CloseFD は接続を閉じ、監視対象から外す。
func (l *Loop) CloseFD(f *FD) {
	f.closed = true
	l.poller.Remove(f.id)
	delete(l.handlers, f.id)
}

// Deliver は「tick 時にこの FD へ data が届く」と予定する(ネットワーク到着)。
func (l *Loop) Deliver(at int, f *FD, data []byte) {
	b := append([]byte(nil), data...)
	l.schedule(at, func() { f.deliver(b) })
}

// FreeWrite は「tick 時にこの FD の送信バッファが n だけ空く」と予定する
// (相手がそこまで受信し、こちらが続きを書けるようになる)。
func (l *Loop) FreeWrite(at int, f *FD, n int) {
	l.schedule(at, func() { f.drain(n) })
}

func (l *Loop) schedule(at int, fn func()) {
	l.world = append(l.world, worldEvent{at: at, seq: l.worldSeq, fn: fn})
	l.worldSeq++
}

// applyWorld は clock 時点までに予定された外界イベントを、時刻 → 登録順で
// 適用する(決定的)。
func (l *Loop) applyWorld() {
	var due, rest []worldEvent
	for _, e := range l.world {
		if e.at <= l.clock {
			due = append(due, e)
		} else {
			rest = append(rest, e)
		}
	}
	sort.SliceStable(due, func(i, j int) bool {
		if due[i].at != due[j].at {
			return due[i].at < due[j].at
		}
		return due[i].seq < due[j].seq
	})
	for _, e := range due {
		e.fn()
	}
	l.world = rest
}

// nextWorldAt は未適用の外界イベントのうち最も早い tick を返す。無ければ -1。
func (l *Loop) nextWorldAt() int {
	next := -1
	for _, e := range l.world {
		if next == -1 || e.at < next {
			next = e.at
		}
	}
	return next
}

// Tick はイベントループの 1 周: 外界を反映 → poll → ready を順に dispatch。
// ready が 1 件も無ければ、次の到着まで clock を進めて「眠る」(epoll_wait が
// ブロックして CPU を手放すのに当たる)。もう予定が無ければ false(ループ終了)。
func (l *Loop) Tick() bool {
	l.applyWorld()
	ready := l.poller.Wait()

	if len(ready) == 0 {
		// 準備できた FD が無い。次の到着まで眠る——ここで初めてスレッドは
		// 止まるが、止まるのは「全接続まとめて待つ epoll_wait の中」であって、
		// どれか 1 本の read の中ではない。os 編の idle 空転と同じ発想。
		next := l.nextWorldAt()
		if next == -1 {
			return false // 予定なし → 何も起こらない。ループ終了
		}
		l.trace = append(l.trace, Step{At: l.clock, Phase: PhaseIdle, FD: -1,
			Note: "wait until t=" + strconv.Itoa(next)})
		l.clock = next
		return true
	}

	// epoll_wait が返す ready 集合。1 回の問い合わせで「今さばける FD 全部」。
	l.trace = append(l.trace, Step{At: l.clock, Phase: PhasePoll, FD: -1,
		Note: readyNote(ready)})

	// 準備できた FD だけを順に処理する。ハンドラはノンブロッキングなので、
	// この for が終われば 1 周分の仕事が終わる——1 スレッドで多重化できる核心。
	for _, r := range ready {
		f := l.fds[r.FD]
		h := l.handlers[r.FD]
		if h == nil || f.closed {
			continue
		}
		l.trace = append(l.trace, Step{At: l.clock, Phase: PhaseDispatch, FD: r.FD,
			Note: r.Events.String()})
		h(f, r.Events)
	}
	l.clock++
	return true
}

// Run はループを回し続け、これ以上何も起きなくなったら止めてトレースを返す。
func (l *Loop) Run() []Step {
	for l.Tick() {
	}
	return l.trace
}

// Clock は現在の論理時刻を返す。
func (l *Loop) Clock() int { return l.clock }

// Trace はこれまでのトレースを返す。
func (l *Loop) Trace() []Step { return l.trace }

// Poller は内部の Poller を返す(観察用)。
func (l *Loop) Poller() *Poller { return l.poller }

// readyNote は ready 集合を "ready {fd1:r, fd2:rw}" の形の文字列にする。
func readyNote(rs []Ready) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = "fd" + strconv.Itoa(r.FD) + ":" + r.Events.String()
	}
	return "ready {" + strings.Join(parts, ", ") + "}"
}

// #endregion loop
