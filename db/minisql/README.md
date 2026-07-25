# db/minisql

SQL の極小サブセット(INSERT / SELECT)を btreewal ストレージの上に載せた
ミニ実データベース。db 編の完結編。

## 肝

- SQL の実行は3段パイプライン: 文字列 → [lexer]トークン → [parser]AST → [engine]実行
- lexer は文字を意味の最小単位に切り分けるだけ。parser は再帰下降でトークンから木を組む。
  engine は AST を見てストレージ操作(Insert / Get / Scan)に翻訳する
- `WHERE id = k` は B-Tree の1点引き(Get)、`WHERE` なしは全走査(Scan)。
  実DBのクエリプランナが「インデックスを使うか全走査か」を選ぶのの最小版

## 簡略化したこと

- テーブルは1つだけ(table 名は受け取るが1本の木に格納)。CREATE TABLE なし
- 列は (id, value) の2つ固定。型は uint64 のみ
- INSERT / SELECT のみ。UPDATE / DELETE / JOIN / 集約なし
- WHERE は `id = 定数` のみ。式評価器はない
- クエリ最適化なし(WHERE有無での分岐だけ)

本文: [教科書の章](../../docs/parts/mini-sql.md) / 実行: `go test ./db/minisql/`
