package gc

import "sort"

// #region heap

// Heap はオブジェクトの集合と、ルート集合を持つ。ルートとは、スタックのローカル変数や
// グローバル変数から「直接」指されているオブジェクト——GC が生死を辿り始める起点だ。
// GC は「ルートから辿れるか」だけでオブジェクトの生死を決める。
type Heap struct {
	objs   map[int]*Object
	roots  map[int]bool
	nextID int
}

// NewHeap は空のヒープを作る。ID は 1 から振る。
func NewHeap() *Heap {
	return &Heap{objs: map[int]*Object{}, roots: map[int]bool{}, nextID: 1}
}

// Alloc は新しいオブジェクトを確保して ID を返す(確保直後は誰からも指されていない)。
func (h *Heap) Alloc(name string) int {
	id := h.nextID
	h.nextID++
	h.objs[id] = &Object{ID: id, Name: name, col: white}
	return id
}

// PointTo は from が to を参照するようにする(from のフィールドに to を代入するイメージ)。
// 同じ参照は重複して持たない。
func (h *Heap) PointTo(from, to int) {
	o := h.objs[from]
	if o == nil {
		return
	}
	for _, r := range o.refs {
		if r == to {
			return
		}
	}
	o.refs = append(o.refs, to)
}

// Unpoint は from → to の参照を1本外す(フィールドに nil を代入するイメージ)。
func (h *Heap) Unpoint(from, to int) {
	o := h.objs[from]
	if o == nil {
		return
	}
	kept := make([]int, 0, len(o.refs))
	for _, r := range o.refs {
		if r != to {
			kept = append(kept, r)
		}
	}
	o.refs = kept
}

// AddRoot はオブジェクトをルートにする(スタック変数がそれを掴んだ状態)。
func (h *Heap) AddRoot(id int) { h.roots[id] = true }

// RemoveRoot はルートから外す(スタック変数がスコープを抜けた状態)。
// これを最後の到達経路にしていたオブジェクト群は、次の GC で回収される。
func (h *Heap) RemoveRoot(id int) { delete(h.roots, id) }

// Get は ID のオブジェクトを返す(無ければ nil)。
func (h *Heap) Get(id int) *Object { return h.objs[id] }

// Live は現在のオブジェクト数を返す。
func (h *Heap) Live() int { return len(h.objs) }

// IDs は生存オブジェクトの ID を昇順で返す。
func (h *Heap) IDs() []int {
	out := make([]int, 0, len(h.objs))
	for id := range h.objs {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// Roots はルート ID を昇順で返す。
func (h *Heap) Roots() []int {
	out := make([]int, 0, len(h.roots))
	for id := range h.roots {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// #endregion heap
