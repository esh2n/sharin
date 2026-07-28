// Package clock は論理時計とベクタークロックを最小構成で実装する。
//
// [leader election](leaderelection)の章で、時刻は共有できないが経過は共有できると書いた。
// 他のノードが書いた時刻は、自分の時計とどれだけずれているか分からないので比べられない。
// だから絶対時刻を使わず、変化を見てからの経過だけを見た。
//
// では、複数のノードで起きた出来事に順序をつけたいときはどうするのか。
// 「この書き込みとあの書き込み、どちらが先か」を知りたい場面は必ず出てくる。
// 時計が合っていないなら、時刻では決められない。
//
// 答えは、時刻を測るのをやめて出来事を数えることになる。自分のところで何か起きたら
// 数を1つ増やす。誰かに知らせるときは、その数を一緒に送る。受け取った側は、
// 自分の数と受け取った数の大きいほうから続きを数える。これだけで、
// 「原因は結果より小さい数を持つ」が保証される。
//
// ただし、この形には片道しかない仕掛けがある。原因なら小さい、は言えるが、
// 小さいなら原因だ、は言えない。関係のない2つの出来事にも、たまたま順序がつく。
// 本当に無関係だったのかを知りたければ、数を1つでなくノードの数だけ持つことになる。
package clock

import "sort"

// #region lamport

// Lamport は1つのノードが持つ、ただの数え上げ。
//
// 数えているのは時間ではなく、自分が知っている出来事の多さになる。
type Lamport struct {
	ID string
	t  int
}

// NewLamport は 0 から数え始める時計を作る。
func NewLamport(id string) *Lamport { return &Lamport{ID: id} }

// Now は今の値を返す。
func (l *Lamport) Now() int { return l.t }

// Local は自分のところで何か起きたときに呼ぶ。数を1つ進める。
func (l *Lamport) Local() int {
	l.t++
	return l.t
}

// Send は誰かに知らせるときに呼ぶ。進めた値を、送る印として返す。
func (l *Lamport) Send() int {
	l.t++
	return l.t
}

// Recv は知らせを受け取ったときに呼ぶ。
//
// 大きいほうから続けるのが肝になる。相手のほうが先に進んでいたら、
// 自分もそこまで追いつく。これで「送信は受信より小さい」が必ず成り立つ。
func (l *Lamport) Recv(remote int) int {
	if remote > l.t {
		l.t = remote
	}
	l.t++
	return l.t
}

// LamportLess は2つの出来事に全順序をつける。
//
// 数が同じときはノード名で決める。こうすると必ずどちらかが先になるが、
// その順序に因果の意味は無い。並べられることと、意味があることは別になる。
func LamportLess(t1 int, id1 string, t2 int, id2 string) bool {
	if t1 != t2 {
		return t1 < t2
	}
	return id1 < id2
}

// #endregion lamport

// #region vector

// Vector はノードごとの数え上げ。自分が知っている「各ノードでの出来事の数」になる。
type Vector map[string]int

// Clone は写しを返す。元は変えない。
func (v Vector) Clone() Vector {
	out := make(Vector, len(v))
	for k, n := range v {
		out[k] = n
	}
	return out
}

// Keys はノード名を名前順で返す(表示と比較を決定的にする)。
func (v Vector) Keys() []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Ord は2つのベクタの関係。
type Ord int

const (
	// Equal は同じ。
	Equal Ord = iota
	// Before は左が右の原因になりうる。
	Before
	// After は右が左の原因になりうる。
	After
	// Concurrent はどちらも相手を知らない。同時に起きたとみなす。
	Concurrent
)

func (o Ord) String() string {
	return [...]string{"同じ", "前", "後", "同時"}[o]
}

// Compare は2つのベクタを比べる。
//
// すべての要素で a <= b かつ1つでも a < b があれば、a は b より前になる。
// 逆向きも同じ。どちらでもなければ同時で、これが Lamport では出せない答えになる。
func Compare(a, b Vector) Ord {
	aLess, bLess := false, false
	seen := map[string]bool{}
	for _, k := range append(a.Keys(), b.Keys()...) {
		if seen[k] {
			continue
		}
		seen[k] = true
		switch {
		case a[k] < b[k]:
			aLess = true
		case a[k] > b[k]:
			bLess = true
		}
	}
	switch {
	case aLess && bLess:
		return Concurrent
	case aLess:
		return Before
	case bLess:
		return After
	default:
		return Equal
	}
}

