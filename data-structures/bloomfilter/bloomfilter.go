// Package bloomfilter はブルームフィルタの最小実装。
//
// ブルームフィルタは「集合に入っているか」を、要素そのものを保存せずに
// ビット配列だけで判定する確率的データ構造。答えは2種類:
//   - 「たぶん入っている」(偽陽性がありうる)
//   - 「絶対に入っていない」(これは確実)
//
// 要素を持たないので圧倒的に省メモリ。その代わり「入っている」は嘘のことがある。
// この非対称性が使いどころを決める(重い検索の前の門番など)。
package bloomfilter

import (
	"hash/fnv"
	"math"
)

// #region types
// Filter はビット配列と、使うハッシュ関数の数 k。
type Filter struct {
	bitset []uint64 // ビットを 64 個ずつ詰めた配列
	m      uint64   // 総ビット数
	k      int      // ハッシュ関数の数
}

// New は「n 件入れたときに偽陽性率が約 p になる」フィルタを作る。
// 最適なビット数 m とハッシュ関数の数 k は n と p から計算で決まる:
//
//	m = -n·ln(p) / (ln2)²      k = (m/n)·ln2
//
// これがブルームフィルタの美しいところ——欲しい精度からサイズが逆算できる。
func New(n int, p float64) *Filter {
	if n <= 0 {
		panic("bloomfilter: n must be positive")
	}
	if p <= 0 || p >= 1 {
		panic("bloomfilter: p must be in (0, 1)")
	}
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	k := int(math.Round(float64(m) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	return &Filter{
		bitset: make([]uint64, (m+63)/64),
		m:      m,
		k:      k,
	}
}

// #endregion types

// #region ops
// positions は key から k 個のビット位置を導く。
// ハッシュを2つ用意して h1 + i·h2 で k 個作る「ダブルハッシュ法」。
// 独立なハッシュを k 個実装しなくて済む定番テクニック。
func (f *Filter) positions(key string) []uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	sum := h.Sum64()
	h1 := sum & 0xffffffff // 下位32bit
	h2 := (sum >> 32) | 1  // 上位32bit(0 だと進まないので奇数化)
	pos := make([]uint64, f.k)
	for i := 0; i < f.k; i++ {
		pos[i] = (h1 + uint64(i)*h2) % f.m
	}
	return pos
}

// Add は key を追加する。k 個のビットを立てるだけ。
func (f *Filter) Add(key string) {
	for _, p := range f.positions(key) {
		f.bitset[p/64] |= 1 << (p % 64)
	}
}

// MayContain は「たぶん入っている」なら true、「絶対に入っていない」なら false。
// k 個のビットが全部立っていれば true。1つでも立っていなければ確実に false。
func (f *Filter) MayContain(key string) bool {
	for _, p := range f.positions(key) {
		if f.bitset[p/64]&(1<<(p%64)) == 0 {
			return false // 1つでも 0 なら、この key は絶対に入れていない
		}
	}
	return true // 全部 1。入っているかも(偽陽性の可能性あり)
}

// #endregion ops

// bits はビット総数を返す(テスト・可視化用)。
func (f *Filter) bits() uint64 { return f.m }

// hashes はハッシュ関数の数 k を返す(テスト・可視化用)。
func (f *Filter) hashes() int { return f.k }
