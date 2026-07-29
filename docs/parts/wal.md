<script setup>
import WalDemo from '../components/WalDemo.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import LaneSteps from '../components/figures/LaneSteps.vue'
import Summary from '../components/Summary.vue'

const naiveLanes = ['データファイル']
const naiveSteps = [
  { label: 'A から 100 引く', lane: 0, action: '書換' },
  { label: 'クラッシュ', note: 'ここで死んだら?', accent: 'danger' },
  { label: 'B に 100 足す', note: '実行されない', lane: 0, action: '書換', dim: true },
]

const walLanes = ['wal.log', 'data.db']
const walSteps = [
  { label: '変更を追記', note: 'set A / set B', lane: 0, action: '追記' },
  { label: 'commit + fsync', note: 'ここで「やる」と確定', lane: 0, action: 'fsync', accent: 'brand' },
  { label: 'ページ書き換え', note: 'A と B の2ページ', lane: 1, action: '書換' },
  { label: 'checkpoint', note: '適用済みログを捨てる', lane: 0, action: '空に' },
]
</script>

# WAL

> 実装: [`db/wal/`](https://github.com/esh2n/sharin/tree/main/db/wal) / 実行: `go test ./db/wal/`

<Summary>
「口座Aから引いてBに足す」のように複数箇所を書き換える処理を、途中でクラッシュしても中途半端にならないよう守る。コツは、ページを書き換える前に「これからやること」をログに書いておくこと。ログに commit を書き終えた瞬間が「やる」の確定線で、そこより前に死ねば無かったことに、後なら必ずやり切る。半分だけ実行、が起きない。
</Summary>

## この章で作るもの

db 編の第2段、**Write-Ahead Log**。題材は送金 — 「口座Aから引いて口座Bに足す」という
**2ページの書き換え**を、クラッシュがいつ起きても中途半端にならないように行う。

先に押さえることが3つある。

- 原則は1つ:「**ページを書き換える前に、やることをログに書け**」(write-ahead = 先行書き込み)
- **commit レコード + fsync が「やる」と確定する境界線**。リカバリは
  commit 済みなら redo(やり直し)、なければ捨てる。「半分だけ実行」が絶対に起きない
- **redo は冪等**(「+100する」ではなく「1100にする」と記録する)だから、
  どこで死んでも「全部やり直す」で必ず正しい状態になる

::: tip 前提章
[ログ構造KV](./log-structured-kv)の「追記・再生・壊れるのは末尾だけ」と、
[ディスクとページ](./disk-and-pages)の「fsync するまでディスクに届いた保証はない」を前提にする。
:::

## 問題設定: 2ページの書き換えは割り込まれる

送金は本質的に2つの書き込みでできている。素朴にページを直接書き換えると:

<FigureBox caption="素朴な送金。2つの書き込みの間でクラッシュすると、100円がこの世から消える">
  <LaneSteps :lanes="naiveLanes" :steps="naiveSteps" />
</FigureBox>

[ログ構造KV](./log-structured-kv)では「追記だけにする」ことでこの問題を避けたが、
B-Tree のような**その場で書き換えたい**構造ではそうもいかない。
書き換えはしたい、でも途中で死んでも壊れたくない。その答えが WAL。

## 解法: 先にログ、後でページ

<FigureBox caption="WAL 付きの送金。各ステップがどのファイルに触るかに注目 — データファイル(3)より先に、必ずログ(1-2)に書く。2 の fsync より前に死ねば「無かったこと」、後に死ねば「必ず完遂」で、中間がない">
  <LaneSteps :lanes="walLanes" :steps="walSteps" />
</FigureBox>

ログへの追記は[ディスクとページ](./disk-and-pages)で見た通りシーケンシャル書き込みで速い。
つまり WAL は「速い書き込み(ログ追記)で保険をかけてから、遅くて危険な書き込み(ページ)をやる」
という構造になっている。

レコードの形はログ構造KVとほぼ同じ length は固定13バイト:

<<< ../../db/wal/wal.go#format{go}

送金の本体。「先にログ、後でページ」の順序がすべて:

<<< ../../db/wal/wal.go#transfer{go}

4つのステップの中身:

<<< ../../db/wal/wal.go#steps{go}

### コードの読みどころ: fsync の位置

`logCommitted` の最後の `db.wal.Sync()` がこの章で一番重い1行。
`Write` はOSのバッファに書くだけで、停電すればまだ消える。
`Sync`(fsync)が返ってきて初めて「ディスクに物理的に届いた」と言える。
だから commit の**後**、ページ書き換えの**前**という位置に fsync がある —
ここが「送金をやる」と外部に約束できる最初の瞬間で、
データベースのトランザクションの「コミット完了」の実体はこの fsync。

## リカバリ: commit があれば redo、なければ捨てる

<<< ../../db/wal/wal.go#recover{go}

### コードの読みどころ: 冪等な redo

recover は「前回どこまで適用したか」を一切知らない。知らなくていい。
Set レコードが「A に +100」ではなく「**A を 900 にする**」という絶対値だから、
適用済みのレコードをもう一度適用しても結果が変わらない(**冪等**)。
「途中まで済んでいるかもしれない作業を、全部やり直しても壊れない」ようにしておくのが、
クラッシュリカバリを単純にする最大のコツ。

クラッシュの3地点とリカバリの動きを整理すると:

| クラッシュ地点 | WAL の状態 | リカバリの動き | 結果 |
|---|---|---|---|
| commit を書く前 | set のみ | commit が無いので捨てる | 送金は無かったことに |
| commit 直後(適用前) | set + commit | redo で両ページを書く | 送金は完遂される |
| ページ適用の途中 | set + commit | redo で両ページを書き直す(冪等) | 送金は完遂される |

3地点ともテストで固定してある。どの行き先も「完全にやる」か「完全にやらない」かで、
**「半分だけ」だけが存在しない** — これが原子性(atomicity)の実体。

**試してみる**: クラッシュ地点を選んで送金し、データファイルが不整合になる瞬間
(適用途中なら合計が 1900 になる)と、リカバリで整合が戻る様子を確認してほしい。

<WalDemo />

## ログ構造KVとの関係

前章と合わせると、ログの使い方が2通りあることになる。

- **ログ構造KV**: ログ自体が正本。読みもログから
- **WAL**: 正本はページ(B-Tree 等)で、ログは書き換えを守る**一時的な安全網**。
  適用が終われば checkpoint で捨てる

実務のデータベースはほぼ後者(+ 前者の発展形である LSM-Tree)。
B-Tree のページストアと WAL を組み合わせると、ようやく「クラッシュしても壊れない
インデックス付きストレージ」になる — db 編の次の段はそこ。

## メリット / デメリット

**メリット**

- 複数ページの書き換えに原子性を与えられる(トランザクションの土台)
- commit 時に必要な fsync は**WAL 1本へのシーケンシャル追記だけ**。
  ページ本体の書き込みは後でまとめてよいので、速さと安全を両立できる
- レプリケーションにも流用できる(WAL をそのまま他のサーバーに送れば複製になる)

**デメリット**

- すべての変更が2回書かれる(ログ + ページ)。**write amplification**
- WAL は伸び続けるので checkpoint の運用が要る(頻度が高いと遅く、低いと復旧が長い)
- fsync は数ms かかる。commit のたびに払う税金になる(グループコミットで緩和する)

**実例**

- PostgreSQL の WAL(`pg_wal/`)、MySQL InnoDB の redo log — 仕組みはこの章と同型
- SQLite の WAL モード(`PRAGMA journal_mode=WAL`)
- etcd / Raft のログ複製 — 「ログを確定してから状態機械に適用」は分散版の同じ発想

## 簡略化したこと

- **redo のみで undo がない**: ページ適用を commit 後に限っているため、
  巻き戻しが必要な状態がそもそも生まれない設計にした。実物(ARIES 系)は性能のために
  commit 前のページ書き出しを許し、その分 undo ログも持つ
- **同時実行なし**: トランザクション分離(ロック、MVCC)は db 編の後の段で
- **CRC なし・torn page 対策なし**: 実物はレコードにチェックサムを付け、
  ページ書き込みが途中で切れる問題には full-page write などで対処する
- **checkpoint を毎回実行**: 実物は WAL を溜めて定期的に行う

## 参考資料

- [PostgreSQL: WAL Introduction](https://www.postgresql.org/docs/current/wal-intro.html) — 本物の設計思想。この章の内容がそのまま出てくる
- [SQLite: Write-Ahead Logging](https://www.sqlite.org/wal.html) — 単一ファイルDBでの WAL の作り方
- ARIES(1992) — undo/redo 両対応のリカバリの古典。名前だけでも知っておくと文献が読める
