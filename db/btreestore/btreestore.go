// Package btreestore は B-Tree を「メモリ上のポインタの木」から
// 「ディスクのページに載った木」に載せ替えた実装。
//
// data-structures/btree はノードを *node ポインタで繋いだメモリ内の木だった。
// ここでは各ノードを1ページに直列化し、ポインタの代わりにページIDで参照する。
// ページの読み書きはすべて bufferpool を通すので、よく使うノード(根に近いほど何度も
// 通る)は自然とキャッシュに残る。これが「B-Tree + バッファプール = 実物のインデックス」。
package btreestore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/esh2n/sharin/db/bufferpool"
)

// メタページ(ページ0)のレイアウト: [rootID 8B][nextID 8B]
// nextID == 0 は「まだ初期化されていないファイル」の印。
const metaPage = 0

// #region node
// node はメモリ上での作業用のノード表現。ディスクにはページとして直列化される。
// leaf でなければ children が len(keys)+1 個ある(B-Tree の不変条件)。
type node struct {
	leaf     bool
	keys     []uint64
	vals     []uint64
	children []uint64 // 子ノードのページID
}

// ページレイアウト: [leaf 1B][numKeys 2B][keys...][vals...][children...(内部ノードのみ)]
func serialize(n *node) []byte {
	buf := make([]byte, bufferpool.PageSize)
	if n.leaf {
		buf[0] = 1
	}
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(n.keys)))
	off := 3
	for _, k := range n.keys {
		binary.BigEndian.PutUint64(buf[off:], k)
		off += 8
	}
	for _, v := range n.vals {
		binary.BigEndian.PutUint64(buf[off:], v)
		off += 8
	}
	if !n.leaf {
		for _, c := range n.children {
			binary.BigEndian.PutUint64(buf[off:], c)
			off += 8
		}
	}
	return buf
}

func deserialize(buf []byte) *node {
	n := &node{leaf: buf[0] == 1}
	num := int(binary.BigEndian.Uint16(buf[1:3]))
	off := 3
	n.keys = make([]uint64, num)
	for i := range n.keys {
		n.keys[i] = binary.BigEndian.Uint64(buf[off:])
		off += 8
	}
	n.vals = make([]uint64, num)
	for i := range n.vals {
		n.vals[i] = binary.BigEndian.Uint64(buf[off:])
		off += 8
	}
	if !n.leaf {
		n.children = make([]uint64, num+1)
		for i := range n.children {
			n.children[i] = binary.BigEndian.Uint64(buf[off:])
			off += 8
		}
	}
	return n
}

// #endregion node

// #region tree
// Tree は uint64 キー → uint64 値の永続 B-Tree。
type Tree struct {
	pool   *bufferpool.Pool
	t      int // 最小次数。ノードのキー数は最大 2t-1
	rootID uint64
	nextID uint64 // 次に割り当てるページID
}

// Open はファイルを開き、既存の木を復元するか、空の木を初期化して返す。
func Open(path string, degree int) (*Tree, error) {
	if degree < 2 {
		return nil, errors.New("btreestore: degree must be >= 2")
	}
	// 2t-1 キー + 値 + 子が1ページに収まることを確かめる。
	maxKeys := 2*degree - 1
	if 3+maxKeys*8*2+(maxKeys+1)*8 > bufferpool.PageSize {
		return nil, fmt.Errorf("btreestore: degree %d too large for page size %d", degree, bufferpool.PageSize)
	}
	pool, err := bufferpool.New(path, 128)
	if err != nil {
		return nil, fmt.Errorf("btreestore: %w", err)
	}
	tr := &Tree{pool: pool, t: degree}

	meta, err := pool.Read(metaPage)
	if err != nil {
		pool.Close()
		return nil, err
	}
	tr.rootID = binary.BigEndian.Uint64(meta[0:8])
	tr.nextID = binary.BigEndian.Uint64(meta[8:16])

	if tr.nextID == 0 {
		// 新規ファイル。空の葉を root にする。
		tr.rootID = 1
		tr.nextID = 2
		if err := tr.writeNode(tr.rootID, &node{leaf: true}); err != nil {
			pool.Close()
			return nil, err
		}
		if err := tr.writeMeta(); err != nil {
			pool.Close()
			return nil, err
		}
	}
	return tr, nil
}

// Close は木を閉じる(dirty ページの書き戻しを含む)。
func (tr *Tree) Close() error {
	if err := tr.writeMeta(); err != nil {
		tr.pool.Close()
		return err
	}
	return tr.pool.Close()
}

func (tr *Tree) writeMeta() error {
	buf := make([]byte, bufferpool.PageSize)
	binary.BigEndian.PutUint64(buf[0:8], tr.rootID)
	binary.BigEndian.PutUint64(buf[8:16], tr.nextID)
	return tr.pool.Write(metaPage, buf)
}

func (tr *Tree) allocate() uint64 {
	id := tr.nextID
	tr.nextID++
	return id
}

func (tr *Tree) readNode(id uint64) (*node, error) {
	buf, err := tr.pool.Read(int(id))
	if err != nil {
		return nil, err
	}
	return deserialize(buf), nil
}

