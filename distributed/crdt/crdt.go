// Package crdt は、合意を取らずに後からまとめられるデータ型を最小構成で実装する。
//
// [論理時計](clock)の章で、同時に起きた2つの出来事を見分けられるようにした。
// 見分けられると、片方を黙って捨てずに済む。では、捨てないなら何をするのか。
//
// 1つの答えが、まとめ方をあらかじめ決めておくことになる。どちらが先かを決めるのを
// やめて、2つを1つにする規則だけを用意する。その規則が3つの性質を持っていれば、
// 順序も回数も関係なくなる。
//
//   - 可換: どちらからまとめても同じ
//   - 結合的: どこから括ってまとめても同じ
//   - 冪等: 同じものを何度まとめても同じ
//
// この3つがそろうと、知らせが入れ替わって届いても、二度届いても、結果が変わらない。
// [調整ループ](reconcile)が取りこぼしに強かったのと同じ理屈が、データの側にも現れる。
// 合意が要らなくなる代わりに、表せる操作が狭くなる。その狭さと、狭さの中で
// どこまでやれるかがこの章になる。
package crdt

import "sort"

// #region counter

// GCounter は増えるだけの数え上げ。
//
// ノードごとに自分の数を持ち、まとめるときは要素ごとに大きいほうを取る。
// [ベクタークロック](clock)とまったく同じ形になっている。あちらは出来事を数え、
// こちらは値を数えているだけで、まとめ方は同一になる。
type GCounter map[string]int

// Inc は自分の要素だけを増やす。他人の要素には触らない。
func (g GCounter) Inc(node string, n int) {
	if n < 0 {
		panic("crdt: 増える一方の数え上げに負を足そうとした")
	}
	g[node] += n
}

// Value は全員のぶんを合計した値を返す。
func (g GCounter) Value() int {
	sum := 0
	for _, v := range g {
		sum += v
	}
	return sum
}

// Merge は2つをまとめた新しいものを返す。元は変えない。
func (g GCounter) Merge(o GCounter) GCounter {
	out := GCounter{}
	for k, v := range g {
		out[k] = v
	}
	for k, v := range o {
		if v > out[k] {
			out[k] = v
		}
	}
	return out
}

// PNCounter は増えも減りもする数え上げ。
//
// 減らす操作を直接は持てないので、減らした量を別の数え上げに足していく。
// 値は2つの差になる。[2の補数](numbers)が引き算を足し算にしたのと同じ発想で、
// 「引けないなら、引く用のものを足す」という形になっている。
type PNCounter struct {
	P GCounter // 増やした量
	N GCounter // 減らした量
}

// NewPN は空の数え上げを作る。
func NewPN() *PNCounter { return &PNCounter{P: GCounter{}, N: GCounter{}} }

// Inc と Dec は、それぞれの側を増やす。どちらも増やすだけになる。
func (c *PNCounter) Inc(node string, n int) { c.P.Inc(node, n) }
func (c *PNCounter) Dec(node string, n int) { c.N.Inc(node, n) }

// Value は差を返す。
func (c *PNCounter) Value() int { return c.P.Value() - c.N.Value() }

// Merge は両側をそれぞれまとめる。
func (c *PNCounter) Merge(o *PNCounter) *PNCounter {
	return &PNCounter{P: c.P.Merge(o.P), N: c.N.Merge(o.N)}
}

// #endregion counter

// #region register

// LWW は最後の書き込みを残す入れ物。時刻とノード名で決める。
//
// 同時に書かれた2つを、決まった規則で片方に倒す。まとめ方としては成立するが、
// 倒されたほうは消える。消えたことも残らない。
type LWW struct {
	Value string
	Stamp int    // 論理時計の値
	Node  string // 同点を解くための持ち主
}

// Set は新しい値を返す。元は変えない。
func (r LWW) Set(v string, stamp int, node string) LWW {
	return LWW{Value: v, Stamp: stamp, Node: node}
}

// Merge は勝ったほうを返す。時刻が同じならノード名で決める。
func (r LWW) Merge(o LWW) LWW {
	if o.Stamp > r.Stamp || (o.Stamp == r.Stamp && o.Node > r.Node) {
		return o
	}
	return r
}

