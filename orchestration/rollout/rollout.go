// Package rollout は Kubernetes のローリング更新を最小構成で実装する。
//
// この編で作ってきた部品が、ここで1つに合わさる。調整ループは宣言された数を
// 守り、スケジューラが置き場所を決め、終了処理が落とさずに止め、ヘルス
// チェックが受けられるかを判定する。ローリング更新は、その全部を使って
// 「動いているものを、止めずに入れ替える」を行う。
//
// 素朴には、全部消してから全部作ればいい。だがその間サービスは止まる。逆に
// 全部作ってから全部消すなら、一瞬とはいえ2倍の資源が要る。現実はその間の
// どこかを選ぶ。何個多く作ってよいか(maxSurge)と、何個まで減ってよいか
// (maxUnavailable)。この2つの幅が、入れ替えの速さと、その間に保たれる容量を
// 決める。
//
// そして肝は、入れ替えを進めてよいかの判断を readiness に委ねていることだ。
// 新しい版が ready にならなければ、古い版は消されない。壊れた版をデプロイ
// しても、置き換えは途中で止まり、全滅しない。この安全は、幅の設定がその
// まま被害の上限になっている。
package rollout

// #region model

// Release は 1 つの版の性質。どれくらいで起動し、そもそも起動しきるか。
type Release struct {
	Version int
	// StartupTicks は起動から ready になるまでの時間。
	StartupTicks int
	// Broken が真なら、何周期経っても ready にならない(壊れた版)。
	Broken bool
}

// Pod は 1 つのレプリカ。どの版かと、起動からの経過を持つ。
type Pod struct {
	Name    string
	Version int

	age int
	rel Release
}

// Ready はリクエストを受けられる状態かを返す。ヘルスチェックの結果にあたる。
func (p *Pod) Ready() bool { return !p.rel.Broken && p.age >= p.rel.StartupTicks }

// Age は起動からの経過を返す。
func (p *Pod) Age() int { return p.age }

// #endregion model

// #region config

// Config は入れ替えの幅を決める設定。
type Config struct {
	// Replicas は保ちたいレプリカ数。
	Replicas int
	// MaxSurge は目標数より何個多く作ってよいか。入れ替えの速さを決める。
	MaxSurge int
	// MaxUnavailable は目標数より何個少なくてよいか。容量の下限を決める。
	MaxUnavailable int
}

// minAvailable は入れ替え中に保つべき ready な数を返す。
func (c Config) minAvailable() int {
	n := c.Replicas - c.MaxUnavailable
	if n < 0 {
		return 0
	}
	return n
}

// maxTotal は入れ替え中に持ってよい Pod の総数を返す。
func (c Config) maxTotal() int { return c.Replicas + c.MaxSurge }

// Deadlocked は幅がどちらも 0 で、入れ替えが1歩も進めない設定かを返す。
// 多く作ることも、減らすこともできないので、何も起こらない。
func (c Config) Deadlocked() bool { return c.MaxSurge == 0 && c.MaxUnavailable == 0 }

// #endregion config

// Snapshot は 1 周期ぶんの様子(観測・説明用)。
type Snapshot struct {
	Step      int
	OldTotal  int
	OldReady  int
	NewTotal  int
	NewReady  int
	Available int // 版を問わず ready な数
	Action    string
}

// Rollout は入れ替えを 1 周期ずつ進める。
type Rollout struct {
	cfg    Config
	target Release
	pods   []*Pod
	seq    int
	step   int

	// MinAvailableSeen は入れ替えを通して観測した ready 数の最小値。
	// 設定した下限を割っていないかを、これで確かめる。
	MinAvailableSeen int

	History []Snapshot
	Log     []string
}

// New は初期版で Replicas 個が ready に揃った状態から始める。
func New(cfg Config, initial Release) *Rollout {
	r := &Rollout{cfg: cfg, target: initial, MinAvailableSeen: cfg.Replicas}
	for i := 0; i < cfg.Replicas; i++ {
		p := r.create(initial)
		p.age = initial.StartupTicks // 起動済みから始める
	}
	return r
}

// Deploy は目標の版を切り替える。ここでは何も作らず、以降の Step で進む。
// 宣言を書き換えるだけという点は、調整ループとまったく同じ考え方になる。
func (r *Rollout) Deploy(rel Release) {
	r.target = rel
	r.logf("目標を v" + itoa(rel.Version) + " に切り替えた")
}

// Pods は Pod 一覧を返す。
func (r *Rollout) Pods() []*Pod { return r.pods }

// Available は今 ready な Pod の数を返す。
func (r *Rollout) Available() int {
	n := 0
	for _, p := range r.pods {
		if p.Ready() {
			n++
		}
	}
	return n
}

