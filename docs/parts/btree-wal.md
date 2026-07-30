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

中心になるのは **txn(トランザクション)** という一時置き場だ。Insert 中の全 `writeNode` は
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

## 設計の観点

- **木の更新は複数ページに散る**: 分割が起きると子も親も、ときには根まで書き換わる。1ページずつは正しくても、まとまりとしては中途半端になりうる。だから原子性は木の外で与える
- **ページ丸ごとで冪等を買う**: 差分ではなくページの結果をログに置けば、何度適用しても同じになる。復旧が「commit 済みを全部やり直す」だけで済む
- **代償はログの太さ**: 冪等さをページ丸ごとで買っているので、ログの量が増える。実物が差分や圧縮へ向かうのは、この代償を薄めるため
- **確定線は木の形に依存しない**: 木がどんな形になっていても、確定したかどうかを決めるのはログの commit レコード1点だけになる
- **fsync の回数が commit の速さ**: 更新が何ページに散っても、commit で待つのはログ1本への追記だけ。散らばる書き込みを待たなくてよいことが、この構成の速さそのものになる
- **完成の判定を持つ**: 「クラッシュしても壊れないインデックス付きストレージ」という目標に対して、どのクラッシュ地点でも整合が戻ることを測れる形にしておく

## メリット・デメリットと実例

| 論点 | この章(ページ単位 WAL) | 差分ログ |
|---|---|---|
| ログの中身 | 変更後のページ全体 | 変更した部分だけ |
| 冪等さ | 何度適用しても同じ | 適用済みかの判定が要る |
| ログの量 | 太い | 細い |
| torn page | ページ全体があるので直せる | 別の備えが要る |
| 実例 | SQLite の WAL モード | InnoDB の redo log |

得るものは、複数ページにまたがる木の更新に原子性が付くこと、そして commit で待つのが
ログへの追記1回で済むことになる。払うのは、全変更が2回書かれること(write amplification)と、
ページ丸ごとを書くログの太さだ。

裏どり:

- **doublewrite buffer**: InnoDB は torn page の対策を WAL でなく別領域への二度書きで行う。ページを一度専用領域へ書き、そこから本来の位置へ書く。PostgreSQL の full-page write と目的は同じで、置き場所が違うという対比になる
- **PostgreSQL は両方持つ**: 通常は差分に近いレコードを書きつつ、checkpoint 後にそのページを最初に触るときだけページ全体を書く(`full_page_writes`)。**この章の作りは、その「全体を書く」側だけを常に行う形**になる
- **checkpoint の頻度が綱引き**: 頻繁だと定常の書き込みが増え、まれだと復旧が長い。PostgreSQL の `checkpoint_timeout` と `max_wal_size` が、その調整つまみになっている
- **redo だけで足りる条件**: commit 前のページを絶対に書き出さないと決めれば undo は要らない。実物がそう決めないのは、メモリに収まらない量の変更を抱えられなくなるからで、[WAL](./wal) で触れた ARIES はその制約を外した設計になる
- **SQLite の WAL は読みを止めない**: ページを本体でなく WAL 側に積むので、読み手は古い本体を読み続けられる。ロールバックジャーナル方式では本体を書き換えるため、読み手が待たされていた

## 簡略化したこと

- **full-page write のみ**: 実物は before/after 差分や圧縮でログ量を減らす
- **checkpoint 毎回**: 実物は WAL を溜めて定期実行し、fsync 回数を減らす
- **同時実行なし**: 1トランザクションずつ。分離レベルは扱わない
- **削除・フリーリストなし**: [B-Treeページストア](./btree-page-store)に揃えた

## 参考資料

- [SQLite: WAL Mode](https://www.sqlite.org/wal.html) — ページ単位 WAL の実物
- [PostgreSQL: WAL Configuration](https://www.postgresql.org/docs/current/wal-configuration.html) — full-page write の設定
- ARIES(1992) — undo/redo リカバリの古典。この章は redo のみの簡略版
