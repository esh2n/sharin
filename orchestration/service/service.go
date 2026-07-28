// Package service は Kubernetes の Service と kube-proxy を最小構成で実装する。
//
// Pod は消えては生まれる。落ちれば作り直され、更新のたびに入れ替わり、その
// たびに IP が変わる。相手の IP を覚えて呼ぶ、という素朴なやり方は成り立たない。
// そこで、変わらない宛先を1つ用意する。それが Service で、ClusterIP という
// 仮想の IP を持つ。呼ぶ側はその IP だけを知っていればよく、後ろで Pod が
// 何度入れ替わっても影響を受けない。
//
// 面白いのは、この仮想 IP に対応する実体がどこにも無いことだ。誰もその IP で
// 待ち受けていない。ルータもプロキシもいない。あるのは、各ノードに配られた
// 「この宛先に出ていくパケットは、この実 IP へ書き換えよ」というルールだけ。
// 書き換えはパケットが出ていく瞬間に、そのノード自身の中で起こる。中央に
// 集約点が無いので、そこが混んだり落ちたりしない。
//
// そして、ルールは各ノードへ配られるものである以上、配り終えるまでの間がある。
// Pod を消してもルールはしばらく残り、その間パケットは死んだ宛先へ飛ぶ。
// 終了処理の章で「転送先一覧が現実に遅れる」と呼んだものの正体がこれになる。
package service

import "sort"

// #region model

// Pod は宛先になりうる1つの実体。ラベルで選ばれ、ready なら宛先になる。
type Pod struct {
	Name   string
	IP     string
	Ready  bool
	labels map[string]string
}

// matches は Pod が selector のラベルをすべて満たすかを返す。
func (p *Pod) matches(selector map[string]string) bool {
	for k, v := range selector {
		if p.labels[k] != v {
			return false
		}
	}
	return true
}

// Service は変わらない宛先。ClusterIP は仮想の IP で、実体を持たない。
type Service struct {
	Name      string
	ClusterIP string
	selector  map[string]string
}

// #endregion model

// #region dataplane

// Node は1台のマシン。kube-proxy が書いたルールを持つ。
//
// rules が「この仮想 IP 宛は、この実 IP のどれかへ書き換えよ」という表になる。
// これがノードごとに存在することが肝で、振り分けはパケットが出ていく
// ノードの中で完結する。中央に集約点がない。
type Node struct {
	Name  string
	rules map[string][]string // ClusterIP → 実 IP の並び
	rr    map[string]int      // 振り分けの順番(ノードごとに独立)
}

// NewNode はルールを持たないノードを作る。
func NewNode(name string) *Node {
	return &Node{Name: name, rules: map[string][]string{}, rr: map[string]int{}}
}

// Rules は clusterIP 宛のルール(実 IP の並び)を返す。
func (n *Node) Rules(clusterIP string) []string {
	return append([]string(nil), n.rules[clusterIP]...)
}

// RuleCount はこのノードが持つルールの本数を返す。
// 実物の iptables 方式では、この本数が Service と Pod の積で増えていく。
func (n *Node) RuleCount() int {
	c := 0
	for _, ips := range n.rules {
		c += len(ips)
	}
	return c
}

// Route は clusterIP 宛のパケットの宛先を、このノードのルールから選ぶ。
// ルールが無ければ書き換えようがないので、パケットは行き場を失う。
func (n *Node) Route(clusterIP string) (string, bool) {
	ips := n.rules[clusterIP]
	if len(ips) == 0 {
		return "", false
	}
	ip := ips[n.rr[clusterIP]%len(ips)]
	n.rr[clusterIP]++
	return ip, true
}

// #endregion dataplane

// #region cluster

// Config はルールが各ノードへ配られるまでの遅れ。
type Config struct {
	// Propagation は制御側が決めてから、各ノードのルールに反映されるまでの時間。
	Propagation int
}

// Cluster は Pod・Service・ノードと、ルールの配布を持つ。
type Cluster struct {
	cfg   Config
	pods  map[string]*Pod
	svcs  map[string]*Service
	nodes []*Node
	now   int

	queue []delivery // 配布中のルール更新

	Sent       int // 生きた宛先に届いたパケット
	Blackholed int // もう受けられない宛先へ送られたパケット
	Dropped    int // ルールが無く、行き場を失ったパケット
	Log        []string
}

// delivery は「いつ・どのノードへ・どのルールを」の配布予定。
type delivery struct {
	at        int
	node      *Node
	clusterIP string
	backends  []string
}

