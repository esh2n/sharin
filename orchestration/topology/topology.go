// Package topology は Kubernetes の topology spread constraints を最小構成で
// 実装する。スケジューラの章の続きにあたる。
//
// あの章では、Pod を散らすか詰めるかを score で選んだ。だが散らすといっても、
// 何に対して散らすのかを決めていなかった。ノードに均等に置けば、ノード1台の
// 障害には耐えられる。だが同じラックの3台に均等に置いていたら、そのラックの
// 電源が落ちたとき3台とも失う。1台の障害と、1区画の障害は別物になる。
//
// 区画のことを topology domain と呼ぶ。ノード、ラック、可用域(ゾーン)、地域。
// どの単位で散らしたいかは、何の障害に耐えたいかで決まる。ゾーンが丸ごと
// 落ちても動き続けたいなら、ゾーンをまたいで散らす必要がある。
//
// 肝は、これが「散らしてほしい」という願いではなく、置ける場所を絞る制約
// として書かれることだ。偏りの許容量(skew)を宣言し、それを超える配置を
// 候補から落とす。score でなく filter の側に立っている。だから、守れないなら
// 置かないという選択もできるし、置けるだけ置くという選択もできる。
package topology

import "sort"

// #region model

// Node は Pod を置ける1台。どの区画に属するかを持つ。
type Node struct {
	Name string
	// Zone は所属する区画。同じ区画は同時に落ちうるとみなす。
	Zone string
	pods []string
}

// Pods はこのノードに載っている Pod を配置順に返す。
func (n *Node) Pods() []string { return append([]string(nil), n.pods...) }

// WhenUnsatisfiable は制約を守れないときの振る舞い。
type WhenUnsatisfiable int

const (
	// DoNotSchedule は守れないなら置かない。可用性を優先する。
	DoNotSchedule WhenUnsatisfiable = iota
	// ScheduleAnyway は守れなくても置く。願いとして扱う。
	ScheduleAnyway
)

func (w WhenUnsatisfiable) String() string {
	if w == ScheduleAnyway {
		return "ScheduleAnyway"
	}
	return "DoNotSchedule"
}

// Constraint は「区画ごとの数の差を、これ以内に収めよ」という宣言。
type Constraint struct {
	// MaxSkew は最も多い区画と最も少ない区画の差の上限。
	MaxSkew int
	// When は守れないときにどうするか。
	When WhenUnsatisfiable
}

// #endregion model

// #region skew

// Cluster はノードの集まりと制約を持つ。
type Cluster struct {
	cfg   Constraint
	nodes []*Node

	Placed  int
	Refused int
	Log     []string
}

// New は制約 cfg のクラスタを作る。
func New(cfg Constraint) *Cluster {
	if cfg.MaxSkew < 1 {
		cfg.MaxSkew = 1
	}
	return &Cluster{cfg: cfg}
}

// AddNode はノードを1台足す。
func (c *Cluster) AddNode(name, zone string) *Node {
	n := &Node{Name: name, Zone: zone}
	c.nodes = append(c.nodes, n)
	return n
}

