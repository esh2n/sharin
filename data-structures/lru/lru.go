// Package lru は LRU(Least Recently Used)キャッシュの最小実装。
//
// 「容量が溢れたら、一番長く使われていないものを捨てる」ためには、
// 全要素を使った順に並べておく必要がある。並び替えを O(1) でやるために
// 双方向リストを使い、キーからリストのノードへ飛ぶために map を使う。
// この「map + 双方向リスト」の組み合わせが LRU のすべて。
package lru

import "errors"

// #region entry
// entry は双方向リストのノード。リストは「最近使った順」に並ぶ。
type entry[K comparable, V any] struct {
	key        K
	value      V
	prev, next *entry[K, V]
}

// Cache は容量固定の LRU キャッシュ。
type Cache[K comparable, V any] struct {
	capacity int
	items    map[K]*entry[K, V]
	head     *entry[K, V] // 最近使った側
	tail     *entry[K, V] // 一番使われていない側(次に追い出される)
}

// New は容量 capacity (>=1) のキャッシュを返す。
func New[K comparable, V any](capacity int) (*Cache[K, V], error) {
	if capacity < 1 {
		return nil, errors.New("lru: capacity must be >= 1")
	}
	return &Cache[K, V]{capacity: capacity, items: map[K]*entry[K, V]{}}, nil
}

// #endregion entry

// #region get
// Get は値を返し、そのキーを「最近使った」先頭に動かす。
// この移動こそが LRU の本体で、map だけでは実現できない部分。
func (c *Cache[K, V]) Get(key K) (V, bool) {
	e, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.moveToFront(e)
	return e.value, true
}

// #endregion get

// #region put
// Put は値を入れ、容量が溢れたら一番使われていない要素(tail)を追い出す。
// 追い出した要素を返すのは、利用側(バッファプール等)が
// 「捨てる前の後始末」(dirty ページの書き戻し)をできるようにするため。
func (c *Cache[K, V]) Put(key K, value V) (evictedKey K, evictedValue V, evicted bool) {
	if e, ok := c.items[key]; ok {
		e.value = value
		c.moveToFront(e)
		return
	}

	e := &entry[K, V]{key: key, value: value}
	c.items[key] = e
	c.pushFront(e)

	if len(c.items) > c.capacity {
		victim := c.tail
		c.unlink(victim)
		delete(c.items, victim.key)
		return victim.key, victim.value, true
	}
	return
}

// #endregion put

// Len は現在の要素数を返す。
func (c *Cache[K, V]) Len() int {
	return len(c.items)
}

// Items は全要素のスナップショットを返す(順序は不定)。recency は変えない。
func (c *Cache[K, V]) Items() map[K]V {
	out := make(map[K]V, len(c.items))
	for k, e := range c.items {
		out[k] = e.value
	}
	return out
}

// pushFront はノードをリスト先頭(最近使った側)に挿入する。
func (c *Cache[K, V]) pushFront(e *entry[K, V]) {
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

// unlink はノードをリストから外す。
func (c *Cache[K, V]) unlink(e *entry[K, V]) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
}

func (c *Cache[K, V]) moveToFront(e *entry[K, V]) {
	if c.head == e {
		return
	}
	c.unlink(e)
	c.pushFront(e)
}
