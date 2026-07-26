package gc

import (
	"reflect"
	"testing"
)

// alive は id がまだヒープに生存しているかを返す。
func alive(h *Heap, id int) bool { return h.Get(id) != nil }

// ルートから辿れるオブジェクトは生き残り、辿れないものは回収される。
func TestReachableSurvivesUnreachableCollected(t *testing.T) {
	h := NewHeap()
	root := h.Alloc("stack")
	a := h.Alloc("A")
	b := h.Alloc("B")
	garbage := h.Alloc("garbage")
	h.AddRoot(root)
	h.PointTo(root, a) // stack -> A -> B は到達可能
	h.PointTo(a, b)
	// garbage は誰からも指されていない

	st := h.Collect()

	if !alive(h, root) || !alive(h, a) || !alive(h, b) {
		t.Error("到達可能なオブジェクトが回収された")
	}
	if alive(h, garbage) {
		t.Error("到達不能な garbage が生き残った")
	}
	if st.Marked != 3 || st.Swept != 1 {
		t.Errorf("stats = marked %d swept %d, want marked 3 swept 1", st.Marked, st.Swept)
	}
	if !reflect.DeepEqual(st.SweptIDs, []int{garbage}) {
		t.Errorf("SweptIDs = %v, want [%d]", st.SweptIDs, garbage)
	}
}

// mark-sweep は循環参照を回収できる——参照カウントが漏らす典型ケース。
func TestCyclesAreCollected(t *testing.T) {
	h := NewHeap()
	root := h.Alloc("stack")
	h.AddRoot(root)

	// 到達可能な循環: root -> A <-> B。生き残るべき。
	a := h.Alloc("A")
	b := h.Alloc("B")
	h.PointTo(root, a)
	h.PointTo(a, b)
	h.PointTo(b, a)

	// 到達不能な循環: E <-> F。ルートから辿れない。回収されるべき。
	e := h.Alloc("E")
	f := h.Alloc("F")
	h.PointTo(e, f)
	h.PointTo(f, e)

	h.Collect()

	if !alive(h, a) || !alive(h, b) {
		t.Error("到達可能な循環が回収された")
	}
	if alive(h, e) || alive(h, f) {
		t.Error("到達不能な循環が回収されなかった(参照カウントならリークする)")
	}
}

// ルートを外すと、そこからしか辿れなかった部分グラフがまとめて回収される。
func TestDropRootCollectsSubgraph(t *testing.T) {
	h := NewHeap()
	root := h.Alloc("stack")
	c := h.Alloc("C")
	d := h.Alloc("D")
	h.AddRoot(root)
	h.PointTo(root, c)
	h.PointTo(c, d)

	if st := h.Collect(); st.Swept != 0 {
		t.Fatalf("最初の GC で %d 個回収された, want 0", st.Swept)
	}

	h.Unpoint(root, c) // stack が C を手放す(唯一の到達経路が切れる)

	st := h.Collect()
	if alive(h, c) || alive(h, d) {
		t.Error("参照が切れた C と D が回収されなかった")
	}
	if st.Swept != 2 {
		t.Errorf("swept = %d, want 2", st.Swept)
	}
	if !alive(h, root) {
		t.Error("ルート自身が回収された")
	}
}

// tricolor の不変条件「black は white を直接指さない」がマークの各手で保たれる。
func TestTricolorInvariantHolds(t *testing.T) {
	h := NewHeap()
	root := h.Alloc("stack")
	a := h.Alloc("A")
	b := h.Alloc("B")
	cobj := h.Alloc("C")
	h.AddRoot(root)
	h.PointTo(root, a)
	h.PointTo(a, b)
	h.PointTo(b, cobj)

	col := h.Start()
	steps := 0
	for {
		// 各手のあとに不変条件を検査。
		for _, id := range h.IDs() {
			if col.Color(id) != black {
				continue
			}
			for _, r := range h.Get(id).Refs() {
				if col.Color(r) == white {
					t.Fatalf("black %d が white %d を指している(不変条件違反)", id, r)
				}
			}
		}
		if !col.MarkStep() {
			break
		}
		steps++
		if steps > 100 {
			t.Fatal("マークが終わらない")
		}
	}
	// 4オブジェクト全て到達可能なので、全て black になっているはず。
	for _, id := range h.IDs() {
		if col.Color(id) != black {
			t.Errorf("obj %d の色 = %s, want black", id, col.Color(id))
		}
	}
	if _, _, _ = a, b, cobj; col.Marking() {
		t.Error("マーク完了後に Marking() が true")
	}
}

