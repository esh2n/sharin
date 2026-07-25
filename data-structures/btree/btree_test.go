package btree

import (
	"math/rand"
	"sort"
	"testing"
)

// checkInvariants は B-Tree の不変条件を全ノードで検証する。
// 1. キーはノード内で昇順
// 2. キー数は最大 2t-1、root 以外は最小 t-1
// 3. 内部ノードの子の数はキー数+1
// 4. すべての葉が同じ深さ
func checkInvariants(t *testing.T, tr *Tree) {
	t.Helper()
	leafDepth := -1
	var walk func(n *node, depth int, isRoot bool)
	walk = func(n *node, depth int, isRoot bool) {
		if !sort.IntsAreSorted(n.keys) {
			t.Fatalf("ノード内のキーが昇順でない: %v", n.keys)
		}
		if len(n.keys) > 2*tr.t-1 {
			t.Fatalf("キー数が最大値を超えている: %v", n.keys)
		}
		if !isRoot && len(n.keys) < tr.t-1 {
			t.Fatalf("root以外でキー数が最小値未満: %v", n.keys)
		}
		if n.leaf {
			if leafDepth == -1 {
				leafDepth = depth
			} else if depth != leafDepth {
				t.Fatalf("葉の深さが揃っていない: %d vs %d", depth, leafDepth)
			}
			return
		}
		if len(n.children) != len(n.keys)+1 {
			t.Fatalf("子の数がキー数+1でない: keys=%d children=%d", len(n.keys), len(n.children))
		}
		for _, c := range n.children {
			walk(c, depth+1, false)
		}
	}
	walk(tr.root, 0, true)
}

func TestInsertAndContains(t *testing.T) {
	tr, err := New(2)
	if err != nil {
		t.Fatal(err)
	}

	keys := []int{50, 20, 80, 10, 30, 70, 90, 60, 40}
	for _, k := range keys {
		tr.Insert(k)
	}
	checkInvariants(t, tr)

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
	tr, _ := New(3)
	rng := rand.New(rand.NewSource(1))
	inserted := map[int]bool{}
	for i := 0; i < 500; i++ {
		k := rng.Intn(10_000)
		tr.Insert(k)
		inserted[k] = true
	}
	checkInvariants(t, tr)

	got := tr.Keys()
	if len(got) != len(inserted) {
		t.Errorf("重複が正しく無視されていない: %d keys, want %d", len(got), len(inserted))
	}
	if !sort.IntsAreSorted(got) {
		t.Error("中間順走査が昇順になっていない")
	}
}

func TestRootSplitGrowsHeight(t *testing.T) {
	tr, _ := New(2) // 最大3キー/ノード
	for _, k := range []int{1, 2, 3} {
		tr.Insert(k)
	}
	if tr.Height() != 0 {
		t.Fatalf("3キーまでは高さ0のはず: %d", tr.Height())
	}
	tr.Insert(4) // rootが溢れて分割 → 高さ1
	if tr.Height() != 1 {
		t.Errorf("root分割後の高さ = %d, want 1", tr.Height())
	}
	checkInvariants(t, tr)
}

func TestHeightStaysLogarithmic(t *testing.T) {
	tests := []struct {
		name   string
		insert func(tr *Tree, n int)
	}{
		{"昇順挿入", func(tr *Tree, n int) {
			for i := 0; i < n; i++ {
				tr.Insert(i)
			}
		}},
		{"ランダム挿入", func(tr *Tree, n int) {
			rng := rand.New(rand.NewSource(42))
			for i := 0; i < n; i++ {
				tr.Insert(rng.Intn(1 << 30))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, _ := New(16) // 最大31キー/ノード
			tt.insert(tr, 10_000)
			checkInvariants(t, tr)
			// 10,000件でも高さは3以下(二分木なら14段)。枝分かれの太さが高さを潰す。
			if h := tr.Height(); h > 3 {
				t.Errorf("高さ = %d, want <= 3", h)
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(1); err == nil {
		t.Error("t<2 はエラーになるべき")
	}
}
