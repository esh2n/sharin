package tcp

// #region sim

// Event はトレース1行: どのステップで、誰から誰へ、どんなセグメントが飛んだか
// (落とされたなら Dropped)。
type Event struct {
	At      int
	From    string
	To      string
	Seg     Segment
	Dropped bool
}

// Sim はパケット網の模擬。2 つのエンドポイントを繋ぎ、離散ステップで時間を進める。
// 特定のセグメント(送出順の通し番号)を落とせるので、損失と再送を決定的に再現できる。
type Sim struct {
	a, b    *Endpoint // a = 能動オープン側(クライアント)、b = 受け身側(サーバ)
	clock   int
	segIdx  int
	dropSet map[int]bool // 落とすセグメントの通し番号
	trace   []Event
}

// NewSim は 2 エンドポイントを繋いだ模擬網を作る。
func NewSim(client, server *Endpoint) *Sim {
	return &Sim{a: client, b: server, dropSet: map[int]bool{}}
}

// Drop は送出順で idx 番目(0 始まり)のセグメントを落とす、と予約する。
func (s *Sim) Drop(idxs ...int) {
	for _, i := range idxs {
		s.dropSet[i] = true
	}
}

// Step は 1 ステップ進める: 両端のタイマを刻み、両端が出すセグメントを集め、
// 落とさないものを相手へ届ける。何かを送出したら true。
func (s *Sim) Step() bool {
	s.clock++
	s.a.tick()
	s.b.tick()
	aSegs := s.a.emit()
	bSegs := s.b.emit()
	for _, seg := range aSegs {
		s.route(s.a, s.b, seg)
	}
	for _, seg := range bSegs {
		s.route(s.b, s.a, seg)
	}
	return len(aSegs)+len(bSegs) > 0
}

// route は 1 セグメントを網に流す。予約された通し番号なら落とし、そうでなければ届ける。
func (s *Sim) route(from, to *Endpoint, seg Segment) {
	idx := s.segIdx
	s.segIdx++
	dropped := s.dropSet[idx]
	s.trace = append(s.trace, Event{At: s.clock, From: from.Name, To: to.Name, Seg: seg, Dropped: dropped})
	if !dropped {
		to.deliver(seg)
	}
}

// RunUntil は done が真になるまで(または maxSteps まで)ステップを回す。
// done を満たして止まったら true。
func (s *Sim) RunUntil(done func() bool, maxSteps int) bool {
	for i := 0; i < maxSteps; i++ {
		if done() {
			return true
		}
		s.Step()
	}
	return done()
}

// Clock は現在のステップ数を返す。
func (s *Sim) Clock() int { return s.clock }

// Trace はセグメントのやり取りの記録を返す。
func (s *Sim) Trace() []Event { return s.trace }

// #endregion sim
