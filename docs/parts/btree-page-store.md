<script setup>
import BTreePageDemo from '../components/BTreePageDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import PageRow from '../components/figures/PageRow.vue'

const nodeLayout = [
  { label: 'leaf', note: '1B' },
  { label: 'numKeys', note: '2B' },
  { label: 'keys', note: '8B×N', state: 'hot' },
  { label: 'vals', note: '8B×N', state: 'hot' },
  { label: 'children', note: '8B×(N+1)', state: 'hot' },
]
</script>

# B-Treeページストア

> 実装: [`db/btreestore/`](https://github.com/esh2n/sharin/tree/main/db/btreestore) / 実行: `go test ./db/btreestore/`

<Summary>
これまで作った部品を合体させる回。メモリの中だけにあった B-Tree を、各ノードを1ページに直列化してディスクに置き、読み書きをバッファプール経由にする。こうすると木は永続化され、しかも根に近いノードは何度も通るので勝手にキャッシュに残る。これが本物のデータベースのインデックスの姿。
</Summary>

## この章で作るもの

[B-Tree](./btree) はメモリ上の `*node` ポインタで枝を繋いだ木だった。それを
[バッファプール](./buffer-pool)の上に載せて、閉じても消えない・大きくてもメモリに
乗り切る木にする。

この章の肝は3つ。

- ノードは `*node` ポインタではなく**1ページ**になる。枝は「ページID」で参照する
- 読み書きは全て[バッファプール](./buffer-pool)を通る。だから根に近いノードは
  勝手にキャッシュに残り、実質メモリ木のように速い
- 挿入アルゴリズムは[メモリ版](./btree)とまったく同じ。変わるのは
  「ポインタを辿る」が「ページを読む」に、「代入」が「書き戻す」に、それだけ

::: tip 前提章
[B-Tree](./btree)(木の構造と proactive split)、[バッファプール](./buffer-pool)
(ページの読み書きの窓口)、[ディスクとページ](./disk-and-pages)(ページと局所性)の上に立つ。
:::

## ポインタをページIDに置き換える

メモリ版のノードは子を `*node` で指していた。ディスクにはポインタを保存できない
(プロセスが変われば意味を失う)ので、代わりに**ページID**で指す。ノード1つを1ページに
このレイアウトで直列化する:

<FigureBox caption="1ノード = 1ページのレイアウト。子への参照はポインタではなくページID(8バイトの整数)。これならディスクに保存でき、再起動しても意味が変わらない">
  <PageRow :cells="nodeLayout" />
</FigureBox>

<<< ../../db/btreestore/btreestore.go#node{go}

## 木の操作は「読んで、いじって、書き戻す」

メモリ版で `n.left` とポインタを辿っていたところが、`readNode(id)` でページを読む操作に
変わる。ノードを変更したら `writeNode(id, n)` で書き戻す。この2つを挟むだけで、
アルゴリズム本体はメモリ版と同一になる。

検索がそれを一番はっきり示す:

<<< ../../db/btreestore/btreestore.go#get{go}

`readNode` の回数 = たどったノードの数 = **ページ読みの回数**。
[ディスクとページ](./disk-and-pages)で見た「速さはページ読み回数で数える」が、
ここでコードの `readNode` 呼び出し回数として目に見える形になる。

挿入も同じ。メモリ版の proactive split をそのまま、ポインタ参照をページID参照に
置き換えただけ:

<<< ../../db/btreestore/btreestore.go#insert{go}

### コードの読みどころ: 分割後に親を読み直す

`insertNonFull` の中で、満杯の子を `splitChild` した後に
`n, err = tr.readNode(id)` と親をもう一度読んでいる。メモリ版なら分割は同じ
`*node` を直接書き換えるので読み直しは不要だった。ページ版では `splitChild` が
親ページを `writeNode` で書き戻しているので、手元の `n` は古い。
**「ページに書き戻したら、手元のメモリコピーは古くなる」**——これがポインタの世界から
ページの世界に移ると必ず出てくる注意点。

## 根の情報はどこに置くのか

木のどこが根か(rootID)、次に使えるページはどこか(nextID)を、
どこかに永続化しないと再起動したとき木を見つけられない。定石は
**ページ0を「メタページ」として予約**し、そこに置くこと。

<<< ../../db/btreestore/btreestore.go#tree{go}

`Open` は起動時にページ0を読み、`nextID == 0` なら「まだ何もない新規ファイル」と判断して
空の葉を根に初期化する。既存ファイルならメタページの rootID から木を再構築できる——
といっても、木のノードはもうディスク上のページとして全部そこにあるので、
「再構築」は rootID を思い出すだけで済む。これが永続データ構造の気持ちよさ。

## 試す: 検索は何ページ読むか

1〜20 を入れた木で、キーを検索するとたどったページ(=読んだページ)が光る。
検索キーを変えて、根から葉までのページ読み回数を見てほしい。

<BTreePageDemo />

木が浅い([B-Tree](./btree)の枝分かれの太さのおかげ)ので、20件でもページ読みは
2〜3回で収まる。そして同じ検索を繰り返せば、これらのページは
[バッファプール](./buffer-pool)に残っているので、2回目からはディスクに触らない。
根のページに至っては、どんな検索でも必ず通るので、事実上ずっとキャッシュに居座る。

## ここまでの db 編が1つになった

この章で、バラバラだった部品が組み上がった。

- [B-Tree](./btree) の構造と分割アルゴリズム
- [ディスクとページ](./disk-and-pages) の「ページ単位・ページ読み回数」
- [バッファプール](./buffer-pool) の「読み書きの唯一の窓口・キャッシュ・遅延書き込み」
- [LRU](./lru-cache) の追い出し(バッファプールの中身)

これで「**永続化された、インデックス付きの、キャッシュの効くストレージ**」になった。
足りないのは1つだけ——クラッシュ耐性。今はページの書き戻しが Close 頼みで、
途中で電源が落ちれば木が壊れうる。それを [WAL](./wal) と繋ぐのが db 編の最終回。

## メリット / デメリット

**メリット**

- 木が永続化され、メモリに乗り切らない大きさでも扱える(必要なページだけ読む)
- [バッファプール](./buffer-pool)のキャッシュがそのまま効く。根はほぼ常駐
- アルゴリズムはメモリ版と同じなので、[B-Tree](./btree)の知識がそのまま生きる

**デメリット**

- 1ページに収まるキー数が次数の上限になる(この実装は256バイトページで次数4)
- Close するまで確実には永続化されない(WAL 未統合。次章の宿題)
- 空きページを再利用しない(削除未実装、nextID は増える一方)

**実例**

- SQLite のファイルフォーマット(ページの中に B-Tree ノードが載る。ほぼこの章の構造)
- PostgreSQL / MySQL InnoDB の B+Tree インデックス(値をリーフに集めた版)
- BoltDB / bbolt(Go 製の組み込み KV。まさに mmap したページ上の B+Tree)

## 簡略化したこと

- **削除は未実装**: メモリ版に揃えた。ページの併合・借用が入る
- **キーも値も uint64 固定**: 実物は可変長・複合キー。ページ内はスロット配列になる
- **B+Tree ではない**: 実物は値をリーフだけに置き、内部ノードをさらに太くする。
  リーフを横に繋いで範囲検索も速くする
- **WAL 未統合・フリーリストなし**: 次章とその先で

## 参考資料

- [SQLite Database File Format](https://www.sqlite.org/fileformat.html) — ページ上の B-Tree の実物
- [bbolt](https://github.com/etcd-io/bbolt) — Go で読めるページ上 B+Tree の実装
- Alex Petrov『Database Internals』2〜4章 — ノードのページレイアウトを最も詳しく扱う
