// Package composite は複合インデックス(複数の列をまとめた索引)の最小実装。
//
// 索引を1列ずつ張るのと、まとめて1本張るのは別物になる。まとめた索引は
// 列を並べた順で辞書のように並ぶので、**左端から連続して指定したぶんだけ**
// 位置を決められる。途中が抜けると、そこから先は絞り込みに使えない。
//
// 電話帳が「姓, 名」の順に並んでいるのと同じ。姓が分かれば場所を決められるし、
// 姓と名が分かればもっと絞れる。だが名だけ分かっても、どこを開けばいいか分からない。
//
// ここでは読んだページ数と、書き込みで触ったページ数を数える。実時間も乱数も使わない。
package composite

import "sort"

// #region model

// 1ページに入る件数。索引は列の値と主キーしか持たないので、行より多く入る。
const (
	RowsPerPage    = 100
	EntriesPerPage = 500
)

// Row は3列の行。
type Row struct {
	ID      int
	A, B, C int
}

// Cond は1つの条件。Op は "=" か ">=" のどちらか。
type Cond struct {
	Col string
	Op  string
	Val int
}

// Entry は索引の1件。Key は列を並べた順の値。
type Entry struct {
	Key []int
	ID  int
}

// Index は複合インデックス。Cols の順がすべてを決める。
type Index struct {
	Cols    []string
	entries []Entry

	reads int
	// Scanned は索引の中で読み進めた件数。位置を決められないほど増える。
	Scanned int
}

// #endregion model

// #region build

// NewRows は n 行を作る。aVals / bVals はそれぞれの列がとる値の種類の数。
func NewRows(n, aVals, bVals int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{ID: i, A: i % aVals, B: (i / aVals) % bVals, C: i}
	}
	return rows
}

func value(r Row, col string) int {
	switch col {
	case "a":
		return r.A
	case "b":
		return r.B
	default:
		return r.C
	}
}

// Build は cols の順で索引を張る。
func Build(rows []Row, cols ...string) *Index {
	es := make([]Entry, len(rows))
	for i, r := range rows {
		k := make([]int, len(cols))
		for j, c := range cols {
			k[j] = value(r, c)
		}
		es[i] = Entry{Key: k, ID: r.ID}
	}
	sort.Slice(es, func(i, j int) bool {
		for k := range cols {
			if es[i].Key[k] != es[j].Key[k] {
				return es[i].Key[k] < es[j].Key[k]
			}
		}
		return es[i].ID < es[j].ID
	})
	return &Index{Cols: cols, entries: es}
}

// Height は根から葉まで降りるのに読むページ数。
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
func (ix *Index) ResetStats() { ix.reads, ix.Scanned = 0, 0 }

// #endregion build

// #region prefix

// Usable は、条件のうち索引で位置決めに使える本数を返す。
//
// 使えるのは「列の並びの左端から連続する等値」まで。そのあとに範囲が1つ来たら
// それも使えるが、そこで打ち切りになる。1つでも抜けたら、そこから先は使えない。
func (ix *Index) Usable(conds []Cond) int {
	byCol := map[string]Cond{}
	for _, c := range conds {
		byCol[c.Col] = c
	}
	n := 0
	for _, col := range ix.Cols {
		c, ok := byCol[col]
		if !ok {
			break // 抜けている。ここから先は位置決めに使えない
		}
		n++
		if c.Op != "=" {
			break // 範囲。ここで打ち切り
		}
	}
	return n
}

// #endregion prefix

// #region lookup

// Plan は1回の問い合わせで読んだページ数の内訳。
type Plan struct {
	// Usable は位置決めに使えた列の本数。
	Usable int
	// Scanned は索引の中で読み進めた件数。
	Scanned int
	// IndexReads は索引を読んだページ数。
	IndexReads int
	// Rows は最後に返した件数。
	Rows int
}

// Lookup は条件で索引を引く。
//
// 位置決めに使えた列で範囲を絞り、残りの条件はそこから1件ずつふるいにかける。
// 使えた本数が少ないほど、なめる件数が増える。
func (ix *Index) Lookup(conds []Cond) Plan {
	ix.ResetStats()
	u := ix.Usable(conds)
	byCol := map[string]Cond{}
	for _, c := range conds {
		byCol[c.Col] = c
	}

	// 位置決めに使えたぶんで、見る範囲を決める。
	lo, hi := 0, len(ix.entries)
	if u > 0 {
		lo = sort.Search(len(ix.entries), func(i int) bool {
			return !less(ix.entries[i].Key, ix.Cols, byCol, u)
		})
		hi = lo
		for hi < len(ix.entries) && inRange(ix.entries[hi].Key, ix.Cols, byCol, u) {
			hi++
		}
		ix.reads += ix.Height()
	} else {
		// どこを開けばいいか分からないので、索引をぜんぶなめる。
		ix.reads += ix.Height()
	}

	scanned := hi - lo
	ix.Scanned = scanned
	if scanned > 1 {
		ix.reads += (scanned - 1) / EntriesPerPage
	}

	// 使えなかった条件は、なめながらふるいにかける。
	rows := 0
	for _, e := range ix.entries[lo:hi] {
		if matches(e.Key, ix.Cols, byCol) {
			rows++
		}
	}
	return Plan{Usable: u, Scanned: scanned, IndexReads: ix.reads, Rows: rows}
}

// less は key が、使えた条件の下限より手前かを返す。
func less(key []int, cols []string, byCol map[string]Cond, u int) bool {
	for i := 0; i < u; i++ {
		c := byCol[cols[i]]
		if key[i] != c.Val {
			return key[i] < c.Val
		}
	}
	return false
}

// inRange は key が、使えた条件の範囲に収まっているかを返す。
func inRange(key []int, cols []string, byCol map[string]Cond, u int) bool {
	for i := 0; i < u; i++ {
		c := byCol[cols[i]]
		if c.Op == "=" && key[i] != c.Val {
			return false
		}
		if c.Op == ">=" && key[i] < c.Val {
			return false
		}
	}
	return true
}

// matches は全条件を満たすかを返す。
func matches(key []int, cols []string, byCol map[string]Cond) bool {
	for i, col := range cols {
		c, ok := byCol[col]
		if !ok {
			continue
		}
		if c.Op == "=" && key[i] != c.Val {
			return false
		}
		if c.Op == ">=" && key[i] < c.Val {
			return false
		}
	}
	return true
}

// ScanPages は索引を使わず表を全部読んだときのページ数。
func ScanPages(rows []Row) int { return (len(rows) + RowsPerPage - 1) / RowsPerPage }

// #endregion lookup

// #region write

// WriteCost は1行足すときに触るページ数を返す。
//
// 本体に1ページ、索引1本につき葉が1ページ。**索引は引くのを速くする代わりに、
// 書くのを重くする**。張った本数がそのまま書き込みに乗る。
func WriteCost(indexes int) int { return 1 + indexes }

// InsertPages は n 行入れたときに触るページ数の合計を返す。
func InsertPages(n, indexes int) int { return n * WriteCost(indexes) }

// #endregion write
