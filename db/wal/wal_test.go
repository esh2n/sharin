package wal

import (
	"os"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir, 4) // 口座4つ、初期残高 1000
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dir
}

func mustBalance(t *testing.T, db *DB, slot int, want int64) {
	t.Helper()
	got, err := db.Balance(slot)
	if err != nil || got != want {
		t.Fatalf("Balance(%d) = (%d, %v), want (%d, nil)", slot, got, err, want)
	}
}

func TestTransfer(t *testing.T) {
	db, _ := openTemp(t)
	if err := db.Transfer(0, 1, 300); err != nil {
		t.Fatal(err)
	}
	mustBalance(t, db, 0, 700)
	mustBalance(t, db, 1, 1300)
}

func TestTransferValidation(t *testing.T) {
	db, _ := openTemp(t)
	if err := db.Transfer(0, 1, 2000); err == nil {
		t.Error("残高不足はエラーになるべき")
	}
	if err := db.Transfer(0, 9, 100); err == nil {
		t.Error("存在しない口座はエラーになるべき")
	}
	if err := db.Transfer(0, 1, -5); err == nil {
		t.Error("負の送金はエラーになるべき")
	}
	if err := db.Transfer(0, 0, 100); err == nil {
		t.Error("同一口座への送金はエラーになるべき")
	}
	if _, err := db.Balance(-1); err == nil {
		t.Error("範囲外の口座はエラーになるべき")
	}
	// 失敗した送金は何も変えない
	mustBalance(t, db, 0, 1000)
	mustBalance(t, db, 1, 1000)
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transfer(0, 1, 100); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	mustBalance(t, db2, 0, 900)
	mustBalance(t, db2, 1, 1100)
}

// クラッシュ地点1: WAL に commit まで書いた直後、ページ適用の前に死んだ。
// リカバリが WAL を再生(redo)して、送金は完遂される。
func TestCrashAfterCommitBeforeApply(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	sets := []Set{{Slot: 0, Value: 900}, {Slot: 1, Value: 1100}}
	if err := db.logCommitted(sets); err != nil {
		t.Fatal(err)
	}
	db.Close() // ページ未適用のままクラッシュ

	db2, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	mustBalance(t, db2, 0, 900)
	mustBalance(t, db2, 1, 1100)
}

// クラッシュ地点2: commit 済み、ページを1枚だけ書いたところで死んだ。
// データファイルは「Aから引いたのにBに足していない」不整合状態。
// リカバリの redo は冪等なので、両方をもう一度上書きして整合が戻る。
func TestCrashMidApply(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	sets := []Set{{Slot: 0, Value: 900}, {Slot: 1, Value: 1100}}
	if err := db.logCommitted(sets); err != nil {
		t.Fatal(err)
	}
	if err := db.applySets(sets[:1]); err != nil { // 1枚目だけ適用してクラッシュ
		t.Fatal(err)
	}
	db.Close()

	db2, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	mustBalance(t, db2, 0, 900)
	mustBalance(t, db2, 1, 1100)
}

// クラッシュ地点3: set レコードは書いたが commit を書く前に死んだ。
// リカバリは commit の無いバッチを無視し、送金は「無かったこと」になる。
// 中途半端に実行される、が絶対に起きないのが原子性。
func TestCrashBeforeCommitDiscardsBatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	sets := []Set{{Slot: 0, Value: 900}, {Slot: 1, Value: 1100}}
	if err := db.logSets(sets); err != nil { // commit レコードなし
		t.Fatal(err)
	}
	db.Close()

	db2, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	mustBalance(t, db2, 0, 1000)
	mustBalance(t, db2, 1, 1000)
}

func TestOpenValidation(t *testing.T) {
	if _, err := Open(t.TempDir(), 0); err == nil {
		t.Error("口座0個はエラーになるべき")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "no-such-dir"), 2); err == nil {
		t.Error("存在しないディレクトリはエラーになるべき")
	}
}

func TestDoubleCloseFails(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err == nil {
		t.Error("二重 Close はエラーになるべき")
	}
}

// チェックポイント後の WAL は空。定常状態では WAL は「一瞬だけ膨らんで戻る」ファイル。
func TestCheckpointEmptiesWal(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Transfer(0, 1, 100); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("チェックポイント後の WAL サイズ = %d, want 0", info.Size())
	}
}
