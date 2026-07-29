// Package total は、全員が同じ順で受け取る配送を2通りで実装する。
//
// [因果順序の配送](causal)では、原因の順だけを守って、並行なものには順序を
// 付けなかった。付けないから誰も待たずに済み、切れても止まらない。
//
// だが、それでは足りない場面がある。同じ口座に対する2件の引き落としが
// 並行に起きたとき、台ごとに適用する順が違えば、残高が食い違う。
// 順序を付けないという判断は、そこでは通らない。
//
// 全員が同じ順で受け取ることを全順序という。作り方は大きく2つになる。
//
//   - 番号を振る係を1台置く。届いた順に番号を付けて配り、全員がその番号順に渡す。
//   - 全員で決める。各自が時刻つきで要求を出し、全員から「あなたより後」を
//     聞くまで渡さない。
//
// どちらにも共通するのは、決める人が要ることと、待つ相手が居ることになる。
// 待つから、相手が黙ると止まる。因果順序が止まらなかったのは、
// 誰も待っていなかったからだ。
//
// 実時間も乱数も使わない。誰にいつ届くかは呼び出し側が明示する。
package total

import (
	"sort"

	"github.com/esh2n/sharin/distributed/clock"
)

// Numbered は番号の付いたメッセージ。
type Numbered struct {
	Seq  int
	From string
	Body string
}

// SeqNode は番号順に渡す1台。次に渡すべき番号と、先に届いた預かりを持つ。
type SeqNode struct {
	Name string
	next int
	hold []Numbered

	// Delivered は渡した順の記録。
	Delivered []Numbered
}

// Order は渡した順の本文を返す。
func (n *SeqNode) Order() []string {
	out := make([]string, len(n.Delivered))
	for i, m := range n.Delivered {
		out[i] = m.Body
	}
	return out
}

// Held はまだ渡せていないものを返す。
func (n *SeqNode) Held() []Numbered { return append([]Numbered(nil), n.hold...) }

// Next は次に渡すべき番号を返す。
func (n *SeqNode) Next() int { return n.next + 1 }

// SeqSim は番号を振る係を1台置く方式。
type SeqSim struct {
	names    []string
	nodes    map[string]*SeqNode
	assigned int
	Log      []string
}

// NewSequencer は台を並べる。番号を振る係は台の外に居るものとして扱う。
func NewSequencer(names ...string) *SeqSim {
	s := &SeqSim{nodes: map[string]*SeqNode{}}
	s.names = append([]string(nil), names...)
	for _, n := range s.names {
		s.nodes[n] = &SeqNode{Name: n}
	}
	return s
}

// Node は1台を返す。
func (s *SeqSim) Node(name string) *SeqNode { return s.nodes[name] }

// Names は台の一覧を返す。
func (s *SeqSim) Names() []string { return append([]string(nil), s.names...) }

// #region sequencer

// Assign は係にメッセージが届いたことにして、番号を付ける。
//
// 番号は係に届いた順に付く。送り手が出した順でも、原因の順でもない。
// ここが後で効いてくる。
func (s *SeqSim) Assign(from, body string) Numbered {
	s.assigned++
	m := Numbered{Seq: s.assigned, From: from, Body: body}
	s.logf("係が「" + body + "」に番号 " + itoa(m.Seq) + " を付けた")
	return m
}

