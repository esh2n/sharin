// Package initcontainer は Pod の中のコンテナの起動順序を最小構成で実装する。
//
// ここまでの章は Pod を1つの塊として扱ってきた。作る、消す、置き場所を決める、
// 数を合わせる。だが Pod の中には複数のコンテナが入っていて、その間にも
// 順序がある。設定を取ってくる処理はアプリより先に終わっていなければならないし、
// 通信を代理するプロキシはアプリより先に立ち上がっていなければならない。
//
// 順序を宣言しなければ、コンテナは並行に立ち上がる。並行ということは、
// どちらが先に準備できるか分からないということだ。速いほうが先になる。
// アプリの起動が速く、プロキシの起動が遅ければ、プロキシ抜きでアプリが
// 動いている時間ができる。その間の通信は外に出られない。
//
// 終了のときは向きが逆になる。プロキシが先に死ぬと、アプリがまだ抱えている
// 処理の出口が塞がる。[graceful shutdown](lifecycle) の章で見た「転送先一覧から
// 外れる前に止まる」事故と同じ形が、Pod の内側でも起きる。
//
// この章は、順序を宣言しない書き方と宣言する書き方を並べて、その差を数える。
package initcontainer

// #region spec

// Kind はコンテナが起動順序のどこに置かれるかを表す。
type Kind int

const (
	// Init は宣言順に1つずつ動き、完了してから次へ進む。
	// すべての Init が終わるまで App は1つも始まらない。
	Init Kind = iota
	// Sidecar も Init 枠に並ぶが、完了を待たずに次へ進む。
	// 動き続けたまま App の起動を許し、停止は App の後になる。
	Sidecar
	// App は本体。Init 枠がすべて片付いてから、まとめて並行に起動する。
	App
)

// Spec は 1 つのコンテナの宣言。起動と停止にかかる時間を台本として与えるので、
// 実時間にも乱数にも依存せず、同じ入力なら必ず同じ結果になる。
type Spec struct {
	Name string
	Kind Kind
	// Boot は準備が整うまでにかかる tick。
	Boot int
	// Fails は最初の何回を失敗させるかの台本。Init の失敗で Pod 全体が
	// 止まることを再現するために使う。
	Fails int
	// Drain は停止要求を受けてから止まるまでにかかる tick。
	Drain int
	// Proxy は、このコンテナが通信の出入口を担うかを表す。
	// 本体が動いているのに出入口が居ない時間を数えるために使う。
	Proxy bool
}

// #endregion spec

// #region state

// Phase はコンテナの状態。
type Phase int

const (
	Waiting  Phase = iota // まだ順番が来ていない
	Booting               // 起動中
	Running               // 稼働中
	Failed                // 失敗して再試行待ち
	Draining              // 停止処理中
	Done                  // 終了済み
)

func (p Phase) String() string {
	switch p {
	case Waiting:
		return "Waiting"
	case Booting:
		return "Booting"
	case Running:
		return "Running"
	case Failed:
		return "Failed"
	case Draining:
		return "Draining"
	case Done:
		return "Done"
	}
	return "Unknown"
}

// Container は宣言に状態を足したもの。
type Container struct {
	spec     Spec
	phase    Phase
	left     int // 残り tick
	attempts int // これまでに失敗した回数
	backoff  int // 再試行までの残り tick
}

// Name はコンテナ名を返す。
func (c *Container) Name() string { return c.spec.Name }

// Kind は起動順序の区分を返す。
func (c *Container) Kind() Kind { return c.spec.Kind }

// Phase は現在の状態を返す。
func (c *Container) Phase() Phase { return c.phase }

// Attempts はこれまでに失敗した回数を返す。
func (c *Container) Attempts() int { return c.attempts }

// #endregion state

// #region pod

// Pod は 1 つの Pod の中身を、時刻を1つずつ進めながら動かす。
type Pod struct {
	now  int
	all  []*Container
	gate []*Container // Init と Sidecar。宣言順に1つずつ通す
	main []*Container // App。gate を通り切ってから並行に起動する
	idx  int          // gate のどこまで通ったか

	terminating bool

	// Exposed は、本体が動いている(または処理を抱えている)のに出入口が
	// 居ない tick 数。順序を宣言しない書き方の代償が、ここに出る。
	Exposed int
	Log     []string
}

// New は宣言からPodを組み立てる。gate は宣言順を保つ。
func New(specs []Spec) *Pod {
	p := &Pod{}
	for _, s := range specs {
		c := &Container{spec: s, phase: Waiting}
		p.all = append(p.all, c)
		if s.Kind == App {
			p.main = append(p.main, c)
		} else {
			p.gate = append(p.gate, c)
		}
	}
	return p
}

// Now は現在の論理時刻を返す。
func (p *Pod) Now() int { return p.now }

// Containers は宣言順のコンテナ一覧を返す。
func (p *Pod) Containers() []*Container { return p.all }

// Ready は本体がすべて稼働しているかを返す。
func (p *Pod) Ready() bool {
	if p.idx < len(p.gate) || len(p.main) == 0 {
		return false
	}
	for _, c := range p.main {
		if c.phase != Running {
			return false
		}
	}
	return true
}

// Finished は全コンテナが終了したかを返す。
func (p *Pod) Finished() bool {
	for _, c := range p.all {
		if c.phase != Done {
			return false
		}
	}
	return true
}

// Terminate は Pod の停止を始める。
func (p *Pod) Terminate() {
	if p.terminating {
		return
	}
	p.terminating = true
	p.logf("Pod の停止を開始")
}

// #endregion pod

// #region tick

