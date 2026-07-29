// Package secondary はセカンダリインデックスの最小実装。
//
// 主キーで引くのは [B-Tree ページストア](btreestore)で作った。だが実際の問い合わせは
// 「年齢が 30 の人」のように、主キー以外で引くほうが多い。そのための索引が
// セカンダリインデックスになる。
//
// 中身は「その列の値 → 主キー」を並べた、もう1本の木でしかない。
// 単純に見えるが、代償がはっきりしている。索引を引いても行そのものは手に入らないので、
// 主キーを持って本体をもう一度読みに行くことになる。**木を2回降りる**。
//
// ここでは読んだページ数だけを数える。実時間も乱数も使わない。
package secondary

import "sort"

// #region model

// PageSize はディスクの読み書きの最小単位に入る件数。
//
// 行そのものは大きいので1ページに 100 行、索引は「値と主キー」しか持たないので
// 1ページに 500 件、という比率にしてある。**索引が小さいことが効き目の源**になる。
const (
	RowsPerPage    = 100
	EntriesPerPage = 500
)

// Row は1行。
type Row struct {
	ID   int
	Age  int
	Name string
}

// Table は行をページに詰めて持つ。ID の順に並んでいる。
type Table struct {
	rows  []Row
	reads int
}

// NewTable は n 行の表を作る。年齢は 0..spread-1 を配る。
//
// 同じ年齢の行は表のあちこちに散る。行が入ってきた順に置かれる、普通の表になる。
func NewTable(n, spread int) *Table {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{ID: i, Age: i % spread, Name: "user"}
	}
	return &Table{rows: rows}
}

// NewClustered は、行そのものを年齢の順に並べた表を作る。
//
// 同じ年齢の行が隣り合うので、まとめて読める。これがクラスタ化された表で、
// **1つの表につき1つの並びしか持てない**のがそのまま制約になる。
func NewClustered(n, spread int) *Table {
	rows := make([]Row, n)
	per := n / spread
	if per == 0 {
		per = 1
	}
	for i := range rows {
		age := i / per
		if age >= spread {
			age = spread - 1
		}
		rows[i] = Row{ID: i, Age: age, Name: "user"}
	}
	return &Table{rows: rows}
}

// Len は行数を返す。
func (t *Table) Len() int { return len(t.rows) }

// Pages は表が占めるページ数を返す。
func (t *Table) Pages() int { return (len(t.rows) + RowsPerPage - 1) / RowsPerPage }

// Reads は読んだページ数の累計を返す。
func (t *Table) Reads() int { return t.reads }

// ResetStats は数え直す。
func (t *Table) ResetStats() { t.reads = 0 }

// #endregion model

// #region scan

// Scan は全ページを読んで、条件に合う行を返す。索引を使わない道。
//
// 読むページ数は行数だけで決まり、何件ヒットするかには左右されない。
func (t *Table) Scan(match func(Row) bool) []Row {
	t.reads += t.Pages()
	var out []Row
	for _, r := range t.rows {
		if match(r) {
			out = append(out, r)
		}
	}
	return out
}

// Fetch は主キーで1行だけ取りに行く。
//
// どのページに入っているかは ID から分かるので、読むのは1ページ。
// ただし**1行につき1ページ**なので、件数ぶん積み上がる。
func (t *Table) Fetch(id int) (Row, bool) {
	if id < 0 || id >= len(t.rows) {
		return Row{}, false
	}
	t.reads++
	return t.rows[id], true
}

// FetchAll は主キーの並びで順に取りに行く。
//
// 同じページに入っている行は、まとめて1回で読める。並びが近いほど得をする。
func (t *Table) FetchAll(ids []int) []Row {
	var out []Row
	last := -1
	for _, id := range ids {
		if id < 0 || id >= len(t.rows) {
			continue
		}
		if p := id / RowsPerPage; p != last {
			t.reads++
			last = p
		}
		out = append(out, t.rows[id])
	}
	return out
}

// #endregion scan

// #region index

// Entry は索引の1件。並べたい値と、そこから本体を引くための主キー。
type Entry struct {
	Key int
	ID  int
	// Extra は索引が持っている追加の列。ここに欲しいものが揃っていれば、
	// 本体を読みに戻らなくて済む(カバリングインデックス)。
	Extra string
}

