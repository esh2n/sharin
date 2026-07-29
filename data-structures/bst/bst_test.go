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

// この章の中心。形は入れる順だけで決まり、それがそのまま探す手数になる。
func TestShapeIsDecidedByInsertionOrder(t *testing.T) {
	const n = 4000

	asc := New()
	for i := 0; i < n; i++ {
		asc.Insert(i)
	}
	shuf := New()
	var s uint64 = 7
	for i := 0; i < n; i++ {
		s = s*6364136223846793005 + 1442695040888963407
		shuf.Insert(int(s>>40) % (4 * n))
	}

	// 昇順に入れると、高さが件数そのものになる(右へ伸びる1本の鎖)。
	if asc.Height() != n-1 {
		t.Fatalf("鎖になっていない: 高さ %d(件数 %d)", asc.Height(), n)
	}
	// ばらばらに入れれば、高さは件数の対数のあたりに収まる。
	if shuf.Height() > 40 {
		t.Fatalf("ばらばらなのに高い: %d", shuf.Height())
	}

	// 高さの差が、そのまま探す手数の差になる。
	asc.ResetStats()
	shuf.ResetStats()
	for i := 0; i < n; i++ {
		asc.Contains(i)
		shuf.Contains(i)
	}
	a := float64(asc.Compares()) / n
	b := float64(shuf.Compares()) / n
	if a < float64(n)/4 {
		t.Fatalf("鎖なのに %.1f 回で済んでいる", a)
	}
	if b > 40 {
		t.Fatalf("ばらばらなのに %.1f 回かかっている", b)
	}
}

// ばらばらに入れたときの高さは、件数の対数のあたりで止まる。
func TestRandomOrderStaysLogarithmic(t *testing.T) {
	for _, n := range []int{1000, 10000, 100000} {
		tr := New()
		var s uint64 = 31
		for i := 0; i < n; i++ {
			s = s*6364136223846793005 + 1442695040888963407
			tr.Insert(int(s>>40) % (4 * n))
		}
		// 期待される高さは 2·log2(n) 前後。余裕を見て 3 倍までに収まること。
		limit := 1
		for 1<<limit < n {
			limit++
		}
		if tr.Height() > 3*limit {
			t.Fatalf("件数 %d で高さ %d(上限 %d)", n, tr.Height(), 3*limit)
		}
	}
}

// 数えまわり。
func TestCompareStats(t *testing.T) {
	tr := New()
	if tr.Compares() != 0 {
		t.Fatal("最初は 0")
	}
	tr.Insert(2)
	tr.Insert(1)
	tr.Insert(3)
	tr.Contains(3) // 2 → 3 で 2 回
	if tr.Compares() != 2 {
		t.Fatalf("%d 回", tr.Compares())
	}
	tr.ResetStats()
	if tr.Compares() != 0 {
		t.Fatal("数え直せていない")
	}
}
