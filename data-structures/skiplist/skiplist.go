// Package skiplist はスキップリストの最小実装。
//
// スキップリストは「確率で階層を作った連結リスト」。最下段は全要素を繋ぐ普通の
// ソート済みリストで、その上に「飛ばし読み用」の疎なリストを積んでいく。
// 各ノードの高さをコイン投げで決めるだけで、平衡木(AVL/赤黒木)の回転のような
// 複雑な操作なしに、検索・挿入・削除が平均 O(log n) になる。
// Redis の sorted set の中身であり、「確率的平衡」の代表例。
package skiplist

import "math/rand"

// maxLevel は段数の上限。2^maxLevel 件くらいまで対数性能が保てる。
const maxLevel = 16

// probability は「次の段にも現れる」確率。0.5 = コイン投げ。
const probability = 0.5

// #region node
// node はキー・値と、各段での「次のノード」への参照 next を持つ。
// len(next) がこのノードの高さ。high なノードほど飛ばし読みに使われる。
type node struct {
	key  int
	val  int
	next []*node // next[i] = 第 i 段での次のノード
}

// SkipList は先頭番兵 head を持つスキップリスト。
type SkipList struct {
	head  *node
	level int // 現在使われている最大段数
	count int
	rng   *rand.Rand

	steps int // Search で進んだ手数の累計
}

// New は空のスキップリストを返す。
func New() *SkipList {
	return &SkipList{
		head:  &node{next: make([]*node, maxLevel)},
		level: 1,
		rng:   rand.New(rand.NewSource(1)),
	}
}

// randomLevel はコイン投げで新ノードの高さを決める。
// 表が続く限り段を増やす: 高さ1が確率1/2、高さ2が1/4、高さ3が1/8…。
// この分布が「上の段ほど疎」を作り、対数性能を生む。
func (sl *SkipList) randomLevel() int {
	lvl := 1
	for lvl < maxLevel && sl.rng.Float64() < probability {
		lvl++
	}
	return lvl
}

// #endregion node

// #region search
// Search は key の値を返す。上の段から降りていく。
// 各段で「次が key を超える手前」まで進み、超えたら1段下りる。
// 上の疎な段で大きく飛ばし、下の段で細かく詰める。これが飛ばし読みになる。
func (sl *SkipList) Search(key int) (int, bool) {
	x := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for x.next[i] != nil && x.next[i].key < key {
			sl.steps++
			x = x.next[i]
		}
		sl.steps++ // 進めずに1段下りるのも1手として数える
	}
	x = x.next[0] // 最下段で1歩進むと候補
	if x != nil && x.key == key {
		return x.val, true
	}
	return 0, false
}

// #endregion search

// #region insert
// Insert は key=value を挿入(既存なら更新)する。
// まず各段で「key が入る位置の直前ノード」を update に記録し、
// 新ノードの高さを randomLevel で決めて、その高さぶんのポインタを繋ぎ替える。
func (sl *SkipList) Insert(key, value int) {
	update := make([]*node, maxLevel)
	x := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for x.next[i] != nil && x.next[i].key < key {
			x = x.next[i]
		}
		update[i] = x // 第 i 段で key の手前に来るノード
	}

	if next := x.next[0]; next != nil && next.key == key {
		next.val = value // 既存キーの更新
		return
	}

	lvl := sl.randomLevel()
	if lvl > sl.level {
		// 新しい高さのぶん、head を直前ノードとして埋める。
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.head
		}
		sl.level = lvl
	}

	n := &node{key: key, val: value, next: make([]*node, lvl)}
	for i := 0; i < lvl; i++ {
		n.next[i] = update[i].next[i]
		update[i].next[i] = n
	}
	sl.count++
}

// #endregion insert

// #region delete
// Delete は key を消す。各段で直前ノードを求め、そのポインタを1つ飛ばしに繋ぎ替える。
func (sl *SkipList) Delete(key int) bool {
	update := make([]*node, maxLevel)
	x := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for x.next[i] != nil && x.next[i].key < key {
			x = x.next[i]
		}
		update[i] = x
	}

	target := x.next[0]
	if target == nil || target.key != key {
		return false
	}
	for i := 0; i < sl.level; i++ {
		if update[i].next[i] != target {
			break // この段より上には target はいない
		}
		update[i].next[i] = target.next[i]
	}
	// 使われなくなった上位段を詰める。
	for sl.level > 1 && sl.head.next[sl.level-1] == nil {
		sl.level--
	}
	sl.count--
	return true
}

// #endregion delete

// #region stats

// Steps は Search で進んだ手数の累計を返す。
//
// 引いた回数で割れば、1回あたり何手たどったかになる。
// 上の段で飛ばせているなら、件数が増えても対数でしか伸びない。
func (sl *SkipList) Steps() int { return sl.steps }

// ResetStats は数え直す。
func (sl *SkipList) ResetStats() { sl.steps = 0 }

// Height は今使われている段数を返す。
func (sl *SkipList) Height() int { return sl.level }

// LevelCounts は各段に居るノードの数を返す。
//
// コイン投げで高さを決めているので、上へ行くほどおよそ半分ずつになる。
func (sl *SkipList) LevelCounts() []int {
	out := make([]int, sl.level)
	for i := 0; i < sl.level; i++ {
		for x := sl.head.next[i]; x != nil; x = x.next[i] {
			out[i]++
		}
	}
	return out
}

// #endregion stats

// Len は要素数を返す。
func (sl *SkipList) Len() int { return sl.count }

// height は現在の段数を返す(テスト・可視化用)。
func (sl *SkipList) height() int { return sl.level }

// Keys は全キーを昇順で返す(最下段をたどるだけ)。
func (sl *SkipList) Keys() []int {
	var out []int
	for x := sl.head.next[0]; x != nil; x = x.next[0] {
		out = append(out, x.key)
	}
	return out
}
