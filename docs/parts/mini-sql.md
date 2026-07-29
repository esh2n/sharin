<script setup>
import MiniSqlDemo from '../components/MiniSqlDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import FlowRow from '../components/figures/FlowRow.vue'

const pipeline = [
  { label: 'SQL文字列', note: '"SELECT * FROM users"' },
  { label: 'lexer', note: 'トークンに切る', state: 'hot' },
  { label: 'parser', note: 'AST を組む', state: 'hot' },
  { label: 'engine', note: 'ストレージ操作へ', state: 'hot' },
  { label: '結果の行' },
]
</script>

# ミニSQL

> 実装: [`db/minisql/`](https://github.com/esh2n/sharin/tree/main/db/minisql) / 実行: `go test ./db/minisql/`

<Summary>
db 編の完結編。クラッシュセーフな B-Tree ストレージに、SQL でアクセスする層を被せる。実行は文字列・トークン・構文木・実行の3段で、コンパイラと同じ骨格になる。最後の段が読むページ数を数えると、WHERE の有無で 1600 行のとき 3 と 530 に分かれ、差は行が増えるほど開く。ここが実物のプランナに当たる。
</Summary>

## この章で作るもの

ここまでの [btreewal](./btree-wal) は Go の関数(`Insert`, `Get`, `Scan`)で叩く
ストレージだった。その上に、SQL 文字列を受け取って実行する層を載せる。

```sql
INSERT INTO users VALUES (1, 100)
SELECT * FROM users WHERE id = 1
```

::: tip 前提章
[B-Tree + WAL](./btree-wal)(この章のストレージ層)の上に立つ。
:::

順に見ていく。

1. **3段のパイプライン**: 文字列 → トークン → 構文木 → 実行。コンパイラと同じ骨格になる
2. **最後の段が道を選ぶ**: `WHERE` があれば1点引き、なければ全走査。この分岐がプランナの最小版
3. **代償はページ数で出る**: 同じ結果を返す2通りの書き方で、読むページが15倍違う

## ① 3段のパイプライン

DB が `SELECT * FROM users` という文字列を受け取ってから結果を返すまで、
処理は3つの段を順に通る。

<FigureBox caption="SQL 実行の3段パイプライン。文字列を機械が扱える形に段階的に変換していく。lexer/parser はコンパイラの前半と同じ">
  <FlowRow :steps="pipeline" />
</FigureBox>

**lexer** は文字の並びを意味の最小単位に切る。`SELECT * FROM users` は
「キーワード SELECT」「記号 *」「キーワード FROM」「識別子 users」の4トークンになる。
空白を捨て、キーワードと識別子を見分けるのが仕事。

<<< ../../db/minisql/lexer.go#lexer{go}

**parser** はトークン列を消費して AST(抽象構文木)を組み立てる。
「INSERT の次は INTO、その次はテーブル名、その次は VALUES…」と、文法の形どおりに
トークンを1つずつ確かめて進む再帰下降方式。

<<< ../../db/minisql/parser.go#ast{go}

<<< ../../db/minisql/parser.go#parser{go}

`parseInsert` を読むと、`expectKeyword("INTO")` → `expect(TokIdent, ...)` →
`expectKeyword("VALUES")` と、期待するトークンを順に消費しているだけになっている。
**このコードの並びが、そのまま `INSERT INTO <表> VALUES (<数>, <数>)` という文法の形**。
再帰下降の読みやすさはここにあって、文法規則1つが関数1つに対応する。
[自作言語](./lang)の parser も、同じ形で書いてある。

## ② 最後の段が道を選ぶ

engine は AST の種類を見て、[btreewal](./btree-wal) の操作を呼ぶ。

<<< ../../db/minisql/engine.go#engine{go}

見どころは `execSelect` の分岐だ。`WHERE id = k` があれば `store.Get(k)` で
[B-Tree を1点引き](./btree-page-store)、なければ木を1回歩いて全件返す。
**この2択が、実DBのクエリプランナの最小版**になる。

分岐しているだけに見えるが、選んだ結果は代償として出る。`EXPLAIN` に当たるものを
足して、実際に読んだページ数を数えられるようにした。

<<< ../../db/minisql/engine.go#plan{go}

見積もりではなく実測なので、実物でいえば `EXPLAIN ANALYZE` のほうにあたる。
行数を変えて測るとこうなった:

| 行数 | 1点引き | 全走査 | 比 |
|---|---|---|---|
| 100 | 3 ページ | 32 ページ | 11倍 |
| 400 | 3 ページ | 131 ページ | 44倍 |
| 1,600 | **3 ページ** | 530 ページ | **177倍** |

**1点引きは 3 ページのまま動かない**。読むのは根から葉までの高さぶんで、
行が16倍になっても高さは変わらないからだ。全走査は行数に比例して増える。
だから比は行が増えるほど開く。テストで、行を16倍にしたとき比が一桁上がることを固定した。

ただし、比べているのは「1行取り出す代償」であって速さの優劣ではない。
全走査は 1600 行返している。全部要るなら全部読むしかない。
**プランナが選んでいるのは、速い遅いではなく、訊かれたことに対する最短の道**になる。

## ③ 代償はページ数で出る

道を選んだあとにも、まだ差がつくところがある。この章の `execSelect` は最初こう書いてあった。

<<< ../../db/minisql/engine.go#nplus1{go}

`Scan()` がキーしか返さないので、値を取りに1件ずつ根から降り直している。
返る結果は正しい。だが 1600 行で測るとこうなった:

| | 読んだページ | 返した行 |
|---|---|---|
| 木を1回歩く | **530** | 1,600 |
| 1件ずつ引き直す | **8,005** | 1,600 |

同じ 1600 行を返すのに、**15倍のページを読んでいる**。増えたぶんは1行あたり 4.7 ページ、
つまり木の高さそのものだ。これは実物でも N+1 と呼ばれる形で、1回で取れるものを
件数ぶん問い合わせに分けてしまう間違いになる。直し方は単純で、1回歩くついでに値も
持ち帰ればいい。テストで、両者の返す行が1件ずつ一致することと、読むページが5倍以上
違うことを固定した。

[バッファプール](./buffer-pool)にも差が出る。プールは 128 ページしか持てない:

| | ヒット | ミス |
|---|---|---|
| 1点引き | 2 | 1 |
| 1件ずつ引き直す | 6,946 | 1,059 |

1点引きは根の付近しか触らないので、ほぼ載っている。降り直すほうは、
プールに載りきらない下の段へ何度も取りに行くことになる。
**同じ SQL でも、engine の書き方ひとつでディスクに降りる回数が変わる**。

### 動かす

SQL を入力すると、lexer が切ったトークン、parser が組んだ AST、engine の実行結果が
段ごとに見える。`WHERE` を付けたり外したりすると、engine が選ぶ道の札が変わる。
文法を外した SQL(例: `SELECT users`)を入れると、parser がどの段で何を期待して
失敗したかも見える。

<MiniSqlDemo />

## db 編、これで完結

第1段の[ログ構造KV](./log-structured-kv)から始まって、ここまで来た。

- [二分探索木](./binary-search-tree) / [B-Tree](./btree): 速い検索
- [ディスクとページ](./disk-and-pages) / [B-Treeページストア](./btree-page-store): 永続化
- [LRU](./lru-cache) / [バッファプール](./buffer-pool): キャッシュ
- [WAL](./wal) / [B-Tree + WAL](./btree-wal): クラッシュ耐性
- この章: SQL でのアクセス

積み上げると「SQL で叩ける、永続化された、インデックス付きの、クラッシュしても
壊れないデータベース」になった。本物には遠く及ばないが、PostgreSQL や SQLite が
内部でやっていることの骨格は、全部この積み木の中にある。

## 設計の観点

- **段を分ける**: lexer と parser を分けると、片方の変更がもう片方に届かない。文法を足すのは parser だけの仕事になる
- **文法を関数の形で書く**: 規則1つに関数1つを対応させると、コードを読めば文法が読める
- **道の選択を1か所に集める**: どう取りに行くかを engine の分岐に閉じ込めておくと、後からコストで選ぶように差し替えられる
- **代償を数えられるようにする**: 「インデックスのほうが速い」は主張なので、ページ数を数える口を用意する
- **1回で取れるものを分けない**: 走査のついでに値を持ち帰るか、後から引き直すかで、読むページが桁で変わる
- **キャッシュの容量を意識する**: プールに載る量を超える読み方をすると、ヒット率ではなくディスクへの往復が効いてくる

## 対照と実例

| | 段の分け方 | 道の選び方 | 実行の仕方 |
|---|---|---|---|
| **この章** | lexer / parser / engine | `WHERE` の有無だけ | 行をまとめて返す |
| SQLite | tokenizer / parser / code generator | コストで選ぶ(統計あり) | VDBE(バイトコード)を回す |
| PostgreSQL | parser / analyzer / planner / executor | コストで選ぶ(統計あり) | 節を1行ずつ流す |
| DuckDB | 同上 | コストで選ぶ | 列をまとめて処理する |
| [自作言語](./lang) | lexer / parser / evaluator | (選択なし) | AST をそのまま歩く |

裏どり:

- **3段は実物も同じ**: [SQLite の構成図](https://www.sqlite.org/arch.html)が tokenizer → parser → code generator → VDBE と並べている。違うのは、この章が AST を直に実行するのに対し、SQLite は**バイトコードに落としてから回す**こと。[バイトコード VM](./bytecode) の章で作ったのと同じ形になる
- **プランナは見積もりで選ぶ**: PostgreSQL は表の行数や値の分布を統計として持ち、道ごとのコストを計算して選ぶ。`EXPLAIN` は見積もり、`EXPLAIN ANALYZE` は実測。**この章の `Explain` は後者に当たる**
- **インデックスが常に勝つわけではない**: 表の大半を返す問い合わせでは、インデックスをたどるより全走査のほうが安い。実物のプランナはこれを見積もりで判断していて、この章の分岐にはその判断が無い
- **N+1 は実物でよく出る**: ORM で1件ずつ関連を引き直す形が代表例。[N+1 の解説](https://docs.djangoproject.com/en/stable/topics/db/optimization/)はどのフレームワークの文書にもある。原因はいつも同じで、**1回で取れるものを件数ぶんに分けている**
- **行を1つずつ流す形**: 実物の executor は結果を一度に作らず、1行ずつ上へ渡す(Volcano モデル)。この章は全部作ってから返すので、行数ぶんのメモリを使う

## 簡略化したこと

- **テーブルは1つ・列は (id, value) 固定**: CREATE TABLE もスキーマもない
- **INSERT / SELECT のみ**: UPDATE / DELETE / JOIN / 集約なし
- **WHERE は `id = 定数` のみ**: 式評価器を持たない。範囲も AND も無い
- **コストで選ばない**: 統計を持たないので、`WHERE` の有無だけで道を決めている
- **行をまとめて返す**: 1行ずつ流す形ではないので、返す行数ぶんメモリを使う
- **バイトコードに落とさない**: AST を直に歩く。実物は中間表現を経由する

## 参考資料

- [SQLite: The Architecture Of SQLite](https://www.sqlite.org/arch.html) — 実物の tokenizer → parser → VDBE
- [Writing a SQL database from scratch](https://notes.eatonphil.com/database-basics.html) — Go で SQL DB を作る連載。この章と同じ3段構成
- [CMU 15-445](https://15445.courses.cs.cmu.edu/) — クエリ実行・最適化まで含む決定版講義
- 実装: [db/minisql](https://github.com/esh2n/sharin/tree/main/db/minisql)
