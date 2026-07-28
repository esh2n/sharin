<script setup>
import BTreeWalDemo from '../components/BTreeWalDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import LaneSteps from '../components/figures/LaneSteps.vue'

const lanes = ['txn', 'wal.log', 'data.db']
const steps = [
  { label: '変更をためる', note: 'split の全ページ', lane: 0, action: '積む' },
  { label: 'WAL に書く', note: '全ページ + commit', lane: 1, action: 'fsync', accent: 'brand' },
  { label: '実ページに適用', note: 'bufferpool へ', lane: 2, action: '書換' },
  { label: 'checkpoint', note: 'WAL を空に', lane: 1, action: '空に' },
]
</script>

# B-Tree + WAL

> 実装: [`db/btreewal/`](https://github.com/esh2n/sharin/tree/main/db/btreewal) / 実行: `go test ./db/btreewal/`

<Summary>
db 編の最終回。前章の B-Treeページストアには穴があった。1回の挿入で split が起きると複数ページが書き換わるので、その途中でクラッシュすると木が壊れる。この穴を、WAL 編で送金を守ったのと同じ手で塞ぐ。挿入を丸ごと1トランザクションにして、先にログ、後でページ。これで「永続化・インデックス付き・キャッシュ効き・クラッシュセーフ」なストレージが完成する。
</Summary>

## この章で塞ぐ穴

[B-Treeページストア](./btree-page-store)は木を永続化できたが、
[WAL](./wal) で送金に見た問題をそのまま抱えていた。**1回の Insert が
split で複数ページを書き換える**のに、それが原子的でない。

```
Insert(10) で split が起きると:
  ページ3(親) を書き換え
  ページ5(左) を書き換え     ← この途中でクラッシュしたら?
  ページ8(右) を書き換え
```

親だけ新しくて左右が古い、という**半分だけ適用された木**が残ると、
木の不変条件が壊れて検索が狂う。[WAL](./wal) の送金とまったく同じ構図で、
違うのは「2つの口座」が「splitで変わる複数ページ」になっただけ。

::: tip 前提章
[B-Treeページストア](./btree-page-store)(ページ上の B-Tree)と
[WAL](./wal)(先にログ・後でページ、commit+fsync、冪等な redo)の上に立つ。
:::

## 解法: 挿入を1トランザクションにする

やることは WAL 編と同じ4ステップ。違いは、変更が「2スロット」ではなく
「split で書き換わる全ページ」になること。

<FigureBox caption="Insert の4ステップ。どのファイルに触るかに注目 — data.db(3)より先に必ず wal.log(2)へ書く。2 の fsync より前に死ねば無かったこと、後なら必ず完遂">
  <LaneSteps :lanes="lanes" :steps="steps" />
</FigureBox>

鍵は **txn(トランザクション)** という一時置き場。Insert 中の全 `writeNode` は
実ページに書かず、まず txn にため込む:

<<< ../../db/btreewal/btreewal.go#tree{go}

### コードの読みどころ: readNode が txn を先に見る

`readNode` が `tr.txn[id]` を先に調べているのが要点。split の途中で親を
書き換えて(txn に積んで)、直後にその親を読み直すとき、実ページはまだ古い。
txn を優先することで、**「まだ確定していないが、この Insert の中では見えている」**
という一貫した世界を作る。これがないと split の最中に古いページを読んで壊れる。

挿入アルゴリズム自体は[B-Treeページストア](./btree-page-store)と一字一句同じ。
`writeNode` の中身が「実ページに書く」から「txn に積む」に変わっただけ:

<<< ../../db/btreewal/btreewal.go#txn{go}

## commit: 先にログ、後でページ

`prepareInsert` で txn に全変更がたまったら、`commit` がそれを確定する。
WAL に全ページを書いて fsync し、それから実ページに適用する:

<<< ../../db/btreewal/btreewal.go#wal{go}

各レコードは**ページ全体**(full-page write)。差分ではなくページまるごとを記録するので、
`recover` の redo は「そのページをこの内容にする」を繰り返すだけ。
何度適用しても結果が同じ(冪等)だから、適用の途中で死んでも全部やり直せば必ず直る。
[WAL](./wal) 編で「+100 ではなく 1100 にする」と書いたのと同じ発想を、ページに広げたもの。

## 試す: クラッシュ地点を変えて挙動を見る

10 を挿入すると split で3ページが書き換わる、という筋書き。
クラッシュ地点を選んで、txn / wal.log / data.db の3つがどう動くか、
そしてリカバリで木が整合に戻るかを確かめてほしい。

<BTreeWalDemo />

- **commit 前**: WAL に何もないので、リカバリはバッチを捨てる。木は挿入前のまま(整合)
- **commit 直後 / 適用途中**: WAL に commit 済み。リカバリが3ページを redo して完遂。
  「適用途中」で一度は不整合になっても、redo が全ページを揃え直す

どの地点で死んでも、木は「完全に挿入された」か「まったく挿入されていない」の
どちらかにしかならない。**半分だけ**が存在することはない。
これが[WAL](./wal)の原子性を、実データ構造の上で実現したということ。

## db 編、完成

ここまでで作った部品が全部つながった。

- [二分探索木](./binary-search-tree) → [B-Tree](./btree): 検索の速い木
- [ディスクとページ](./disk-and-pages) → [B-Treeページストア](./btree-page-store): 木をディスクへ
- [LRU](./lru-cache) → [バッファプール](./buffer-pool): ページのキャッシュ
- [ログ構造KV](./log-structured-kv) → [WAL](./wal) → この章: クラッシュ耐性

組み上がったのは「**永続化された、インデックス付きの、キャッシュの効く、
クラッシュしても壊れないキーバリューストア**」。実データベースの心臓部そのもの。
残る大物は「このストレージに SQL でアクセスする」層で、それがミニSQL編。

## メリット / デメリット

**メリット**

- 複数ページにまたがる木の更新に原子性を与えられる(トランザクションの土台)
- commit 時の fsync は WAL への追記1回だけ。ページ本体の書き込みは後でよい
- リカバリが単純(commit 済みを redo するだけ。full-page write で冪等)

**デメリット**

- 全ページ変更が2回書かれる(WAL + 実ページ)。write amplification
- ページ全体をログに書くので WAL が太い(実物は差分ログや圧縮で減らす)
- 同時実行は未対応。複数トランザクションのロック/MVCC はこの先

**実例**

- SQLite の WAL モード(まさにページ単位の WAL)
- PostgreSQL の full-page write(チェックポイント後の最初の更新はページ全体を WAL に書く)
- InnoDB の redo log + doublewrite buffer

## 簡略化したこと

- **full-page write のみ**: 実物は before/after 差分や圧縮でログ量を減らす
- **checkpoint 毎回**: 実物は WAL を溜めて定期実行し、fsync 回数を減らす
- **同時実行なし**: 1トランザクションずつ。分離レベルは扱わない
- **削除・フリーリストなし**: [B-Treeページストア](./btree-page-store)に揃えた

## 参考資料

- [SQLite: WAL Mode](https://www.sqlite.org/wal.html) — ページ単位 WAL の実物
- [PostgreSQL: WAL Configuration](https://www.postgresql.org/docs/current/wal-configuration.html) — full-page write の設定
- ARIES(1992) — undo/redo リカバリの古典。この章は redo のみの簡略版
