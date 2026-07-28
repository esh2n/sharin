// Package lifecycle は Pod の終了処理(graceful shutdown)を最小構成で実装する。
//
// Pod は必ず消える。スケールダウンでも、更新でも、ノードの入れ替えでも、
// 動いている Pod は止められる。このとき何も考えずに止めると、処理中の
// リクエストが落ちる。落ちるのは障害のときだけではない。むしろ、正常な
// デプロイのたびに少しずつ落ちる。
//
// 事故の原因は、終了のときに2つのことが並行して起こることにある。
// 1つはトラフィックの切り離し。ロードバランサの転送先一覧から、この Pod を
// 外す。もう1つはプロセスの停止。SIGTERM を送り、アプリを終わらせる。
// この2つは別々の仕組みが担当していて、どちらが先に効くかは保証されない。
// 切り離しより先に停止が効くと、まだ振られてくるリクエストを受けられない
// Pod ができあがる。それが落ちるリクエストの正体だ。
//
// 対策も、原因が分かれば単純になる。SIGTERM を受けても、すぐには止まらず
// 少し待つ(preStop)。その間に切り離しが伝わりきる。そして処理中の
// リクエストを終わらせるまで、強制終了(SIGKILL)を待つ(猶予期間)。
// この章は、事故と対策の両方を再現する。
package lifecycle

// #region model

// Phase は Pod の終了に関する状態。
type Phase int

const (
	Ready       Phase = iota // 稼働中。トラフィックを受ける
	Terminating              // 終了処理中
	Stopped                  // 停止済み
)

func (p Phase) String() string {
	switch p {
	case Ready:
		return "Ready"
	case Terminating:
		return "Terminating"
	case Stopped:
		return "Stopped"
	}
	return "Unknown"
}

// Pod は 1 つのレプリカ。処理中のリクエストを残り時間の形で抱える。
type Pod struct {
	Name      string
	phase     Phase
	accepting bool  // 新規リクエストを受け付けるか
	inflight  []int // 処理中のリクエストの残り tick
	sigtermAt int   // SIGTERM を送る時刻(-1 なら予定なし)
	killAt    int   // SIGKILL を送る時刻(-1 なら予定なし)
	removeAt  int   // 転送先一覧から外れる時刻(-1 なら予定なし)
}

// Phase は現在の状態を返す。
func (p *Pod) Phase() Phase { return p.phase }

// Inflight は処理中のリクエスト数を返す。
func (p *Pod) Inflight() int { return len(p.inflight) }

// Accepting は新規リクエストを受け付けるかを返す。
func (p *Pod) Accepting() bool { return p.accepting }

// #endregion model

// #region config

// Config は終了処理の設定。事故が起きるかどうかは、この3つの大小で決まる。
type Config struct {
	// Propagation は、削除を決めてから転送先一覧に反映されるまでの遅れ。
	// 制御の伝播にかかる時間であり、こちらから短くはできない。
	Propagation int
	// PreStop は SIGTERM を送る前に待つ時間。ここを Propagation 以上に
	// 取れば、切り離しが伝わりきってから停止が始まる。
	PreStop int
	// Grace は SIGTERM から SIGKILL までの猶予。処理中のリクエストを
	// 終わらせるための時間で、足りなければ道連れになる。
	Grace int
	// Work は 1 リクエストの処理にかかる時間。
	Work int
}

// #endregion config

// #region endpoints

// Sim は「トラフィックが流れているところに Pod の削除を差し込む」状況を
// 時刻を1つずつ進めながら再現する。時計は論理時刻なので再現性がある。
type Sim struct {
	cfg  Config
	now  int
	pods []*Pod
	rr   int // 転送先を選ぶ順番(round-robin)

	Served  int // 完了したリクエスト
	Dropped int // 落ちたリクエスト
	Log     []string
}

// New は replicas 個の Pod が稼働している状態から始める。
func New(cfg Config, replicas int) *Sim {
	s := &Sim{cfg: cfg}
	for i := 0; i < replicas; i++ {
		s.pods = append(s.pods, &Pod{
			Name: "pod-" + itoa(i+1), phase: Ready, accepting: true,
			sigtermAt: -1, killAt: -1, removeAt: -1,
		})
	}
	return s
}

// Now は現在の論理時刻を返す。
func (s *Sim) Now() int { return s.now }

// Pods は Pod 一覧を返す。
func (s *Sim) Pods() []*Pod { return s.pods }

