// Package btree はメモリ内 B-Tree の最小実装(挿入と検索のみ)。
//
// B-Tree の存在理由はディスクにある。ディスクは「ページ」(数KB)単位でしか読めないので、
// 1ページ = 1ノードに大量のキーを詰めて枝分かれを太くすれば、
// 数億件でも数回のページ読みで目的のキーに届く(木が浅くなる)。
package btree

import (
	"errors"
	"sort"
)

// #region node
// node は1ノード = 1ページに相当する。
// 最小次数 t のとき、キー数は最大 2t-1、root 以外は最小 t-1 に保たれる。
type node struct {
	keys     []int
	children []*node // 内部ノードでは常に len(keys)+1 個
	leaf     bool
}

// Tree は最小次数 t の B-Tree。
type Tree struct {
	root *node
	t    int
}

// New は最小次数 t (>= 2) の空の木を返す。t=2 なら1ノード最大3キーの 2-3-4 木。
func New(t int) (*Tree, error) {
	if t < 2 {
		return nil, errors.New("btree: minimum degree must be >= 2")
	}
	return &Tree{root: &node{leaf: true}, t: t}, nil
}

// #endregion node

// #region search
// Contains は key が存在するかを返す。
// 各ノードでキー列を二分探索し、見つからなければ「key が挟まる位置」の子へ降りる。
// 訪れるノード数 = 木の高さ+1 で、これがディスクなら「ページ読み回数」になる。
func (tr *Tree) Contains(key int) bool {
	n := tr.root
	for {
		pos := sort.SearchInts(n.keys, key)
		if pos < len(n.keys) && n.keys[pos] == key {
			return true
		}
		if n.leaf {
			return false
		}
		n = n.children[pos]
	}
}

// #endregion search

// #region insert
// Insert は key を挿入する(既存なら何もしない)。
// 降りる途中で「満杯のノードを先に割っておく」のが CLRS 流の proactive split。
// こうすると分割が親に波及して戻る処理が要らず、一方通行で書ける。
func (tr *Tree) Insert(key int) {
	if len(tr.root.keys) == 2*tr.t-1 {
		// root が満杯なら新しい root を作って割る。木が高くなるのはこの瞬間だけ。
		newRoot := &node{children: []*node{tr.root}}
		tr.root = newRoot
		tr.splitChild(newRoot, 0)
	}
	tr.insertNonFull(tr.root, key)
}

func (tr *Tree) insertNonFull(n *node, key int) {
	pos := sort.SearchInts(n.keys, key)
	if pos < len(n.keys) && n.keys[pos] == key {
		return // 重複は無視
	}
	if n.leaf {
		n.keys = append(n.keys, 0)
		copy(n.keys[pos+1:], n.keys[pos:])
		n.keys[pos] = key
		return
	}
	if len(n.children[pos].keys) == 2*tr.t-1 {
		tr.splitChild(n, pos)
		// 分割で昇格したキーと比較して、降りる先を選び直す。
		if key > n.keys[pos] {
			pos++
		} else if key == n.keys[pos] {
			return
		}
	}
	tr.insertNonFull(n.children[pos], key)
}

// #endregion insert

// #region split
// splitChild は parent.children[i] (満杯: 2t-1 キー)を2つに割り、
// 真ん中のキーを parent に昇格させる。
//
//	割る前:  parent [.. keys ..]
//	         child  [a b c M d e f]   (t=4 の例。M が真ん中)
//	割った後: parent [.. M ..]
//	         left   [a b c]  right [d e f]
func (tr *Tree) splitChild(parent *node, i int) {
	t := tr.t
	child := parent.children[i]
	mid := child.keys[t-1]

	right := &node{leaf: child.leaf}
	right.keys = append(right.keys, child.keys[t:]...)
	if !child.leaf {
		right.children = append(right.children, child.children[t:]...)
		child.children = child.children[:t]
	}
	child.keys = child.keys[:t-1]

	// parent の位置 i に mid を、i+1 に right を差し込む。
	parent.keys = append(parent.keys, 0)
	copy(parent.keys[i+1:], parent.keys[i:])
	parent.keys[i] = mid
	parent.children = append(parent.children, nil)
	copy(parent.children[i+2:], parent.children[i+1:])
	parent.children[i+1] = right
}

// #endregion split

// Height は木の高さを返す(rootのみなら0)。
func (tr *Tree) Height() int {
	h := 0
	for n := tr.root; !n.leaf; n = n.children[0] {
		h++
	}
	return h
}

// Keys は全キーを昇順で返す(中間順走査)。
func (tr *Tree) Keys() []int {
	var out []int
	var walk func(n *node)
	walk = func(n *node) {
		for i, k := range n.keys {
			if !n.leaf {
				walk(n.children[i])
			}
			out = append(out, k)
		}
		if !n.leaf {
			walk(n.children[len(n.keys)])
		}
	}
	walk(tr.root)
	return out
}