func (tr *Tree) writeNode(id uint64, n *node) error {
	return tr.pool.Write(int(id), serialize(n))
}

// #endregion tree

// #region get
// Get は key の値を返す。根から降りて、各ノードで二分探索するだけ。
// たどったノードの数がページ読みの回数で、根に近いページはキャッシュに載っている。
func (tr *Tree) Get(key uint64) (uint64, bool, error) {
	id := tr.rootID
	for {
		n, err := tr.readNode(id)
		if err != nil {
			return 0, false, err
		}
		pos := sort.Search(len(n.keys), func(i int) bool { return n.keys[i] >= key })
		if pos < len(n.keys) && n.keys[pos] == key {
			return n.vals[pos], true, nil
		}
		if n.leaf {
			return 0, false, nil
		}
		id = n.children[pos]
	}
}

// #endregion get

// #region insert
// Insert は key=value を挿入(既存キーなら値を更新)する。
// アルゴリズムはメモリ版 btree と同じ proactive split。違いはポインタでなく
// ページIDを辿り、変更したノードを writeNode で書き戻すこと。
func (tr *Tree) Insert(key, value uint64) error {
	root, err := tr.readNode(tr.rootID)
	if err != nil {
		return err
	}
	if len(root.keys) == 2*tr.t-1 {
		newRootID := tr.allocate()
		newRoot := &node{leaf: false, children: []uint64{tr.rootID}}
		if err := tr.writeNode(newRootID, newRoot); err != nil {
			return err
		}
		tr.rootID = newRootID
		if err := tr.splitChild(newRootID, 0); err != nil {
			return err
		}
	}
	return tr.insertNonFull(tr.rootID, key, value)
}

func (tr *Tree) insertNonFull(id, key, value uint64) error {
	n, err := tr.readNode(id)
	if err != nil {
		return err
	}
	pos := sort.Search(len(n.keys), func(i int) bool { return n.keys[i] >= key })
	if pos < len(n.keys) && n.keys[pos] == key {
		n.vals[pos] = value // 更新
		return tr.writeNode(id, n)
	}
	if n.leaf {
		n.keys = insertUint64(n.keys, pos, key)
		n.vals = insertUint64(n.vals, pos, value)
		return tr.writeNode(id, n)
	}

	child, err := tr.readNode(n.children[pos])
	if err != nil {
		return err
	}
	if len(child.keys) == 2*tr.t-1 {
		if err := tr.splitChild(id, pos); err != nil {
			return err
		}
		// 分割で親が変わったので読み直す。
		n, err = tr.readNode(id)
		if err != nil {
			return err
		}
		if key > n.keys[pos] {
			pos++
		} else if key == n.keys[pos] {
			n.vals[pos] = value
			return tr.writeNode(id, n)
		}
	}
	return tr.insertNonFull(n.children[pos], key, value)
}

// splitChild は parent.children[i] (満杯)を2つに割り、真ん中を parent へ昇格させる。
func (tr *Tree) splitChild(parentID uint64, i int) error {
	parent, err := tr.readNode(parentID)
	if err != nil {
		return err
	}
	childID := parent.children[i]
	child, err := tr.readNode(childID)
	if err != nil {
		return err
	}
	t := tr.t

	rightID := tr.allocate()
	right := &node{leaf: child.leaf}
	right.keys = append(right.keys, child.keys[t:]...)
	right.vals = append(right.vals, child.vals[t:]...)
	if !child.leaf {
		right.children = append(right.children, child.children[t:]...)
		child.children = child.children[:t]
	}
	midKey, midVal := child.keys[t-1], child.vals[t-1]
	child.keys = child.keys[:t-1]
	child.vals = child.vals[:t-1]

	parent.keys = insertUint64(parent.keys, i, midKey)
	parent.vals = insertUint64(parent.vals, i, midVal)
	parent.children = insertUint64(parent.children, i+1, rightID)

	if err := tr.writeNode(childID, child); err != nil {
		return err
	}
	if err := tr.writeNode(rightID, right); err != nil {
		return err
	}
	return tr.writeNode(parentID, parent)
}

// insertUint64 は s の位置 i に x を差し込んだ新しいスライスを返す。
func insertUint64(s []uint64, i int, x uint64) []uint64 {
	s = append(s, 0)
	copy(s[i+1:], s[i:])
	s[i] = x
	return s
}

// #endregion insert

// Scan は全キーを昇順で返す(中間順走査)。
func (tr *Tree) Scan() ([]uint64, error) {
	var out []uint64
	var walk func(id uint64) error
	walk = func(id uint64) error {
		n, err := tr.readNode(id)
		if err != nil {
			return err
		}
		for i := range n.keys {
			if !n.leaf {
				if err := walk(n.children[i]); err != nil {
					return err
				}
			}
			out = append(out, n.keys[i])
		}
		if !n.leaf {
			return walk(n.children[len(n.keys)])
		}
		return nil
	}
	if err := walk(tr.rootID); err != nil {
		return nil, err
	}
	return out, nil
}
