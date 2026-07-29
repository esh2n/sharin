package skiplist

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/esh2n/sharin/data-structures/bst"
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

// この章の中心その1。件数を1000倍にしても、たどる手数は対数でしか伸びない。
func TestStepsGrowLogarithmically(t *testing.T) {
	avg := func(n int) float64 {
		sl := New()
		for i := 0; i < n; i++ {
			sl.Insert(i, i)
		}
		sl.ResetStats()
		for i := 0; i < n; i++ {
			sl.Search(i)
		}
		return float64(sl.Steps()) / float64(n)
	}
	small, big := avg(1000), avg(100000)
	// 100倍に増やしても、手数は倍にもならない(対数なので +log2(100) ≒ +6.6 程度)。
	if big > small*2 {
		t.Fatalf("対数で伸びていない: %.2f → %.2f", small, big)
	}
	if big < small {
		t.Fatalf("増えて減るのはおかしい: %.2f → %.2f", small, big)
	}
}

// この章の中心その2。入れる順に依らない。二分探索木は昇順で1本の鎖に落ちる。
func TestOrderDoesNotMatter(t *testing.T) {
	const n = 4000

	// 昇順に入れる。
	asc := New()
	for i := 0; i < n; i++ {
		asc.Insert(i, i)
	}
	// ばらばらに入れる。
	shuf := New()
	var s uint64 = 7
	for i := 0; i < n; i++ {
		s = s*6364136223846793005 + 1442695040888963407
		shuf.Insert(int(s>>40)%n, i)
	}

	if asc.Height() > 2*shuf.Height() {
		t.Fatalf("昇順で高くなった: %d vs %d", asc.Height(), shuf.Height())
	}
	// 高さは件数ではなく、その対数のあたりに収まる。
	if asc.Height() > 32 {
		t.Fatalf("高さが %d(件数 %d)", asc.Height(), n)
	}

	// 同じ昇順を二分探索木に入れると、高さが件数そのものになる。
	tr := bst.New()
	for i := 0; i < n; i++ {
		tr.Insert(i)
	}
	if tr.Height() < n-1 {
		t.Fatalf("木が鎖に落ちていない: 高さ %d(件数 %d)", tr.Height(), n)
	}
}

// 各段のノード数は、上へ行くほどおよそ半分ずつになる。
func TestLevelsHalveGoingUp(t *testing.T) {
	sl := New()
	for i := 0; i < 100000; i++ {
		sl.Insert(i, i)
	}
	counts := sl.LevelCounts()
	if len(counts) < 10 {
		t.Fatalf("段が少なすぎる: %d", len(counts))
	}
	if counts[0] != 100000 {
		t.Fatalf("最下段に全部いない: %d", counts[0])
	}
	// 下から数段を見て、おおむね半分ずつ減っていること。
	for i := 0; i < 8; i++ {
		ratio := float64(counts[i+1]) / float64(counts[i])
		if ratio < 0.3 || ratio > 0.7 {
			t.Fatalf("%d段目→%d段目の比が %.3f", i, i+1, ratio)
		}
	}
}

// いちばん高いノードが消えると、使われなくなった段は詰められる。
func TestHeightShrinksAfterDelete(t *testing.T) {
	sl := New()
	for i := 0; i < 50; i++ {
		sl.Insert(i, i)
	}
	if sl.Height() < 2 {
		t.Fatalf("段が伸びていない: %d", sl.Height())
	}
	for i := 0; i < 50; i++ {
		sl.Delete(i)
	}
	if sl.Len() != 0 {
		t.Fatalf("残っている: %d", sl.Len())
	}
	if sl.Height() != 1 {
		t.Fatalf("段が詰められていない: %d", sl.Height())
	}
}
