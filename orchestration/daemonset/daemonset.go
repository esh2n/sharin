// Package daemonset は Kubernetes の DaemonSet を最小構成で実装する。
//
// この編の最初に作った調整ループは「Pod は3個であれ」と数を宣言した。
// スケジューラは、その3個をどこに置くかを決めた。数が先にあって、場所は
// 後から選ばれる。負荷を捌く仕事なら、これで筋が通っている。3個で足りる
// なら、どこに置いても3個ぶんの仕事はできる。
//
// だが、そうでない仕事もある。各ノードのログを集める、各ノードの状態を
// 監視する、各ノードのネットワークを設定する。これらは「3個」では足りない。
// ノードが5台あれば5個、10台あれば10個要る。しかも、どのノードに置くかを
// 選ぶ余地がない。ログを集めたいノードに置かなければ意味がないからだ。
//
// つまり、数と場所の関係が逆になっている。場所が先にあって、数は後から
// 決まる。ノードが増えれば勝手に増え、減れば勝手に減る。宣言するのは
// 「どこに要るか」で、数は宣言しない。
//
// この逆転が、いくつかの性質を連れてくる。スケジューラの出番がないこと、
// ノードの追加に自動で追随すること、そして汚れ(taint)の扱いが普通の Pod と
// 違うこと。監視や収集は、他の Pod が避けるノードにも置かれてほしい。
package daemonset

import "sort"

// #region model

// Node は Pod を置く1台。ラベルと汚れを持つ。
type Node struct {
	Name   string
	labels map[string]string
	taints []string
	// Ready が偽なら、そのノードには置けない。
	Ready bool
}

// matches はノードが selector をすべて満たすかを返す。
func (n *Node) matches(selector map[string]string) bool {
	for k, v := range selector {
		if n.labels[k] != v {
			return false
		}
	}
	return true
}

// tolerated は汚れがすべて許容されているかを返す。
func (n *Node) tolerated(tolerations []string) bool {
	for _, t := range n.taints {
		ok := false
		for _, tol := range tolerations {
			if tol == t || tol == "*" {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Spec は「どこに要るか」の宣言。数はどこにも書かない。
type Spec struct {
	Name string
	// Selector はどのノードに置くか。空ならすべてのノード。
	Selector map[string]string
	// Tolerations は許容する汚れ。"*" ならすべて。
	Tolerations []string
}

// Pod は1つのノードに置かれた実体。
type Pod struct {
	Name string
	Node string
}

// #endregion model

// #region reconcile

// Action は1回の調整で打った手。
type Action struct {
	Kind string // "create" / "delete"
	Node string
}

// Set は「対象のノードすべてに1つずつ」を保つ。
type Set struct {
	spec  Spec
	nodes []*Node
	pods  map[string]*Pod // ノード名 → Pod

	Log []string
}

// New は宣言 spec の集合を作る。
func New(spec Spec) *Set {
	return &Set{spec: spec, pods: map[string]*Pod{}}
}

// AddNode はノードを1台足す。次の調整で自動的に Pod が置かれる。
func (s *Set) AddNode(name string, ready bool, labels map[string]string, taints ...string) *Node {
	if labels == nil {
		labels = map[string]string{}
	}
	n := &Node{Name: name, labels: labels, taints: taints, Ready: ready}
	s.nodes = append(s.nodes, n)
	return n
}

// RemoveNode はノードを取り除く。載っていた Pod も一緒に消える。
func (s *Set) RemoveNode(name string) {
	for i, n := range s.nodes {
		if n.Name != name {
			continue
		}
		s.nodes = append(s.nodes[:i], s.nodes[i+1:]...)
		delete(s.pods, name)
		s.logf(name + " が取り除かれた(載っていた Pod も消える)")
		return
	}
}

// SetReady はノードの状態を変える。
func (s *Set) SetReady(name string, ready bool) {
	for _, n := range s.nodes {
		if n.Name == name {
			n.Ready = ready
			return
		}
	}
}

// Nodes はノードを名前順に返す。
func (s *Set) Nodes() []*Node {
	out := append([]*Node(nil), s.nodes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Pods は Pod をノード名順に返す。
func (s *Set) Pods() []*Pod {
	names := make([]string, 0, len(s.pods))
	for n := range s.pods {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Pod, len(names))
	for i, n := range names {
		out[i] = s.pods[n]
	}
	return out
}

// PodOn はノード name に Pod が載っているかを返す。
func (s *Set) PodOn(name string) bool { _, ok := s.pods[name]; return ok }

// Targets は置くべきノードを名前順に返す。ここが「どこに要るか」の答えで、
// この個数がそのまま必要な Pod の数になる。数は宣言せず、ここから導かれる。
func (s *Set) Targets() []string {
	var out []string
	for _, n := range s.Nodes() {
		if n.Ready && n.matches(s.spec.Selector) && n.tolerated(s.spec.Tolerations) {
			out = append(out, n.Name)
		}
	}
	return out
}

// Desired は必要な Pod の数を返す。宣言された数ではなく、対象ノードの数。
func (s *Set) Desired() int { return len(s.Targets()) }

// Reconcile は対象のノードすべてに1つずつ載っている状態へ寄せる。
//
// 調整ループと同じ形だが、数え方が違う。あちらは宣言された数と現状を比べた。
// こちらは対象のノードの集合と、Pod が載っているノードの集合を比べる。
// 数でなく集合の差を埋めるので、ノードが増えれば自動的に増える。
func (s *Set) Reconcile() []Action {
	var actions []Action

	want := map[string]bool{}
	for _, n := range s.Targets() {
		want[n] = true
	}

	// 置くべきなのに載っていないノードへ置く。
	for _, n := range s.Targets() {
		if !s.PodOn(n) {
			s.pods[n] = &Pod{Name: s.spec.Name + "-" + n, Node: n}
			actions = append(actions, Action{Kind: "create", Node: n})
			s.logf(n + " に " + s.pods[n].Name + " を作成")
		}
	}

	// 置くべきでないのに載っているノードから消す。
	for _, n := range sortedKeys(s.pods) {
		if !want[n] {
			s.logf(n + " から " + s.pods[n].Name + " を削除(対象から外れた)")
			delete(s.pods, n)
			actions = append(actions, Action{Kind: "delete", Node: n})
		}
	}
	return actions
}

// Converged は対象すべてに1つずつ載っているかを返す。
func (s *Set) Converged() bool {
	if len(s.pods) != s.Desired() {
		return false
	}
	for _, n := range s.Targets() {
		if !s.PodOn(n) {
			return false
		}
	}
	return true
}

// #endregion reconcile

func sortedKeys(m map[string]*Pod) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Set) logf(msg string) { s.Log = append(s.Log, msg) }
