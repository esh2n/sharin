// Package ingress は Kubernetes の Ingress を最小構成で実装する。
//
// Service はクラスタの中で使う宛先だった。仮想 IP を1つ用意して、後ろの Pod へ
// 振り分ける。だが外から来る利用者は、仮想 IP を知らないし、そもそもクラスタの
// 中に入れない。外からの入口が別に要る。
//
// 素朴には、Service を1つずつ外へ出せばよさそうに見える。だが Service の
// 単位で外へ出すと、外から見える口がサービスの数だけできる。証明書も、
// ドメインも、その数だけ要る。10 個のサービスがあれば 10 個の入口を運用する
// ことになる。
//
// Ingress は、入口を1つに束ねてから中で振り分ける。同じホスト名の下で
// /api は api サービスへ、/ は web サービスへ。ここで扱うのは IP とポートでは
// なく、ホスト名とパスになる。層が1つ上がっていて、そのぶん何ができるかが
// 変わる。
//
// 振り分けの規則には順序の問題がつきまとう。/api と / のどちらも "/api/users"
// に当たるからだ。ここでは長いパスを優先する。書いた順でなく特定度で決まる
// ので、規則を足す順番を気にしなくてよくなる。
package ingress

import "sort"

// #region model

// Backend は振り分け先の Service。ここから先は Service の仕事になる。
type Backend struct {
	Service string
	Port    int
}

// Rule は「このホストのこのパスは、この Service へ」という規則1つ。
type Rule struct {
	Host string // 空ならすべてのホストに一致
	Path string // 前方一致。"/" はすべてに当たる
	Backend
}

// specificity は規則の特定度を返す。大きいほど優先される。
// ホストを指定しているほうが、パスが長いほうが、より特定的とみなす。
func (r Rule) specificity() (int, int) {
	host := 0
	if r.Host != "" {
		host = 1
	}
	return host, len(r.Path)
}

// matches は host / path がこの規則に当たるかを返す。
func (r Rule) matches(host, path string) bool {
	if r.Host != "" && r.Host != host {
		return false
	}
	return hasPrefix(path, r.Path)
}

// #endregion model

// #region route

// Result は1回の振り分けの結果。
type Result struct {
	Matched bool
	Backend Backend
	Rule    Rule
	Reason  string
}

// Ingress は入口1つと、その下の振り分け規則を持つ。
type Ingress struct {
	rules   []Rule
	Default *Backend // どの規則にも当たらないときの行き先(nil なら 404)

	Routed   int
	NotFound int
	Log      []string
}

// New は規則を持たない入口を作る。
func New() *Ingress { return &Ingress{} }

// Add は規則を1つ足す。足すたびに特定度の高い順へ並べ直すので、
// 書いた順は結果に影響しない。
func (i *Ingress) Add(r Rule) {
	if r.Path == "" {
		r.Path = "/"
	}
	i.rules = append(i.rules, r)
	sort.SliceStable(i.rules, func(a, b int) bool {
		ha, la := i.rules[a].specificity()
		hb, lb := i.rules[b].specificity()
		if ha != hb {
			return ha > hb // ホスト指定があるほうが先
		}
		if la != lb {
			return la > lb // パスが長いほうが先
		}
		return i.rules[a].Service < i.rules[b].Service // 同点は名前順(決定的)
	})
}

// Rules は評価される順に規則を返す。
func (i *Ingress) Rules() []Rule { return append([]Rule(nil), i.rules...) }

// Route は host / path のリクエストをどの Service へ渡すかを決める。
//
// 上から順に見て、最初に当たった規則を採る。並びは特定度順なので、
// より特定的な規則が先に当たる。/api と / の両方がある状態で "/api/users"
// が来たら、長いほうの /api が勝つ。
func (i *Ingress) Route(host, path string) Result {
	for _, r := range i.rules {
		if !r.matches(host, path) {
			continue
		}
		i.Routed++
		return Result{Matched: true, Backend: r.Backend, Rule: r,
			Reason: "規則 " + label(r) + " に一致"}
	}
	if i.Default != nil {
		i.Routed++
		return Result{Matched: true, Backend: *i.Default, Reason: "どの規則にも当たらず既定へ"}
	}
	i.NotFound++
	i.logf(host + path + " に当たる規則がない")
	return Result{Reason: "当たる規則がなく、既定も無い"}
}

// #endregion route

// label は規則を読める形にする(説明用)。
func label(r Rule) string {
	h := r.Host
	if h == "" {
		h = "*"
	}
	return h + r.Path + " → " + r.Service + ":" + itoa(r.Port)
}

// hasPrefix は s が prefix で始まるかを返す(strings を避ける)。
func hasPrefix(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (i *Ingress) logf(msg string) { i.Log = append(i.Log, msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	k := len(buf)
	for n > 0 {
		k--
		buf[k] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[k:])
}
