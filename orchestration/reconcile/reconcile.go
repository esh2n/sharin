// Package reconcile は Kubernetes の心臓である調整ループ(reconciliation loop)を
// 最小構成で実装する。
//
// Kubernetes に「Pod を 3 個動かせ」と手続きで命じることはない。代わりに
// 「あるべき状態(desired state)は 3 個だ」と宣言する。あとはコントローラが、
// 宣言された状態と、今実際にある状態(observed state)を、繰り返し見比べて、
// 差があれば埋める。足りなければ作り、多ければ消す。これが調整ループだ。
// 肝は 2 つある。宣言的であること。人は「どうやって 3 個にするか」の手順でなく
// 「3 個であってほしい」という状態だけを書く。そして level-triggered であること。
// コントローラは「Pod が死んだ」というイベントに反応するのでなく、毎回まるごと
// 現状を数え直して収束させる。だからイベントを取りこぼしても、コントローラが
// 落ちて復帰しても、次の調整で必ず追いつく。作成もスケールも障害回復も、
// この 1 つのループが等しく処理する。
package reconcile

import "sort"

// #region types

// Phase は Pod のライフサイクル状態。
type Phase int

const (
	Pending Phase = iota // 作られたがまだ起動していない
	Running              // 起動して稼働中
	Failed               // 落ちた(ノード障害・クラッシュ)
)

func (p Phase) String() string {
	switch p {
	case Pending:
		return "Pending"
	case Running:
		return "Running"
	case Failed:
		return "Failed"
	}
	return "Unknown"
}

// Pod は 1 つのワークロード実体。observed state を構成する。
type Pod struct {
	Name  string
	Phase Phase
}

// Cluster は実際にある状態(observed state)を保持する世界。
type Cluster struct {
	pods map[string]*Pod
	seq  int
}

// NewCluster は空のクラスタを作る。
func NewCluster() *Cluster { return &Cluster{pods: map[string]*Pod{}} }

// create は Pending の Pod を 1 つ足し、その名前を返す(連番で決定的)。
func (c *Cluster) create() string {
	c.seq++
	name := "pod-" + itoa(c.seq)
	c.pods[name] = &Pod{Name: name, Phase: Pending}
	return name
}

// delete は Pod を消す。
func (c *Cluster) delete(name string) { delete(c.pods, name) }

// Pods は名前順に Pod 一覧を返す(観測用・決定的)。
func (c *Cluster) Pods() []Pod {
	names := make([]string, 0, len(c.pods))
	for n := range c.pods {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Pod, len(names))
	for i, n := range names {
		out[i] = *c.pods[n]
	}
	return out
}

// StartPending は Pending の Pod を Running にする(スケジュール後の起動を模す)。
func (c *Cluster) StartPending() {
	for _, p := range c.pods {
		if p.Phase == Pending {
			p.Phase = Running
		}
	}
}

// Fail は name の Pod を Failed にする(ノード障害・クラッシュ)。
func (c *Cluster) Fail(name string) {
	if p, ok := c.pods[name]; ok {
		p.Phase = Failed
	}
}

// alive は生きている(Pending か Running の)Pod 名を名前順で返す。Failed は死。
func (c *Cluster) alive() []string {
	var names []string
	for n, p := range c.pods {
		if p.Phase != Failed {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// #endregion types

// #region reconcile

// Action は調整で起こした操作(観測・説明用)。
type Action struct {
	Kind string // "create" / "delete"
	Pod  string
}

// Controller は「Pod を desired 個に保つ」責務を持つ(ReplicaSet 相当)。
type Controller struct{ desired int }

// New は目標レプリカ数 desired のコントローラを作る。
func New(desired int) *Controller {
	if desired < 0 {
		desired = 0
	}
	return &Controller{desired: desired}
}

// SetDesired は目標を宣言的に変更する(スケール)。手順でなく「あるべき数」を書く。
func (c *Controller) SetDesired(n int) {
	if n < 0 {
		n = 0
	}
	c.desired = n
}

// Desired は現在の目標数を返す。
func (c *Controller) Desired() int { return c.desired }

// Reconcile は observed state と desired state を見比べ、差を埋める操作を行う。
// この 1 回の呼び出しが、作成・スケール・障害回復のどれも等しく処理する。
// 何度呼んでも収束先は同じ(冪等)。イベントに反応せず、毎回現状を数え直す
// (level-triggered)ので、取りこぼしやコントローラの再起動に強い。
func (c *Controller) Reconcile(cl *Cluster) []Action {
	var actions []Action

	// まず死んだ Pod を掃除する(Failed は生きた数に数えないので、この後で
	// 作り直しの対象になる)。
	for _, p := range cl.Pods() {
		if p.Phase == Failed {
			cl.delete(p.Name)
			actions = append(actions, Action{Kind: "delete", Pod: p.Name})
		}
	}

	alive := cl.alive()
	switch {
	case len(alive) < c.desired:
		// 足りない。差のぶんだけ作る(障害回復もスケールアップもここ)。
		for i := 0; i < c.desired-len(alive); i++ {
			actions = append(actions, Action{Kind: "create", Pod: cl.create()})
		}
	case len(alive) > c.desired:
		// 多い。余りを消す(スケールダウン)。名前順で末尾から。
		excess := alive[c.desired:]
		for _, name := range excess {
			cl.delete(name)
			actions = append(actions, Action{Kind: "delete", Pod: name})
		}
	}
	// 差がなければ actions は空(冪等・何もしない)。
	return actions
}

// Converged は observed が desired に一致し、全 Pod が Running かを返す。
func (c *Controller) Converged(cl *Cluster) bool {
	running := 0
	for _, p := range cl.Pods() {
		if p.Phase == Failed {
			return false
		}
		if p.Phase == Running {
			running++
		}
	}
	return running == c.desired && len(cl.Pods()) == c.desired
}

// #endregion reconcile

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