// Done は目標の版だけが Replicas 個そろって ready になったかを返す。
func (r *Rollout) Done() bool {
	ready := 0
	for _, p := range r.pods {
		if p.Version != r.target.Version {
			return false // 古い版が残っている
		}
		if p.Ready() {
			ready++
		}
	}
	return ready == r.cfg.Replicas
}

// #region step

// Step は入れ替えを1周期進める。起動を進めてから、作る手と消す手を1つずつ打つ。
//
// 判断はどちらも「今 ready な数」に基づく。新しい版が ready にならない限り
// 古い版は消えないので、壊れた版をデプロイしても置き換えは途中で止まる。
func (r *Rollout) Step() {
	for _, p := range r.pods {
		p.age++
	}

	var acts string

	// ① 目標の版が足りず、総数に余裕があれば作る(maxSurge の枠内)。
	if r.countVersion(r.target.Version) < r.cfg.Replicas && len(r.pods) < r.cfg.maxTotal() {
		p := r.create(r.target)
		acts = "create " + p.Name + "(v" + itoa(p.Version) + ")"
	}

	// ② 消しても下限を割らないなら、古い版を1つ消す。
	// ここが安全弁になる。余裕がなければ古い版は残り、容量が保たれる。
	if old := r.oldest(); old != nil && r.canRemove(old) {
		r.remove(old)
		if acts != "" {
			acts += " / "
		}
		acts += "delete " + old.Name + "(v" + itoa(old.Version) + ")"
	}

	if acts == "" {
		acts = "動けない(新しい版が ready になるのを待つ)"
	}
	r.logf(acts)
	r.snapshot(acts)
	r.step++
}

// #endregion step

// Run は最大 max 周期まで進め、完了したらそこで止める。
func (r *Rollout) Run(max int) {
	for i := 0; i < max && !r.Done(); i++ {
		r.Step()
	}
}

// Stalled は完了していないのに、これ以上進めない状態かを返す。
// 壊れた版をデプロイしたときにここへ落ち着く。
func (r *Rollout) Stalled() bool {
	if r.Done() {
		return false
	}
	canCreate := r.countVersion(r.target.Version) < r.cfg.Replicas && len(r.pods) < r.cfg.maxTotal()
	old := r.oldest()
	canDelete := old != nil && r.canRemove(old)
	if canCreate || canDelete {
		return false
	}
	// 起動待ちの Pod が残っていれば、まだ進む見込みがある。
	for _, p := range r.pods {
		if !p.Ready() && !p.rel.Broken {
			return false
		}
	}
	return true
}

func (r *Rollout) create(rel Release) *Pod {
	r.seq++
	p := &Pod{Name: "pod-" + itoa(r.seq), Version: rel.Version, rel: rel}
	r.pods = append(r.pods, p)
	return p
}

func (r *Rollout) remove(target *Pod) {
	out := r.pods[:0]
	for _, p := range r.pods {
		if p != target {
			out = append(out, p)
		}
	}
	r.pods = out
}

// #region guard

// canRemove は p を消しても ready な数が下限を割らないかを返す。
//
// p がそもそも ready でなければ、消しても ready な数は変わらない。だから
// いつでも消せる。当たり前に見えて、これが無いと壊れた版から戻れなくなる。
// ready にならない Pod が枠を埋めたまま、下限に張り付いて動けなくなるからだ。
func (r *Rollout) canRemove(p *Pod) bool {
	if !p.Ready() {
		return true
	}
	return r.Available() > r.cfg.minAvailable()
}

// oldest は目標でない版の Pod を1つ返す。ready でないものを先に選ぶ。
// 消しても損をしない相手から片づけるほうが、入れ替えが詰まりにくい。
func (r *Rollout) oldest() *Pod {
	var fallback *Pod
	for _, p := range r.pods {
		if p.Version == r.target.Version {
			continue
		}
		if !p.Ready() {
			return p
		}
		if fallback == nil {
			fallback = p
		}
	}
	return fallback
}

// #endregion guard

func (r *Rollout) countVersion(v int) int {
	n := 0
	for _, p := range r.pods {
		if p.Version == v {
			n++
		}
	}
	return n
}

func (r *Rollout) snapshot(action string) {
	s := Snapshot{Step: r.step, Action: action}
	for _, p := range r.pods {
		if p.Version == r.target.Version {
			s.NewTotal++
			if p.Ready() {
				s.NewReady++
			}
		} else {
			s.OldTotal++
			if p.Ready() {
				s.OldReady++
			}
		}
	}
	s.Available = s.OldReady + s.NewReady
	if s.Available < r.MinAvailableSeen {
		r.MinAvailableSeen = s.Available
	}
	r.History = append(r.History, s)
}

func (r *Rollout) logf(msg string) { r.Log = append(r.Log, "step "+itoa(r.step)+": "+msg) }

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
