// Package pdb は Kubernetes の PodDisruptionBudget を最小構成で実装する。
//
// ここまでの章で、Pod は色々な理由で消えてきた。更新で入れ替わり、ノードの
// 集約で移され、障害で落ちる。1つずつ見れば、どれも正しい動きだ。だが同時に
// 起これば話が変わる。3つのうち2つを更新のために止めている最中に、残り1つが
// 載っているノードを集約のために空にしたら、全部消える。
//
// それぞれの仕組みは自分の都合しか知らない。ローリング更新は自分の入れ替え
// 幅しか見ないし、Cluster Autoscaler は自分の集約しか見ない。全体で何個
// 残っているかを見ている者が、どこにもいない。
//
// そこで、消す側でなくアプリの側から下限を宣言する。「web は常に2つ以上
// 動いていること」。消そうとする側は、消す前にこの宣言に照らして許可を求める。
// 割ってしまうなら断られ、待つ。これが PodDisruptionBudget だ。
//
// 肝は、これが守れるのは自発的な退避だけ、という点にある。ノードが突然
// 落ちるのは誰にも止められない。止められるものと止められないものを、
// はっきり分けているところにこの仕組みの性格が出ている。
package pdb

import "sort"

// #region model

// Pod は1つのレプリカ。どのノードに載っていて、受けられる状態かを持つ。
type Pod struct {
	Name   string
	Node   string
	Ready  bool
	labels map[string]string

	readyAt int // この時刻に ready になる(-1 なら予定なし)
}

func (p *Pod) matches(selector map[string]string) bool {
	for k, v := range selector {
		if p.labels[k] != v {
			return false
		}
	}
	return true
}

// Budget は「この条件の Pod は常に何個以上」という宣言。
// 消す側でなく、アプリの側が書く。
type Budget struct {
	Name         string
	MinAvailable int
	selector     map[string]string
}

// #endregion model

// #region cluster

// Config は退避された Pod の代わりが立ち上がるまでの時間。
type Config struct {
	// StartupTicks は作り直された Pod が ready になるまでの周期。
	StartupTicks int
	// ReplaceNode は代わりの Pod を置くノード。
	ReplaceNode string
}

// Cluster は Pod と宣言を持ち、退避の可否を判定する。
type Cluster struct {
	cfg     Config
	pods    map[string]*Pod
	budgets []*Budget
	now     int
	seq     int

	Evicted int // 許可されて退避した数
	Denied  int // 断られた数
	Crashed int // 止められない理由で消えた数
	Log     []string
}

// New は空のクラスタを作る。
func New(cfg Config) *Cluster {
	return &Cluster{cfg: cfg, pods: map[string]*Pod{}}
}

// AddPod は Pod を1つ足す。
func (c *Cluster) AddPod(name, node string, labels map[string]string, ready bool) *Pod {
	p := &Pod{Name: name, Node: node, Ready: ready, labels: labels, readyAt: -1}
	c.pods[name] = p
	return p
}

// AddBudget は下限の宣言を1つ足す。
func (c *Cluster) AddBudget(name string, minAvailable int, selector map[string]string) *Budget {
	b := &Budget{Name: name, MinAvailable: minAvailable, selector: selector}
	c.budgets = append(c.budgets, b)
	return b
}

// Pods は Pod 一覧を名前順に返す。
func (c *Cluster) Pods() []*Pod {
	names := make([]string, 0, len(c.pods))
	for n := range c.pods {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Pod, len(names))
	for i, n := range names {
		out[i] = c.pods[n]
	}
	return out
}

// Available は selector に合い、かつ ready な Pod の数を返す。
// ready でない Pod は数えない。作り直し中のものは頭数に入らない。
func (c *Cluster) Available(selector map[string]string) int {
	n := 0
	for _, p := range c.pods {
		if p.Ready && p.matches(selector) {
			n++
		}
	}
	return n
}

// Tick は時刻を進め、立ち上がり終えた Pod を ready にする。
// ここは調整ループが作り直した Pod が起動していく過程にあたる。
func (c *Cluster) Tick() {
	c.now++
	for _, p := range c.pods {
		if p.readyAt >= 0 && c.now >= p.readyAt {
			p.Ready = true
			p.readyAt = -1
			c.logf(p.Name + " が ready になった")
		}
	}
}

// #endregion cluster

// #region evict

// Evict は自発的な退避を試みる。消した後も、その Pod にかかる宣言の下限を
// 保てるときだけ許可する。割ってしまうなら断る。
//
// 判定に使うのは ready な数だ。作り直し中の Pod は頭数に入らないので、
// 前に退避したぶんが立ち上がるまで、次の退避は断られ続ける。この待ちが
// 「一度に何個まで止めてよいか」を実質的に決めている。
func (c *Cluster) Evict(name string) bool {
	p, ok := c.pods[name]
	if !ok {
		return false
	}
	for _, b := range c.budgets {
		if !p.matches(b.selector) {
			continue
		}
		after := c.Available(b.selector)
		if p.Ready {
			after-- // この Pod を消すと1つ減る
		}
		if after < b.MinAvailable {
			c.Denied++
			c.logf(name + " の退避を断った(" + b.Name + " の下限 " + itoa(b.MinAvailable) +
				" を割る。今 " + itoa(c.Available(b.selector)) + ")")
			return false
		}
	}
	c.remove(p)
	c.Evicted++
	c.logf(name + " を退避した")
	c.replace(p)
	return true
}

// Crash は止められない理由で Pod が消えることを表す。ノードの障害や
// カーネルの停止がこれにあたる。宣言は一切参照しない。参照したところで
// 止められないからだ。宣言が守るのは、こちらから止められるものだけになる。
func (c *Cluster) Crash(name string) {
	p, ok := c.pods[name]
	if !ok {
		return
	}
	c.remove(p)
	c.Crashed++
	c.logf(name + " が落ちた(宣言では止められない)")
	c.replace(p)
}

// Drain は node に載っている Pod を、載っている順に退避しようとする。
// 断られたぶんは残る。ノードを空にするには、何度も呼ぶことになる。
func (c *Cluster) Drain(node string) (evicted int, remaining []string) {
	for _, p := range c.Pods() {
		if p.Node != node {
			continue
		}
		if c.Evict(p.Name) {
			evicted++
			continue
		}
		remaining = append(remaining, p.Name)
	}
	return evicted, remaining
}

// #endregion evict

// replace は消えた Pod の代わりを作る。調整ループの代役で、作られた直後は
// ready でない。立ち上がるまでの間、その Pod は頭数に入らない。
func (c *Cluster) replace(old *Pod) {
	if c.cfg.ReplaceNode == "" {
		return
	}
	c.seq++
	name := old.Name + "-r" + itoa(c.seq)
	c.pods[name] = &Pod{
		Name: name, Node: c.cfg.ReplaceNode, Ready: c.cfg.StartupTicks == 0,
		labels: old.labels, readyAt: c.now + c.cfg.StartupTicks,
	}
	if c.cfg.StartupTicks == 0 {
		c.pods[name].readyAt = -1
	}
}

func (c *Cluster) remove(p *Pod) { delete(c.pods, p.Name) }

func (c *Cluster) logf(msg string) { c.Log = append(c.Log, "t="+itoa(c.now)+" "+msg) }

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
