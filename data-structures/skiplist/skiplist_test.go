package skiplist

import (
	"math/rand"
	"sort"
	"testing"
)

func TestInsertSearch(t *testing.T) {
	sl := New()
	keys := []int{3, 6, 9, 2, 11, 1, 19, 7}
	for _, k := range keys {
		sl.Insert(k, k*10)
	}
	for _, k := range keys {
		if v, ok := sl.Search(k); !ok || v != k*10 {
			t.Errorf("Search(%d) = (%d, %v), want (%d, true)", k, v, ok, k*10)
		}
	}
	if _, ok := sl.Search(100); ok {
		t.Error("存在しないキーはヒットしないべき")
	}
	if sl.Len() != len(keys) {
		t.Errorf("Len = %d, want %d", sl.Len(), len(keys))
	}
}

func TestUpdateExistingKey(t *testing.T) {
	sl := New()
	sl.Insert(5, 50)
	sl.Insert(5, 500)
	if v, _ := sl.Search(5); v != 500 {
		t.Errorf("上書き後 = %d, want 500", v)
	}
	if sl.Len() != 1 {
		t.Errorf("上書きで件数は増えないべき: Len = %d", sl.Len())
	}
}

func TestDelete(t *testing.T) {
	sl := New()
	for _, k := range []int{1, 2, 3, 4, 5} {
		sl.Insert(k, k)
	}
	if !sl.Delete(3) {
		t.Error("Delete は存在すれば true を返すべき")
	}
	if _, ok := sl.Search(3); ok {
		t.Error("削除後はヒットしないべき")
	}
	if sl.Delete(3) {
		t.Error("2回目の Delete は false のはず")
	}
	// 残りは全部引ける
	for _, k := range []int{1, 2, 4, 5} {
		if _, ok := sl.Search(k); !ok {
			t.Errorf("%d が消えてしまった", k)
		}
	}
	if sl.Len() != 4 {
		t.Errorf("Len = %d, want 4", sl.Len())
	}
}

// 昇順走査が常にソート済みで返ること(最下段は普通の連結リスト)。
func TestOrderedTraversal(t *testing.T) {
	sl := New()
	rng := rand.New(rand.NewSource(1))
	seen := map[int]bool{}
	for i := 0; i < 500; i++ {
		k := rng.Intn(1000)
		sl.Insert(k, k)
		seen[k] = true
	}
	got := sl.Keys()
	if len(got) != len(seen) {
		t.Errorf("重複が無視されていない: %d, want %d", len(got), len(seen))
	}
	if !sort.IntsAreSorted(got) {
		t.Error("Keys が昇順でない")
	}
}

// 高さが対数オーダーに収まること(確率的だが、大きく外れないはず)。
func TestHeightStaysLogarithmic(t *testing.T) {
	sl := New()
	for i := 0; i < 10000; i++ {
		sl.Insert(i, i)
	}
	// 10000 件なら理想の高さは log2(10000)≈13。乱数なので余裕を持って 25 以下。
	if h := sl.height(); h > 25 {
		t.Errorf("高さ = %d, 対数オーダーから外れている", h)
	}
}

func TestEmpty(t *testing.T) {
	sl := New()
	if _, ok := sl.Search(1); ok {
		t.Error("空リストは何も含まないべき")
	}
	if sl.Delete(1) {
		t.Error("空リストの Delete は false のはず")
	}
	if len(sl.Keys()) != 0 {
		t.Error("空リストの Keys は空のはず")
	}
}
