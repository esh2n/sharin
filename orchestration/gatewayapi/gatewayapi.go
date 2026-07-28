// Package gatewayapi は Gateway API を最小構成で実装する。
//
// [Ingress](ingress) は入口を1つに束ねて、ホスト名とパスで中へ振り分けた。
// 振り分けの仕組みとしてはこれで足りている。足りなかったのは別のところで、
// 1つのオブジェクトに全部が入っていることだった。
//
// 証明書、公開するホスト名、どのポートで待つか。これらはクラスタを運用する側の
// 持ち物になる。一方、/api を自分のサービスへ向ける、といった規則はアプリを
// 作る側の持ち物だ。Ingress ではこれが同じオブジェクトに同居しているので、
// アプリ側に書かせようとすると入口ごと触れてしまうし、運用側が抱えると
// 規則を足すたびに依頼が要る。
//
// Gateway API はこれを役割で分ける。入口は Gateway、振り分けは HTTPRoute。
// 別のオブジェクトなので、[RBAC](rbac) で別々に権限を切れる。
//
// 分けたぶん、繋ぎ方が問題になる。ここで面白い形が出てくる。Route が親の
// Gateway を指名し、Gateway が「どの名前空間の Route を受け入れるか」を宣言する。
// 両方が同意しなければ繋がらない。片側だけでは、勝手に他人の入口へ相乗りする
// ことも、勝手に他人の規則を取り込むこともできない。
//
// もう1つ Ingress に無かったのが、振り分けの順序が仕様で決まっていることだ。
// Ingress の細かい挙動は実装ごとに違い、annotation で補うのが普通だった。
package gatewayapi

import "sort"

// #region objects

// Listener は Gateway が待ち受ける1つの口。運用側の持ち物になる。
type Listener struct {
	Name     string
	Port     int
	Hostname string // "" はすべて、"*.example.com" のような形も書ける
	// AllowedFrom は受け入れる Route の名前空間。空なら Gateway と同じ名前空間だけ。
	AllowedFrom []string
	// AllowAll はすべての名前空間から受け入れる。
	AllowAll bool
}

// Gateway は入口。どのポートでどのホスト名を待つか、誰の規則を受け入れるかを持つ。
type Gateway struct {
	Name      string
	Namespace string
	Listeners []Listener
}

// Backend は振り分け先。Weight は同じ規則の中での取り分になる。
type Backend struct {
	Service string
	Port    int
	Weight  int
}

// Match は1つの当たり判定。指定した項目がすべて一致したときだけ当たる。
type Match struct {
	// PathType は "Exact" か "PathPrefix"。空なら "PathPrefix" として扱う。
	PathType string
	Path     string
	Headers  map[string]string
	Query    map[string]string
	Method   string
}

// Rule は当たり判定と振り分け先の組。
type Rule struct {
	Matches  []Match
	Backends []Backend
}

// HTTPRoute は振り分けの規則。アプリ側の持ち物になる。
type HTTPRoute struct {
	Name      string
	Namespace string
	// ParentRefs は繋ぎたい Gateway の名前。指名した側の同意になる。
	ParentRefs []string
	Hostnames  []string
	Rules      []Rule
}

// #endregion objects

// #region attach

// Attachment は 1 つの Route が 1 つの Gateway に繋がったかどうかの結果。
type Attachment struct {
	Route    string
	Gateway  string
	Listener string
	Attached bool
	Why      string // 繋がらなかった理由
}

// attach は Route を Gateway に繋げられるかを判定する。
//
// 判定は3つで、どれも「双方が同意しているか」を見ている。Route が親を指名して
// いること、Listener がその名前空間を許していること、ホスト名が重なっていること。
// 片方だけの意思では繋がらないので、他人の入口に勝手に相乗りできない。
func attach(g Gateway, r HTTPRoute) Attachment {
	a := Attachment{Route: r.Namespace + "/" + r.Name, Gateway: g.Name}

	named := false
	for _, p := range r.ParentRefs {
		if p == g.Name {
			named = true
			break
		}
	}
	if !named {
		a.Why = "Route が この Gateway を親に指名していない"
		return a
	}

	var lastWhy string
	for _, l := range g.Listeners {
		if !l.allows(g.Namespace, r.Namespace) {
			lastWhy = "Listener " + l.Name + " が名前空間 " + r.Namespace + " を受け入れていない"
			continue
		}
		if !hostsIntersect(l.Hostname, r.Hostnames) {
			lastWhy = "Listener " + l.Name + " のホスト名 " + orAny(l.Hostname) + " と重ならない"
			continue
		}
		a.Listener = l.Name
		a.Attached = true
		return a
	}
	a.Why = lastWhy
	if a.Why == "" {
		a.Why = "受け入れる Listener が無い"
	}
	return a
}

