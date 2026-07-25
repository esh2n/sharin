// Package btreewal は B-Treeページストア([db/btreestore])に WAL を重ねて、
// クラッシュしても壊れないインデックスにしたもの。db 編の最終回。
//
// btreestore の弱点は「1回の Insert が split で複数ページを書き換えるのに、
// その途中でクラッシュすると木が壊れる」ことだった。
// WAL 編で送金の2ページ更新を守ったのと同じ手を、B-Tree のページ群に使う:
//
//  1. Insert の全ページ変更を「トランザクション(txn)」にため込む(まだディスクに書かない)
//  2. txn を丸ごと WAL に書いて fsync する ← ここで「やる」と確定
//  3. txn を実ページ(bufferpool)に適用する
//  4. WAL を空にする(checkpoint)
//
// full-page write(ページ全体をログに書く)なので redo は冪等。どこで死んでも直せる。
package btreewal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/esh2n/sharin/db/bufferpool"
)

const metaPage = 0

// #region node
type node struct {
	leaf     bool
	keys     []uint64
	vals     []uint64
	children []uint64
}

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
// Tree は WAL で守られた永続 B-Tree。
// txn は「今の Insert でまだ確定していないページ変更」の一時置き場。
type Tree struct {
	pool   *bufferpool.Pool
	wal    *os.File
	t      int
	rootID uint64
	nextID uint64
	txn    map[uint64][]byte // pageID -> 新しいページ内容(未確定)
}

