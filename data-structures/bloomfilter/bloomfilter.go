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

	added int // 入れた回数
}

// New は「n 件入れたときに偽陽性率が約 p になる」フィルタを作る。
// 最適なビット数 m とハッシュ関数の数 k は n と p から計算で決まる:
//
//	m = -n·ln(p) / (ln2)²      k = (m/n)·ln2
//
// 欲しい精度からサイズを逆算できるのが、この道具のいちばんの取り柄になる。
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
	f.added++
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
// #region stats

// Added は入れた回数を返す。
func (f *Filter) Added() int { return f.added }

// Bits は総ビット数、Hashes は使うハッシュ関数の数を返す。
func (f *Filter) Bits() uint64 { return f.m }
func (f *Filter) Hashes() int  { return f.k }

// FillRatio は立っているビットの割合を返す。
//
// 詰まり具合がそのまま精度になる。半分埋まれば、k 個すべてが
// たまたま立っている確率は (1/2)^k になる。
func (f *Filter) FillRatio() float64 {
	var on int
	for _, w := range f.bitset {
		on += popcount(w)
	}
	return float64(on) / float64(f.m)
}

// EstimatedRate は今の詰まり具合から見込まれる偽陽性率を返す。
//
// 立っている割合の k 乗。入れた件数を知らなくても、ビットを見るだけで
// 「もう効いていない」が分かる。
func (f *Filter) EstimatedRate() float64 {
	r := f.FillRatio()
	out := 1.0
	for i := 0; i < f.k; i++ {
		out *= r
	}
	return out
}

func popcount(w uint64) int {
	n := 0
	for w != 0 {
		w &= w - 1
		n++
	}
	return n
}

// #endregion stats

func (f *Filter) bits() uint64 { return f.m }

// hashes はハッシュ関数の数 k を返す(テスト・可視化用)。
func (f *Filter) hashes() int { return f.k }
