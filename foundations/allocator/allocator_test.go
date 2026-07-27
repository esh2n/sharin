package allocator

import "testing"

func TestAllocBasic(t *testing.T) {
	a := New(100)
	o1, ok1 := a.Alloc(30)
	o2, ok2 := a.Alloc(20)
	if !ok1 || !ok2 {
		t.Fatal("allocations should succeed")
	}
	// 重ならず、詰めて配置される。
	if o1 != 0 || o2 != 30 {
		t.Fatalf("offsets: got %d,%d want 0,30", o1, o2)
	}
	if a.FreeBytes() != 50 {
		t.Fatalf("free bytes: got %d want 50", a.FreeBytes())
	}
}

func TestAllocSplitsBlock(t *testing.T) {
	a := New(100)
	a.Alloc(40) // 100 の空きを 40(確保) + 60(空き)に分割
	blocks := a.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks after split, got %d", len(blocks))
	}
	if blocks[0].Free || blocks[0].Size != 40 {
		t.Fatalf("first block should be 40 used: %+v", blocks[0])
	}
	if !blocks[1].Free || blocks[1].Size != 60 {
		t.Fatalf("remainder should be 60 free: %+v", blocks[1])
	}
}

func TestAllocFailsWhenFull(t *testing.T) {
	a := New(50)
	a.Alloc(50)
	if _, ok := a.Alloc(1); ok {
		t.Fatal("allocation beyond capacity should fail")
	}
}

func TestFreeAndDoubleFree(t *testing.T) {
	a := New(100)
	o, _ := a.Alloc(30)
	if !a.Free(o) {
		t.Fatal("free should succeed")
	}
	if a.Free(o) {
		t.Fatal("double free should fail")
	}
	if a.Free(999) {
		t.Fatal("freeing unknown offset should fail")
	}
}

// TestCoalesce はこの章の主眼。解放時に隣り合う空きを併合して、
// 大きな確保ができるようになることを固定する。
func TestCoalesce(t *testing.T) {
	a := New(100)
	o1, _ := a.Alloc(30) // A @0
	o2, _ := a.Alloc(30) // B @30
	a.Alloc(30)          // C @60(残り 10 は空き)

	// A と C を解放。B が真ん中に残るので空きは細切れ。
	a.Free(o1) // [0..30] 空き
	// C を解放前の状態を作るため、まず C のオフセットを取り直す。ここでは A のみ解放済み。
	// 断片化: 50 を確保できない(連続空きが足りない)。
	if _, ok := a.Alloc(50); ok {
		t.Fatal("should not allocate 50 while fragmented")
	}

	// B を解放すると [0..30] と [30..60] と後続が併合され、大きな空きになる。
	a.Free(o2)
	if a.LargestFree() < 40 {
		t.Fatalf("after coalescing, largest free should grow: %d", a.LargestFree())
	}
}

// TestFragmentation は、空きの総量は足りても連続領域が足りず確保に失敗する
// 外部断片化を固定する。
func TestFragmentation(t *testing.T) {
	a := New(90)
	o1, _ := a.Alloc(30) // @0
	a.Alloc(30)          // @30 (B, 確保したまま)
	o3, _ := a.Alloc(30) // @60

	a.Free(o1) // [0..30] 空き
	a.Free(o3) // [60..90] 空き

	// 空きの総量は 60 だが、B が真ん中を占めるので連続は 30 まで。
	if a.FreeBytes() != 60 {
		t.Fatalf("free bytes: got %d want 60", a.FreeBytes())
	}
	if a.LargestFree() != 30 {
		t.Fatalf("largest free: got %d want 30", a.LargestFree())
	}
	// 総量は足りるのに 40 は確保できない(外部断片化)。
	if _, ok := a.Alloc(40); ok {
		t.Fatal("40 should fail despite 60 free (external fragmentation)")
	}
	// 断片化の度合いは 0 より大きい。
	if a.Fragmentation() <= 0 {
		t.Fatalf("fragmentation should be positive, got %v", a.Fragmentation())
	}
}

func TestBumpAllocator(t *testing.T) {
	b := NewBump(100)
	o1, ok1 := b.Alloc(40)
	o2, ok2 := b.Alloc(40)
	if !ok1 || !ok2 || o1 != 0 || o2 != 40 {
		t.Fatalf("bump allocs: %d,%d ok=%v,%v", o1, o2, ok1, ok2)
	}
	if b.Used() != 80 {
		t.Fatalf("used: got %d want 80", b.Used())
	}
	// 溢れたら失敗。
	if _, ok := b.Alloc(40); ok {
		t.Fatal("bump should fail when over capacity")
	}
	// Reset で全部一度に解放。
	b.Reset()
	if b.Used() != 0 {
		t.Fatal("reset should free everything")
	}
	if _, ok := b.Alloc(100); !ok {
		t.Fatal("after reset, full alloc should succeed")
	}
}