// New は空のクラスタを作る。
func New(cfg Config) *Cluster {
	return &Cluster{cfg: cfg, pods: map[string]*Pod{}, svcs: map[string]*Service{}}
}

// AddNode はノードを1台足す。今ある Service のルールが、遅れて配られる。
func (c *Cluster) AddNode(name string) *Node {
	n := NewNode(name)
	c.nodes = append(c.nodes, n)
	c.publish()
	return n
}

// Nodes はノード一覧を返す。
func (c *Cluster) Nodes() []*Node { return c.nodes }

// AddService は変わらない宛先を1つ作る。
func (c *Cluster) AddService(name, clusterIP string, selector map[string]string) *Service {
	s := &Service{Name: name, ClusterIP: clusterIP, selector: selector}
	c.svcs[name] = s
	c.publish()
	return s
}

// AddPod は宛先になりうる Pod を足す。
func (c *Cluster) AddPod(name, ip string, labels map[string]string, ready bool) *Pod {
	p := &Pod{Name: name, IP: ip, Ready: ready, labels: labels}
	c.pods[name] = p
	c.publish()
	return p
}

// SetReady は Pod の ready を切り替える。宛先から出入りする。
func (c *Cluster) SetReady(name string, ready bool) {
	if p, ok := c.pods[name]; ok {
		p.Ready = ready
		c.publish()
	}
}

// RemovePod は Pod を消す。ルールの更新は遅れて配られるので、その間
// 消えた Pod の IP がルールに残り続ける。
func (c *Cluster) RemovePod(name string) {
	delete(c.pods, name)
	c.publish()
}

// #endregion cluster

// #region select

// Endpoints は制御側から見た宛先一覧を返す。セレクタのラベルが合い、かつ
// ready な Pod の IP を名前順に並べたもの。
//
// ここは「あるべき宛先」であって、各ノードが今持っているルールとは別物になる。
// この2つがずれている間が、パケットの落ちる窓になる。
func (c *Cluster) Endpoints(svcName string) []string {
	s, ok := c.svcs[svcName]
	if !ok {
		return nil
	}
	var names []string
	for name, p := range c.pods {
		if p.Ready && p.matches(s.selector) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	ips := make([]string, 0, len(names))
	for _, n := range names {
		ips = append(ips, c.pods[n].IP)
	}
	return ips
}

// publish は今あるべき宛先を計算し、各ノードへの配布を予定する。
// 反映は Propagation のぶん先になる。制御側が決めた瞬間には変わらない。
func (c *Cluster) publish() {
	for _, s := range c.svcs {
		want := c.Endpoints(s.Name)
		for _, n := range c.nodes {
			c.queue = append(c.queue, delivery{
				at: c.now + c.cfg.Propagation, node: n, clusterIP: s.ClusterIP, backends: want,
			})
		}
	}
}

// #endregion select

// #region propagate

// Tick は時刻を1つ進め、配布の時刻に達したルールを各ノードへ書き込む。
func (c *Cluster) Tick() {
	c.now++
	var rest []delivery
	for _, d := range c.queue {
		if c.now < d.at {
			rest = append(rest, d)
			continue
		}
		d.node.rules[d.clusterIP] = d.backends
		if len(d.backends) == 0 {
			delete(d.node.rules, d.clusterIP)
		}
	}
	c.queue = rest
}

// Send は node から clusterIP 宛にパケットを1つ出す。
//
// 宛先を決めるのは、制御側の一覧ではなく、そのノードが今持っているルールだ。
// ルールが古ければ、もう受けられない相手へ書き換えられる。届かない。
func (c *Cluster) Send(node *Node, clusterIP string) bool {
	ip, ok := node.Route(clusterIP)
	if !ok {
		c.Dropped++
		c.logf(node.Name + " に " + clusterIP + " のルールがない。行き場を失う")
		return false
	}
	if p := c.podByIP(ip); p == nil || !p.Ready {
		c.Blackholed++
		c.logf(node.Name + " のルールが " + ip + " を指しているが、そこはもう受けられない")
		return false
	}
	c.Sent++
	return true
}

// #endregion propagate

// Converged は全ノードのルールが、あるべき宛先と一致しているかを返す。
func (c *Cluster) Converged() bool {
	for _, s := range c.svcs {
		want := c.Endpoints(s.Name)
		for _, n := range c.nodes {
			if !sameIPs(n.rules[s.ClusterIP], want) {
				return false
			}
		}
	}
	return true
}

func (c *Cluster) podByIP(ip string) *Pod {
	for _, p := range c.pods {
		if p.IP == ip {
			return p
		}
	}
	return nil
}

func sameIPs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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
