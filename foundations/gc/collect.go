package gc

import "sort"

// #region collect

// Stats は 1 回の GC の結果。
type Stats struct {
	Before   int   // GC 前のオブジェクト数
	Marked   int   // 到達(生存)と判定した数
	Swept    int   // 回収した数
	After    int   // GC 後のオブジェクト数
	SweptIDs []int // 回収した ID(昇順)
}

// Collection は 1 回の mark-sweep の途中経過を表す。tricolor マーキングを
// 1 手ずつ進められるので、GC の内部をそのまま観察できる。
// gray は「到達したが子をまだ走査していない」オブジェクトの作業リスト(ワークリスト)。
type Collection struct {
	heap *Heap
	gray []int
}

// Start はマーキングを始める。まず全オブジェクトを white に戻し、
// ルートを gray にしてワークリストに積む——ここが到達判定の起点。
func (h *Heap) Start() *Collection {
	for _, o := range h.objs {
		o.col = white
	}
	c := &Collection{heap: h}
	for _, id := range h.Roots() {
		if o := h.objs[id]; o != nil && o.col == white {
			o.col = gray
			c.gray = append(c.gray, id)
		}
	}
	return c
}

// MarkStep は gray を1つ取り出して black にし、その参照先の white を gray にする。
// これを繰り返すと到達集合が波紋のように広がる。tricolor の不変条件——
// 「black は white を直接指さない」——を各手で保つのが肝で、これが並行 GC の基礎になる。
// gray が尽きたら(到達集合が確定したら)false を返す。
func (c *Collection) MarkStep() bool {
	if len(c.gray) == 0 {
		return false
	}
	id := c.gray[0]
	c.gray = c.gray[1:]
	o := c.heap.objs[id]
	o.col = black
	for _, r := range o.refs {
		if child := c.heap.objs[r]; child != nil && child.col == white {
			child.col = gray
			c.gray = append(c.gray, r)
		}
	}
	return true
}

// Marking はまだマーク中(gray が残っている)かを返す。
func (c *Collection) Marking() bool { return len(c.gray) > 0 }

// Color は id の現在のマーキング色を返す。
func (c *Collection) Color(id int) color {
	if o := c.heap.objs[id]; o != nil {
		return o.col
	}
	return white
}

// GrayIDs は現在ワークリストに積まれている(gray の)ID を返す。
func (c *Collection) GrayIDs() []int {
	out := make([]int, len(c.gray))
	copy(out, c.gray)
	return out
}

// Sweep はマーキング完了後に呼ぶ。到達しなかった white のオブジェクトを全て解放する。
// マーク済みのオブジェクトは white に戻して次の GC に備える。
//
// 生存オブジェクトが回収対象を指すことは起こり得ない——指していれば、そのオブジェクトは
// マーク中に gray になり生存側に入るからだ。だからダングリング参照は生じない。
func (c *Collection) Sweep() Stats {
	before := len(c.heap.objs)
	var swept []int
	marked := 0
	for id, o := range c.heap.objs {
		if o.col == white {
			swept = append(swept, id)
		} else {
			marked++
		}
	}
	sort.Ints(swept)
	for _, id := range swept {
		delete(c.heap.objs, id)
	}
	for _, o := range c.heap.objs {
		o.col = white
	}
	return Stats{
		Before:   before,
		Marked:   marked,
		Swept:    len(swept),
		After:    len(c.heap.objs),
		SweptIDs: swept,
	}
}

// Collect は mark(最後まで)→ sweep を一気に行う便利関数。
// 実機の naive な mark-sweep は、この間プログラムを止める(stop-the-world)。
func (h *Heap) Collect() Stats {
	c := h.Start()
	for c.MarkStep() {
	}
	return c.Sweep()
}

// #endregion collect