// Start でルートが gray・他が white になり、MarkStep で gray が減る。
func TestStartAndGrayWorklist(t *testing.T) {
	h := NewHeap()
	root := h.Alloc("stack")
	a := h.Alloc("A")
	h.AddRoot(root)
	h.PointTo(root, a)

	col := h.Start()
	if col.Color(root) != gray {
		t.Errorf("Start 後のルート色 = %s, want gray", col.Color(root))
	}
	if col.Color(a) != white {
		t.Errorf("Start 後の A 色 = %s, want white", col.Color(a))
	}
	if got := col.GrayIDs(); !reflect.DeepEqual(got, []int{root}) {
		t.Errorf("gray worklist = %v, want [%d]", got, root)
	}
	col.MarkStep() // root -> black, A -> gray
	if col.Color(root) != black || col.Color(a) != gray {
		t.Errorf("1手後: root %s A %s, want black/gray", col.Color(root), col.Color(a))
	}
}

// PointTo は重複参照を作らず、Unpoint と Refs が正しく働く。
func TestPointToDedupAndRefs(t *testing.T) {
	h := NewHeap()
	a := h.Alloc("A")
	b := h.Alloc("B")
	cobj := h.Alloc("C")
	h.PointTo(a, b)
	h.PointTo(a, b) // 重複は無視される
	h.PointTo(a, cobj)
	if got := h.Get(a).Refs(); !reflect.DeepEqual(got, []int{b, cobj}) {
		t.Errorf("refs = %v, want [%d %d]", got, b, cobj)
	}
	h.Unpoint(a, b) // b だけ外し、cobj は残す
	if got := h.Get(a).Refs(); !reflect.DeepEqual(got, []int{cobj}) {
		t.Errorf("Unpoint 後の refs = %v, want [%d]", got, cobj)
	}
}

// 存在しない ID への操作は安全に無視される。
func TestOperationsOnMissingID(t *testing.T) {
	h := NewHeap()
	h.PointTo(99, 100) // panic しない
	h.Unpoint(99, 100)
	if h.Get(99) != nil {
		t.Error("存在しない ID が nil でない")
	}
	col := h.Start()
	if col.Color(99) != white {
		t.Error("存在しない ID の色は white 扱い")
	}
}

// Live / IDs / Roots のアクセサ。
func TestAccessors(t *testing.T) {
	h := NewHeap()
	x := h.Alloc("X")
	y := h.Alloc("Y")
	h.AddRoot(y)
	if h.Live() != 2 {
		t.Errorf("Live = %d, want 2", h.Live())
	}
	if got := h.IDs(); !reflect.DeepEqual(got, []int{x, y}) {
		t.Errorf("IDs = %v, want [%d %d]", got, x, y)
	}
	if got := h.Roots(); !reflect.DeepEqual(got, []int{y}) {
		t.Errorf("Roots = %v, want [%d]", got, y)
	}
	h.RemoveRoot(y)
	if len(h.Roots()) != 0 {
		t.Error("RemoveRoot 後にルートが残っている")
	}
}

// color.String の全分岐。
func TestColorString(t *testing.T) {
	cases := map[color]string{white: "white", gray: "gray", black: "black", color(9): "?"}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("color(%d).String() = %q, want %q", c, got, want)
		}
	}
}

// 空ヒープの GC は何も回収しない。
func TestEmptyHeap(t *testing.T) {
	h := NewHeap()
	st := h.Collect()
	if st.Before != 0 || st.Swept != 0 || st.Marked != 0 || st.After != 0 {
		t.Errorf("空ヒープの stats = %+v, want すべて 0", st)
	}
}