// Lost は、まとめたときに消えたほうを返す(観測用)。
// 消えた値が何だったかを知りたいなら、まとめる前に自分で持っておくしかない。
func (r LWW) Lost(o LWW) (LWW, bool) {
	m := r.Merge(o)
	switch {
	case m == r && r != o:
		return o, true
	case m == o && r != o:
		return r, true
	}
	return LWW{}, false
}

// #endregion register

// #region set

// TwoPSet は足すのと消すのを、それぞれ集合で覚える。
//
// 消したものは「消した側」に入るので、後から足し直しても出てこない。
// まとめ方としては正しいが、使い方が制限される。
type TwoPSet struct {
	Added   map[string]bool
	Removed map[string]bool
}

// NewTwoP は空の集合を作る。
func NewTwoP() *TwoPSet {
	return &TwoPSet{Added: map[string]bool{}, Removed: map[string]bool{}}
}

func (s *TwoPSet) Add(v string)    { s.Added[v] = true }
func (s *TwoPSet) Remove(v string) { s.Removed[v] = true }

// Has は、足されていて消されていないかを返す。
func (s *TwoPSet) Has(v string) bool { return s.Added[v] && !s.Removed[v] }

// Values は今ある要素を名前順で返す。
func (s *TwoPSet) Values() []string {
	var out []string
	for v := range s.Added {
		if s.Has(v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// Merge は両側の和を取る。
func (s *TwoPSet) Merge(o *TwoPSet) *TwoPSet {
	out := NewTwoP()
	for _, m := range []map[string]bool{s.Added, o.Added} {
		for v := range m {
			out.Added[v] = true
		}
	}
	for _, m := range []map[string]bool{s.Removed, o.Removed} {
		for v := range m {
			out.Removed[v] = true
		}
	}
	return out
}

// tag は1回の追加を区別する印。誰が何回目に足したかで一意になる。
type tag struct {
	Node string
	Seq  int
}

// ORSet は追加ごとに印をつけ、消すときは「そのとき見えていた印」だけを消す。
//
// 見ていない印は消えないので、並行して足された値は残る。
// 消してから足し直すと新しい印がつくので、ちゃんと出てくる。
type ORSet struct {
	live map[string]map[tag]bool
	seq  map[string]int
}

// NewOR は空の集合を作る。
func NewOR() *ORSet {
	return &ORSet{live: map[string]map[tag]bool{}, seq: map[string]int{}}
}

// Add は新しい印をつけて足す。
func (s *ORSet) Add(node, v string) {
	s.seq[node]++
	if s.live[v] == nil {
		s.live[v] = map[tag]bool{}
	}
	s.live[v][tag{Node: node, Seq: s.seq[node]}] = true
}

// Remove は、今このノードから見えている印だけを消す。
//
// ここが肝になる。値そのものを消すのではなく、自分が観測した追加を取り消す。
// 見えていない追加は残るので、並行して足されたものは生き残る。
func (s *ORSet) Remove(v string) {
	delete(s.live, v)
}

// Has は要素があるかを返す。印が1つでも残っていればある。
func (s *ORSet) Has(v string) bool { return len(s.live[v]) > 0 }

// Values は今ある要素を名前順で返す。
func (s *ORSet) Values() []string {
	var out []string
	for v := range s.live {
		if s.Has(v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// Tags は要素についている印の数を返す(観測用)。
func (s *ORSet) Tags(v string) int { return len(s.live[v]) }

// Merge は印の和を取る。片方で消され、もう片方で足されていれば、足したほうが残る。
func (s *ORSet) Merge(o *ORSet) *ORSet {
	out := NewOR()
	for _, src := range []*ORSet{s, o} {
		for v, tags := range src.live {
			if out.live[v] == nil {
				out.live[v] = map[tag]bool{}
			}
			for t := range tags {
				out.live[v][t] = true
			}
		}
		for n, q := range src.seq {
			if q > out.seq[n] {
				out.seq[n] = q
			}
		}
	}
	return out
}

// #endregion set
