package lifecycle

import "testing"

// run は 2 レプリカに一定の負荷を流しながら pod-1 を削除し、
// 落ち着くまで進めた結果を返す。
func run(cfg Config, rps int) *Sim {
	s := New(cfg, 2)
	s.Tick(rps) // 平常運転
	s.Terminate("pod-1")
	for i := 0; i < 20; i++ {
		s.Tick(rps)
	}
	return s
}

// preStop がないと、切り離しが伝わる前に SIGTERM が効く。
// 転送先には残っているのに受け付けを止めた Pod ができ、リクエストが落ちる。
func TestNoPreStopDropsRequests(t *testing.T) {
	s := run(Config{Propagation: 3, PreStop: 0, Grace: 10, Work: 1}, 4)
	if s.Safe() {
		t.Fatal("preStop なしなら落ちるはずが 0 件だった")
	}
	if s.Dropped == 0 {
		t.Fatalf("落ちた件数が 0: served=%d", s.Served)
	}
}

// preStop を伝播の遅れ以上に取れば、切り離しが伝わりきってから停止が始まる。
// 同じ設定でも 1 件も落ちなくなる。
func TestPreStopCoversPropagation(t *testing.T) {
	s := run(Config{Propagation: 3, PreStop: 3, Grace: 10, Work: 1}, 4)
	if !s.Safe() {
		t.Fatalf("preStop が伝播を覆えば落ちないはずが %d 件落ちた\n%v", s.Dropped, s.Log)
	}
	if s.Served == 0 {
		t.Fatal("1 件も処理していない")
	}
}

// preStop が伝播より短いと、その差のぶんだけ落ちる。
// 対策の効き方が連続的であることを示す。
func TestShortPreStopStillDrops(t *testing.T) {
	full := run(Config{Propagation: 4, PreStop: 4, Grace: 10, Work: 1}, 4)
	half := run(Config{Propagation: 4, PreStop: 2, Grace: 10, Work: 1}, 4)
	none := run(Config{Propagation: 4, PreStop: 0, Grace: 10, Work: 1}, 4)
	if full.Dropped != 0 {
		t.Fatalf("覆えていれば 0 件のはずが %d", full.Dropped)
	}
	if !(none.Dropped > half.Dropped && half.Dropped > 0) {
		t.Fatalf("preStop が短いほど落ちるはず: none=%d half=%d", none.Dropped, half.Dropped)
	}
}

// 猶予が処理時間より短いと、処理中のリクエストが強制終了に巻き込まれる。
func TestShortGraceKillsInflight(t *testing.T) {
	s := run(Config{Propagation: 2, PreStop: 2, Grace: 1, Work: 5}, 4)
	if s.Safe() {
		t.Fatal("猶予が足りなければ処理中が道連れになるはず")
	}
}

// 猶予が処理時間を覆えば、処理中のリクエストは最後まで完了する。
func TestGraceCoversInflight(t *testing.T) {
	s := run(Config{Propagation: 2, PreStop: 2, Grace: 10, Work: 5}, 4)
	if !s.Safe() {
		t.Fatalf("猶予が足りていれば落ちないはずが %d 件\n%v", s.Dropped, s.Log)
	}
}

// 処理中が捌けたら、猶予を待たずに自分から停止する。
func TestStopsEarlyWhenDrained(t *testing.T) {
	s := New(Config{Propagation: 1, PreStop: 1, Grace: 100, Work: 1}, 2)
	s.Terminate("pod-1")
	for i := 0; i < 6; i++ {
		s.Tick(2)
	}
	if s.pods[0].Phase() != Stopped {
		t.Fatalf("捌け終えたら停止するはずが %s", s.pods[0].Phase())
	}
}

// 転送先一覧は、削除を決めてすぐには変わらない。
// 遅れて反映されるからこそ、その間に振られてくる。
func TestEndpointRemovalIsDelayed(t *testing.T) {
	s := New(Config{Propagation: 3, PreStop: 3, Grace: 10, Work: 1}, 2)
	s.Terminate("pod-1")
	if len(s.Endpoints()) != 2 {
		t.Fatalf("削除直後はまだ両方が転送先のはず: %v", s.Endpoints())
	}
	for i := 0; i < 3; i++ {
		s.Tick(2)
	}
	if len(s.Endpoints()) != 1 || s.Endpoints()[0] != "pod-2" {
		t.Fatalf("伝播後は pod-2 だけのはず: %v", s.Endpoints())
	}
}

// SIGTERM を受けても、処理中のリクエストは続く。止まるのは新規だけ。
func TestSigtermStopsNewOnly(t *testing.T) {
	s := New(Config{Propagation: 0, PreStop: 0, Grace: 10, Work: 4}, 1)
	s.Tick(2) // 2 件を受け付ける
	s.Terminate("pod-1")
	s.Tick(0) // SIGTERM が効く
	p := s.pods[0]
	if p.Accepting() {
		t.Fatal("SIGTERM 後に新規を受け付けている")
	}
	if p.Inflight() == 0 {
		t.Fatal("処理中のリクエストまで消えている")
	}
}

// 転送先が 1 つもなくなれば、来たリクエストは行き場を失う。
// レプリカを 1 つしか置かないと、更新のたびにこれが起きる。
func TestNoEndpointsDropsAll(t *testing.T) {
	s := New(Config{Propagation: 0, PreStop: 0, Grace: 0, Work: 1}, 1)
	s.Terminate("pod-1")
	s.Tick(0) // 転送先から外れ、停止する
	before := s.Dropped
	s.Tick(3)
	if s.Dropped != before+3 {
		t.Fatalf("転送先がなければ全部落ちるはず: %d → %d", before, s.Dropped)
	}
}

// 終了処理中でない Pod への Terminate は何もしない(冪等)。
func TestTerminateIsIdempotent(t *testing.T) {
	s := New(Config{Propagation: 2, PreStop: 2, Grace: 5, Work: 1}, 2)
	s.Terminate("pod-1")
	n := len(s.Log)
	s.Terminate("pod-1") // 2 度目
	s.Terminate("pod-9") // 存在しない
	if len(s.Log) != n {
		t.Fatalf("2 度目の Terminate が効いてしまった: %v", s.Log[n:])
	}
}

func TestPhaseAndItoaStrings(t *testing.T) {
	if Ready.String() != "Ready" || Terminating.String() != "Terminating" ||
		Stopped.String() != "Stopped" || Phase(9).String() != "Unknown" {
		t.Fatal("Phase の文字列が違う")
	}
	if itoa(0) != "0" || itoa(120) != "120" || itoa(-7) != "-7" {
		t.Fatalf("itoa が違う: %s %s %s", itoa(0), itoa(120), itoa(-7))
	}
}
