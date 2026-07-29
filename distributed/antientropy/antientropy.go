// Package antientropy は、台どうしの持ち物のずれを、全件送らずに見つけて直す。
//
// [クォーラム](quorum)の章では、読んだついでに古い台を直した。だがこれには穴がある。
// 読まれない key はいつまでも古いままになる。誰も見ていない値ほど直らない。
//
// だから、読みとは別に台どうしを突き合わせる仕組みが要る。素朴にやるなら
// 全部の key を送り合って比べればよいが、それでは持ち物の数だけ通信が出る。
// 100万件あって違いが1件でも、100万件送ることになる。
//
// 木にすると、この無駄が消える。まず全体の要約(ハッシュ)を1つ交換する。
// 一致すれば、中身は見ずに「同じ」と分かる。違えば半分ずつに割って、
// 違うほうだけ降りていく。降りる回数は木の高さぶんで、送る中身は
// 違いのあった区画だけになる。
//
// 実時間も乱数も使わない。同じ内容なら必ず同じ要約になる。
package antientropy

import (
	"sort"

	"github.com/esh2n/sharin/distributed/quorum"
)

// #region store

// Store は1台が持つ key と値。値の新しさは [クォーラム](quorum)と同じ版番号で決める。
type Store struct {
	buckets int
	data    map[string]quorum.Value
}

// NewStore は区画の数を決めて台を作る。区画の数は2のべき乗にする。
func NewStore(buckets int) *Store {
	if buckets < 1 {
		buckets = 1
	}
	return &Store{buckets: buckets, data: map[string]quorum.Value{}}
}

// Buckets は区画の数を返す。
func (s *Store) Buckets() int { return s.buckets }

// Put は値を置く。版番号が古ければ何もしない。
func (s *Store) Put(key, data string, stamp int) {
	v := quorum.Value{Data: data, Stamp: stamp}
	s.data[key] = quorum.Newer(s.data[key], v)
}

// Get は値を返す。
func (s *Store) Get(key string) (quorum.Value, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Keys は持っている key を名前順で返す。
func (s *Store) Keys() []string {
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Bucket は key が入る区画を返す。
func (s *Store) Bucket(key string) int { return int(hash(key) % uint64(s.buckets)) }

// #endregion store

// #region tree

// Tree は要約の木。葉が区画1つぶんの要約で、内側は子2つの要約になる。
type Tree struct {
	nodes  []uint64
	leaves int
}

// Tree はこの台の持ち物から木を作る。
//
// 葉は区画ごとに、その中の key を名前順に並べて混ぜたもの。名前順にするので、
// 走査の順が違っても同じ要約になる。内側は左右の子を混ぜるだけ。
func (s *Store) Tree() *Tree {
	t := &Tree{nodes: make([]uint64, 2*s.buckets), leaves: s.buckets}

	byBucket := make([][]string, s.buckets)
	for _, k := range s.Keys() {
		b := s.Bucket(k)
		byBucket[b] = append(byBucket[b], k)
	}
	for i, keys := range byBucket {
		var h uint64
		for _, k := range keys {
			v := s.data[k]
			h = mix(h, hash(k+"="+v.Data+"@"+itoa(v.Stamp)))
		}
		t.nodes[s.buckets+i] = h
	}
	for i := s.buckets - 1; i >= 1; i-- {
		l, r := t.nodes[2*i], t.nodes[2*i+1]
		if l == 0 && r == 0 {
			continue // 空どうしは空のまま。空の区画が一致することを保てる
		}
		t.nodes[i] = mix(l, r)
	}
	return t
}

// Root は全体の要約を返す。ここが一致すれば、中身は見なくてよい。
func (t *Tree) Root() uint64 { return t.nodes[1] }

// Depth は木の高さを返す。降りる回数はここで決まる。
func (t *Tree) Depth() int {
	d := 0
	for n := t.leaves; n > 1; n /= 2 {
		d++
	}
	return d
}

// Diff は2つの木を突き合わせ、違う区画の番号を返す。
//
// 根から降りて、一致したところで打ち切る。だから比べる数は
// 持ち物の数ではなく、違いの数と木の高さで決まる。
func Diff(a, b *Tree) (buckets []int, compared int) {
	if a.leaves != b.leaves {
		return nil, 0 // 区画の切り方が違うと比べようがない
	}
	var walk func(i int)
	walk = func(i int) {
		compared++
		if a.nodes[i] == b.nodes[i] {
			return // ここから下は全部同じ
		}
		if i >= a.leaves {
			buckets = append(buckets, i-a.leaves)
			return
		}
		walk(2 * i)
		walk(2*i + 1)
	}
	walk(1)
	return buckets, compared
}

// #endregion tree

// #region sync

// Result は突き合わせの結果。
type Result struct {
	// Buckets は違いのあった区画。
	Buckets []int
	// Compared は比べた要約の数。
	Compared int
	// Sent は実際に中身を送った件数。
	Sent int
	// Updated は書き換わった key。
	Updated []string
}

// Sync は木で違いを探し、違った区画の中身だけを交換して両方を同じにする。
//
// 木が言うのは「この区画が違う」までになる。どちらが新しいかは
// [クォーラム](quorum)の版番号で決める。木は在処しか教えてくれない。
func Sync(a, b *Store) Result {
	buckets, compared := Diff(a.Tree(), b.Tree())
	res := Result{Buckets: buckets, Compared: compared}
	if len(buckets) == 0 {
		return res
	}

	want := map[int]bool{}
	for _, i := range buckets {
		want[i] = true
	}
	for _, k := range union(a, b) {
		if !want[a.Bucket(k)] {
			continue
		}
		res.Sent++
		av, aok := a.Get(k)
		bv, bok := b.Get(k)
		best := quorum.Newer(av, bv)
		if !aok || av != best {
			a.data[k] = best
			res.Updated = append(res.Updated, k)
			continue
		}
		if !bok || bv != best {
			b.data[k] = best
			res.Updated = append(res.Updated, k)
		}
	}
	sort.Strings(res.Updated)
	return res
}

// CompareAll は木を使わずに全件突き合わせる(対照用)。
//
// 違いが1件でも、持ち物の数だけ送ることになる。
func CompareAll(a, b *Store) Result {
	res := Result{}
	for _, k := range union(a, b) {
		res.Compared++
		res.Sent++
		av, aok := a.Get(k)
		bv, bok := b.Get(k)
		best := quorum.Newer(av, bv)
		if !aok || av != best {
			a.data[k] = best
			res.Updated = append(res.Updated, k)
			continue
		}
		if !bok || bv != best {
			b.data[k] = best
			res.Updated = append(res.Updated, k)
		}
	}
	sort.Strings(res.Updated)
	return res
}

// #endregion sync

// union は両方の key をまとめて名前順で返す。
func union(a, b *Store) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range append(a.Keys(), b.Keys()...) {
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// hash は FNV-1a(64bit)。
func hash(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// mix は2つの要約を1つにする。順番を入れ替えると別の値になる。
func mix(a, b uint64) uint64 {
	h := hash("")
	h ^= a
	h *= 1099511628211
	h ^= b
	h *= 1099511628211
	return h
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