// allows は Listener がその名前空間の Route を受け入れるかを返す。
// 既定は「同じ名前空間だけ」で、開くには明示が要る。
func (l Listener) allows(gatewayNS, routeNS string) bool {
	if l.AllowAll {
		return true
	}
	if len(l.AllowedFrom) == 0 {
		return routeNS == gatewayNS
	}
	for _, ns := range l.AllowedFrom {
		if ns == routeNS {
			return true
		}
	}
	return false
}

// hostsIntersect は Listener のホスト名と Route のホスト名が重なるかを返す。
func hostsIntersect(listener string, routes []string) bool {
	if len(routes) == 0 {
		return true // Route がホストを指定しなければ Listener に従う
	}
	for _, h := range routes {
		if hostMatch(listener, h) || hostMatch(h, listener) {
			return true
		}
	}
	return false
}

// hostMatch は pattern が host を含むかを返す。pattern が空ならすべてを含む。
func hostMatch(pattern, host string) bool {
	if pattern == "" || pattern == host {
		return true
	}
	if len(pattern) > 2 && pattern[0] == '*' && pattern[1] == '.' {
		suffix := pattern[1:] // ".example.com"
		return len(host) > len(suffix) && host[len(host)-len(suffix):] == suffix
	}
	return false
}

func orAny(h string) string {
	if h == "" {
		return "(指定なし)"
	}
	return h
}

// #endregion attach

// #region cluster

// Cluster は Gateway と Route を集めて、繋がりと振り分けを決める。
type Cluster struct {
	gateways []Gateway
	routes   []HTTPRoute
	hits     map[string]int // 重み付き分岐を決定的に回すための計数
}

// New は空のクラスタを作る。
func New() *Cluster { return &Cluster{hits: map[string]int{}} }

// AddGateway は入口を足す。
func (c *Cluster) AddGateway(g Gateway) { c.gateways = append(c.gateways, g) }

// AddRoute は規則を足す。
func (c *Cluster) AddRoute(r HTTPRoute) { c.routes = append(c.routes, r) }

// Attachments は Gateway 名順、Route 名順に、繋がりの判定を返す。
func (c *Cluster) Attachments() []Attachment {
	var out []Attachment
	for _, g := range c.gateways {
		for _, r := range c.routes {
			out = append(out, attach(g, r))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Gateway != out[j].Gateway {
			return out[i].Gateway < out[j].Gateway
		}
		return out[i].Route < out[j].Route
	})
	return out
}

// #endregion cluster

// #region request

// Request は入ってきた1件。
type Request struct {
	Gateway string
	Host    string
	Path    string
	Method  string
	Headers map[string]string
	Query   map[string]string
}

// Result は振り分けの結果と、そこに至った理由。
type Result struct {
	Backend  Backend
	Route    string
	Found    bool
	Why      string
	Priority Priority // 勝った規則の特定度
}

// Priority は仕様で定められた優先順位を数値にしたもの。
// 大きいほど優先される。比較は上から順に見る。
type Priority struct {
	ExactHost  int // ホスト名が完全一致なら 1、ワイルドカードなら 0
	ExactPath  int // Exact なら 1、PathPrefix なら 0
	PathLen    int // パスの長さ
	HeaderHits int
	QueryHits  int
	Method     int
}

// beats は p が q より優先されるかを返す。上から順に見て、最初に差がついた
// ところで決まる。ここが仕様で決まっていることが Ingress との一番の違いになる。
func (p Priority) beats(q Priority) bool {
	for _, d := range [][2]int{
		{p.ExactHost, q.ExactHost},
		{p.ExactPath, q.ExactPath},
		{p.PathLen, q.PathLen},
		{p.Method, q.Method},
		{p.HeaderHits, q.HeaderHits},
		{p.QueryHits, q.QueryHits},
	} {
		if d[0] != d[1] {
			return d[0] > d[1]
		}
	}
	return false
}

