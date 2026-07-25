package bst

import (
	"math/rand"
	"sort"
	"testing"
)

func TestInsertAndContains(t *testing.T) {
	tr := New()
	keys := []int{50, 20, 80, 10, 30, 90}
	for _, k := range keys {
		tr.Insert(k)
	}
	for _, k := range keys {
		if !tr.Contains(k) {
			t.Errorf("Contains(%d) = false, want true", k)
		}
	}
	for _, k := range []int{0, 55, 100} {
		if tr.Contains(k) {
			t.Errorf("Contains(%d) = true, want false", k)
		}
	}
}

func TestKeysAreSorted(t *testing.T) {
	tr := New()
	rng := rand.New(rand.NewSource(1))
	seen := map[int]bool{}
	for i := 0; i < 300; i++ {
		k := rng.Intn(1000)
		tr.Insert(k)
		seen[k] = true
	}
	got := tr.Keys()
	if len(got) != len(seen) {
		t.Errorf("重複が無視されていない: %d, want %d", len(got), len(seen))
	}
	if !sort.IntsAreSorted(got) {
		t.Error("中間順走査が昇順になっていない")
	}
}

// この章の肝: ランダム挿入なら高さは log 程度に収まるが、
// 昇順挿入では「常に右の子」になり、高さ = 件数-1 の一本鎖に崩壊する。
func TestHeightDegeneratesOnSortedInsert(t *testing.T) {
	t.Run("ランダム挿入は浅い", func(t *testing.T) {
		tr := New()
		rng := rand.New(rand.NewSource(42))
		for i := 0; i < 1023; i++ {
			tr.Insert(rng.Intn(1 << 30))
		}
		// 理想は log2(1024)-1 = 9。ランダムでも期待値 ~1.39*log2(n) 程度に収まる。
		if h := tr.Height(); h > 30 {
			t.Errorf("ランダム挿入の高さ = %d, want <= 30", h)
		}
	})

	t.Run("昇順挿入は一本鎖", func(t *testing.T) {
		tr := New()
		for i := 0; i < 100; i++ {
			tr.Insert(i)
		}
		if h := tr.Height(); h != 99 {
			t.Errorf("昇順挿入の高さ = %d, want 99 (連結リストと同じ)", h)
		}
	})
}

func TestEmptyTree(t *testing.T) {
	tr := New()
	if tr.Contains(1) {
		t.Error("空の木に何も入っていないはず")
	}
	if h := tr.Height(); h != -1 {
		t.Errorf("空の木の高さ = %d, want -1", h)
	}
	if len(tr.Keys()) != 0 {
		t.Error("空の木のKeysは空のはず")
	}
}
