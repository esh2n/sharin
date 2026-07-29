// Package bst は二分探索木(binary search tree)の最小実装(挿入と検索のみ)。
//
// ルールは1つだけ:「左の子孫はすべて自分より小さく、右の子孫はすべて大きい」。
// このルールが保たれている限り、検索も挿入も「大小比較して片側へ降りる」だけで済み、
// 比較のたびに候補が(木が偏っていなければ)半分になる。
package bst

// #region node
// node は1つのキーと左右の子を持つ。
type node struct {
	key         int
	left, right *node
}

// Tree は二分探索木。平衡化はしない(それがこの章の主題)。
type Tree struct {
	root *node

	// compares は Contains で鍵を見比べた回数の累計。
	// 「1回の比較で候補が半分になる」が本当かは、ここを数えれば分かる。
	compares int
}

// New は空の木を返す。
func New() *Tree {
	return &Tree{}
}

// #endregion node

// #region search
// Contains は key の有無を返す。大小比較して左右どちらかへ降りるだけ。
// ループ回数は最大で木の高さ+1。
func (tr *Tree) Contains(key int) bool {
	for n := tr.root; n != nil; {
		tr.compares++
		switch {
		case key == n.key:
			return true
		case key < n.key:
			n = n.left
		default:
			n = n.right
		}
	}
	return false
}

// #endregion search

// #region insert
// Insert は key を挿入する(既存なら何もしない)。
// 検索と同じ道を降りて、行き止まり(nil)の場所に新しいノードを置く。
// どこに置かれるかは「今までに入った順序」だけで決まる。ここに崩れる種がある。
func (tr *Tree) Insert(key int) {
	pos := &tr.root
	for *pos != nil {
		n := *pos
		switch {
		case key == n.key:
			return // 重複は無視
		case key < n.key:
			pos = &n.left
		default:
			pos = &n.right
		}
	}
	*pos = &node{key: key}
}

// #endregion insert

// #region stats

// Compares は Contains で鍵を見比べた回数の累計を返す。
func (tr *Tree) Compares() int { return tr.compares }

// ResetStats は数え直す。
func (tr *Tree) ResetStats() { tr.compares = 0 }

// #endregion stats

// Height は木の高さを返す(空なら-1、rootのみなら0)。
func (tr *Tree) Height() int {
	var height func(n *node) int
	height = func(n *node) int {
		if n == nil {
			return -1
		}
		return 1 + max(height(n.left), height(n.right))
	}
	return height(tr.root)
}

// Keys は全キーを昇順で返す(中間順走査)。
func (tr *Tree) Keys() []int {
	var out []int
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			return
		}
		walk(n.left)
		out = append(out, n.key)
		walk(n.right)
	}
	walk(tr.root)
	return out
}
