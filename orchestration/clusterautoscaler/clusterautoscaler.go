// Package clusterautoscaler は Kubernetes の Cluster Autoscaler を最小構成で
// 実装する。この編の締めにあたり、スケジューラとオートスケーラが合流する。
//
// 水平オートスケーラは Pod の数を増やした。だが Pod は置き場所がなければ
// 動かない。全ノードが埋まっていれば、増やした Pod は Pending のまま残る。
// 数を増やしただけでは、捌ける量は増えない。足りないのがノードそのもの
// なら、ノードを増やすしかない。それをやるのが Cluster Autoscaler だ。
//
// 素朴には「Pending の Pod があればノードを増やす」でよさそうに見える。だが
// それでは足りない。1 台に収まらない大きさを要求している Pod は、何台
// 足しても置けない。増やしても無駄なのに、増やし続けることになる。だから
// 実物は、増やす前に確かめる。新しいノードを1台仮に置いてみて、その Pod が
// そこに載るかをスケジューラで判定する。載らないなら増やさない。
//
// 縮小も同じ形をとる。使用率の低いノードを消す前に、そこに載っている Pod が
// 他のノードに収まるかを、やはりスケジューラで確かめる。収まらないなら
// 消さない。増やすときも減らすときも、判断はスケジューラに委ねられている。
// だからこの実装は、前章のスケジューラをそのまま部品として使う。
package clusterautoscaler

import (
	"sort"

	"github.com/esh2n/sharin/orchestration/scheduler"
)

// #region config

// Config はノードを増減させる設定。
type Config struct {
	// NodeCap は1台あたりの容量。ここでは全ノード同じとする。
	NodeCap scheduler.Resources
	// MinNodes / MaxNodes はノード数の下限と上限。
	MinNodes int
	MaxNodes int
	// BootTicks はノードを足してから使えるようになるまでの時間。
	// Pod の起動に比べて桁違いに遅いことが、この層の性質を決める。
	BootTicks int
	// ScaleDownUtil はこの使用率(百分率)を下回ったノードを縮小の候補にする。
	ScaleDownUtil int
	// Strategy は Pod をノードに割り当てるときの方針。
	Strategy scheduler.Strategy
}

// #endregion config

// #region autoscaler

// Autoscaler はノード数を負荷に合わせて増減させる。
type Autoscaler struct {
	cfg   Config
	sched *scheduler.Scheduler

	nodes   []*scheduler.Node        // 使えるノード
	booting []*scheduler.Node        // 起動中でまだ使えないノード
	bootAt  map[string]int           // 起動中のノードが使えるようになる時刻
	specs   map[string]scheduler.Pod // Pod 名 → 要求(再配置のときに要る)
	place   map[string]string        // Pod 名 → 載っているノード
	pending []scheduler.Pod          // 置き場所が見つからない Pod

	seq     int // Pod の通し番号
	nodeSeq int // ノードの通し番号
	now     int

	Log []string
}

// New は下限ぶんのノードが起動済みの状態から始める。
func New(cfg Config) *Autoscaler {
	a := &Autoscaler{
		cfg:    cfg,
		sched:  scheduler.New(cfg.Strategy),
		bootAt: map[string]int{},
		specs:  map[string]scheduler.Pod{},
		place:  map[string]string{},
	}
	for i := 0; i < cfg.MinNodes; i++ {
		a.addNode(true)
	}
	return a
}

// Nodes は使えるノード一覧を返す。
func (a *Autoscaler) Nodes() []*scheduler.Node { return a.nodes }

// Booting は起動中のノード数を返す。
func (a *Autoscaler) Booting() int { return len(a.booting) }

// Pending は置き場所が見つからない Pod 名を返す。
func (a *Autoscaler) Pending() []string {
	out := make([]string, 0, len(a.pending))
	for _, p := range a.pending {
		out = append(out, p.Name)
	}
	return out
}