// #endregion request

// #region route

type winner struct {
	route string
	rule  Rule
	prio  Priority
}

// Route は1件のリクエストを振り分ける。
//
// 繋がっている Route だけを見る。繋がっていない Route の規則は、どれだけ
// 一致していても使われない。これが役割を分けたことの意味になる。
func (c *Cluster) Route(req Request) Result {
	var best *winner
	for _, g := range c.gateways {
		if g.Name != req.Gateway {
			continue
		}
		for _, r := range c.routes {
			if !attach(g, r).Attached {
				continue
			}
			if !hostsIntersect(req.Host, r.Hostnames) {
				continue
			}
			for _, rule := range r.Rules {
				p, ok := matchRule(rule, r, req)
				if !ok {
					continue
				}
				cand := winner{route: r.Namespace + "/" + r.Name, rule: rule, prio: p}
				if best == nil || cand.prio.beats(best.prio) ||
					(!best.prio.beats(cand.prio) && cand.route < best.route) {
					b := cand
					best = &b
				}
			}
		}
	}
	if best == nil {
		return Result{Why: "どの規則にも当たらない"}
	}
	return Result{
		Backend:  c.pick(best.route, best.rule.Backends),
		Route:    best.route,
		Found:    true,
		Priority: best.prio,
	}
}

// matchRule は規則が当たるかを判定し、当たったときの特定度を返す。
// 規則の中の Matches は「どれか1つ当たればよい」で、その中で最も特定度の
// 高いものを採る。
func matchRule(rule Rule, r HTTPRoute, req Request) (Priority, bool) {
	var best Priority
	found := false
	for _, m := range rule.Matches {
		p, ok := matchOne(m, r, req)
		if !ok {
			continue
		}
		if !found || p.beats(best) {
			best = p
			found = true
		}
	}
	return best, found
}

func matchOne(m Match, r HTTPRoute, req Request) (Priority, bool) {
	var p Priority

	if m.PathType == "Exact" {
		if m.Path != req.Path {
			return p, false
		}
		p.ExactPath = 1
	} else if !prefixOf(m.Path, req.Path) {
		return p, false
	}
	p.PathLen = len(m.Path)

	if m.Method != "" {
		if m.Method != req.Method {
			return p, false
		}
		p.Method = 1
	}
	for k, v := range m.Headers {
		if req.Headers[k] != v {
			return p, false
		}
		p.HeaderHits++
	}
	for k, v := range m.Query {
		if req.Query[k] != v {
			return p, false
		}
		p.QueryHits++
	}

	// ホスト名の特定度。完全一致で書かれているほうが強い。
	p.ExactHost = 0
	for _, h := range r.Hostnames {
		if h == req.Host {
			p.ExactHost = 1
			break
		}
	}
	return p, true
}

// prefixOf はパスの前方一致を、区切りを跨がない形で判定する。
// "/api" は "/api/users" に当たるが "/apiary" には当たらない。
func prefixOf(prefix, path string) bool {
	if prefix == "" || prefix == "/" {
		return true
	}
	if len(path) < len(prefix) || path[:len(prefix)] != prefix {
		return false
	}
	return len(path) == len(prefix) || path[len(prefix)] == '/'
}

// pick は重みに従って振り分け先を選ぶ。
//
// 乱数を使わず、その規則に何件目かを数えて割り当てる。同じ順で投げれば
// 同じ結果になるので、比率が本当に守られているかをテストで確かめられる。
func (c *Cluster) pick(key string, backends []Backend) Backend {
	if len(backends) == 0 {
		return Backend{}
	}
	if len(backends) == 1 {
		return backends[0]
	}
	sorted := append([]Backend(nil), backends...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Service < sorted[j].Service })

	total := 0
	for _, b := range sorted {
		total += b.Weight
	}
	if total <= 0 {
		return sorted[0]
	}
	n := c.hits[key] % total
	c.hits[key]++
	acc := 0
	for _, b := range sorted[:len(sorted)-1] {
		acc += b.Weight
		if n < acc {
			return b
		}
	}
	return sorted[len(sorted)-1]
}

// #endregion route
