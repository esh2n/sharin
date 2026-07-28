// Package preemption は PriorityClass と preemption を最小構成で実装する。
//
// [スケジューラ](scheduler)の章では、置けるノードが無ければ Pod は Pending の
// まま残るとした。だがそれで困る Pod がある。決済を捌く本番のアプリと、夜間の
// バッチ処理が同じクラスタに載っているとき、空きが無いからといって本番の
// アプリを待たせるわけにはいかない。バッチを退かせばよい。
//
// そのための仕組みが優先度になる。優先度は2つの意味を持っていて、1つは
// 待ち行列の順序、もう1つは場所を奪う権利だ。前者は当たり前に見えるが、
// 後者はそうではない。すでに動いている Pod を、何も悪いことをしていないのに
// 止めるということだからだ。
//
// 奪うと決めたら、次は「誰を、何個」を決めることになる。ここが素朴に作ると
// 間違えるところで、優先度の低いものから順に止めていくと、止めなくてよい
// ものまで止めてしまう。実物は逆向きに考える。まず全部外し、それから戻せる
// だけ戻す。この順序でないと最小にならない。
//
// そして追い出された Pod は消えるわけではない。待ち行列に戻り、別の場所を
// 探す。探した先で、さらに下を追い出すことがある。
package preemption

import (
	"sort"

	"github.com/esh2n/sharin/orchestration/scheduler"
)

// #region model

// Pod は優先度を持つワークロード。
type Pod struct {
	Name string
	// App は保護([PodDisruptionBudget](pdb))をまとめる単位。
	App string
	// Priority は大きいほど高い。等しい相手からは奪えない。
	Priority int
	Req      scheduler.Resources
}

// NodeSpec はノード1台の宣言。
type NodeSpec struct {
	Name string
	Cap  scheduler.Resources
}

// Eviction は 1 件の追い出し。誰が誰をどこから追い出したか、そのとき
// 保護を破ったかを残す。
type Eviction struct {
	Victim   string
	By       string
	Node     string
	Violates bool
}

type node struct {
	name string
	cap  scheduler.Resources
	pods []Pod
}

// free は drop に入っている Pod を居ないものとみなしたときの空きを返す。
// 「外してみたら入るか」を試すために、実際に外さずに計算する。
func (n *node) free(drop map[string]bool) scheduler.Resources {
	f := n.cap
	for _, p := range n.pods {
		if drop[p.Name] {
			continue
		}
		f.CPU -= p.Req.CPU
		f.Mem -= p.Req.Mem
	}
	return f
}

func (n *node) remove(name string) {
	var rest []Pod
	for _, p := range n.pods {
		if p.Name != name {
			rest = append(rest, p)
		}
	}
	n.pods = rest
}

// fits は要求 req が空き free に収まるかを返す。
func fits(req, free scheduler.Resources) bool {
	return req.CPU <= free.CPU && req.Mem <= free.Mem
}

// #endregion model

// #region cluster

// maxRounds は待ち行列を回す上限。追い出しが玉突きを起こすので、
// 際限なく回らないように区切っておく。
const maxRounds = 64

type queued struct {
	pod Pod
	seq int
}

// Cluster はノードと待ち行列を持ち、優先度に従って配置を決める。
type Cluster struct {
	nodes   []*node
	queue   []queued
	stuck   []Pod
	seq     int
	budgets map[string]int // App → 最低これだけは残す

	Evictions []Eviction
	Log       []string
}

// New はノードの宣言と保護の設定からクラスタを作る。
// budgets は App ごとの最低稼働数で、nil でもよい。
func New(specs []NodeSpec, budgets map[string]int) *Cluster {
	c := &Cluster{budgets: map[string]int{}}
	for _, s := range specs {
		c.nodes = append(c.nodes, &node{name: s.Name, cap: s.Cap})
	}
	for k, v := range budgets {
		c.budgets[k] = v
	}
	return c
}

// Place はすでに動いている Pod をノードに直接載せる(初期状態を作る用)。
func (c *Cluster) Place(p Pod, nodeName string) {
	for _, n := range c.nodes {
		if n.name == nodeName {
			n.pods = append(n.pods, p)
			return
		}
	}
}

// Submit は Pod を待ち行列に入れる。
func (c *Cluster) Submit(p Pod) {
	c.seq++
	c.queue = append(c.queue, queued{pod: p, seq: c.seq})
}