// Submit は Pod を投入する。すぐに置ければ置き、置けなければ Pending に積む。
func (a *Autoscaler) Submit(req scheduler.Resources) string {
	a.seq++
	p := scheduler.Pod{Name: "pod-" + itoa(a.seq), Req: req}
	a.specs[p.Name] = p
	if !a.tryPlace(p) {
		a.pending = append(a.pending, p)
		a.logf(p.Name + " は置き場所がない。Pending のまま")
	}
	return p.Name
}

// Tick は時刻を1つ進める。起動中のノードを使えるようにし、Pending を置き直し、
// 足りなければ増やし、余っていれば減らす。
func (a *Autoscaler) Tick() {
	a.now++
	a.bootReady()
	a.drainPending()
	a.scaleUp()
	a.scaleDown()
}

// bootReady は起動時刻に達したノードを使える状態にする。
// それまでは Pod を置けない。ノードの起動は Pod に比べて桁違いに遅く、
// その待ち時間の間、Pending の Pod はただ待つことになる。
func (a *Autoscaler) bootReady() {
	var still []*scheduler.Node
	for _, n := range a.booting {
		if a.now >= a.bootAt[n.Name] {
			delete(a.bootAt, n.Name)
			a.nodes = append(a.nodes, n)
			a.logf(n.Name + " が使えるようになった")
			continue
		}
		still = append(still, n)
	}
	a.booting = still
}

// drainPending は Pending の Pod を、今あるノードへ置けるだけ置く。
func (a *Autoscaler) drainPending() {
	var rest []scheduler.Pod
	for _, p := range a.pending {
		if !a.tryPlace(p) {
			rest = append(rest, p)
		}
	}
	a.pending = rest
}

// #endregion autoscaler

// #region scaleup

// scaleUp は Pending の Pod があるとき、ノードを1台足してよいかを判定する。
//
// 肝は、足す前に確かめることだ。新しいノードを仮に1台こしらえて、その Pod が
// そこに載るかをスケジューラの filter で見る。載らないなら足しても無駄なので
// 足さない。1台に収まらない要求をしている Pod のために、ノードを無限に
// 増やし続ける、という事故はこれで防げる。
func (a *Autoscaler) scaleUp() {
	if len(a.pending) == 0 || len(a.nodes)+len(a.booting) >= a.cfg.MaxNodes {
		return
	}
	// 仮のノードを1台こしらえて、Pending の Pod が載るかを見る。
	hypo := scheduler.NewNode("hypothetical", a.cfg.NodeCap)
	helped := false
	for _, p := range a.pending {
		if feasible, _ := scheduler.Filter(p, []*scheduler.Node{hypo}); len(feasible) > 0 {
			helped = true
			break
		}
	}
	if !helped {
		a.logf("ノードを足しても Pending は解消しない。増やさない")
		return
	}
	n := a.addNode(false)
	a.logf(n.Name + " を追加(起動に " + itoa(a.cfg.BootTicks) + " 周期かかる)")
}

// #endregion scaleup

// #region scaledown

// scaleDown は使用率の低いノードを1台探し、その Pod が他へ収まるなら消す。
//
// ここでもスケジューラに判定を委ねる。そのノードを取り除いた世界を作り、
// 全 Pod を置き直せるかを試す。1つでも置けなければ消さない。使用率が低い
// だけでは消してよい理由にならず、行き先があることまで確かめて初めて消せる。
func (a *Autoscaler) scaleDown() {
	if len(a.nodes) <= a.cfg.MinNodes || len(a.pending) > 0 {
		return // 下限、または置けていない Pod があるうちは減らさない
	}
	for _, victim := range a.nodes {
		if util(victim) >= a.cfg.ScaleDownUtil {
			continue
		}
		if nodes, place, ok := a.simulateWithout(victim); ok {
			a.nodes, a.place = nodes, place
			a.logf(victim.Name + " は使用率 " + itoa(util(victim)) + "% で、載っている Pod も他へ移せる。削除")
			return
		}
		a.logf(victim.Name + " は使用率が低いが、載っている Pod の行き先がない。残す")
	}
}

