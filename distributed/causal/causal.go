// Package causal は、原因の順を守って配るしくみを最小構成で実装する。
//
// [論理時計](clock)の章で、どちらが先かを見分ける道具を作った。だがそこで
// 分かるのは「見分けられる」ところまでで、それを使って何をするかは別の話になる。
//
// 使いどころが配送になる。放送で流したメッセージは、届く順が送った順と違う。
// 経路が違えば追い越しが起きるし、届かずに再送されることもある。すると、
// ある投稿への返信が、その投稿より先に見えてしまう。
//
// 送り手ごとに通し番号を振れば、同じ相手からの追い越しは直せる。だが
// 返信は別の相手から来るので、これでは直らない。原因と結果が別の送り手に
// またがっているところが、この問題の要点になる。
//
// 直し方は、届いてもすぐには渡さないことになる。メッセージに「送った時点で
// 何を見ていたか」を載せておき、受け取り側はそれが自分の見たものに収まるまで
// 保留する。収まったら渡す。判定は数え上げの比較だけで書ける。
//
// 実時間も乱数も使わない。誰にいつ届くかは呼び出し側が明示するので、
// 結果は必ず再現する。
package causal

import "github.com/esh2n/sharin/distributed/clock"

// #region mode

// Mode は配る順の決め方。
type Mode int

const (
	// Fifo は送り手ごとの順だけを守る。同じ相手からの追い越しは直るが、
	// 別の相手をまたぐ原因と結果は守れない。
	Fifo Mode = iota
	// Causal は原因の順を守る。原因になったものを渡すまで、結果は保留する。
	Causal
)

func (m Mode) String() string {
	if m == Causal {
		return "原因の順"
	}
	return "送り手ごとの順"
}

// #endregion mode

// #region message

// Message は放送で流す1件。
//
// Deps が「送った時点で送り手が見ていたもの」になる。送り手自身の分は
// この放送を含めた数で、他人の分は渡し終えた数。[ベクタークロック](clock)
// そのものを載せている。
type Message struct {
	From string
	Seq  int
	Body string
	Deps clock.Vector
}

// deliverable は、このメッセージを今渡してよいかを返す。
//
// 送り手 j からのメッセージ m を、seen まで渡し終えた側で見るとき:
//
//	① m.Deps[j] == seen[j] + 1     この送り手の次の1件であること(飛ばさない)
//	② m.Deps[k] <= seen[k]  (k≠j)  送り手が見ていたものを自分も見ていること
//
// ①だけなら送り手ごとの順で、②が付くと原因の順になる。
// 判定に要るのは数の大小だけで、時刻も順序づけも要らない。
func deliverable(m Message, seen clock.Vector, mode Mode) bool {
	if m.Deps[m.From] != seen[m.From]+1 {
		return false
	}
	if mode == Fifo {
		return true
	}
	for k, want := range m.Deps {
		if k == m.From {
			continue
		}
		if want > seen[k] {
			return false
		}
	}
	return true
}

// #endregion message

// #region node

// Node は1台。渡し終えた数と、まだ渡せていない預かりを持つ。
type Node struct {
	Name string
	mode Mode
	seen clock.Vector
	hold []Message

	// Delivered は渡した順の記録。自分が出した放送もここに入る。
	Delivered []Message
}

func newNode(name string, mode Mode) *Node {
	return &Node{Name: name, mode: mode, seen: clock.Vector{}}
}

// Seen は渡し終えた数を返す。
func (n *Node) Seen() clock.Vector { return n.seen.Clone() }

// Held はまだ渡せていないメッセージを返す。
func (n *Node) Held() []Message { return append([]Message(nil), n.hold...) }

// Order は渡した順の本文を返す。
func (n *Node) Order() []string {
	out := make([]string, len(n.Delivered))
	for i, m := range n.Delivered {
		out[i] = m.Body
	}
	return out
}

// Recv は1件届いたことにして、渡せるものをすべて渡す。渡した順に返す。
//
// 届いたものをそのまま渡すのではなく、いったん預かりに入れてから
// 渡せるかを見る。1件渡すと条件が変わるので、渡せなくなるまで繰り返す。
func (n *Node) Recv(m Message) []Message {
	if n.known(m) {
		return nil
	}
	n.hold = append(n.hold, m)
	return n.drain()
}