// Placement は Pod 名から載っているノード名への対応を返す。
func (c *Cluster) Placement() map[string]string {
	out := map[string]string{}
	for _, n := range c.nodes {
		for _, p := range n.pods {
			out[p.Name] = n.name
		}
	}
	return out
}

// Pending はどこにも置けなかった Pod 名を返す。
func (c *Cluster) Pending() []string {
	var out []string
	for _, p := range c.stuck {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// PodsOn はノードに載っている Pod 名を返す。
func (c *Cluster) PodsOn(nodeName string) []string {
	for _, n := range c.nodes {
		if n.name != nodeName {
			continue
		}
		var out []string
		for _, p := range n.pods {
			out = append(out, p.Name)
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// #endregion cluster

// #region run

// Run は待ち行列が空になるまで回す。
//
// 1 周ごとに、優先度のいちばん高い Pod を取り出して置こうとする。置けなければ
// 奪いにいく。奪えば犠牲が行列に戻るので、行列は減るとは限らない。
func (c *Cluster) Run() {
	for i := 0; i < maxRounds && len(c.queue) > 0; i++ {
		c.sortQueue()
		q := c.queue[0]
		c.queue = c.queue[1:]

		if c.place(q.pod) {
			continue
		}
		if c.preempt(q.pod) {
			continue
		}
		// 奪う相手も居ない。ここで諦める。実物では unschedulable として
		// 別の行列に移り、状況が変わるまで再試行されない。
		c.stuck = append(c.stuck, q.pod)
		c.logf(q.pod.Name + " はどこにも置けない(奪える相手も居ない)")
	}
}

// sortQueue は優先度の高い順、同じなら入った順に並べる。
// これが優先度の1つめの意味、待ち行列の順序になる。
func (c *Cluster) sortQueue() {
	sort.SliceStable(c.queue, func(i, j int) bool {
		if c.queue[i].pod.Priority != c.queue[j].pod.Priority {
			return c.queue[i].pod.Priority > c.queue[j].pod.Priority
		}
		return c.queue[i].seq < c.queue[j].seq
	})
}

// place は奪わずに置けるノードを探して置く。
//
// 候補を絞るところは[スケジューラ](scheduler)の Filter をそのまま使う。
// 実物でも preemption は通常のスケジューリングが失敗した後で走る仕組みなので、
// 同じ判定を通ってから来ることになる。
func (c *Cluster) place(p Pod) bool {
	var probes []*scheduler.Node
	for _, n := range c.nodes {
		// 空きぶんの容量を持つ仮のノードとして渡す。
		probes = append(probes, scheduler.NewNode(n.name, n.free(nil)))
	}
	feasible, _ := scheduler.Filter(scheduler.Pod{Name: p.Name, Req: p.Req}, probes)
	if len(feasible) == 0 {
		return false
	}
	// 候補が複数あれば空きの大きいほうへ。同点は名前順で決定的にする。
	best := c.byName(feasible[0].Name)
	for _, f := range feasible[1:] {
		n := c.byName(f.Name)
		if n.free(nil).CPU > best.free(nil).CPU || (n.free(nil).CPU == best.free(nil).CPU && n.name < best.name) {
			best = n
		}
	}
	best.pods = append(best.pods, p)
	c.logf(p.Name + "(優先度 " + itoa(p.Priority) + ") を " + best.name + " に置いた")
	return true
}

func (c *Cluster) byName(name string) *node {
	for _, n := range c.nodes {
		if n.name == name {
			return n
		}
	}
	return nil
}

// #endregion run

// #region victims

// selectVictims は、ノード n に p を置くために追い出す Pod を選ぶ。
//
// 素朴には「優先度の低いものから、入るまで外す」と考えたくなる。だがそれだと
// 外さなくてよいものまで外してしまう。小さいものを2つ外しても足りず、
// 結局大きいものも外す、という順序になったとき、先に外した2つは無駄になる。
//
// 実物は逆に考える。まず p より低いものを全部外し、それでも入らないなら
// このノードは候補にしない。入るなら、優先度の高いものから戻していく。
// 戻しても p が入るなら、その Pod は外す必要が無かったということになる。
func (c *Cluster) selectVictims(n *node, p Pod) ([]Pod, bool) {
	drop := map[string]bool{}
	var lower []Pod
	for _, q := range n.pods {
		if q.Priority < p.Priority {
			lower = append(lower, q)
			drop[q.Name] = true
		}
	}
	if !fits(p.Req, n.free(drop)) {
		// 低いものを全部どけても入らない。奪っても意味がない。
		return nil, false
	}

	sort.SliceStable(lower, func(i, j int) bool {
		if lower[i].Priority != lower[j].Priority {
			return lower[i].Priority > lower[j].Priority
		}
		return lower[i].Name < lower[j].Name
	})

	var victims []Pod
	for _, q := range lower {
		delete(drop, q.Name)
		if fits(p.Req, n.free(drop)) {
			continue // 戻しても入る。この Pod は助かる
		}
		drop[q.Name] = true
		victims = append(victims, q)
	}
	return victims, true
}

// violations は victims を追い出したときに破ってしまう保護の数を返す。
//
// [PodDisruptionBudget](pdb) の章では、自発的な退去は保護に阻まれるとした。
// 奪うほうは阻まれない。数えはするが、他に道が無ければ破って進む。
func (c *Cluster) violations(victims []Pod) int {
	killed := map[string]int{}
	for _, v := range victims {
		killed[v.App]++
	}
	apps := make([]string, 0, len(killed))
	for a := range killed {
		apps = append(apps, a)
	}
	sort.Strings(apps)

	total := 0
	for _, a := range apps {
		min, ok := c.budgets[a]
		if !ok {
			continue
		}
		if left := c.countApp(a) - killed[a]; left < min {
			total += min - left
		}
	}
	return total
}

func (c *Cluster) countApp(app string) int {
	n := 0
	for _, nd := range c.nodes {
		for _, p := range nd.pods {
			if p.App == app {
				n++
			}
		}
	}
	return n
}

// #endregion victims

// #region preempt

type candidate struct {
	node       *node
	victims    []Pod
	violations int
	topVictim  int // 犠牲の中でいちばん高い優先度
}

// preempt は奪ってでも置く。候補ノードごとに犠牲を計算し、いちばん小さい
// 犠牲で済むノードを選ぶ。
//
// 選び方の順序が、そのまま何を大事にしているかの宣言になる。
// ① 保護を破る数が少ない ② 犠牲の優先度がなるべく低い ③ 犠牲の数が少ない。
// 保護を最優先に見るが、破らずに済む選択肢が無ければ破る。
func (c *Cluster) preempt(p Pod) bool {
	var best *candidate
	for _, n := range c.nodes {
		victims, ok := c.selectVictims(n, p)
		if !ok || len(victims) == 0 {
			continue
		}
		cand := candidate{node: n, victims: victims, violations: c.violations(victims)}
		for _, v := range victims {
			if v.Priority > cand.topVictim {
				cand.topVictim = v.Priority
			}
		}
		if best == nil || better(cand, *best) {
			cp := cand
			best = &cp
		}
	}
	if best == nil {
		return false
	}

	for _, v := range best.victims {
		violates := c.violations([]Pod{v}) > 0
		best.node.remove(v.Name)
		c.Evictions = append(c.Evictions, Eviction{
			Victim: v.Name, By: p.Name, Node: best.node.name, Violates: violates,
		})
		msg := p.Name + "(優先度 " + itoa(p.Priority) + ") が " + best.node.name +
			" から " + v.Name + "(優先度 " + itoa(v.Priority) + ") を追い出した"
		if violates {
			msg += "。保護を破っている"
		}
		c.logf(msg)
		// 追い出された Pod は消えない。行列に戻って別の場所を探す。
		c.seq++
		c.queue = append(c.queue, queued{pod: v, seq: c.seq})
	}
	best.node.pods = append(best.node.pods, p)
	c.logf(p.Name + " を " + best.node.name + " に置いた(奪って作った場所)")
	return true
}

// better は候補 a が候補 b より望ましいかを返す。
func better(a, b candidate) bool {
	if a.violations != b.violations {
		return a.violations < b.violations
	}
	if a.topVictim != b.topVictim {
		return a.topVictim < b.topVictim
	}
	if len(a.victims) != len(b.victims) {
		return len(a.victims) < len(b.victims)
	}
	return a.node.name < b.node.name
}

// #endregion preempt

func (c *Cluster) logf(msg string) { c.Log = append(c.Log, msg) }

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