// Endpoints は今トラフィックが振られる先(転送先一覧)を返す。
//
// ここが Pod の生死を見ていないことが大事だ。この一覧は制御側が管理する
// もので、Pod が実際に止まったかどうかを知らない。削除を決めても
// Propagation のぶんは残り続けるし、その間に Pod が死んでいても残り続ける。
// 一覧が現実に追いついていない、この隙間が事故の置き場所になる。
func (s *Sim) Endpoints() []string {
	var out []string
	for _, p := range s.pods {
		if p.removeAt < 0 || s.now < p.removeAt {
			out = append(out, p.Name)
		}
	}
	return out
}

// #endregion endpoints

// #region terminate

// Terminate は Pod の削除を始める。ここで2つのことが同時に動き出す。
// 転送先一覧からの除去(Propagation 後に反映)と、停止の予定(PreStop 後に
// SIGTERM、その Grace 後に SIGKILL)だ。どちらが先に効くかで結果が変わる。
func (s *Sim) Terminate(name string) {
	for _, p := range s.pods {
		if p.Name != name || p.phase != Ready {
			continue
		}
		p.phase = Terminating
		p.removeAt = s.now + s.cfg.Propagation
		p.sigtermAt = s.now + s.cfg.PreStop
		p.killAt = p.sigtermAt + s.cfg.Grace
		s.logf(p.Name + " の削除を開始(転送先から外れるのは t=" + itoa(p.removeAt) +
			"、SIGTERM は t=" + itoa(p.sigtermAt) + ")")
		return
	}
}

// #endregion terminate

// #region step

// Tick は時刻を1つ進める。まず今の時刻に予定されている終了処理を反映し、
// その状態で rps 件のリクエストを振り、処理中のリクエストを進める。
// 順序が結果を決める。予定を先に反映するので、SIGTERM が効いた直後の
// 時刻に振られてきたリクエストは、ちゃんと落ちる。
func (s *Sim) Tick(rps int) {
	s.lifecycleStep()
	s.route(rps)
	s.advance()
	s.now++
}

// route は rps 件のリクエストを転送先一覧へ順に振る。
// 転送先に残っている Pod が受け付けを止めていれば、そのリクエストは落ちる。
func (s *Sim) route(rps int) {
	eps := s.Endpoints()
	if len(eps) == 0 {
		s.Dropped += rps
		if rps > 0 {
			s.logf("転送先が空。" + itoa(rps) + " 件が行き場を失う")
		}
		return
	}
	for i := 0; i < rps; i++ {
		name := eps[s.rr%len(eps)]
		s.rr++
		p := s.find(name)
		if p == nil || !p.accepting {
			// 転送先には残っているのに、受け付けを止めている。これが事故。
			s.Dropped++
			s.logf(name + " は受け付けを止めているのに振られた。1 件失う")
			continue
		}
		p.inflight = append(p.inflight, s.cfg.Work)
	}
}

// advance は処理中のリクエストを1 tick 進め、終わったものを数える。
func (s *Sim) advance() {
	for _, p := range s.pods {
		var rest []int
		for _, left := range p.inflight {
			if left-1 <= 0 {
				s.Served++
				continue
			}
			rest = append(rest, left-1)
		}
		p.inflight = rest
	}
}

// lifecycleStep は終了処理の予定を時刻に照らして進める。
func (s *Sim) lifecycleStep() {
	for _, p := range s.pods {
		if p.phase != Terminating {
			continue
		}
		if p.removeAt >= 0 && s.now == p.removeAt {
			s.logf(p.Name + " が転送先一覧から外れた")
		}
		if p.sigtermAt >= 0 && s.now == p.sigtermAt {
			// SIGTERM。ここから新規は受けない。処理中のぶんは続ける。
			p.accepting = false
			s.logf(p.Name + " に SIGTERM。新規の受け付けを止める(処理中 " + itoa(len(p.inflight)) + " 件)")
		}
		if !p.accepting && len(p.inflight) == 0 {
			// 処理中が捌けた。猶予を待たずに自分から終わる。
			p.phase = Stopped
			s.logf(p.Name + " は処理中を捌き終えて停止")
			continue
		}
		if p.killAt >= 0 && s.now >= p.killAt {
			// 猶予切れ。処理中のリクエストは道連れになる。
			if n := len(p.inflight); n > 0 {
				s.Dropped += n
				s.logf(p.Name + " が猶予切れで SIGKILL。処理中 " + itoa(n) + " 件を道連れにする")
			}
			p.inflight = nil
			p.phase = Stopped
		}
	}
}

// #endregion step

// Safe は終了処理を通してリクエストを1件も落とさなかったかを返す。
func (s *Sim) Safe() bool { return s.Dropped == 0 }

func (s *Sim) find(name string) *Pod {
	for _, p := range s.pods {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (s *Sim) logf(msg string) { s.Log = append(s.Log, "t="+itoa(s.now)+" "+msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
