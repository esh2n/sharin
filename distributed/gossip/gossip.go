// Package gossip はゴシップによる伝播と障害検知(SWIM)を最小構成で実装する。
//
// [CRDT](crdt)の章で、後からまとめられる形を作った。まとめ方が決まっていれば、
// どの順で届いても同じ値に落ち着く。だがそこには、そもそも誰に届けるのか、
// という話が無かった。
//
// 素朴には全員に配ればよい。だがそれだと、1回の更新で台数ぶんの通信が出る。
// 全員が全員を見張ると、通信は台数の2乗になる。100台なら1万通りだ。
//
// ゴシップの答えは、毎周期ランダムに1台だけ選んで話しかけることになる。
// 1台が出す通信は台数に関係なく一定で、それでも知らせは全体に広まる。
//
// もう1つ、この仕組みが同時に解いているのが障害検知になる。話しかけて返事が
// 無ければ死んだ、としたいところだが、それでは足りない。返事が無いのは、
// 相手が死んだからかもしれないし、自分と相手の間だけが切れているからかもしれない。
// 1台では区別できない。
//
// だから他の何台かに「あなたから話しかけてみて」と頼む。誰か1人でも返事を
// もらえたなら、相手は生きている。そして、それでも駄目なときも即断せず、
// まず疑いという中間状態に置く。本人が「生きている」と反論する機会を残す。
package gossip

import "sort"

// #region member

// State は、あるノードから見た他のノードの様子。
type State int

const (
	// Alive は生きていると見ている。
	Alive State = iota
	// Suspect は疑わしい。まだ死んだとは決めていない。
	Suspect
	// Dead は死んだと決めた。
	Dead
)

func (s State) String() string {
	return [...]string{"生きている", "疑わしい", "死んだ"}[s]
}

// Member は1台についての見立て。
//
// Inc は本人だけが上げられる番号になる。疑いをかけられた本人が「生きている」と
// 反論するとき、この番号を1つ上げて広める。番号が大きいほうが新しいので、
// 古い疑いを上書きできる。[論理時計](clock)の単調な数え上げと同じ役目になる。
type Member struct {
	Name  string
	State State
	Inc   int
}

// Merge は2つの見立てを1つにする。
//
// 番号が大きいほうが勝つ。同じなら、悪い知らせのほうが勝つ。
// この規則は可換で結合的で冪等なので、[CRDT](crdt)と同じように、
// どの順で何度届いても同じところへ落ち着く。
func Merge(a, b Member) Member {
	switch {
	case a.Inc > b.Inc:
		return a
	case b.Inc > a.Inc:
		return b
	case a.State >= b.State:
		return a
	default:
		return b
	}
}

// #endregion member

// #region node

// Node は1台。他の全員についての見立てを持つ。
type Node struct {
	Name string
	view map[string]Member
	// suspectAt は疑い始めた周期。一定の周期を過ぎても反論が無ければ死んだと決める。
	suspectAt map[string]int
}

func newNode(name string, all []string) *Node {
	n := &Node{Name: name, view: map[string]Member{}, suspectAt: map[string]int{}}
	for _, m := range all {
		n.view[m] = Member{Name: m, State: Alive}
	}
	return n
}

// View はそのノードから見た1台の様子を返す。
func (n *Node) View(name string) Member { return n.view[name] }

