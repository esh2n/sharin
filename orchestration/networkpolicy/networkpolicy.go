// Package networkpolicy は Kubernetes の NetworkPolicy を最小構成で実装する。
//
// ここまでの章は、Pod をどう作り、どこに置き、どう繋ぐかを扱ってきた。
// 繋ぐところまでは Service がやったが、そこには誰が誰に繋いでよいかの話が
// 無かった。実際、既定では全部が全部に繋がる。web も、api も、db も、
// 監視の agent も、互いに素通しになっている。
//
// これは意図された既定で、そうでないと何も動かないからだ。だが本番では
// 困る。web が乗っ取られたら、そこから db に直接繋がれる。db に必要なのは
// api からの接続だけで、web から繋がる理由はどこにもない。
//
// NetworkPolicy は、この「繋いでよい相手」をラベルの条件で宣言する。
// 肝は既定の反転にある。ある Pod に対して1つでも方針が向けられた瞬間、
// その Pod への通信は既定で拒否になり、明示的に許した分だけが通る。
// 方針を書くとは、許可を足すことであると同時に、既定を落とすことでもある。
package networkpolicy

import "sort"

// #region model

// Pod は通信の端点。ラベルで選ばれる。
type Pod struct {
	Name   string
	labels map[string]string
}

// matches は Pod が selector をすべて満たすかを返す。
// 空の selector はすべてに一致する(全 Pod を指す書き方)。
func (p *Pod) matches(selector map[string]string) bool {
	for k, v := range selector {
		if p.labels[k] != v {
			return false
		}
	}
	return true
}

// Rule は「この条件の相手からなら通してよい」という許可1つ。
type Rule struct {
	From map[string]string // 送り元のラベル条件
	Port int               // 0 ならすべてのポート
}

// Policy はある条件の Pod への通信について、許可する相手を並べたもの。
//
// 向きに注意が要る。Selector は「守られる側」で、Rules は「入ってくる側」を
// 指す。つまり方針は受け側に付き、送り側には付かない。
type Policy struct {
	Name     string
	Selector map[string]string
	Rules    []Rule
}

// #endregion model

// #region decide

// Verdict は1回の判定の結果と、その理由。
type Verdict struct {
	Allowed bool
	Reason  string
	Policy  string // 効いた方針の名前(既定で通った場合は空)
}

// Cluster は Pod と方針を持ち、通信の可否を判定する。
type Cluster struct {
	pods     map[string]*Pod
	policies []*Policy

	Allowed int
	Denied  int
	Log     []string
}

// New は空のクラスタを作る。方針が1つも無いうちは、すべて通る。
func New() *Cluster { return &Cluster{pods: map[string]*Pod{}} }

// AddPod は端点を1つ足す。
func (c *Cluster) AddPod(name string, labels map[string]string) *Pod {
	p := &Pod{Name: name, labels: labels}
	c.pods[name] = p
	return p
}

// AddPolicy は方針を1つ足す。この瞬間、選ばれた Pod への通信は
// 既定で拒否に変わる。許可を足したつもりが、同時に既定を落としている。
func (c *Cluster) AddPolicy(p *Policy) *Policy {
	c.policies = append(c.policies, p)
	c.logf("方針 " + p.Name + " を追加(選ばれた Pod への通信は既定で拒否になる)")
	return p
}

// Pods は Pod を名前順に返す。
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

// Protected は dst が1つでも方針に選ばれているかを返す。
// 選ばれていれば既定は拒否、そうでなければ既定は許可になる。
func (c *Cluster) Protected(dst *Pod) bool {
	for _, pol := range c.policies {
		if dst.matches(pol.Selector) {
			return true
		}
	}
	return false
}

// Decide は src から dst の port への通信が通るかを判定する。
//
// 判定は2段になっている。まず dst を選ぶ方針が1つでもあるかを見る。
// 無ければ既定のまま通す。あれば既定は拒否に変わり、その方針のどれか1つが
// src を許可しているときだけ通す。許可は足し算で、1つでも合えば通る。
// 逆に言えば、方針を足しても他の方針で許されている通信は止まらない。
func (c *Cluster) Decide(src, dst *Pod, port int) Verdict {
	if !c.Protected(dst) {
		return Verdict{Allowed: true, Reason: dst.Name + " に向けられた方針が無いので既定で通る"}
	}
	for _, pol := range c.policies {
		if !dst.matches(pol.Selector) {
			continue
		}
		for _, r := range pol.Rules {
			if !src.matches(r.From) {
				continue
			}
			if r.Port != 0 && r.Port != port {
				continue
			}
			return Verdict{Allowed: true, Policy: pol.Name,
				Reason: pol.Name + " が " + src.Name + " からの接続を許可している"}
		}
	}
	return Verdict{Reason: dst.Name + " は方針で守られていて、" + src.Name + " を許可する規則が無い"}
}

// Connect は判定したうえで数を記録する。
func (c *Cluster) Connect(srcName, dstName string, port int) Verdict {
	src, dst := c.pods[srcName], c.pods[dstName]
	if src == nil || dst == nil {
		return Verdict{Reason: "端点が見つからない"}
	}
	v := c.Decide(src, dst, port)
	if v.Allowed {
		c.Allowed++
	} else {
		c.Denied++
		c.logf(srcName + " → " + dstName + ":" + itoa(port) + " を遮断(" + v.Reason + ")")
	}
	return v
}

// #endregion decide

// #region matrix

// Edge は誰から誰への通信が通るかの1マス。
type Edge struct {
	From    string
	To      string
	Allowed bool
	Policy  string
}

// Matrix は全 Pod の組み合わせについて可否を返す。
// 方針を足したときに、どこが閉じてどこが開いたままかを一望するために使う。
func (c *Cluster) Matrix(port int) []Edge {
	pods := c.Pods()
	var out []Edge
	for _, src := range pods {
		for _, dst := range pods {
			if src == dst {
				continue
			}
			v := c.Decide(src, dst, port)
			out = append(out, Edge{From: src.Name, To: dst.Name, Allowed: v.Allowed, Policy: v.Policy})
		}
	}
	return out
}

// #endregion matrix

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
