// Package bufferpool はバッファプール(ページキャッシュ)の最小実装。
//
// ディスク上のページ([ディスクとページ]編)とメモリの間に立つ唯一の窓口。
// 読み書きはすべてここを通り、よく使うページをメモリに置いておくことで
// 「同じページなら2回目からタダ同然」を実現する。
// 容量が溢れたときに誰を捨てるかは LRU([data-structures/lru])に任せる。
package bufferpool

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/esh2n/sharin/data-structures/lru"
)

// PageSize は1ページのバイト数。教材用に小さくしてある(実物は 4KB〜16KB)。
const PageSize = 256

// page はキャッシュに乗ったページ。dirty はディスクより新しい変更を持つ印。
type page struct {
	data  []byte
	dirty bool
}

// #region pool
// Pool は「ファイル + ページキャッシュ」。読み書きはすべてこの窓口を通る。
type Pool struct {
	mu      sync.Mutex
	f       *os.File
	cache   *lru.Cache[int, *page]
	hits    int // キャッシュにあった回数
	misses  int // ディスクまで読みに行った回数
	flushes int // dirty ページを書き戻した回数
}

// New はファイルを開き、容量 capacity ページのプールを返す。
func New(path string, capacity int) (*Pool, error) {
	cache, err := lru.New[int, *page](capacity)
	if err != nil {
		return nil, fmt.Errorf("bufferpool: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("bufferpool: open: %w", err)
	}
	return &Pool{f: f, cache: cache}, nil
}

// #endregion pool

// #region fetch
// fetch はページをキャッシュ経由で手に入れる。
// キャッシュに無ければディスクから読み(miss)、キャッシュに入れる。
// そのとき誰かが追い出されたら、dirty なら先にディスクへ書き戻す — ここが肝。
func (p *Pool) fetch(id int) (*page, error) {
	if id < 0 {
		return nil, fmt.Errorf("bufferpool: page id %d out of range", id)
	}
	if pg, ok := p.cache.Get(id); ok {
		p.hits++
		return pg, nil
	}
	p.misses++

	// ディスクから読む。まだ書かれたことのないページはゼロで埋まっていることにする。
	buf := make([]byte, PageSize)
	if _, err := p.f.ReadAt(buf, int64(id)*PageSize); err != nil && err != io.EOF {
		return nil, fmt.Errorf("bufferpool: read page %d: %w", id, err)
	}

	pg := &page{data: buf}
	if victimID, victim, evicted := p.cache.Put(id, pg); evicted && victim.dirty {
		if err := p.writeBack(victimID, victim); err != nil {
			return nil, err
		}
	}
	return pg, nil
}

// #endregion fetch

// Read はページ内容のコピーを返す。
func (p *Pool) Read(id int) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pg, err := p.fetch(id)
	if err != nil {
		return nil, err
	}
	out := make([]byte, PageSize)
	copy(out, pg.data)
	return out, nil
}

// #region write
// Write はページを書き換える。書き先はディスクではなく**キャッシュ上のページ**で、
// dirty の印を付けるだけ。ディスクへの書き戻しは追い出されるときか FlushAll まで遅延する。
// 「書き込みをまとめて遅らせる」ことが速さの源泉で、その代償がクラッシュ時の消失
// (これを守るのが WAL 編)。
func (p *Pool) Write(id int, data []byte) error {
	if len(data) != PageSize {
		return fmt.Errorf("bufferpool: data must be %d bytes, got %d", PageSize, len(data))
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	pg, err := p.fetch(id)
	if err != nil {
		return err
	}
	copy(pg.data, data)
	pg.dirty = true
	return nil
}

// #endregion write

func (p *Pool) writeBack(id int, pg *page) error {
	if _, err := p.f.WriteAt(pg.data, int64(id)*PageSize); err != nil {
		return fmt.Errorf("bufferpool: write back page %d: %w", id, err)
	}
	pg.dirty = false
	p.flushes++
	return nil
}

// FlushAll はキャッシュに残っている dirty ページをすべて書き戻す。
func (p *Pool) FlushAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.flushAllLocked()
}

func (p *Pool) flushAllLocked() error {
	for id, pg := range p.cache.Items() {
		if pg.dirty {
			if err := p.writeBack(id, pg); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stats は (ヒット数, ミス数, 書き戻し数) を返す。
func (p *Pool) Stats() (hits, misses, flushes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits, p.misses, p.flushes
}

// Close は dirty ページを書き戻してからファイルを閉じる。
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.flushAllLocked(); err != nil {
		p.f.Close()
		return err
	}
	return p.f.Close()
}