// simulateWithout は victim を除いた世界を作り、全 Pod を置き直せるかを試す。
// 成功したときだけ、新しいノード一式と配置を返す。
func (a *Autoscaler) simulateWithout(victim *scheduler.Node) ([]*scheduler.Node, map[string]string, bool) {
	fresh := a.freshNodes(victim)
	if len(fresh) == 0 {
		return nil, nil, false
	}
	place, ok := a.packInto(fresh)
	if !ok {
		return nil, nil, false // 1つでも置けなければ、この縮小は諦める
	}
	return fresh, place, true
}

// freshNodes は今のノードを、何も載っていない状態で作り直す(except は除く)。
// 置き直しの試算は、この空のノードの上で行う。
func (a *Autoscaler) freshNodes(except *scheduler.Node) []*scheduler.Node {
	var out []*scheduler.Node
	for _, n := range a.nodes {
		if n != except {
			out = append(out, scheduler.NewNode(n.Name, n.Cap))
		}
	}
	return out
}

// packInto は今ある Pod を nodes へ置き直す。1つでも置けなければ false。
func (a *Autoscaler) packInto(nodes []*scheduler.Node) (map[string]string, bool) {
	place := map[string]string{}
	for _, name := range a.podOrder() {
		r := a.sched.Schedule(a.specs[name], nodes)
		if !r.Scheduled() {
			return nil, false
		}
		place[name] = r.Node
	}
	return place, true
}

// podOrder は置き直す順序を返す。要求の大きい順に置くほうが収まりやすく、
// 何より順序が固定なので結果が再現する。
func (a *Autoscaler) podOrder() []string {
	names := make([]string, 0, len(a.place))
	for name := range a.place {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		pi, pj := a.specs[names[i]], a.specs[names[j]]
		if pi.Req.CPU != pj.Req.CPU {
			return pi.Req.CPU > pj.Req.CPU
		}
		return names[i] < names[j]
	})
	return names
}

// Remove は Pod を消す。空いた資源を反映するため、残りを置き直す。
func (a *Autoscaler) Remove(name string) {
	if _, ok := a.place[name]; !ok {
		return
	}
	delete(a.place, name)
	delete(a.specs, name)
	fresh := a.freshNodes(nil)
	if place, ok := a.packInto(fresh); ok {
		a.nodes, a.place = fresh, place
	}
}

// #endregion scaledown

// tryPlace は Pod を今あるノードへ置く。置けたら true。
func (a *Autoscaler) tryPlace(p scheduler.Pod) bool {
	r := a.sched.Schedule(p, a.nodes)
	if !r.Scheduled() {
		return false
	}
	a.place[p.Name] = r.Node
	return true
}

// addNode はノードを1台足す。ready なら即座に使え、そうでなければ起動中になる。
func (a *Autoscaler) addNode(ready bool) *scheduler.Node {
	a.nodeSeq++
	n := scheduler.NewNode("node-"+itoa(a.nodeSeq), a.cfg.NodeCap)
	if ready {
		a.nodes = append(a.nodes, n)
	} else {
		a.booting = append(a.booting, n)
		a.bootAt[n.Name] = a.now + a.cfg.BootTicks
	}
	return n
}

// util はノードの使用率(CPU とメモリの平均、百分率)を返す。
func util(n *scheduler.Node) int {
	c := pct(n.Used().CPU, n.Cap.CPU)
	m := pct(n.Used().Mem, n.Cap.Mem)
	return (c + m) / 2
}

func pct(a, b int) int {
	if b <= 0 {
		return 0
	}
	return a * 100 / b
}

// Placement は Pod がどのノードに載っているかを返す。
func (a *Autoscaler) Placement() map[string]string {
	out := map[string]string{}
	for k, v := range a.place {
		out[k] = v
	}
	return out
}

func (a *Autoscaler) logf(msg string) { a.Log = append(a.Log, "t="+itoa(a.now)+" "+msg) }

// itoa は小さな整数を文字列にする(strconv を避ける)。
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
