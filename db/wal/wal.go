// Package wal は Write-Ahead Log の最小実装。
//
// 題材は送金: 口座Aから引いて口座Bに足す、という「2ページの書き換え」を
// クラッシュがいつ起きても中途半端にならないように行う。
// 原則は1つ:「**ページを書き換える前に、やることをログに書け**」(write-ahead)。
//
//  1. WAL に変更内容(set レコード)を追記する
//  2. WAL に commit レコードを追記して fsync する ← ここが「やる」と決まる瞬間
//  3. データファイルのページを書き換える
//  4. 完了したら WAL を空にする(チェックポイント)
//
// どの瞬間に死んでも、再起動時のリカバリが「commit 済みなら redo、なければ捨てる」で
// つじつまを合わせる。
package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const initialBalance = 1000

// Set は「slot 番の口座の残高を Value にする」という変更1件。
// 「+100する」ではなく「1100にする」と書くのがポイントで、
// これにより同じレコードを何度適用しても結果が変わらない(冪等)。
type Set struct {
	Slot  int32
	Value int64
}

// #region format
// WAL レコード: [op 1B][slot 4B][value 8B]
// op=1: set(変更内容)、op=2: commit(このバッチを確定する印。slot/value は未使用)
const recordSize = 13

const (
	opSet    = 1
	opCommit = 2
)

func encodeRecord(op byte, s Set) []byte {
	buf := make([]byte, recordSize)
	buf[0] = op
	binary.BigEndian.PutUint32(buf[1:5], uint32(s.Slot))
	binary.BigEndian.PutUint64(buf[5:13], uint64(s.Value))
	return buf
}

// #endregion format

// DB は固定数の口座(データファイル)と WAL ファイルを持つ。
type DB struct {
	mu    sync.Mutex
	data  *os.File // 口座ごとに 8B の残高が並ぶ「ページ」の列
	wal   *os.File
	slots int
}

// Open はデータファイルと WAL を開き、リカバリを実行して整合した状態で返す。
func Open(dir string, slots int) (*DB, error) {
	if slots <= 0 {
		return nil, errors.New("wal: slots must be positive")
	}
	data, err := os.OpenFile(filepath.Join(dir, "data.db"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("wal: open data: %w", err)
	}
	walF, err := os.OpenFile(filepath.Join(dir, "wal.log"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		data.Close()
		return nil, fmt.Errorf("wal: open wal: %w", err)
	}
	db := &DB{data: data, wal: walF, slots: slots}
	if err := db.initData(); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.recover(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// initData はデータファイルが新規なら初期残高で埋める。
func (db *DB) initData() error {
	info, err := db.data.Stat()
	if err != nil {
		return err
	}
	if info.Size() >= int64(db.slots)*8 {
		return nil
	}
	for i := 0; i < db.slots; i++ {
		if err := db.writeSlot(Set{Slot: int32(i), Value: initialBalance}); err != nil {
			return err
		}
	}
	return nil
}

// Close はファイルを閉じる。
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	err1 := db.data.Close()
	err2 := db.wal.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// Balance は口座の残高を返す。
func (db *DB) Balance(slot int) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.readSlot(slot)
}

func (db *DB) readSlot(slot int) (int64, error) {
	if slot < 0 || slot >= db.slots {
		return 0, fmt.Errorf("wal: slot %d out of range", slot)
	}
	var buf [8]byte
	if _, err := db.data.ReadAt(buf[:], int64(slot)*8); err != nil {
		return 0, fmt.Errorf("wal: read slot: %w", err)
	}
	return int64(binary.BigEndian.Uint64(buf[:])), nil
}

func (db *DB) writeSlot(s Set) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(s.Value))
	if _, err := db.data.WriteAt(buf[:], int64(s.Slot)*8); err != nil {
		return fmt.Errorf("wal: write slot: %w", err)
	}
	return nil
}

// #region transfer
// Transfer は from から to へ amount を送金する。
// 「先にログ、後でページ」の順序がこの関数のすべて。
func (db *DB) Transfer(from, to int, amount int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if amount <= 0 {
		return errors.New("wal: amount must be positive")
	}
	if from == to {
		return errors.New("wal: from and to must differ")
	}
	a, err := db.readSlot(from)
	if err != nil {
		return err
	}
	b, err := db.readSlot(to)
	if err != nil {
		return err
	}
	if a < amount {
		return fmt.Errorf("wal: insufficient balance in slot %d", from)
	}

	sets := []Set{
		{Slot: int32(from), Value: a - amount},
		{Slot: int32(to), Value: b + amount},
	}
	if err := db.logCommitted(sets); err != nil { // 1-2. ログに書いて commit
		return err
	}
	if err := db.applySets(sets); err != nil { // 3. ページを書き換える
		return err
	}
	return db.checkpoint() // 4. 完了したので WAL を空にする
}

// #endregion transfer

// #region steps
// logSets は変更内容を WAL に追記する(まだ「やる」とは決まっていない)。
func (db *DB) logSets(sets []Set) error {
	for _, s := range sets {
		if _, err := db.wal.Write(encodeRecord(opSet, s)); err != nil {
			return fmt.Errorf("wal: append: %w", err)
		}
	}
	return nil
}

// logCommitted は変更内容 + commit レコードを書き、fsync でディスクに届いたことを保証する。
// この fsync が完了した瞬間が「送金は実行される」と確定する境界線。
func (db *DB) logCommitted(sets []Set) error {
	if err := db.logSets(sets); err != nil {
		return err
	}
	if _, err := db.wal.Write(encodeRecord(opCommit, Set{})); err != nil {
		return fmt.Errorf("wal: append commit: %w", err)
	}
	return db.wal.Sync()
}

// applySets は変更をデータファイルに適用する。
func (db *DB) applySets(sets []Set) error {
	for _, s := range sets {
		if err := db.writeSlot(s); err != nil {
			return err
		}
	}
	return nil
}

// checkpoint は適用済みの WAL を空にする。次のリカバリで再生するものが無くなる。
func (db *DB) checkpoint() error {
	if err := db.wal.Truncate(0); err != nil {
		return fmt.Errorf("wal: checkpoint: %w", err)
	}
	_, err := db.wal.Seek(0, 0)
	return err
}

// #endregion steps

// #region recover
// recover は起動時に WAL を読み、commit 済みバッチだけをデータファイルに再適用(redo)する。
// commit の無い書きかけバッチは捨てる。redo は冪等(Set は絶対値)なので、
// 前回どこまで適用済みだったかを知らなくても、全部やり直せば必ず正しい状態になる。
func (db *DB) recover() error {
	raw, err := os.ReadFile(db.wal.Name())
	if err != nil {
		return fmt.Errorf("wal: read wal: %w", err)
	}

	var pending []Set
	for off := 0; off+recordSize <= len(raw); off += recordSize {
		rec := raw[off : off+recordSize]
		s := Set{
			Slot:  int32(binary.BigEndian.Uint32(rec[1:5])),
			Value: int64(binary.BigEndian.Uint64(rec[5:13])),
		}
		switch rec[0] {
		case opSet:
			pending = append(pending, s)
		case opCommit:
			if err := db.applySets(pending); err != nil { // redo
				return err
			}
			pending = nil
		}
	}
	// ループを抜けた時点で pending に残っているものは commit されていない書きかけ。
	// 何もせず捨てる。
	return db.checkpoint()
}

// #endregion recover
