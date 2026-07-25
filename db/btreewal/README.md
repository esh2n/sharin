# db/btreewal

WAL で守った永続 B-Tree。db 編の最終回。btreestore + WAL。

## 肝

- btreestore の弱点は「1回の Insert が split で複数ページを書き換えるので、
  途中でクラッシュすると木が壊れる」こと。WAL 編の手法をページ群に適用して守る
- Insert の全ページ変更をトランザクション(txn)にため、WAL に書いて fsync してから
  実ページに適用する。commit + fsync が「やる」の確定線
- full-page write(ページ全体をログに記録)なので redo は冪等。どこで死んでも直せる

## 簡略化したこと

- ログはページ全体を書く(before/after でなく after のみ)。実物は差分や圧縮を使う
- checkpoint は毎回。実物は WAL を溜めて定期実行
- 同時実行なし。トランザクション分離(ロック/MVCC)は扱わない
- 削除・フリーリストは未実装(btreestore に揃えた)

本文: [教科書の章](../../docs/parts/btree-wal.md) / 実行: `go test ./db/btreewal/`