// Open はデータファイルと WAL を開き、リカバリしてから返す。
func Open(dir string, degree int) (*Tree, error) {
	if degree < 2 {
		return nil, errors.New("btreewal: degree must be >= 2")
	}
	maxKeys := 2*degree - 1
	if 3+maxKeys*8*2+(maxKeys+1)*8 > bufferpool.PageSize {
		return nil, fmt.Errorf("btreewal: degree %d too large for page size %d", degree, bufferpool.PageSize)
	}
	pool, err := bufferpool.New(filepath.Join(dir, "data.db"), 128)
	if err != nil {
		return nil, fmt.Errorf("btreewal: open data: %w", err)
	}
	wal, err := os.OpenFile(filepath.Join(dir, "wal.log"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("btreewal: open wal: %w", err)
	}
	tr := &Tree{pool: pool, wal: wal, t: degree, txn: map[uint64][]byte{}}

	if err := tr.recover(); err != nil {
		tr.closeFilesOnly()
		return nil, err
	}

	meta, err := pool.Read(metaPage)
	if err != nil {
		tr.closeFilesOnly()
		return nil, err
	}
	tr.rootID = binary.BigEndian.Uint64(meta[0:8])
	tr.nextID = binary.BigEndian.Uint64(meta[8:16])
	if tr.nextID == 0 {
		tr.rootID = 1
		tr.nextID = 2
		tr.writeNode(tr.rootID, &node{leaf: true})
		tr.stageMeta()
		if err := tr.commit(); err != nil {
			tr.closeFilesOnly()
			return nil, err
		}
	}
	return tr, nil
}

// Close は木を閉じる。
func (tr *Tree) Close() error {
	return tr.closeFilesOnly()
}

// closeFilesOnly はファイルを閉じるだけ(checkpoint やメタ書き込みをしない)。
// テストでは「ページ適用の前にプロセスが死ぬ」クラッシュの再現に使う。
func (tr *Tree) closeFilesOnly() error {
	err1 := tr.pool.Close()
	err2 := tr.wal.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (tr *Tree) stageMeta() {
	buf := make([]byte, bufferpool.PageSize)
	binary.BigEndian.PutUint64(buf[0:8], tr.rootID)
	binary.BigEndian.PutUint64(buf[8:16], tr.nextID)
	tr.txn[metaPage] = buf
}

func (tr *Tree) allocate() uint64 {
	id := tr.nextID
	tr.nextID++
	return id
}

// readNode は txn にあればそれを、なければ実ページを読む。
// prepareInsert 中に自分が書いた(まだ未確定の)ノードを読み返すために txn を優先する。
func (tr *Tree) readNode(id uint64) (*node, error) {
	if buf, ok := tr.txn[id]; ok {
		return deserialize(buf), nil
	}
	buf, err := tr.pool.Read(int(id))
	if err != nil {
		return nil, err
	}
	return deserialize(buf), nil
}

// writeNode は変更を txn にため込むだけ。実ページにはまだ書かない。
func (tr *Tree) writeNode(id uint64, n *node) {
	tr.txn[id] = serialize(n)
}

// #endregion tree

// #region txn
// Insert は key=value を1トランザクションとして挿入する。
// prepareInsert で全変更をため、commit で WAL 経由に確定する。
func (tr *Tree) Insert(key, value uint64) error {
	tr.prepareInsert(key, value)
	return tr.commit()
}

// prepareInsert は B-Tree 挿入を実行するが、変更は txn に積むだけで確定しない。
// アルゴリズムは btreestore と同一(proactive split)。
func (tr *Tree) prepareInsert(key, value uint64) {
	root, _ := tr.readNode(tr.rootID)
	if len(root.keys) == 2*tr.t-1 {
		newRootID := tr.allocate()
		tr.writeNode(newRootID, &node{leaf: false, children: []uint64{tr.rootID}})
		tr.rootID = newRootID
		tr.splitChild(newRootID, 0)
	}
	tr.insertNonFull(tr.rootID, key, value)
	tr.stageMeta()
}

func (tr *Tree) insertNonFull(id, key, value uint64) {
	n, _ := tr.readNode(id)
	pos := sort.Search(len(n.keys), func(i int) bool { return n.keys[i] >= key })
	if pos < len(n.keys) && n.keys[pos] == key {
		n.vals[pos] = value
		tr.writeNode(id, n)
		return
	}
	if n.leaf {
		n.keys = insertUint64(n.keys, pos, key)
		n.vals = insertUint64(n.vals, pos, value)
		tr.writeNode(id, n)
		return
	}
	child, _ := tr.readNode(n.children[pos])
	if len(child.keys) == 2*tr.t-1 {
		tr.splitChild(id, pos)
		n, _ = tr.readNode(id)
		if key > n.keys[pos] {
			pos++
		} else if key == n.keys[pos] {
			n.vals[pos] = value
			tr.writeNode(id, n)
			return
		}
	}
	tr.insertNonFull(n.children[pos], key, value)
}

func (tr *Tree) splitChild(parentID uint64, i int) {
	parent, _ := tr.readNode(parentID)
	childID := parent.children[i]
	child, _ := tr.readNode(childID)
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

	tr.writeNode(childID, child)
	tr.writeNode(rightID, right)
	tr.writeNode(parentID, parent)
}

func insertUint64(s []uint64, i int, x uint64) []uint64 {
	s = append(s, 0)
	copy(s[i+1:], s[i:])
	s[i] = x
	return s
}

// commit は txn を「WAL に書く → 実ページに適用 → WAL を空にする」で確定する。
func (tr *Tree) commit() error {
	if len(tr.txn) == 0 {
		return nil
	}
	if err := tr.logToWAL(); err != nil {
		return err
	}
	if err := tr.applyTxn(); err != nil {
		return err
	}
	return tr.checkpoint()
}

// #endregion txn

// #region wal
// WAL レコード(固定長): [op 1B][pageID 8B][data PageSize]
// op=1: page(この内容にする)、op=2: commit(このバッチを確定。pageID/data は未使用)
const opPage = 1
const opCommit = 2

var recordSize = 9 + bufferpool.PageSize

// logToWAL は txn の全ページを WAL に書き、commit レコード + fsync で確定させる。
// この fsync が完了した瞬間が「この Insert はやる」と決まる境界線。
func (tr *Tree) logToWAL() error {
	for id, data := range tr.txn {
		rec := make([]byte, recordSize)
		rec[0] = opPage
		binary.BigEndian.PutUint64(rec[1:9], id)
		copy(rec[9:], data)
		if _, err := tr.wal.Write(rec); err != nil {
			return fmt.Errorf("btreewal: wal write: %w", err)
		}
	}
	commit := make([]byte, recordSize)
	commit[0] = opCommit
	if _, err := tr.wal.Write(commit); err != nil {
		return fmt.Errorf("btreewal: wal commit: %w", err)
	}
	return tr.wal.Sync()
}

// applyTxn は txn の全ページを実ページ(bufferpool)に書く。
func (tr *Tree) applyTxn() error {
	for id, data := range tr.txn {
		if err := tr.pool.Write(int(id), data); err != nil {
			return fmt.Errorf("btreewal: apply page %d: %w", id, err)
		}
	}
	return nil
}

// checkpoint は適用済みの WAL を空にし、txn をクリアする。
func (tr *Tree) checkpoint() error {
	tr.txn = map[uint64][]byte{}
	if err := tr.wal.Truncate(0); err != nil {
		return fmt.Errorf("btreewal: checkpoint: %w", err)
	}
	_, err := tr.wal.Seek(0, 0)
	return err
}

// recover は起動時に WAL を読み、commit 済みバッチのページを実ページに redo する。
// commit の無い書きかけバッチは捨てる。redo は full-page write なので冪等。
func (tr *Tree) recover() error {
	raw, err := os.ReadFile(tr.wal.Name())
	if err != nil {
		return fmt.Errorf("btreewal: read wal: %w", err)
	}

	pending := map[uint64][]byte{}
	for off := 0; off+recordSize <= len(raw); off += recordSize {
		rec := raw[off : off+recordSize]
		switch rec[0] {
		case opPage:
			id := binary.BigEndian.Uint64(rec[1:9])
			data := append([]byte(nil), rec[9:]...)
			pending[id] = data
		case opCommit:
			for id, data := range pending {
				if err := tr.pool.Write(int(id), data); err != nil {
					return err
				}
			}
			pending = map[uint64][]byte{}
		}
	}
	// commit されなかった書きかけは捨てる。WAL を空に戻す。
	if err := tr.wal.Truncate(0); err != nil {
		return err
	}
	_, err = tr.wal.Seek(0, 0)
	return err
}

// #endregion wal

// #region query
// Get は key の値を返す。
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

// Scan は全キーを昇順で返す。
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

// #endregion query
