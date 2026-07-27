// Package allocator はメモリアロケータ(malloc/free の中身)を最小構成で実装する。
//
// プログラムが「n バイトください」と言ったとき、その裏で何が起きているか。
// アロケータは 1 枚の連続したメモリ(ヒープ)を管理し、要求に応じて切り出して
// 貸し、返されたら回収する。素朴なのは空きブロックの一覧(free list)を持ち、
// 要求に足る最初の空きを見つけて貸す(first-fit)、大きすぎれば切り分ける方式だ。
// 問題は断片化。確保と解放を繰り返すと、空きの総量は足りているのに、細切れで
// 大きな塊が取れなくなる。返却時に隣り合う空きを併合(coalesce)して、これと戦う。
// 対極にあるのが bump アロケータで、ポインタを進めるだけで貸すが、個別の解放が
// できない。速さと引き換えに柔軟さを捨てた割り切りだ。
package allocator

// #region block

// Block はヒープ上の 1 区画。アドレス順に並ぶ。
type Block struct {
	Offset int
	Size   int
	Free   bool
}

// Allocator は free list 方式のアロケータ。blocks は隙間なくヒープ全体を覆う。
type Allocator struct {
	size   int
	blocks []Block
}

// New は size バイトのヒープ(全体が 1 つの空きブロック)を作る。
func New(size int) *Allocator {
	return &Allocator{size: size, blocks: []Block{{Offset: 0, Size: size, Free: true}}}
}

// Blocks は現在の区画一覧を返す(観測用)。
func (a *Allocator) Blocks() []Block { return a.blocks }

// #endregion block

// #region alloc

// Alloc は n バイトを first-fit で確保し、その先頭オフセットを返す。
// 足る最初の空きブロックを使い、大きすぎれば余りを空きとして切り分ける。
func (a *Allocator) Alloc(n int) (int, bool) {
	if n <= 0 {
		return 0, false
	}
	for i := range a.blocks {
		b := &a.blocks[i]
		if !b.Free || b.Size < n {
			continue
		}
		off := b.Offset
		if b.Size > n {
			// 余りを空きブロックとして直後に挿入する(分割)。
			remainder := Block{Offset: b.Offset + n, Size: b.Size - n, Free: true}
			b.Size = n
			b.Free = false
			a.blocks = append(a.blocks[:i+1], append([]Block{remainder}, a.blocks[i+1:]...)...)
		} else {
			b.Free = false // ぴったりなら分割せず貸す
		}
		return off, true
	}
	return 0, false // 足る空きがない
}

// #endregion alloc

// #region free

// Free は offset のブロックを解放し、隣り合う空きと併合する。
func (a *Allocator) Free(offset int) bool {
	for i := range a.blocks {
		if a.blocks[i].Offset == offset {
			if a.blocks[i].Free {
				return false // 二重解放
			}
			a.blocks[i].Free = true
			a.coalesce()
			return true
		}
	}
	return false // そのオフセットのブロックがない
}

// coalesce は隣り合う空きブロックを 1 つにまとめる(断片化対策)。
func (a *Allocator) coalesce() {
	merged := make([]Block, 0, len(a.blocks))
	for _, b := range a.blocks {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if last.Free && b.Free {
				last.Size += b.Size // 直前も今も空き → まとめる
				continue
			}
		}
		merged = append(merged, b)
	}
	a.blocks = merged
}

// FreeBytes は空きの総バイト数。
func (a *Allocator) FreeBytes() int {
	total := 0
	for _, b := range a.blocks {
		if b.Free {
			total += b.Size
		}
	}
	return total
}

// LargestFree は連続した最大の空きブロックのサイズ(ここまでしか一度に確保できない)。
func (a *Allocator) LargestFree() int {
	max := 0
	for _, b := range a.blocks {
		if b.Free && b.Size > max {
			max = b.Size
		}
	}
	return max
}

// Fragmentation は外部断片化の度合い(0=断片なし, 1に近いほど細切れ)。
// 空きは十分でも連続領域が小さいほど大きくなる。
func (a *Allocator) Fragmentation() float64 {
	free := a.FreeBytes()
	if free == 0 {
		return 0
	}
	return 1 - float64(a.LargestFree())/float64(free)
}

// #endregion free

// #region bump

// Bump は bump(ポインタ前進)アロケータ。確保はポインタを進めるだけで速いが、
// 個別の解放ができない。Reset で全部まとめて解放する(アリーナ方式)。
type Bump struct {
	size int
	next int
}

// NewBump は size バイトの bump アロケータを作る。
func NewBump(size int) *Bump { return &Bump{size: size} }

// Alloc は next を n だけ進めて確保する。溢れたら失敗。
func (b *Bump) Alloc(n int) (int, bool) {
	if n <= 0 || b.next+n > b.size {
		return 0, false
	}
	off := b.next
	b.next += n
	return off, true
}

// Used は確保済みバイト数。
func (b *Bump) Used() int { return b.next }

// Reset は全確保を一度に解放する(個別解放はできない)。
func (b *Bump) Reset() { b.next = 0 }

// #endregion bump
