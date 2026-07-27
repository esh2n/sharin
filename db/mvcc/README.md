# db/mvcc — MVCC(多版型同時実行制御)の最小実装

PostgreSQL などが採る「上書きせず版を積む」同時実行制御を、決定的な Go でモデル化する。
3 本柱:

1. **版とスナップショット読み**(`mvcc.go`): key → 版のリスト。トランザクションは開始時点の
   タイムスタンプで見える版だけを読む。読み手はロックを取らず、書き手と衝突しない。
2. **first-committer-wins**(`mvcc.go` の Commit): 同じキーへの並行書き込みは先勝ち。
   後からコミットした方は ErrWriteConflict で必ず気づかされる(lost update の防止)。
3. **直列化可能性**(`ssi.go`): SI でもすり抜ける write skew(当直医の例)を、読み集合の
   検証で防ぐ。読んだキーがコミットまでに書き換えられていたら中止(ErrRWConflict)。
   実際の PostgreSQL SSI は rw-antidependency の環検出でより精密に行う。

## 実行

```sh
go test -race -cover ./db/mvcc/
```

`-race`・`go vet` クリーン、カバレッジ 100%。write skew が SI では通り
Serializable では止まることを、同じ筋書きの対で固定している。
教科書の該当章は `docs/parts/mvcc.md`。