// Deliver は to に番号つきメッセージが届いたことにする。渡せたものを返す。
//
// 番号順にしか渡さない。手前の番号が来ていなければ、後ろは預かる。
func (s *SeqSim) Deliver(to string, m Numbered) []Numbered {
	n := s.nodes[to]
	if m.Seq <= n.next {
		s.logf(to + " に " + itoa(m.Seq) + " が届いたが、すでに渡し済み")
		return nil
	}
	for _, h := range n.hold {
		if h.Seq == m.Seq {
			s.logf(to + " に " + itoa(m.Seq) + " が届いたが、すでに預かっている")
			return nil
		}
	}
	n.hold = append(n.hold, m)

	var out []Numbered
	for {
		i := -1
		for j, h := range n.hold {
			if h.Seq == n.next+1 {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		h := n.hold[i]
		n.hold = append(n.hold[:i], n.hold[i+1:]...)
		n.next = h.Seq
		n.Delivered = append(n.Delivered, h)
		out = append(out, h)
	}

	if len(out) == 0 {
		s.logf(to + " に " + itoa(m.Seq) + " が届いたが、" + itoa(n.next+1) + " がまだ来ていないので渡せない")
	} else {
		s.logf(to + " が " + itoa(len(out)) + " 件渡した(次は " + itoa(n.next+1) + ")")
	}
	return out
}

// #endregion sequencer

// Msg は時刻つきの1件。Body が空なら、時刻を知らせるだけの合図。
type Msg struct {
	From string
	T    int
	Body string
}

// VoteNode は全員で決める方式の1台。
type VoteNode struct {
	Name string
	lam  *clock.Lamport
	// heard は各台から最後に聞いた時刻。ここが判定の材料になる。
	heard map[string]int
	queue []Msg

	// Delivered は渡した順の記録。
	Delivered []Msg
}

// Order は渡した順の本文を返す。
func (n *VoteNode) Order() []string {
	out := make([]string, len(n.Delivered))
	for i, m := range n.Delivered {
		out[i] = m.Body
	}
	return out
}

// Queue はまだ渡せていない要求を、決まった順で返す。
func (n *VoteNode) Queue() []Msg {
	out := append([]Msg(nil), n.queue...)
	sortByStamp(out)
	return out
}

// Heard は各台から最後に聞いた時刻を返す。
func (n *VoteNode) Heard() map[string]int {
	out := map[string]int{}
	for k, v := range n.heard {
		out[k] = v
	}
	return out
}

// #region stamp

// sortByStamp は時刻で並べ、同時刻は名前で決める。
//
// 同じ時刻のものに順序を付けるのは、[論理時計](clock)の LamportLess と同じ手になる。
// 名前で決めるので、どの台で並べても同じ順になる。
func sortByStamp(ms []Msg) {
	sort.SliceStable(ms, func(i, j int) bool {
		return clock.LamportLess(ms[i].T, ms[i].From, ms[j].T, ms[j].From)
	})
}

// #endregion stamp

// VoteSim は全員で決める方式。
type VoteSim struct {
	names []string
	nodes map[string]*VoteNode
	Log   []string
}

// NewVoting は台を並べる。係は置かない。
func NewVoting(names ...string) *VoteSim {
	s := &VoteSim{nodes: map[string]*VoteNode{}}
	s.names = append([]string(nil), names...)
	for _, n := range s.names {
		s.nodes[n] = &VoteNode{Name: n, lam: clock.NewLamport(n), heard: map[string]int{}}
	}
	return s
}

// Node は1台を返す。
func (s *VoteSim) Node(name string) *VoteNode { return s.nodes[name] }

// Names は台の一覧を返す。
func (s *VoteSim) Names() []string { return append([]string(nil), s.names...) }

// Request は from が時刻つきで要求を出す。自分の待ち行列にも入る。
func (s *VoteSim) Request(from, body string) Msg {
	n := s.nodes[from]
	m := Msg{From: from, T: n.lam.Send(), Body: body}
	n.queue = append(n.queue, m)
	n.heard[from] = m.T
	s.logf(from + " が時刻 " + itoa(m.T) + " で「" + body + "」を出した")
	return m
}

// Beat は from が「今の時刻」だけを知らせる。要求は出さない。
//
// これが要る理由は判定の条件にある。全員から「あなたより後」を聞かないと
// 渡せないので、黙っている台が1つでもあると全体が止まる。
func (s *VoteSim) Beat(from string) Msg {
	n := s.nodes[from]
	m := Msg{From: from, T: n.lam.Send()}
	n.heard[from] = m.T
	s.logf(from + " が時刻 " + itoa(m.T) + " を知らせた")
	return m
}

// Deliver は to にメッセージが届いたことにする。渡せたものを返す。
func (s *VoteSim) Deliver(to string, m Msg) []Msg {
	n := s.nodes[to]
	n.heard[to] = n.lam.Recv(m.T)
	n.heard[m.From] = m.T
	if m.Body != "" && !n.has(m) {
		n.queue = append(n.queue, m)
	}
	got := s.drain(n)
	if len(got) == 0 {
		s.logf(to + " は渡せない(待ち行列 " + itoa(len(n.queue)) + " 件)")
	} else {
		s.logf(to + " が " + itoa(len(got)) + " 件渡した")
	}
	return got
}

func (n *VoteNode) has(m Msg) bool {
	for _, q := range n.queue {
		if q.From == m.From && q.T == m.T {
			return true
		}
	}
	return false
}

// #region deliverable

// deliverable は待ち行列の先頭を渡してよいかを返す。
//
// 条件は1つだけになる。先頭より後の時刻を、他の全員から聞いていること。
// 聞いていれば、それより小さい時刻の要求はもう出てこない。
// 過半数ではなく全員なのが、この方式の弱点そのものになる。
func (s *VoteSim) deliverable(n *VoteNode, head Msg) bool {
	for _, k := range s.names {
		if k == head.From {
			continue
		}
		if n.heard[k] <= head.T {
			return false
		}
	}
	return true
}

func (s *VoteSim) drain(n *VoteNode) []Msg {
	var out []Msg
	for len(n.queue) > 0 {
		sortByStamp(n.queue)
		head := n.queue[0]
		if !s.deliverable(n, head) {
			return out
		}
		n.queue = n.queue[1:]
		n.Delivered = append(n.Delivered, head)
		out = append(out, head)
	}
	return out
}

// #endregion deliverable

// #region agree

// Agreed は、全員が同じ順で渡し終えているかを返す。
//
// 途中まででも、短いほうが長いほうの先頭に一致していればよい。
// 全順序が守れているなら、先に進んでいる台の記録は、遅れている台の記録の続きになる。
func Agreed(orders ...[]string) bool {
	for i := 1; i < len(orders); i++ {
		a, b := orders[0], orders[i]
		if len(b) < len(a) {
			a, b = b, a
		}
		for j := range a {
			if a[j] != b[j] {
				return false
			}
		}
	}
	return true
}

// #endregion agree

func (s *SeqSim) logf(msg string)  { s.Log = append(s.Log, msg) }
func (s *VoteSim) logf(msg string) { s.Log = append(s.Log, msg) }

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
