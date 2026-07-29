package btreewal

import (
	"path/filepath"
	"sort"
	"testing"
)

func openTemp(t *testing.T) (*Tree, string) {
	t.Helper()
	dir := t.TempDir()
	tr, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr, dir
}

func TestInsertGet(t *testing.T) {
	tr, _ := openTemp(t)
	for i := uint64(0); i < 50; i++ {
		if err := tr.Insert(i, i*10); err != nil {
			t.Fatal(err)
		}
	}
	for i := uint64(0); i < 50; i++ {
		got, ok, err := tr.Get(i)
		if err != nil || !ok || got != i*10 {
			t.Fatalf("Get(%d) = (%d, %v, %v)", i, got, ok, err)
		}
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < 100; i++ {
		if err := tr.Insert(i*3, i); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}

	tr2, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	got, err := tr2.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 || !sort.SliceIsSorted(got, func(a, b int) bool { return got[a] < got[b] }) {
		t.Errorf("再open後の Scan が壊れている: %d 件", len(got))
	}
}

// クラッシュ地点1: 変更を WAL に書いて commit した直後、ページ適用の前に死んだ。
// リカバリが WAL を redo して、Insert は完遂される。
func TestCrashAfterCommitBeforeApply(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < 20; i++ { // 木を育てておく
		if err := tr.Insert(i, i); err != nil {
			t.Fatal(err)
		}
	}
	// 次の Insert を prepare + WAL commit まで進め、ページ適用の前にクラッシュ。
	tr.prepareInsert(999, 999)
	if err := tr.logToWAL(); err != nil {
		t.Fatal(err)
	}
	tr.closeFilesOnly() // apply も checkpoint もせずに死ぬ

	tr2, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	if got, ok, _ := tr2.Get(999); !ok || got != 999 {
		t.Errorf("commit 済みの Insert は redo で復元されるべき: (%d, %v)", got, ok)
	}
	if _, ok, _ := tr2.Get(5); !ok {
		t.Error("既存キーも残っているべき")
	}
}

// クラッシュ地点2: 変更を prepare したが WAL に commit する前に死んだ。
// リカバリはそのバッチを捨て、Insert は「無かったこと」になる。
func TestCrashBeforeCommit(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < 20; i++ {
		if err := tr.Insert(i, i); err != nil {
			t.Fatal(err)
		}
	}
	tr.prepareInsert(999, 999) // WAL に書かずにクラッシュ
	tr.closeFilesOnly()

	tr2, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	if _, ok, _ := tr2.Get(999); ok {
		t.Error("commit されなかった Insert は消えるべき")
	}
	if _, ok, _ := tr2.Get(5); !ok {
		t.Error("commit 済みの既存キーは残っているべき")
	}
}

func TestValidation(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "nope"), 4); err == nil {
		t.Error("存在しないディレクトリはエラーになるべき")
	}
	if _, err := Open(t.TempDir(), 1); err == nil {
		t.Error("次数 < 2 はエラーになるべき")
	}
}

// ScanRows は1回歩くだけで、キーと値をそろえて返す。
func TestScanRowsMatchesGet(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	const n = 200
	for i := 1; i <= n; i++ {
		if err := tr.Insert(uint64(i), uint64(i*10)); err != nil {
			t.Fatal(err)
		}
	}

	tr.ResetStats()
	pairs, err := tr.ScanRows()
	if err != nil {
		t.Fatal(err)
	}
	walkReads := tr.Reads()

	if len(pairs) != n {
		t.Fatalf("件数: %d", len(pairs))
	}
	for i, p := range pairs {
		if p.Key != uint64(i+1) || p.Value != uint64((i+1)*10) {
			t.Fatalf("%d 番目: %+v", i, p)
		}
		if i > 0 && pairs[i-1].Key >= p.Key {
			t.Fatalf("昇順でない: %d, %d", pairs[i-1].Key, p.Key)
		}
	}

	// 1回歩くだけなので、読むのは木のノード数で止まる。件数は超えない。
	if walkReads > n {
		t.Errorf("1回歩いて %d ページ読んだ(%d 件)", walkReads, n)
	}

	// 1件ずつ引き直すと、高さぶんが件数だけ積み上がる。
	tr.ResetStats()
	for i := 1; i <= n; i++ {
		if _, _, err := tr.Get(uint64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := tr.Reads(); got <= walkReads {
		t.Errorf("引き直しのほうが読まないのはおかしい: %d, %d", got, walkReads)
	}

	// バッファプールの数え方も動いている。
	hits, misses := tr.PoolStats()
	if hits+misses == 0 {
		t.Error("プールの数が両方 0")
	}
	tr.ResetStats()
	if h, m := tr.PoolStats(); h != 0 || m != 0 || tr.Reads() != 0 {
		t.Errorf("数え直せていない: %d, %d, %d", h, m, tr.Reads())
	}
}
