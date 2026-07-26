package eventloop

import (
	"errors"
	"testing"
)

// echoHandler は受信を全部読み、そのまま書き返す典型的なエコーサーバのハンドラ。
// 送信バッファは十分ある前提(バックプレッシャは別テストで扱う)。
func echoHandler(f *FD, ev Interest) {
	if !ev.Has(Readable) {
		return
	}
	b, err := f.Read(1 << 20)
	if err != nil {
		return
	}
	_, _ = f.Write(b)
}

func TestNonBlockingReadReturnsWouldBlock(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 64)

	// 何も届いていない → read はスレッドを止めず即 ErrWouldBlock。
	if _, err := f.Read(10); !errors.Is(err, ErrWouldBlock) {
		t.Fatalf("empty read: want ErrWouldBlock, got %v", err)
	}

	f.deliver([]byte("hello"))
	b, err := f.Read(3)
	if err != nil {
		t.Fatalf("read after deliver: %v", err)
	}
	if string(b) != "hel" {
		t.Fatalf("partial read: want %q got %q", "hel", b)
	}
	b, err = f.Read(1 << 20)
	if err != nil || string(b) != "lo" {
		t.Fatalf("rest read: want %q got %q err=%v", "lo", b, err)
	}
	if _, err := f.Read(1); !errors.Is(err, ErrWouldBlock) {
		t.Fatalf("drained read: want ErrWouldBlock, got %v", err)
	}
}

func TestNonBlockingWriteRespectsSendBuffer(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 4) // 送信バッファは 4 バイトだけ

	n, err := f.Write([]byte("hello")) // 5 バイト書こうとする
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 4 {
		t.Fatalf("partial write: want 4 got %d", n)
	}
	// 満杯 → もう書けない。ErrWouldBlock。
	if _, err := f.Write([]byte("o")); !errors.Is(err, ErrWouldBlock) {
		t.Fatalf("full write: want ErrWouldBlock, got %v", err)
	}
	// 相手が受信して空きが戻れば、また書ける。
	f.drain(4)
	if n, err := f.Write([]byte("o")); err != nil || n != 1 {
		t.Fatalf("write after drain: n=%d err=%v", n, err)
	}
	if string(f.Sent()) != "hello" {
		t.Fatalf("sent: want %q got %q", "hello", f.Sent())
	}
}

func TestClosedFDRejectsIO(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 64)
	l.CloseFD(f)
	if _, err := f.Read(1); err == nil || errors.Is(err, ErrWouldBlock) {
		t.Fatalf("read on closed: want plain error, got %v", err)
	}
	if _, err := f.Write([]byte("x")); err == nil || errors.Is(err, ErrWouldBlock) {
		t.Fatalf("write on closed: want plain error, got %v", err)
	}
}

func TestPollerReportsOnlyReadyAndInterested(t *testing.T) {
	l := NewLoop()
	a := l.Open("a", 8)
	b := l.Open("b", 8)
	c := l.Open("c", 8)
	p := l.Poller()
	p.Add(a, Readable)
	p.Add(b, Readable)
	p.Add(c, Writable) // c は書き込み可だけ待つ

	a.deliver([]byte("x")) // a は読める
	// b は何も届いていない。c は out>0 なので Writable が発火する。

	ready := p.Wait()
	if len(ready) != 2 {
		t.Fatalf("ready count: want 2 got %d (%v)", len(ready), ready)
	}
	// 登録順(a, c)で返る。
	if ready[0].FD != a.ID() || ready[0].Events != Readable {
		t.Fatalf("ready[0]: got %+v", ready[0])
	}
	if ready[1].FD != c.ID() || ready[1].Events != Writable {
		t.Fatalf("ready[1]: got %+v", ready[1])
	}

	// a に届いたデータへの関心を Writable だけにすると、読めても報告されない
	// (interest でマスクされる)。
	p.Modify(a.ID(), Writable)
	ready = p.Wait()
	for _, r := range ready {
		if r.FD == a.ID() && r.Events.Has(Readable) {
			t.Fatalf("masked interest still reported Readable: %+v", r)
		}
	}
}

