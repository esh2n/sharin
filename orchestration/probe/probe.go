// Package probe は Kubernetes のヘルスチェック(probe)を最小構成で実装する。
//
// 前章では、Pod を止めるときにリクエストを落とさない方法を作った。この章は
// その裏返しで、Pod を使い始めるときの話になる。
//
// プロセスが起動したことと、リクエストを処理できることは別だ。起動直後は
// 接続プールを張り、キャッシュを温め、設定を読んでいる。この間に振られても
// 応えられない。逆に、しばらく動いた後で応答しなくなることもある。デッド
// ロックで固まる、内部の状態が壊れる。プロセスは生きているので、外からは
// 動いているように見える。
//
// Kubernetes はこれを、外から定期的に叩いて確かめる。単純な仕組みだが、
// 肝は検査そのものでなく、失敗したときに何をするかの違いにある。readiness
// が落ちればトラフィックから外すだけで、Pod は生き続ける。liveness が落ちれば
// 再起動する。同じ検査の仕組みで、結果の扱いだけが違う。そして扱いが違う
// ことを忘れると、遅いだけの Pod を殺し続ける事故になる。
package probe

// #region model

// Behavior は Pod の中身がいつ応答できるかを決める台本。
// 実際のアプリの代わりに、健康状態を時刻の関数として与える。
type Behavior struct {
	// StartupTicks は起動してから応答できるようになるまでの時間。
	StartupTicks int
	// HangAt は起動からこの時間が経つと応答しなくなる時刻。0 なら壊れない。
	HangAt int
	// HangFor は応答しない長さ。0 なら二度と戻らない。
	HangFor int
}

// healthy は起動から age 経過した時点で応答できるかを返す。
func (b Behavior) healthy(age int) bool {
	if age < b.StartupTicks {
		return false // まだ温まっていない
	}
	if b.HangAt <= 0 || age < b.HangAt {
		return true
	}
	if b.HangFor <= 0 {
		return false // 二度と戻らない
	}
	return age >= b.HangAt+b.HangFor // 一時的に詰まっただけ
}

// Pod は検査される 1 つのレプリカ。
type Pod struct {
	Name     string
	Restarts int // 再起動した回数

	b   Behavior
	age int // 起動(または再起動)からの経過

	startup gate
	ready   gate
	live    gate
}

// Age は起動からの経過を返す。
func (p *Pod) Age() int { return p.age }

// Ready はトラフィックを受ける状態かを返す。
func (p *Pod) Ready() bool { return p.ready.passing }

// Healthy は中身が実際に応答できるかを返す(検査の結果ではなく事実)。
func (p *Pod) Healthy() bool { return p.b.healthy(p.age) }

// restart は Pod を作り直す。経過も検査の状態もすべて初期化される。
func (p *Pod) restart() {
	p.age = 0
	p.Restarts++
	p.startup = gate{}
	p.ready = gate{}
	p.live = gate{passing: true} // 生きている前提から始める
}

// #endregion model

// #region probe

// Probe は 1 種類の検査の設定。readiness も liveness も同じ型で表す。
// 違うのは設定ではなく、失敗したときに何をするかだけ。
type Probe struct {
	InitialDelay     int // 起動から最初の検査までの待ち
	Period           int // 検査の間隔。0 ならこの検査を使わない
	FailureThreshold int // 何回続けて失敗したら落ちたと決めるか
	SuccessThreshold int // 何回続けて成功したら戻ったと決めるか
}

// enabled はこの検査を使うかを返す。
func (p Probe) enabled() bool { return p.Period > 0 }

// due は起動から age の時点が検査のタイミングかを返す。
func (p Probe) due(age int) bool {
	if !p.enabled() || age < p.InitialDelay {
		return false
	}
	return (age-p.InitialDelay)%p.Period == 0
}

// gate は 1 種類の検査の判定状態。連続回数で判定を切り替える。
//
// 1 回の失敗で判定を変えないのが肝になる。ネットワークの瞬断でも検査は
// 失敗するので、1 回で決めると健全な Pod が外れたり再起動されたりする。
// 連続回数を要求することで、たまたまの失敗と本当の異常を分ける。
type gate struct {
	ok      int
	ng      int
	passing bool
}