// Nodes はノードを名前順に返す。
func (c *Cluster) Nodes() []*Node {
	out := append([]*Node(nil), c.nodes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Zones は区画を名前順に返す。
func (c *Cluster) Zones() []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range c.nodes {
		if !seen[n.Zone] {
			seen[n.Zone] = true
			out = append(out, n.Zone)
		}
	}
	sort.Strings(out)
	return out
}

// Count は区画ごとの Pod 数を返す。
func (c *Cluster) Count(zone string) int {
	n := 0
	for _, node := range c.nodes {
		if node.Zone == zone {
			n += len(node.pods)
		}
	}
	return n
}

// Skew は最も多い区画と最も少ない区画の差を返す。これが偏りの尺度になる。
//
// 数えるのは区画ごとであって、ノードごとではない。ノードに均等でも、
// 区画に偏っていれば skew は大きい。何を数えるかが、何の障害に備えるかを決める。
func (c *Cluster) Skew() int {
	zones := c.Zones()
	if len(zones) == 0 {
		return 0
	}
	min, max := -1, 0
	for _, z := range zones {
		n := c.Count(z)
		if n > max {
			max = n
		}
		if min < 0 || n < min {
			min = n
		}
	}
	return max - min
}

// #endregion skew

// #region place

// Result は1回の配置の結果。
type Result struct {
	Placed bool
	Node   string
	Zone   string
	Skew   int
	Reason string
}

// Place は Pod を1つ置く。制約を守れる区画の中で、いちばん少ない区画を選ぶ。
//
// 候補を絞ってから選ぶという順序が、スケジューラの章と同じ形になっている。
// 違うのは、絞る条件が「置いた後の偏り」であることだ。今の状態でなく、
// 置いた後どうなるかを見て判断する。
func (c *Cluster) Place(name string) Result {
	zones := c.Zones()
	if len(zones) == 0 {
		return Result{Reason: "ノードが1台も無い"}
	}

	// ① 置いた後も制約を守れる区画だけを候補にする。
	var feasible []string
	for _, z := range zones {
		if c.skewAfter(z) <= c.cfg.MaxSkew {
			feasible = append(feasible, z)
		}
	}

	if len(feasible) == 0 {
		if c.cfg.When == DoNotSchedule {
			c.Refused++
			c.logf(name + " はどの区画に置いても偏りが " + itoa(c.cfg.MaxSkew) + " を超える。置かない")
			return Result{Skew: c.Skew(), Reason: "制約を守れる区画が無く、守れないなら置かない設定"}
		}
		// 願いとして扱う設定なら、守れなくても置く。
		feasible = zones
		c.logf(name + " は制約を守れないが、置く設定なので置く")
	}

	// ② 候補のうち、いちばん少ない区画を選ぶ。同数なら名前順。
	best := feasible[0]
	for _, z := range feasible {
		if c.Count(z) < c.Count(best) {
			best = z
		}
	}

	// ③ 区画の中では、載っている数がいちばん少ないノードへ。
	var node *Node
	for _, n := range c.Nodes() {
		if n.Zone != best {
			continue
		}
		if node == nil || len(n.pods) < len(node.pods) {
			node = n
		}
	}
	node.pods = append(node.pods, name)
	c.Placed++
	return Result{Placed: true, Node: node.Name, Zone: best, Skew: c.Skew(),
		Reason: name + " を区画 " + best + " の " + node.Name + " へ"}
}

// skewAfter は zone に1つ置いた後の偏りを返す。
func (c *Cluster) skewAfter(zone string) int {
	min, max := -1, 0
	for _, z := range c.Zones() {
		n := c.Count(z)
		if z == zone {
			n++
		}
		if n > max {
			max = n
		}
		if min < 0 || n < min {
			min = n
		}
	}
	return max - min
}

// PlaceN は n 個を順に置く。
func (c *Cluster) PlaceN(prefix string, n int) []Result {
	out := make([]Result, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, c.Place(prefix+"-"+itoa(i)))
	}
	return out
}

// #endregion place

// #region survive

// LoseZone は区画を1つ失ったとき、残る Pod の数を返す。
// 散らし方の良し悪しは、最後はこの数で測る。
func (c *Cluster) LoseZone(zone string) int {
	total := 0
	for _, z := range c.Zones() {
		if z != zone {
			total += c.Count(z)
		}
	}
	return total
}

// WorstZoneLoss は最も痛い区画を失ったときに残る数を返す。
func (c *Cluster) WorstZoneLoss() int {
	worst := -1
	for _, z := range c.Zones() {
		if n := c.LoseZone(z); worst < 0 || n < worst {
			worst = n
		}
	}
	if worst < 0 {
		return 0
	}
	return worst
}

// LoseNode はノードを1台失ったとき、残る Pod の数を返す。
func (c *Cluster) LoseNode(name string) int {
	total := 0
	for _, n := range c.nodes {
		if n.Name != name {
			total += len(n.pods)
		}
	}
	return total
}

// #endregion survive

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
