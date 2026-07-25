package btreestore

import (
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
)

func openTemp(t *testing.T) (*Tree, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	tr, err := Open(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr, path
}

func TestInsertAndGet(t *testing.T) {
	tr, _ := openTemp(t)
	pairs := map[uint64]uint64{50: 500, 20: 200, 80: 800, 10: 100, 30: 300}
	for k, v := range pairs {
		if err := tr.Insert(k, v); err != nil {
			t.Fatalf("Insert(%d): %v", k, err)
		}
	}
	for k, want := range pairs {
		got, ok, err := tr.Get(k)
		if err != nil || !ok || got != want {
			t.Errorf("Get(%d) = (%d, %v, %v), want (%d, true, nil)", k, got, ok, err, want)
		}
	}
	if _, ok, _ := tr.Get(999); ok {
		t.Error("存在しないキーはヒットしないべき")
	}
}

func TestUpdateExistingKey(t *testing.T) {
	tr, _ := openTemp(t)
	if err := tr.Insert(1, 100); err != nil {
		t.Fatal(err)
	}
	if err := tr.Insert(1, 999); err != nil {
		t.Fatal(err)
	}
	got, _, _ := tr.Get(1)
	if got != 999 {
		t.Errorf("上書き後 = %d, want 999", got)
	}
}

// ページに載せた木が、閉じて開き直しても復元されること。
// これがメモリ版 btree との決定的な違い(永続化)。
func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	tr, err := Open(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]uint64, 200)
	for i := range keys {
		keys[i] = uint64(i * 7 % 1000)
		if err := tr.Insert(keys[i], keys[i]+1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}

	tr2, err := Open(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	for _, k := range keys {
		got, ok, err := tr2.Get(k)
		if err != nil || !ok || got != k+1 {
			t.Fatalf("再open後 Get(%d) = (%d, %v, %v)", k, got, ok, err)
		}
	}
}

func TestManyKeysStaySorted(t *testing.T) {
	tr, _ := openTemp(t)
	rng := rand.New(rand.NewSource(1))
	inserted := map[uint64]bool{}
	for i := 0; i < 1000; i++ {
		k := uint64(rng.Intn(100_000))
		if err := tr.Insert(k, k); err != nil {
			t.Fatal(err)
		}
		inserted[k] = true
	}

	got, err := tr.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(inserted) {
		t.Errorf("キー数 = %d, want %d (重複が正しく無視されていない)", len(got), len(inserted))
	}
	if !sort.SliceIsSorted(got, func(a, b int) bool { return got[a] < got[b] }) {
		t.Error("Scan が昇順でない")
	}
}

func TestValidation(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "x", "y.db"), 4); err == nil {
		t.Error("開けないパスはエラーになるべき")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "z.db"), 1); err == nil {
		t.Error("次数 < 2 はエラーになるべき")
	}
}
