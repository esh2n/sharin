// Package virtmem は仮想メモリのアドレス変換を最小構成で実装する。
//
// 各プロセスは「0 番地から始まる、自分だけの連続したメモリ」を持っているかの
// ように振る舞える。だが物理メモリは 1 つで、多数のプロセスが分け合う。この
// 錯覚を作るのが仮想メモリだ。プロセスが触るのは仮想アドレスで、CPU の MMU が
// それを物理アドレスへ変換する。仕組みはページ単位。仮想アドレスを上位のページ
// 番号と下位のオフセットに分け、ページテーブルでページ番号を物理フレーム番号に
// 引く。まだ物理メモリを割り当てていないページに触るとページフォルト(page fault)が
// 起き、そこで初めてフレームを割り当てる(デマンドページング)。変換は毎回だと
// 遅いので、直近の対応を TLB という小さなキャッシュに覚える。プロセスごとに別の
// ページテーブルを持つので、同じ仮想アドレスでも別の物理フレームを指す(隔離)。
package virtmem

import (
	"errors"
	"math/bits"
)

// #region address

var (
	// ErrSegfault はアドレス空間に含まれない(未マップの)ページに触ったとき。
	ErrSegfault = errors.New("virtmem: segmentation fault")
	// ErrOOM は割り当てる物理フレームが尽きたとき。
	ErrOOM = errors.New("virtmem: out of physical frames")
)

// FrameAllocator は物理フレームを配る(全プロセスで共有する物理メモリの管理役)。
type FrameAllocator struct {
	next uint
	max  uint
}

// NewFrameAllocator は max フレームぶんの物理メモリを作る。
func NewFrameAllocator(max uint) *FrameAllocator { return &FrameAllocator{max: max} }

func (fa *FrameAllocator) alloc() (uint, bool) {
	if fa.next >= fa.max {
		return 0, false
	}
	f := fa.next
	fa.next++
	return f, true
}

// MMU は 1 プロセスぶんのアドレス変換器(ページテーブル + TLB)。
type MMU struct {
	pageSize uint
	pageBits uint // log2(pageSize)。オフセットのビット幅
	fa       *FrameAllocator

	valid map[uint]bool // このページはアドレス空間に含まれるか
	frame map[uint]uint // ページ番号 → 物理フレーム(常駐しているページ)
	tlb   map[uint]uint // ページ番号 → フレーム(直近の変換キャッシュ)

	Hits, Misses, Faults int // 観測用
}

// New は pageSize(2 の冪)のページを使う MMU を作る。fa は物理フレームの供給元。
func New(pageSize uint, fa *FrameAllocator) *MMU {
	return &MMU{
		pageSize: pageSize,
		pageBits: uint(bits.TrailingZeros(pageSize)),
		fa:       fa,
		valid:    map[uint]bool{},
		frame:    map[uint]uint{},
		tlb:      map[uint]uint{},
	}
}

// Map は [vpageStart, vpageStart+count) のページをアドレス空間に加える。
// この時点では物理フレームは割り当てない(触れられて初めて割り当てる)。
func (m *MMU) Map(vpageStart, count uint) {
	for i := uint(0); i < count; i++ {
		m.valid[vpageStart+i] = true
	}
}

// #endregion address

// #region translate

// Translate は仮想アドレスを物理アドレスに変換する。
// 手順: TLB を引く → なければページテーブル → 未常駐ならページフォルトで割り当て。
func (m *MMU) Translate(vaddr uint) (uint, error) {
	vpage := vaddr >> m.pageBits       // 上位 = ページ番号
	offset := vaddr & (m.pageSize - 1) // 下位 = ページ内オフセット

	// 1. TLB(直近の対応のキャッシュ)を引く。当たれば一発。
	if f, ok := m.tlb[vpage]; ok {
		m.Hits++
		return f<<m.pageBits | offset, nil
	}
	m.Misses++

	// 2. アドレス空間に含まれないページ = segfault。
	if !m.valid[vpage] {
		return 0, ErrSegfault
	}

	// 3. ページテーブルを引く。まだ物理フレームが無ければページフォルト。
	f, resident := m.frame[vpage]
	if !resident {
		m.Faults++
		var ok bool
		f, ok = m.fa.alloc() // ここで初めて物理フレームを割り当てる(デマンドページング)
		if !ok {
			return 0, ErrOOM
		}
		m.frame[vpage] = f
	}

	m.tlb[vpage] = f // 次回のために TLB に載せる
	return f<<m.pageBits | offset, nil
}

// FlushTLB は TLB を空にする(文脈切替時などに実際に行われる)。
func (m *MMU) FlushTLB() { m.tlb = map[uint]uint{} }

// FrameOf は vpage に割り当てられた物理フレームを返す(観測用。未常駐なら false)。
func (m *MMU) FrameOf(vpage uint) (uint, bool) {
	f, ok := m.frame[vpage]
	return f, ok
}

// #endregion translate