// Tick は時刻を1つ進める。停止中なら停止処理を、そうでなければ起動処理を進め、
// その時刻の終わりに「本体が出入口なしで動いていないか」を数える。
func (p *Pod) Tick() {
	if p.terminating {
		p.terminateStep()
	} else {
		p.startStep()
	}
	if p.exposed() {
		p.Exposed++
	}
	p.now++
}

// #endregion tick

// #region exposed

// exposed は本体が動いている(処理を抱えている)のに出入口が居ない状態かを返す。
//
// 両側で Draining を数えているのが大事なところになる。本体は停止処理の最中も
// まだ処理を抱えているし、出入口は停止処理の最中もまだ通してくれる。
// 数えたいのは、本体がまだ生きているのに出入口が消えた時間だけだ。
func (p *Pod) exposed() bool {
	live, gateway := false, false
	for _, c := range p.all {
		alive := c.phase == Running || c.phase == Draining
		if c.spec.Proxy {
			gateway = gateway || alive
			continue
		}
		if c.spec.Kind == App && alive {
			live = true
		}
	}
	return live && !gateway
}

// #endregion exposed

// #region start

// startStep は起動処理を1 tick 進める。
//
// gate は1つずつしか進めない。1つが片付いた時刻に次を開始し、開始した時刻には
// 進捗させない。この「開始と進捗を同じ時刻に重ねない」規則で、宣言順が
// そのまま時間順になる。
func (p *Pod) startStep() {
	for p.idx < len(p.gate) {
		c := p.gate[p.idx]
		if c.phase == Waiting {
			p.begin(c)
			return
		}
		p.progress(c)
		if c.phase == Done || (c.spec.Kind == Sidecar && c.phase == Running) {
			p.idx++
			continue
		}
		return
	}
	// gate を通り切った。本体はここから並行に立ち上がる。
	// 並行ということは、どれが先に Running になるかは Boot の短さで決まる。
	for _, c := range p.main {
		if c.phase == Waiting {
			p.begin(c)
			continue
		}
		p.progress(c)
	}
}

// #endregion start

// #region boot

// begin はコンテナの起動を始める。この時刻には進捗させない。
func (p *Pod) begin(c *Container) {
	c.phase = Booting
	c.left = c.spec.Boot
	p.logf(c.spec.Name + " の起動を開始")
	if c.left <= 0 {
		p.settle(c)
	}
}

// progress は起動中・再試行待ちのコンテナを1 tick 進める。
func (p *Pod) progress(c *Container) {
	if c.phase == Failed {
		c.backoff--
		if c.backoff <= 0 {
			c.phase = Booting
			c.left = c.spec.Boot
			p.logf(c.spec.Name + " を再試行する")
		}
		return
	}
	if c.phase != Booting {
		return
	}
	c.left--
	if c.left > 0 {
		return
	}
	p.settle(c)
}

// settle は起動の結末をつける。台本で失敗が残っていれば失敗させ、
// 待ち時間を伸ばして再試行を待つ。実物の CrashLoopBackOff と同じ形になる。
func (p *Pod) settle(c *Container) {
	if c.attempts < c.spec.Fails {
		c.attempts++
		c.phase = Failed
		c.backoff = backoffFor(c.attempts)
		p.logf(c.spec.Name + " が失敗(" + itoa(c.attempts) + " 回目)。" +
			itoa(c.backoff) + " tick 待って再試行する。この間、後続は1つも始まらない")
		return
	}
	if c.spec.Kind == Init {
		c.phase = Done
		p.logf(c.spec.Name + " が完了")
		return
	}
	c.phase = Running
	p.logf(c.spec.Name + " が稼働開始")
}

// backoffFor は失敗回数に応じて待ち時間を倍にしていく(上限つき)。
func backoffFor(attempts int) int {
	d := 1
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= 8 {
			return 8
		}
	}
	return d
}

// #endregion boot

// #region terminate

// terminateStep は停止処理を1 tick 進める。
//
// 本体が先、Sidecar は後。この順序が Sidecar を Sidecar たらしめている。
// 本体と同じ枠に置いたコンテナは本体と同時に止まり始めるので、停止が速ければ
// 本体より先に消える。
func (p *Pod) terminateStep() {
	// 起動の途中で消された場合、まだ順番待ちや起動中だった Init は打ち切られる。
	for _, c := range p.gate {
		if c.spec.Kind == Init {
			p.drain(c)
		}
	}
	for _, c := range p.main {
		p.drain(c)
	}
	if !p.mainDone() {
		return
	}
	for _, c := range p.gate {
		if c.spec.Kind != Sidecar {
			continue
		}
		p.drain(c)
	}
}

// drain は 1 つのコンテナを停止方向へ1 tick 進める。
func (p *Pod) drain(c *Container) {
	switch c.phase {
	case Running:
		c.phase = Draining
		c.left = c.spec.Drain
		p.logf(c.spec.Name + " の停止を開始(残り処理 " + itoa(c.left) + " tick)")
		if c.left <= 0 {
			c.phase = Done
			p.logf(c.spec.Name + " が停止")
		}
	case Draining:
		c.left--
		if c.left <= 0 {
			c.phase = Done
			p.logf(c.spec.Name + " が停止")
		}
	case Waiting, Booting, Failed:
		// 起動しきる前に止められた分は、そのまま終わる。
		c.phase = Done
	}
}

// mainDone は本体がすべて終了したかを返す。
func (p *Pod) mainDone() bool {
	for _, c := range p.main {
		if c.phase != Done {
			return false
		}
	}
	return true
}

// #endregion terminate

func (p *Pod) logf(msg string) { p.Log = append(p.Log, "t="+itoa(p.now)+" "+msg) }

// itoa は小さな整数を文字列にする(strconv を避ける)。
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
