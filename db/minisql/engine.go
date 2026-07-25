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
	// WHERE なし: 全件を昇順で返す(Scan)。
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

// #endregion engine