// record は 1 回の検査結果を記録し、判定が変わったかを返す。
func (g *gate) record(pass bool, p Probe) bool {
	if pass {
		g.ok++
		g.ng = 0
		if !g.passing && g.ok >= max1(p.SuccessThreshold) {
			g.passing = true
			return true
		}
		return false
	}
	g.ng++
	g.ok = 0
	if g.passing && g.ng >= max1(p.FailureThreshold) {
		g.passing = false
		return true
	}
	return false
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// #endregion probe

// #region sim

// Config は 3 種類の検査の設定。
type Config struct {
	// Readiness は「今リクエストを受けられるか」を見る。失敗しても再起動しない。
	Readiness Probe
	// Liveness は「まだ生きているか」を見る。失敗すると再起動する。
	Liveness Probe
	// Startup は「起動が終わったか」を見る。これが通るまで他の2つは動かない。
	Startup Probe
}

// Sim はトラフィックを流しながら検査を回す。時計は論理時刻で決定的。
type Sim struct {
	cfg  Config
	now  int
	pods []*Pod
	rr   int

	Served  int
	Dropped int
	Log     []string
}

// New は台本 bs のぶんだけ Pod を作る。
func New(cfg Config, bs ...Behavior) *Sim {
	s := &Sim{cfg: cfg}
	for i, b := range bs {
		s.pods = append(s.pods, &Pod{
			Name: "pod-" + itoa(i+1), b: b,
			live: gate{passing: true}, // 生きている前提から始める
		})
	}
	return s
}

// Now は現在の論理時刻を返す。
func (s *Sim) Now() int { return s.now }

// Pods は Pod 一覧を返す。
func (s *Sim) Pods() []*Pod { return s.pods }

// Restarts は全 Pod の再起動回数の合計を返す。
func (s *Sim) Restarts() int {
	n := 0
	for _, p := range s.pods {
		n += p.Restarts
	}
	return n
}

// Endpoints はトラフィックが振られる先を返す。readiness を通った Pod だけ。
//
// readiness を使わない設定なら、ここは全 Pod を返す。起動中でも振られる
// ということで、それが事故になる。
func (s *Sim) Endpoints() []string {
	var out []string
	for _, p := range s.pods {
		if !s.cfg.Readiness.enabled() || p.ready.passing {
			out = append(out, p.Name)
		}
	}
	return out
}

// Tick は時刻を1つ進める。検査を回してから、リクエストを振る。
func (s *Sim) Tick(rps int) {
	s.check()
	s.route(rps)
	for _, p := range s.pods {
		p.age++
	}
	s.now++
}

// check は今のタイミングに当たる検査を回し、結果を反映する。
func (s *Sim) check() {
	for _, p := range s.pods {
		healthy := p.b.healthy(p.age)

		// 起動用の検査。これが通るまで、他の2つは一切動かない。
		// 起動が遅いことと、動いてから壊れたことを区別するための仕掛け。
		if s.cfg.Startup.enabled() && !p.startup.passing {
			if !s.cfg.Startup.due(p.age) || !p.startup.record(healthy, s.cfg.Startup) {
				continue // まだ起動が終わっていない。他の検査は一切動かさない
			}
			s.logf(p.Name + " の起動検査が通った。ここから readiness と liveness が動き出す")
		}

		if s.cfg.Readiness.due(p.age) && p.ready.record(healthy, s.cfg.Readiness) {
			if p.ready.passing {
				s.logf(p.Name + " が readiness を通った。転送先に入る")
			} else {
				s.logf(p.Name + " が readiness に落ちた。転送先から外す(再起動はしない)")
			}
		}

		if s.cfg.Liveness.due(p.age) && p.live.record(healthy, s.cfg.Liveness) && !p.live.passing {
			// liveness の失敗は再起動。readiness と違い、取り返しがつかない。
			s.logf(p.Name + " が liveness に落ちた。再起動する(" + itoa(p.Restarts+1) + " 回目)")
			p.restart()
		}
	}
}

// route はリクエストを転送先へ順に振る。中身が応答できなければ落ちる。
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
		if p == nil || !p.b.healthy(p.age) {
			// 転送先には入っているのに、中身が応えられない。
			s.Dropped++
			s.logf(name + " は応答できない状態なのに振られた。1 件失う")
			continue
		}
		s.Served++
	}
}

// Safe は 1 件も落とさなかったかを返す。
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

// #endregion sim

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