// known は、すでに渡したか預かっているかを返す。二度渡さないための番人。
func (n *Node) known(m Message) bool {
	if m.Deps[m.From] <= n.seen[m.From] {
		return true
	}
	for _, h := range n.hold {
		if h.From == m.From && h.Seq == m.Seq {
			return true
		}
	}
	return false
}

// drain は渡せるものが無くなるまで渡す。
func (n *Node) drain() []Message {
	var out []Message
	for {
		i := -1
		for j, m := range n.hold {
			if deliverable(m, n.seen, n.mode) {
				i = j
				break
			}
		}
		if i < 0 {
			return out
		}
		m := n.hold[i]
		n.hold = append(n.hold[:i], n.hold[i+1:]...)
		n.apply(m)
		out = append(out, m)
	}
}

// apply は渡したことにして、数え上げを進める。
func (n *Node) apply(m Message) {
	n.seen[m.From] = m.Deps[m.From]
	n.Delivered = append(n.Delivered, m)
}

// #endregion node

// #region sim

// Sim は放送する側と受け取る側をまとめて動かす。
type Sim struct {
	mode  Mode
	names []string
	nodes map[string]*Node
	Log   []string
}

// New は台を並べる。mode で配る順の決め方を選ぶ。
func New(mode Mode, names ...string) *Sim {
	s := &Sim{mode: mode, nodes: map[string]*Node{}}
	s.names = append([]string(nil), names...)
	for _, n := range s.names {
		s.nodes[n] = newNode(n, mode)
	}
	return s
}

// Node は1台を返す。
func (s *Sim) Node(name string) *Node { return s.nodes[name] }

// Names は台の一覧を返す。
func (s *Sim) Names() []string { return append([]string(nil), s.names...) }

// Mode は配る順の決め方を返す。
func (s *Sim) Mode() Mode { return s.mode }

// Broadcast は from から全員へ流す。出した本人にはその場で渡る。
//
// 送り手は自分の数を1つ進めてから、その時点で見ているものをまるごと載せる。
// 「この放送より前に自分が見たもの」が、そのまま原因の一覧になる。
func (s *Sim) Broadcast(from, body string) Message {
	n := s.nodes[from]
	n.seen[from]++
	m := Message{From: from, Seq: n.seen[from], Body: body, Deps: n.seen.Clone()}
	n.Delivered = append(n.Delivered, m)
	s.logf(from + " が「" + body + "」を流した")
	return m
}

// Deliver は to にメッセージが届いたことにする。渡せたものを返す。
//
// 届くことと渡すことを分けているのが、この章の全部になる。
func (s *Sim) Deliver(to string, m Message) []Message {
	n := s.nodes[to]
	got := n.Recv(m)
	switch {
	case len(got) == 0 && len(n.hold) > 0:
		s.logf(to + " に「" + m.Body + "」が届いたが、まだ渡せない(預かり " + itoa(len(n.hold)) + " 件)")
	case len(got) > 1:
		s.logf(to + " に「" + m.Body + "」が届いて、預かりもまとめて " + itoa(len(got)) + " 件渡した")
	case len(got) == 1:
		s.logf(to + " に「" + m.Body + "」が届いて、そのまま渡した")
	default:
		s.logf(to + " に「" + m.Body + "」が届いたが、すでに渡し済み")
	}
	return got
}

// Broadcasts は from から流して、指定した相手にその場で届ける。
func (s *Sim) Broadcasts(from, body string, to ...string) Message {
	m := s.Broadcast(from, body)
	for _, t := range to {
		s.Deliver(t, m)
	}
	return m
}

// Relation は2つのメッセージの前後を返す。並行なら Concurrent。
func Relation(a, b Message) clock.Ord { return clock.Compare(a.Deps, b.Deps) }

// #endregion sim

func (s *Sim) logf(msg string) { s.Log = append(s.Log, msg) }

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
