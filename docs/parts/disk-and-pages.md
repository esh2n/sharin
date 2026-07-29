<script setup>
import FigureBox from '../components/figures/FigureBox.vue'
import PageRow from '../components/figures/PageRow.vue'
import BarScale from '../components/figures/BarScale.vue'
import Summary from '../components/Summary.vue'

// 図: 1バイト読みたくてもページごと
const wantOneByte = [
  { label: 'page 0', state: 'dim' },
  { label: 'page 1', state: 'dim' },
  { label: 'page 2', state: 'hot', note: '欲しい1バイトはここ' },
  { label: 'page 3', state: 'dim' },
  { label: 'page 4', state: 'dim' },
]

// 図: シーケンシャル読み
const sequential = [
  { label: 'page 0', state: 'hot', note: '1' },
  { label: 'page 1', state: 'hot', note: '2' },
  { label: 'page 2', state: 'hot', note: '3' },
  { label: 'page 3', state: 'hot', note: '4' },
  { label: 'page 4', state: 'dim' },
  { label: 'page 5', state: 'dim' },
]

// 図: ランダム読み
const random = [
  { label: 'page 0', state: 'dim' },
  { label: 'page 1', state: 'hot', note: '3' },
  { label: 'page 2', state: 'dim' },
  { label: 'page 3', state: 'hot', note: '1' },
  { label: 'page 4', state: 'dim' },
  { label: 'page 5', state: 'hot', note: '2' },
]

// 図: アクセス時間の比(対数目盛ではなく比を体感する)
const latency = [
  { label: 'メモリ(RAM)', text: '約 0.0001 ms', frac: 0.00001, tone: 'good' },
  { label: 'SSD ランダム読み', text: '約 0.1 ms', frac: 0.01 },
  { label: 'HDD ランダム読み', text: '約 10 ms', frac: 1, tone: 'bad' },
]
</script>

# ディスクとページ

<Summary>
ディスクは1バイトずつではなく「ページ」というまとまり単位でしか読み書きできない。この一点から、ディスク上のデータ構造の速さは「ページを何回読むか」で測る、という物差しが生まれる。B-Tree も WAL もバッファプールも、この物理的な事情への対処として出てくる。コードは書かない、前提を掴むための章。
</Summary>

## この章で知ること

コードは書かない前提章。[B-Tree](./btree) や db 編(WAL、バッファプール)の土台になる、
ストレージの3つの事実を図で掴む。

- ディスクは1バイトずつ読めない。**ページ**という固定サイズの塊単位でしか読み書きできない
- だからディスク上のデータ構造の速さは、比較回数ではなく**ページ読み回数**で数える
- 同じページを続けて触れば**キャッシュ**が効いてメモリ速度になる(局所性の価値)

## ページ: ディスクの読み書きの最小単位

ディスク(HDD/SSD)は本のようなもので、**1文字だけ読むことはできず、ページ単位でめくる**。
ハードウェア自体がセクタ(512B〜4KB)単位でしか転送できず、その上で OS やデータベースが
扱いやすい固定サイズに切り直す。これが「ページ」で、代表的なサイズは:

| 誰のページか | サイズ |
|---|---|
| OS のページキャッシュ | 4KB |
| PostgreSQL | 8KB |
| MySQL (InnoDB) | 16KB |

たとえ欲しいデータが1バイトでも、そのバイトを含む**ページがまるごと**メモリに運ばれる:

<FigureBox caption="1バイト読みたくても、転送されるのはページまるごと。これが「読み1回」の実体">
  <PageRow :cells="wantOneByte" />
</FigureBox>

ここから大事な発想の転換が生まれる。ページの中の処理(数百キーの二分探索でも)は
メモリ上で済むので、ディスクの読みに比べれば誤差。だから
**ディスク上のデータ構造の速さは「ページを何回読むか」だけで数えればいい**。
B-Tree の章で「速さの単位はページ読み回数」と言っているのはこの意味。

## アクセス時間: メモリとディスクは別世界

「ページ読み1回」がどれくらい高くつくのか。桁で見る:

<FigureBox caption="1回のアクセスにかかる時間の比較(バーの長さは実際の比)。メモリのバーは見えないほど短い">
  <BarScale :bars="latency" />
</FigureBox>

メモリと HDD の差は**10万倍**。人間の感覚に直すと、メモリが「机の上の紙を見る(1秒)」なら、
HDD のランダム読みは「**倉庫まで往復する(1日以上)**」に相当する。SSD でも1000倍の差がある。
「ディスクを1回でも読まずに済むならどんな工夫でも元が取れる」のはこの桁差のせい。

## ランダム読みとシーケンシャル読み

同じ「ページを4枚読む」でも、並び方で速さが変わる。

<FigureBox caption="シーケンシャル読み: 隣のページを順番に。先読み(prefetch)が効いて数字以上に速い">
  <PageRow :cells="sequential" />
</FigureBox>

<FigureBox caption="ランダム読み: 飛び飛びのページを行ったり来たり。HDDでは毎回ヘッドの移動(シーク)が入る">
  <PageRow :cells="random" />
</FigureBox>

- **HDD**: ランダム読みのたびに物理的なヘッド移動(約10ms)が入る。シーケンシャルなら
  移動なしで連続転送できるので、100倍以上の差がつく
- **SSD**: 物理的な移動はないので差は縮むが、それでもシーケンシャルが有利。
  OS が「次も隣を読むだろう」と先読みしてくれるのと、SSD 内部も連続アクセスに最適化されているため

db 編で WAL(書き込みを追記だけにするログ)を作るとき、この
「**追記(シーケンシャル)は書き込みの中で最速**」という事実が設計の根拠になる。

## キャッシュ: 同じページなら2回目からタダ同然

読んだページはメモリに保持される(OS のページキャッシュ、DB のバッファプール)。
つまり「ページ読み回数」で数えるべきなのは正確には**キャッシュにないページ**を読む回数で、
同じページばかり触るアクセスパターンは実質メモリ速度になる。

これが[B-Tree の章](./btree)のデモで見られる、挿入の局所性の価値の正体になる:

- **昇順キー(UUIDv7 等)**: 挿入が常に右端の同じページに当たる。そのページはキャッシュに
  乗りっぱなしなので、ディスク読みがほぼ発生しない
- **ランダムキー(UUIDv4 等)**: 毎回違うページに当たる。テーブルが大きくなると
  キャッシュに乗り切らず、挿入のたびにディスク読みが発生する

## まとめ: この章の3行

1. ディスクは**ページ単位**でしか読めない(1バイトでもページまるごと)
2. だから速さは**ページ読み回数**で数える。1回の重さはメモリの数千〜10万倍
3. **並び(シーケンシャル)と再訪(キャッシュ)**は桁違いに安い。データ構造の設計は
   この2つの割引をどう引き出すかの勝負

## 参考資料

- [Latency Numbers Every Programmer Should Know](https://colin-scott.github.io/personal_website/research/interactive_latency.html) — 年代別のレイテンシ比較
- [PostgreSQL: Database Page Layout](https://www.postgresql.org/docs/current/storage-page-layout.html) — 8KB ページの中身の実物
- Alex Petrov『Database Internals』1〜3章 — ページとB-Treeの関係を最も丁寧に扱う本