func TestPollerRemoveStopsWatching(t *testing.T) {
	l := NewLoop()
	a := l.Open("a", 8)
	b := l.Open("b", 8)
	p := l.Poller()
	p.Add(a, Readable)
	p.Add(b, Readable)
	if p.Len() != 2 {
		t.Fatalf("len: want 2 got %d", p.Len())
	}
	p.Remove(a.ID())
	if p.Len() != 1 || p.Watching()[0] != b.ID() {
		t.Fatalf("after remove: len=%d watching=%v", p.Len(), p.Watching())
	}
	if p.InterestOf(a.ID()) != 0 {
		t.Fatalf("removed interest: want 0 got %v", p.InterestOf(a.ID()))
	}
	// 未登録 FD への Modify は無視される(パニックしない)。
	p.Modify(a.ID(), Writable)
	if p.InterestOf(a.ID()) != 0 {
		t.Fatalf("modify on absent fd should be no-op")
	}
}

func TestOneLoopServesManyConnections(t *testing.T) {
	l := NewLoop()
	conns := []*FD{l.Open("c1", 64), l.Open("c2", 64), l.Open("c3", 64)}
	for _, f := range conns {
		l.Register(f, Readable, echoHandler)
	}
	// 3 接続に、時刻をずらしてデータが到着する。1 本のループが順にさばく。
	l.Deliver(0, conns[0], []byte("aaa"))
	l.Deliver(1, conns[1], []byte("bb"))
	l.Deliver(1, conns[2], []byte("cccc")) // c2 と c3 は同時刻
	l.Deliver(3, conns[0], []byte("z"))    // c1 に追加

	l.Run()

	want := []string{"aaaz", "bb", "cccc"}
	for i, f := range conns {
		if string(f.Sent()) != want[i] {
			t.Fatalf("conn %d echoed %q, want %q", i, f.Sent(), want[i])
		}
	}
	// トレースに 3 本すべてへの dispatch があること = 1 ループが多重化した証拠。
	seen := map[int]bool{}
	for _, s := range l.Trace() {
		if s.Phase == PhaseDispatch {
			seen[s.FD] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("dispatched to %d distinct fds, want 3", len(seen))
	}
}

func TestSimultaneousArrivalsDispatchInOrder(t *testing.T) {
	l := NewLoop()
	a := l.Open("a", 64)
	b := l.Open("b", 64)
	var order []int
	h := func(f *FD, ev Interest) {
		order = append(order, f.ID())
		_, _ = f.Read(1 << 20)
	}
	l.Register(a, Readable, h)
	l.Register(b, Readable, h)
	// 同時刻に両方到着 → 登録順(a, b)で dispatch される(決定的)。
	l.Deliver(0, b, []byte("b")) // 予定登録は b が先でも
	l.Deliver(0, a, []byte("a")) // 登録順(a,b)で処理される
	l.Run()
	if len(order) != 2 || order[0] != a.ID() || order[1] != b.ID() {
		t.Fatalf("dispatch order: got %v, want [a,b]=%d,%d", order, a.ID(), b.ID())
	}
}

func TestIdleWaitsUntilArrival(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 64)
	l.Register(f, Readable, echoHandler)
	l.Deliver(5, f, []byte("late")) // t=5 まで何も来ない

	l.Run()

	if string(f.Sent()) != "late" {
		t.Fatalf("sent: want %q got %q", "late", f.Sent())
	}
	// t=0 で idle に入り、t=5 まで clock を飛ばしたことがトレースに出る。
	var idled bool
	for _, s := range l.Trace() {
		if s.Phase == PhaseIdle && s.At == 0 {
			idled = true
		}
	}
	if !idled {
		t.Fatalf("expected an idle step at t=0, trace=%v", l.Trace())
	}
}

func TestBackpressureRegistersWritable(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 2) // 送信バッファは 2 バイトだけ

	// 書ききれなかった残りを覚えておくハンドラ(バックプレッシャ処理)。
	var pending []byte
	var handler func(*FD, Interest)
	handler = func(f *FD, ev Interest) {
		if ev.Has(Readable) {
			b, err := f.Read(1 << 20)
			if err == nil {
				pending = append(pending, b...)
			}
		}
		// 書けるだけ書き、残ったら Writable も待つ。全部掃けたら Readable のみに戻す。
		if len(pending) > 0 {
			n, _ := f.Write(pending)
			pending = pending[n:]
		}
		if len(pending) > 0 {
			l.Watch(f, Readable|Writable) // まだ残る → 空きを待つ
		} else {
			l.Watch(f, Readable)
		}
	}
	l.Register(f, Readable, handler)

	l.Deliver(0, f, []byte("hello")) // 5 バイト。一度には送れない
	l.FreeWrite(2, f, 2)             // t=2 に 2 バイト空く
	l.FreeWrite(4, f, 2)             // t=4 にさらに 2 バイト空く

	l.Run()

	if string(f.Sent()) != "hello" {
		t.Fatalf("backpressure: sent %q, want %q", f.Sent(), "hello")
	}
	if len(pending) != 0 {
		t.Fatalf("pending not fully flushed: %q", pending)
	}
}

