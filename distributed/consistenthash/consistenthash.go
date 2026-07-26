// Package consistenthash はコンシステントハッシュ法(ハッシュリング)の最小実装。
//
// 「どのキーをどのノードに置くか」を決めるのがシャーディング。素朴に hash(key) % N で
// 割ると、ノードが1台増減しただけでほぼ全キーの割り当てが変わり(再ハッシュ地獄)、
// キャッシュが吹き飛ぶ。
//
// コンシステントハッシュは、ノードもキーも同じ円環[0, 2^32)上に配置し、キーは「時計回りで
// 最初に出会うノード」に属する、と決める。こうするとノードの増減で動くのは、その担当弧の
// 分だけ——平均して K/N 個のキーで済む。
//
// さらに1物理ノードを円環上の多数の点(仮想ノード=vnode)として置くことで、担当弧の偏りを
// ならし、負荷を平準化する。
package consistenthash

import (
	"hash/crc32"
	"sort"
)

// Hash は任意のバイト列を円環上の点(uint32)へ写す関数。差し替え可能にしてテストを決定的にする。
type Hash func([]byte) uint32

// defaultHash は CRC32(IEEE)。似た短い文字列(vnode の "node#i")でも点がよく散らばり、
// 負荷が平準化する(FNV-1a はこの用途だと偏りが大きい。groupcache も crc32 を使う)。
func defaultHash(b []byte) uint32 {
	return crc32.ChecksumIEEE(b)
}

// Ring はハッシュリング。points はソート済みの円環上の点、owner が点→物理ノード。
type Ring struct {
	replicas int               // 物理1ノードあたりの仮想点数(vnode)
	hash     Hash              //
	points   []uint32          // ソート済みの円環上の点
	owner    map[uint32]string // 点 -> 物理ノード id
	nodes    map[string]struct{}
}

// New はリングを作る。replicas は仮想ノード数(1物理ノードを円環上に何点置くか)。
// fn が nil なら FNV-1a を使う。
func New(replicas int, fn Hash) *Ring {
	if replicas < 1 {
		replicas = 1
	}
	if fn == nil {
		fn = defaultHash
	}
	return &Ring{
		replicas: replicas,
		hash:     fn,
		owner:    map[uint32]string{},
		nodes:    map[string]struct{}{},
	}
}

// vpoint は node の i 番目の仮想点のバイト列。"node#i" を素直にハッシュする。
func vpoint(node string, i int) []byte {
	return []byte(node + "#" + itoa(i))
}

// itoa は小さな非負整数を10進文字列に(strconv を避けた最小実装)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// Add は物理ノードを1つ以上追加する。各ノードを replicas 個の仮想点として円環に置く。
func (r *Ring) Add(nodes ...string) {
	for _, node := range nodes {
		if _, ok := r.nodes[node]; ok {
			continue // 既存はスキップ(二重登録しない)
		}
		r.nodes[node] = struct{}{}
		for i := 0; i < r.replicas; i++ {
			p := r.hash(vpoint(node, i))
			if _, taken := r.owner[p]; taken {
				continue // まれな衝突は捨てる(既存の点を尊重)
			}
			r.owner[p] = node
			r.points = append(r.points, p)
		}
	}
	sort.Slice(r.points, func(i, j int) bool { return r.points[i] < r.points[j] })
}

// Remove は物理ノードを取り除く。そのノードが持つ点だけを消す(他ノードの割り当ては不変)。
func (r *Ring) Remove(node string) {
	if _, ok := r.nodes[node]; !ok {
		return
	}
	delete(r.nodes, node)
	kept := r.points[:0]
	for _, p := range r.points {
		if r.owner[p] == node {
			delete(r.owner, p)
			continue
		}
		kept = append(kept, p)
	}
	r.points = kept
}

// #region get
// Get はキーの担当ノードを返す。円環上でキーの点から時計回りに最初に出会うノード。
// 空リングなら ok=false。
func (r *Ring) Get(key string) (node string, ok bool) {
	if len(r.points) == 0 {
		return "", false
	}
	h := r.hash([]byte(key))
	// h 以上で最小の点を二分探索。無ければ先頭へ回り込む(円環)。
	i := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if i == len(r.points) {
		i = 0
	}
	return r.owner[r.points[i]], true
}

// #endregion get

// GetN はキーの担当ノードから時計回りに、重複しない物理ノードを最大 n 個返す。
// レプリケーション先(1次 + バックアップ)を選ぶのに使う。n がノード数を超えれば全ノード。
func (r *Ring) GetN(key string, n int) []string {
	if len(r.points) == 0 || n <= 0 {
		return nil
	}
	h := r.hash([]byte(key))
	start := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if start == len(r.points) {
		start = 0
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, n)
	for i := 0; i < len(r.points) && len(out) < n; i++ {
		owner := r.owner[r.points[(start+i)%len(r.points)]]
		if _, dup := seen[owner]; dup {
			continue
		}
		seen[owner] = struct{}{}
		out = append(out, owner)
	}
	return out
}

// Nodes は登録済みの物理ノード id を返す(順不同)。
func (r *Ring) Nodes() []string {
	out := make([]string, 0, len(r.nodes))
	for n := range r.nodes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
