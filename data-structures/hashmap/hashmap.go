// Package hashmap はチェイン法によるハッシュマップの最小実装。
//
// ハッシュマップが平均 O(1) で引ける仕組みは2つの部品でできている:
//   - ハッシュ関数でキーを「バケット番号」に変換し、その1バケットだけ見る
//   - 衝突(別のキーが同じバケットに落ちる)はバケット内のリストで吸収する(チェイン法)
//
// そして「1バケットあたり平均何個か」(負荷率)が上がりすぎたらバケットを増やす。
// これを怠ると衝突が増えて O(1) が O(n) に劣化する。
package hashmap

import "hash/fnv"

// maxLoadFactor は「件数 / バケット数」の上限。これを超えたらバケットを倍増する。
const maxLoadFactor = 0.75

// #region types
type entry[K comparable, V any] struct {
	key K
	val V
}

// Map はキー K → 値 V のハッシュマップ。hash はキーをハッシュ値に変換する関数。
// slots[i] は「i 番のバケットに落ちた要素のリスト」(チェイン)。
type Map[K comparable, V any] struct {
	hash  func(K) uint64
	slots [][]entry[K, V]
	count int

	// probes は Get で「鍵を1つ見比べた」回数の累計。
	// 平均 O(1) が本当かどうかは、ここを数えれば分かる。
	probes int
	// resizes は配り直した回数。
	resizes int
}

// New は空のマップを返す。hash にはキーのハッシュ関数を渡す(HashString / HashInt など)。
func New[K comparable, V any](hash func(K) uint64) *Map[K, V] {
	return &Map[K, V]{
		hash:  hash,
		slots: make([][]entry[K, V], 8), // 小さめの初期バケット数
	}
}

// #endregion types

// #region ops
// bucketIndex はキーが落ちるバケット番号を返す。
// ハッシュ値をバケット数で割った余り。バケット数が2の冪ならビットマスクでも同じ。
func (m *Map[K, V]) bucketIndex(key K) int {
	return int(m.hash(key) % uint64(len(m.slots)))
}

// Put は key=value を入れる(既存キーなら更新)。
func (m *Map[K, V]) Put(key K, value V) {
	i := m.bucketIndex(key)
	for j := range m.slots[i] {
		if m.slots[i][j].key == key {
			m.slots[i][j].val = value // 更新
			return
		}
	}
	m.slots[i] = append(m.slots[i], entry[K, V]{key, value})
	m.count++

	if float64(m.count)/float64(len(m.slots)) > maxLoadFactor {
		m.resize()
	}
}

// Get は key の値を返す。落ちるバケットを1つ求め、その中のリストだけを線形探索する。
// 負荷率が低ければリストは平均1個ほどなので、これが平均 O(1) の正体。
func (m *Map[K, V]) Get(key K) (V, bool) {
	i := m.bucketIndex(key)
	for _, e := range m.slots[i] {
		m.probes++
		if e.key == key {
			return e.val, true
		}
	}
	var zero V
	return zero, false
}

// Delete は key を消す。消したら true。
func (m *Map[K, V]) Delete(key K) bool {
	i := m.bucketIndex(key)
	for j := range m.slots[i] {
		if m.slots[i][j].key == key {
			// バケット内リストから j 番を取り除く(順序は問わないので末尾で埋める)。
			last := len(m.slots[i]) - 1
			m.slots[i][j] = m.slots[i][last]
			m.slots[i] = m.slots[i][:last]
			m.count--
			return true
		}
	}
	return false
}

// #endregion ops

// #region stats

// Probes は Get で鍵を見比べた回数の累計を返す。
//
// 引いた回数で割れば、1回あたり何個たどったかになる。
// 負荷率を守れていれば、件数がいくら増えてもここは 1 前後で止まる。
func (m *Map[K, V]) Probes() int { return m.probes }

// Resizes は配り直した回数を返す。倍々に増やすので、件数の対数ぶんしか起きない。
func (m *Map[K, V]) Resizes() int { return m.resizes }

// LoadFactor は「件数 / バケット数」。ここが上限を超えると配り直す。
func (m *Map[K, V]) LoadFactor() float64 {
	return float64(m.count) / float64(len(m.slots))
}

// ResetStats は数え直す。
func (m *Map[K, V]) ResetStats() { m.probes = 0 }

// #endregion stats

// #region ops2

// Len は要素数を返す。
func (m *Map[K, V]) Len() int { return m.count }

// buckets は現在のバケット数を返す(テスト・可視化用)。
func (m *Map[K, V]) buckets() int { return len(m.slots) }

// Keys は全キーを返す(順序は不定)。
func (m *Map[K, V]) Keys() []K {
	keys := make([]K, 0, m.count)
	for _, bucket := range m.slots {
		for _, e := range bucket {
			keys = append(keys, e.key)
		}
	}
	return keys
}

// #endregion ops2

// #region resize
// resize はバケット数を倍にして、全要素を配り直す(rehash)。
// バケット数が変わると bucketIndex の余りも変わるので、全要素の引っ越しが必要。
// 1回の resize は O(n) かかる。この「たまに重い」が、平均 O(1) の裏側にある。
func (m *Map[K, V]) resize() {
	m.resizes++
	old := m.slots
	m.slots = make([][]entry[K, V], len(old)*2)
	for _, bucket := range old {
		for _, e := range bucket {
			i := m.bucketIndex(e.key)
			m.slots[i] = append(m.slots[i], e)
		}
	}
}

// #endregion resize

// #region hashers
// HashString は文字列の FNV-1a ハッシュ。よく散らばる定番の非暗号ハッシュ。
func HashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// HashInt は整数のハッシュ。下位ビットの偏りを混ぜる簡単な撹拌(splitmix 風)。
func HashInt(n int) uint64 {
	x := uint64(n)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// #endregion hashers
