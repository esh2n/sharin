package minisql

import (
	"fmt"

	"github.com/esh2n/sharin/db/btreewal"
)

// #region engine
// Row は1行。このミニ実装は (key, value) の2列だけ。
type Row struct {
	Key   uint64
	Value uint64
}

// DB は AST を実行するエンジン。ストレージは btreewal(クラッシュセーフな B-Tree)。
// このミニ SQL には「テーブル」の概念が1つしかなく、table 名は受け取るが1本の木に格納する。
type DB struct {
	store *btreewal.Tree
}

// Open はデータベースを開く。
func Open(dir string) (*DB, error) {
	store, err := btreewal.Open(dir, 4)
	if err != nil {
		return nil, err
	}
	return &DB{store: store}, nil
}

// Close はデータベースを閉じる。
func (db *DB) Close() error {
	return db.store.Close()
}

// Exec は SQL 文字列を解析して実行し、SELECT なら結果の行を返す。
// SQL の3段パイプラインの最終段: AST を見て、ストレージ操作に translate する。
func (db *DB) Exec(sql string) ([]Row, error) {
	stmt, err := Parse(sql)
	if err != nil {
		return nil, err
	}
	switch s := stmt.(type) {
	case *InsertStmt:
		return nil, db.store.Insert(s.Key, s.Value)
	case *SelectStmt:
		return db.execSelect(s)
	default:
		return nil, fmt.Errorf("minisql: cannot execute %T", stmt)
	}
}

// execSelect は WHERE の有無で「1件 Get」か「全件 Scan」に振り分ける。
// これは実DBのクエリプランナが「インデックスで1点引く」か「全走査する」かを
// 選ぶのの、最小版。
func (db *DB) execSelect(s *SelectStmt) ([]Row, error) {
	if s.WhereKey != nil {
		// WHERE id = k: B-Tree を1点引き(前章までで作った Get)。
		v, ok, err := db.store.Get(*s.WhereKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return []Row{{Key: *s.WhereKey, Value: v}}, nil
	}
	// WHERE なし: 木を1回歩いて、キーと値をそろえて持ち帰る。
	pairs, err := db.store.ScanRows()
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, Row{Key: p.Key, Value: p.Value})
	}
	return rows, nil
}

// #endregion engine

// #region nplus1

// scanThenGet は「キーを並べてから1件ずつ引き直す」書き方。
//
// 直す前の execSelect はこうなっていた。返す結果は同じだが、
// 1行につき根から葉まで降り直すので、読むページが行数に比例して増える。
// 実物でも N+1 と呼ばれる形で、比較のために残してある。
func (db *DB) scanThenGet() ([]Row, error) {
	keys, err := db.store.Scan()
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(keys))
	for _, k := range keys {
		v, ok, err := db.store.Get(k)
		if err != nil {
			return nil, err
		}
		if ok {
			rows = append(rows, Row{Key: k, Value: v})
		}
	}
	return rows, nil
}

// #endregion nplus1

// #region plan

// Plan は execSelect がどちらの道を選んだかと、その代償。
type Plan struct {
	// Access は "index"(WHERE で1点引き)か "scan"(全走査)。
	Access string
	// Reads は読んだノード(ページ)の数。
	Reads int
	// Hits と Misses は、そのうちバッファプールに載っていた数と、
	// ディスクまで取りに行った数。
	Hits, Misses int
	// Rows は返した行数。
	Rows int
}

// Explain は SELECT を実行して、選んだ道と実際に読んだページ数を返す。
//
// 見積もりではなく実測なので、実物でいえば EXPLAIN ANALYZE のほうにあたる。
// 実DBのプランナは、これを事前に見積もって道を選ぶ。
func (db *DB) Explain(sql string) (Plan, []Row, error) {
	stmt, err := Parse(sql)
	if err != nil {
		return Plan{}, nil, err
	}
	s, ok := stmt.(*SelectStmt)
	if !ok {
		return Plan{}, nil, fmt.Errorf("minisql: EXPLAIN can only run on SELECT, got %T", stmt)
	}

	access := "scan"
	if s.WhereKey != nil {
		access = "index"
	}
	return db.measure(access, func() ([]Row, error) { return db.execSelect(s) })
}

// measure は実行の前後で数え直して、読んだページ数を拾う。
func (db *DB) measure(access string, run func() ([]Row, error)) (Plan, []Row, error) {
	db.store.ResetStats()
	rows, err := run()
	if err != nil {
		return Plan{}, nil, err
	}
	h, m := db.store.PoolStats()
	return Plan{
		Access: access,
		Reads:  db.store.Reads(),
		Hits:   h,
		Misses: m,
		Rows:   len(rows),
	}, rows, nil
}

// #endregion plan