func TestLevelTriggeredKeepsReporting(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 64)
	// 毎回 1 バイトずつしか読まないハンドラ。level-triggered なら、未読が
	// 残る限り次の周でも Readable が報告され続ける。
	reads := 0
	l.Register(f, Readable, func(f *FD, ev Interest) {
		if _, err := f.Read(1); err == nil {
			reads++
		}
	})
	l.Deliver(0, f, []byte("abc"))
	l.Run()
	if reads != 3 {
		t.Fatalf("level-triggered reads: want 3 got %d", reads)
	}
}

func TestCloseOnEOFMarkerTerminates(t *testing.T) {
	l := NewLoop()
	f := l.Open("c1", 64)
	l.Register(f, Readable, func(f *FD, ev Interest) {
		b, err := f.Read(1 << 20)
		if err != nil {
			return
		}
		if string(b) == "bye" {
			l.CloseFD(f) // EOF 相当 → 監視から外す
			return
		}
		_, _ = f.Write(b)
	})
	l.Deliver(0, f, []byte("hi"))
	l.Deliver(1, f, []byte("bye"))
	l.Run()

	if string(f.Sent()) != "hi" {
		t.Fatalf("sent: want %q got %q", "hi", f.Sent())
	}
	if l.Poller().Len() != 0 {
		t.Fatalf("fd should be unwatched after close, len=%d", l.Poller().Len())
	}
}

func TestEmptyLoopTerminatesImmediately(t *testing.T) {
	l := NewLoop()
	if l.Tick() {
		t.Fatalf("empty loop Tick: want false")
	}
	if tr := l.Run(); tr != nil {
		t.Fatalf("empty loop trace: want nil got %v", tr)
	}
	if l.Clock() != 0 {
		t.Fatalf("empty loop clock: want 0 got %d", l.Clock())
	}
}

func TestDispatchSkipsClosedMidRound(t *testing.T) {
	// 同じ周に 2 FD が ready。片方のハンドラがもう片方を閉じても、
	// 閉じた FD のハンドラは呼ばれない(f.closed ガード)。
	l := NewLoop()
	a := l.Open("a", 64)
	b := l.Open("b", 64)
	bCalled := false
	l.Register(a, Readable, func(f *FD, ev Interest) {
		_, _ = f.Read(1 << 20)
		l.CloseFD(b) // 同じ周で後続の b を閉じる
	})
	l.Register(b, Readable, func(f *FD, ev Interest) {
		bCalled = true
	})
	l.Deliver(0, a, []byte("a"))
	l.Deliver(0, b, []byte("b"))
	l.Run()
	if bCalled {
		t.Fatalf("closed fd handler should not be called")
	}
}

func TestInterestString(t *testing.T) {
	cases := []struct {
		in   Interest
		want string
	}{
		{0, "-"},
		{Readable, "r"},
		{Writable, "w"},
		{Readable | Writable, "rw"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Fatalf("Interest(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
	if !(Readable | Writable).Has(Readable) {
		t.Fatalf("Has(Readable) should be true")
	}
	if Readable.Has(Writable) {
		t.Fatalf("Readable.Has(Writable) should be false")
	}
}

func TestApplyWorldOrdersByTickThenSeq(t *testing.T) {
	// 異なる時刻の複数イベントが同一周でまとめて適用されるとき、時刻昇順で
	// 適用されることを直接確かめる(通常フローでは idle が最小時刻へ飛ぶため
	// 踏みにくい経路)。
	l := NewLoop()
	f := l.Open("c1", 64)
	var log []byte
	l.schedule(3, func() { log = append(log, '3') })
	l.schedule(1, func() { log = append(log, '1') })
	l.schedule(2, func() { log = append(log, '2') })
	_ = f
	l.clock = 5 // 3 件すべて期限到来にする
	l.applyWorld()
	if string(log) != "123" {
		t.Fatalf("apply order: want 123 got %q", log)
	}
	if len(l.world) != 0 {
		t.Fatalf("all events should be consumed, left %d", len(l.world))
	}
}

func TestFDAccessors(t *testing.T) {
	l := NewLoop()
	f := l.Open("named", 8)
	if f.ID() != 1 || f.Name() != "named" {
		t.Fatalf("accessors: id=%d name=%q", f.ID(), f.Name())
	}
	f.deliver([]byte("xyz"))
	if f.Buffered() != 3 {
		t.Fatalf("Buffered: want 3 got %d", f.Buffered())
	}
}
