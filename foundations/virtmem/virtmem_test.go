package virtmem

import (
	"errors"
	"testing"
)

const pageSize = 256 // 8 ビットのオフセット

// TestAddressSplit は仮想アドレスがページ番号とオフセットに分かれ、
// オフセットが変換後も保たれることを固定する。
func TestAddressSplit(t *testing.T) {
	fa := NewFrameAllocator(16)
	m := New(pageSize, fa)
	m.Map(0, 4) // ページ 0..3 をアドレス空間に

	// 仮想アドレス 0x0140 = ページ 1, オフセット 0x40。
	paddr, err := m.Translate(0x0140)
	if err != nil {
		t.Fatalf("translate failed: %v", err)
	}
	// オフセット(下位 8 ビット)は変換で変わらない。
	if paddr&(pageSize-1) != 0x40 {
		t.Fatalf("offset not preserved: paddr=%#x", paddr)
	}
	// ページ 1 は最初にフォルトしたページなのでフレーム 0 に載る → 物理 0x0040。
	if paddr != 0x0040 {
		t.Fatalf("paddr: got %#x want 0x0040", paddr)
	}
}

// TestDemandPagingFault は、未常駐ページへの初回アクセスでページフォルトが起き、
// フレームが割り当てられ、2 回目はフォルトしないことを固定する。
func TestDemandPagingFault(t *testing.T) {
	fa := NewFrameAllocator(16)
	m := New(pageSize, fa)
	m.Map(0, 4)

	if _, err := m.Translate(0x0010); err != nil { // ページ 0 初回
		t.Fatalf("first access failed: %v", err)
	}
	if m.Faults != 1 {
		t.Fatalf("first access should fault once, got %d", m.Faults)
	}
	// 同じページの別オフセットは、もうフォルトしない(常駐済み)。
	m.Translate(0x00a0)
	if m.Faults != 1 {
		t.Fatalf("second access to same page should not fault, faults=%d", m.Faults)
	}
	// 別のページはフォルトする。
	m.Translate(0x0110)
	if m.Faults != 2 {
		t.Fatalf("new page should fault, faults=%d", m.Faults)
	}
}

// TestSegfault はアドレス空間に含まれないページへのアクセスが弾かれることを固定する。
func TestSegfault(t *testing.T) {
	fa := NewFrameAllocator(16)
	m := New(pageSize, fa)
	m.Map(0, 2) // ページ 0..1 だけ有効

	if _, err := m.Translate(0x0500); !errors.Is(err, ErrSegfault) { // ページ 5
		t.Fatalf("unmapped access should segfault, got %v", err)
	}
}

// TestTLBHits は、同じページへの再アクセスが TLB ヒットになることを固定する。
func TestTLBHits(t *testing.T) {
	fa := NewFrameAllocator(16)
	m := New(pageSize, fa)
	m.Map(0, 4)

	m.Translate(0x0000) // miss(初回)
	m.Translate(0x0001) // hit(同じページ 0)
	m.Translate(0x0002) // hit
	if m.Hits != 2 || m.Misses != 1 {
		t.Fatalf("expected 2 hits, 1 miss; got hits=%d misses=%d", m.Hits, m.Misses)
	}

	// TLB を流すと、次は再び miss。
	m.FlushTLB()
	m.Translate(0x0003) // miss(TLB 空)
	if m.Misses != 2 {
		t.Fatalf("after flush should miss, misses=%d", m.Misses)
	}
}

// TestProcessIsolation はこの章の主眼。2 つのプロセスが同じ仮想アドレスを使っても、
// 別々のページテーブルなので異なる物理フレームを指すことを固定する。
func TestProcessIsolation(t *testing.T) {
	fa := NewFrameAllocator(16) // 物理メモリは共有
	a := New(pageSize, fa)
	b := New(pageSize, fa)
	a.Map(0, 1)
	b.Map(0, 1)

	// 両プロセスとも同じ仮想アドレス 0x0020 に触る。
	pa, _ := a.Translate(0x0020)
	pb, _ := b.Translate(0x0020)

	// 同じ仮想アドレスなのに、物理アドレスは異なる(隔離されている)。
	if pa == pb {
		t.Fatalf("same virtual addr should map to different physical: pa=%#x pb=%#x", pa, pb)
	}
	// それぞれ別のフレーム(0 と 1)に載る。
	fa0, _ := a.FrameOf(0)
	fb0, _ := b.FrameOf(0)
	if fa0 == fb0 {
		t.Fatal("two processes should get distinct physical frames")
	}
}

// TestOOM は物理フレームが尽きるとエラーになることを固定する。
func TestOOM(t *testing.T) {
	fa := NewFrameAllocator(2) // フレームは 2 枚だけ
	m := New(pageSize, fa)
	m.Map(0, 4)

	m.Translate(0x0000)                                         // フレーム 0
	m.Translate(0x0100)                                         // フレーム 1
	if _, err := m.Translate(0x0200); !errors.Is(err, ErrOOM) { // 3 枚目は無い
		t.Fatalf("should run out of frames, got %v", err)
	}
}