// Index は「値 → 主キー」を並べた、もう1本の木。
type Index struct {
	entries []Entry
	reads   int
}

// Build は表の Age で索引を張る。値が同じものは主キーの順に並ぶ。
func Build(t *Table, extra bool) *Index {
	es := make([]Entry, len(t.rows))
	for i, r := range t.rows {
		es[i] = Entry{Key: r.Age, ID: r.ID}
		if extra {
			es[i].Extra = r.Name
		}
	}
	sort.Slice(es, func(i, j int) bool {
		if es[i].Key != es[j].Key {
			return es[i].Key < es[j].Key
		}
		return es[i].ID < es[j].ID
	})
	return &Index{entries: es}
}

// Height は根から葉まで降りるのに読むページ数。
//
// 1ページに EntriesPerPage 件入るので、件数が増えても対数でしか伸びない。
func (ix *Index) Height() int {
	h, n := 1, len(ix.entries)
	for n > EntriesPerPage {
		n = (n + EntriesPerPage - 1) / EntriesPerPage
		h++
	}
	return h
}

// Reads は読んだページ数の累計を返す。
func (ix *Index) Reads() int { return ix.reads }

// ResetStats は数え直す。
func (ix *Index) ResetStats() { ix.reads = 0 }

// Lookup は key に一致する件を索引から拾い、主キーの並びを返す。
//
// 根から葉まで降りて、そこから横に読み進める。読むページは
// 「高さ + ヒットが載っている葉の数」になる。
func (ix *Index) Lookup(key int) []Entry {
	lo := sort.Search(len(ix.entries), func(i int) bool { return ix.entries[i].Key >= key })
	hi := lo
	for hi < len(ix.entries) && ix.entries[hi].Key == key {
		hi++
	}
	ix.reads += ix.Height()
	if n := hi - lo; n > 1 {
		// 葉をまたいで続くぶん。1枚目は降りるときに読んでいる。
		ix.reads += (n - 1) / EntriesPerPage
	}
	return ix.entries[lo:hi]
}

// #endregion index

// #region plans

// Plan は1回の問い合わせで読んだページ数の内訳。
type Plan struct {
	Name string
	// IndexReads は索引を読んだページ数。
	IndexReads int
	// TableReads は本体を読んだページ数。
	TableReads int
	Rows       int
}

// Total は合計。
func (p Plan) Total() int { return p.IndexReads + p.TableReads }

// ByScan は索引を使わず、全ページを読む。
func ByScan(t *Table, key int) Plan {
	t.ResetStats()
	rows := t.Scan(func(r Row) bool { return r.Age == key })
	return Plan{Name: "全走査", TableReads: t.Reads(), Rows: len(rows)}
}

// BySecondary は索引で主キーを拾い、そのぶん本体を読みに戻る。
//
// 主キーの並びがバラバラなので、1行につき1ページ読むことになりやすい。
func BySecondary(t *Table, ix *Index, key int) Plan {
	t.ResetStats()
	ix.ResetStats()
	es := ix.Lookup(key)
	for _, e := range es {
		t.Fetch(e.ID) // 1件ずつ本体へ戻る
	}
	return Plan{Name: "索引 + 本体", IndexReads: ix.Reads(), TableReads: t.Reads(), Rows: len(es)}
}

// ByClustered は本体が索引と同じ順に並んでいる場合(NewClustered で作った表)。
//
// 拾った主キーが固まっているので、同じページの行はまとめて読める。
func ByClustered(t *Table, ix *Index, key int) Plan {
	t.ResetStats()
	ix.ResetStats()
	es := ix.Lookup(key)
	ids := make([]int, len(es))
	for i, e := range es {
		ids[i] = e.ID
	}
	sort.Ints(ids)
	t.FetchAll(ids)
	return Plan{Name: "並びをそろえた索引", IndexReads: ix.Reads(), TableReads: t.Reads(), Rows: len(es)}
}

// ByCovering は欲しい列が索引に揃っている場合。本体を読まない。
func ByCovering(t *Table, ix *Index, key int) Plan {
	t.ResetStats()
	ix.ResetStats()
	es := ix.Lookup(key)
	for _, e := range es {
		_ = e.Extra // 索引の中で用が足りる
	}
	return Plan{Name: "本体に戻らない索引", IndexReads: ix.Reads(), Rows: len(es)}
}

// #endregion plans