// Members は見立てを名前順で返す。
func (n *Node) Members() []Member {
	var out []Member
	for _, m := range n.view {
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// apply は受け取った見立てを取り込む。
func (n *Node) apply(m Member) {
	cur, ok := n.view[m.Name]
	if !ok {
		n.view[m.Name] = m
		return
	}
	next := Merge(cur, m)
	if next.State != Suspect {
		delete(n.suspectAt, m.Name)
	}
	n.view[m.Name] = next
}

// #endregion node

// #region sim

// Config は検知の性質を決める。
type Config struct {
	// Indirect は、返事が無かったときに頼む相手の数。
	Indirect int
	// SuspectFor は、疑ってから死んだと決めるまでの周期数。
	SuspectFor int
}

// Sim は台数ぶんのノードを、周期を1つずつ進めながら動かす。
type Sim struct {
	cfg   Config
	names []string
	nodes map[string]*Node

	down  map[string]bool            // 落ちているノード
	block map[string]map[string]bool // 一方向に届かない組

	rnd  uint64
	tick int

	// Pings はこの系で行われた問い合わせの総数。台数への依存を見るために数える。
	Pings int
	Log   []string
}

// New は台数と設定から系を作る。seed で選び方を固定するので、結果は再現する。
func New(cfg Config, seed uint64, names ...string) *Sim {
	s := &Sim{cfg: cfg, nodes: map[string]*Node{}, down: map[string]bool{},
		block: map[string]map[string]bool{}, rnd: seed | 1}
	s.names = append([]string(nil), names...)
	sort.Strings(s.names)
	for _, n := range s.names {
		s.nodes[n] = newNode(n, s.names)
	}
	return s
}

// next は seed から決まる乱数を返す(実時間を使わないための線形合同法)。
func (s *Sim) next(n int) int {
	s.rnd = s.rnd*6364136223846793005 + 1442695040888963407
	return int((s.rnd >> 33) % uint64(n))
}

// Node は1台を返す。
func (s *Sim) Node(name string) *Node { return s.nodes[name] }

// Tick は現在の周期を返す。
func (s *Sim) Tick() int { return s.tick }

// Kill はノードを落とす。落ちたノードは返事をしない。
func (s *Sim) Kill(name string) {
	s.down[name] = true
	s.logf(name + " が落ちた")
}

// Revive はノードを起こす。起きたノードは番号を上げて生存を主張する。
//
// 主張できるのは本人だけになる。他人は疑うことしかできない。
func (s *Sim) Revive(name string) {
	delete(s.down, name)
	n := s.nodes[name]
	me := n.view[name]
	me.Inc++
	me.State = Alive
	n.view[name] = me
	s.logf(name + " が起きて、番号 " + itoa(me.Inc) + " で生存を主張する")
}

// Block は from から to への通信だけを止める。両方とも生きているのに届かない状態。
func (s *Sim) Block(from, to string) {
	if s.block[from] == nil {
		s.block[from] = map[string]bool{}
	}
	s.block[from][to] = true
	s.logf(from + " から " + to + " へ届かなくなった(どちらも生きている)")
}

// reachable は from から to へ問い合わせが通るかを返す。
func (s *Sim) reachable(from, to string) bool {
	if s.down[from] || s.down[to] {
		return false
	}
	return !s.block[from][to]
}

// #endregion sim

// #region round

// Round は1周期進める。
//
// 各ノードが1台だけ選んで問い合わせる。返事が無ければ他の何台かに頼み、
// それでも駄目なら疑う。疑ってから一定の周期が過ぎたら死んだと決める。
func (s *Sim) Round() {
	for _, name := range s.names {
		if s.down[name] {
			continue
		}
		s.probe(s.nodes[name])
		s.expire(s.nodes[name])
	}
	s.tick++
}

// probe は1台を選んで問い合わせる。
func (s *Sim) probe(n *Node) {
	target := s.pick(n)
	if target == "" {
		return
	}

	s.Pings++
	if s.reachable(n.Name, target) {
		s.exchange(n, s.nodes[target])
		return
	}

	// 直接は届かない。他の何台かに頼んでみる。
	// ここを飛ばすと、自分と相手の間だけが切れている場合も死んだことにしてしまう。
	if s.askOthers(n, target) {
		s.exchange(n, s.nodes[target])
		return
	}

	cur := n.view[target]
	if cur.State == Alive {
		cur.State = Suspect
		n.view[target] = cur
		n.suspectAt[target] = s.tick
		s.logf(n.Name + " が " + target + " を疑い始めた")
	}
}

// pick は問い合わせる相手を1台選ぶ。死んだと決めた相手は選ばない。
func (s *Sim) pick(n *Node) string {
	var cand []string
	for _, m := range s.names {
		if m != n.Name && n.view[m].State != Dead {
			cand = append(cand, m)
		}
	}
	if len(cand) == 0 {
		return ""
	}
	return cand[s.next(len(cand))]
}

// askOthers は他の何台かに、代わりに問い合わせてもらう。
//
// 誰か1人でも届いたなら、相手は生きている。自分に届かないことと、
// 相手が死んでいることを、ここで初めて区別できる。
func (s *Sim) askOthers(n *Node, target string) bool {
	asked := 0
	for _, m := range s.names {
		if asked >= s.cfg.Indirect {
			break
		}
		if m == n.Name || m == target || s.down[m] {
			continue
		}
		asked++
		s.Pings++
		if s.reachable(m, target) {
			s.logf(n.Name + " は " + target + " に届かないが、" + m + " からは届いた")
			return true
		}
	}
	return false
}

// exchange は互いの見立てを交換する。知らせはこれに相乗りして広まる。
func (s *Sim) exchange(a, b *Node) {
	for _, m := range b.Members() {
		a.apply(m)
	}
	for _, m := range a.Members() {
		b.apply(m)
	}
}

// expire は、疑ってから一定の周期が過ぎた相手を死んだと決める。
func (s *Sim) expire(n *Node) {
	for _, m := range n.Members() {
		if m.State != Suspect {
			continue
		}
		since, ok := n.suspectAt[m.Name]
		if !ok {
			n.suspectAt[m.Name] = s.tick
			continue
		}
		if s.tick-since >= s.cfg.SuspectFor {
			m.State = Dead
			n.view[m.Name] = m
			delete(n.suspectAt, m.Name)
			s.logf(n.Name + " が " + m.Name + " を死んだと決めた")
		}
	}
}

// #endregion round

// #region observe

// Agreed は、生きている全員が name について同じ見立てを持っているかを返す。
func (s *Sim) Agreed(name string) (State, bool) {
	var first State
	set := false
	for _, n := range s.names {
		if s.down[n] {
			continue
		}
		st := s.nodes[n].view[name].State
		if !set {
			first, set = st, true
			continue
		}
		if st != first {
			return first, false
		}
	}
	return first, set
}

// RunUntilAgreed は全員の見立てがそろうまで進める(上限つき)。
// そろった周期数を返す。そろわなければ -1。
func (s *Sim) RunUntilAgreed(name string, want State, limit int) int {
	for i := 0; i < limit; i++ {
		if got, ok := s.Agreed(name); ok && got == want {
			return i
		}
		s.Round()
	}
	if got, ok := s.Agreed(name); ok && got == want {
		return limit
	}
	return -1
}

// #endregion observe

func (s *Sim) logf(msg string) { s.Log = append(s.Log, "t="+itoa(s.tick)+" "+msg) }

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