// VClock は1つのノードが持つベクタ。
type VClock struct {
	ID string
	v  Vector
}

// NewVClock は空のベクタから始める時計を作る。
func NewVClock(id string) *VClock { return &VClock{ID: id, v: Vector{}} }

// Now は今のベクタの写しを返す。
func (c *VClock) Now() Vector { return c.v.Clone() }

// Local は自分の要素だけを1つ進める。
func (c *VClock) Local() Vector {
	c.v[c.ID]++
	return c.Now()
}

// Send は自分の要素を進めて、ベクタごと送る印として返す。
func (c *VClock) Send() Vector {
	c.v[c.ID]++
	return c.Now()
}

// Recv は受け取ったベクタと自分のベクタを、要素ごとに大きいほうで合わせる。
//
// 要素ごとに最大を取ることが、そのまま「知っていることの合併」になっている。
// 相手が知っていたことは、受け取った時点で自分も知っていることになる。
func (c *VClock) Recv(remote Vector) Vector {
	for _, k := range remote.Keys() {
		if remote[k] > c.v[k] {
			c.v[k] = remote[k]
		}
	}
	c.v[c.ID]++
	return c.Now()
}

// #endregion vector

// #region sim

// Event は1つの出来事。2種類の時計の値を両方持つので、後から比べられる。
type Event struct {
	ID      int
	Node    string
	Label   string
	Lamport int
	Vector  Vector
}

// Msg は1通の知らせ。送った時点の2つの時計を運ぶ。
type Msg struct {
	From    string
	Lamport int
	Vector  Vector
}

// Sim は複数のノードで起きた出来事を、両方の時計で記録する。
type Sim struct {
	lam   map[string]*Lamport
	vec   map[string]*VClock
	seq   int
	Nodes []string

	Events []Event
}

// New はノードを並べて始める。
func New(nodes ...string) *Sim {
	s := &Sim{lam: map[string]*Lamport{}, vec: map[string]*VClock{}}
	s.Nodes = append([]string(nil), nodes...)
	sort.Strings(s.Nodes)
	for _, n := range s.Nodes {
		s.lam[n] = NewLamport(n)
		s.vec[n] = NewVClock(n)
	}
	return s
}

func (s *Sim) record(node, label string, lt int, v Vector) Event {
	s.seq++
	e := Event{ID: s.seq, Node: node, Label: label, Lamport: lt, Vector: v}
	s.Events = append(s.Events, e)
	return e
}

// Local はそのノードだけで何かが起きたことにする。
func (s *Sim) Local(node, label string) Event {
	return s.record(node, label, s.lam[node].Local(), s.vec[node].Local())
}

// Send は知らせを1通出す。戻り値の Msg を Recv に渡す。
func (s *Sim) Send(node, label string) (Event, Msg) {
	lt := s.lam[node].Send()
	v := s.vec[node].Send()
	return s.record(node, label, lt, v), Msg{From: node, Lamport: lt, Vector: v}
}

// Recv は知らせを受け取る。
func (s *Sim) Recv(node string, m Msg, label string) Event {
	return s.record(node, label, s.lam[node].Recv(m.Lamport), s.vec[node].Recv(m.Vector))
}

// ByLamport は Lamport の値で全順序に並べた出来事を返す。
//
// 必ず一列に並ぶ。だがその列は、因果を表していない。
func (s *Sim) ByLamport() []Event {
	out := append([]Event(nil), s.Events...)
	sort.SliceStable(out, func(i, j int) bool {
		return LamportLess(out[i].Lamport, out[i].Node, out[j].Lamport, out[j].Node)
	})
	return out
}

// Concurrent は同時に起きた組をすべて返す(出来事の番号の小さい順)。
func (s *Sim) Concurrent() [][2]Event {
	var out [][2]Event
	for i := 0; i < len(s.Events); i++ {
		for j := i + 1; j < len(s.Events); j++ {
			if Compare(s.Events[i].Vector, s.Events[j].Vector) == Concurrent {
				out = append(out, [2]Event{s.Events[i], s.Events[j]})
			}
		}
	}
	return out
}

// Relation は2つの出来事の関係を、両方の時計で見た結果として返す。
func (s *Sim) Relation(a, b Event) (byLamport string, byVector Ord) {
	switch {
	case a.Lamport < b.Lamport:
		byLamport = "前"
	case a.Lamport > b.Lamport:
		byLamport = "後"
	default:
		byLamport = "同じ数"
	}
	return byLamport, Compare(a.Vector, b.Vector)
}

// #endregion sim
