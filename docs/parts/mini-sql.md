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
db 編の完結編。これまで作ってきたクラッシュセーフな B-Tree ストレージに、SQL でアクセスできる層を被せる。SQL の実行は「文字列 → トークン → 構文木 → 実行」の3段パイプラインで、これはコンパイラやインタプリタと同じ骨格。INSERT と SELECT だけの極小サブセットだが、DB が SQL 文字列をどう解釈してストレージ操作に変えるかの全体像が掴める。
</Summary>

## この章で作るもの

ここまでの [btreewal](./btree-wal) は Go の関数(`Insert`, `Get`, `Scan`)で叩く
ストレージだった。その上に **SQL 文字列を受け取って実行する**層を載せる。

```sql
INSERT INTO users VALUES (1, 100)
SELECT * FROM users WHERE id = 1
```

この章の肝は3つ。

- SQL の実行は **3段のパイプライン**: 文字列 → [lexer]トークン → [parser]AST → [engine]実行
- これは**コンパイラ/インタプリタと同じ骨格**。自作言語編でも同じ3段が出てくる
- engine は AST を見て、`WHERE id=k` なら [B-Tree](./btree-page-store)の1点引き、
  `WHERE` なしなら全走査、と**ストレージ操作に翻訳する**だけ

::: tip 前提章
[B-Tree + WAL](./btree-wal)(この章のストレージ層)の上に立つ。
:::

## SQL 実行の全体像

DB が `SELECT * FROM users` という文字列を受け取ってから結果を返すまで、
処理は3つの段を順に通る。

<FigureBox caption="SQL 実行の3段パイプライン。文字列を機械が扱える形に段階的に変換していく。lexer/parser はコンパイラの前半と同じ">
  <FlowRow :steps="pipeline" />
</FigureBox>

## 1. lexer: 文字をトークンに切る

lexer(字句解析器)は文字の並びを、意味の最小単位(トークン)に切り分ける。
`SELECT * FROM users` は「キーワード SELECT」「記号 *」「キーワード FROM」
「識別子 users」の4トークンになる。空白を捨て、キーワードと識別子を見分けるのが仕事。

<<< ../../db/minisql/lexer.go#lexer{go}

## 2. parser: トークンから構文木を組む

parser(構文解析器)はトークン列を消費して **AST(抽象構文木)** を組み立てる。
「INSERT の次は INTO、その次はテーブル名、その次は VALUES…」と、文法の形どおりに
トークンを1つずつ確かめて進む **再帰下降**方式。文法に合わなければそこでエラーにする。

<<< ../../db/minisql/parser.go#ast{go}

<<< ../../db/minisql/parser.go#parser{go}

### コードの読みどころ: expect が文法そのもの

`parseInsert` を読むと、`expectKeyword("INTO")` → `expect(TokIdent, ...)` →
`expectKeyword("VALUES")` と、**期待するトークンを順に消費**しているだけ。
このコードの並びが、そのまま `INSERT INTO <表> VALUES (<数>, <数>)` という文法の形に
なっている。再帰下降パーサの読みやすさはここにあって、文法規則1つが関数1つに対応する。

## 3. engine: AST をストレージ操作に翻訳する

最終段。engine は AST の種類を見て、[btreewal](./btree-wal) の操作を呼ぶ。
ここで**これまで作った全部が動員される**。

<<< ../../db/minisql/engine.go#engine{go}

見どころは `execSelect`。`WHERE id = k` があれば `store.Get(k)` で
[B-Tree を1点引き](./btree-page-store)、なければ `store.Scan()` で全件走査する。
この「インデックスで引くか、全走査か」の選択が、実DBの**クエリプランナ**の最小版。
そして `Insert` は [WAL](./btree-wal) で守られているので、この SQL の裏では
ちゃんとトランザクションが回っている。

## 試す: 文字列がトークンとASTになる様子

SQL を入力すると、lexer が切ったトークン、parser が組んだ AST、engine の実行結果が
段ごとに見える。プリセットを順に押すと、INSERT でデータが入り、SELECT で引ける。
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

積み上げると「**SQL で叩ける、永続化された、インデックス付きの、
クラッシュしても壊れないデータベース**」になった。もちろん本物には遠く及ばないが、
PostgreSQL や SQLite が内部でやっていることの骨格は、全部この積み木の中にある。

## 実物との距離

このミニ SQL が省いた各段には、実DBでは膨大な世界が広がっている。

- **parser**: 実SQLは JOIN・サブクエリ・集約・式と文法が巨大。ANTLR 等で生成することも
- **planner/optimizer**: 「どのインデックスを使い、どの順で JOIN するか」をコスト計算で
  選ぶ。DBの性能の要。この章は WHERE 有無の分岐だけ
- **executor**: 実物は行を一度に返さず1行ずつ流す(Volcano/イテレータモデル)
- **型・スキーマ**: CREATE TABLE、複数列、型チェック、制約

これらは lang 編(パーサとインタプリタ)や、この先の発展編で個別に深掘りできる。

## 簡略化したこと

- **テーブルは1つ・列は (id, value) 固定**: CREATE TABLE もスキーマもない
- **INSERT / SELECT のみ**: UPDATE / DELETE / JOIN / 集約なし
- **WHERE は `id = 定数` のみ**: 式評価器を持たない
- **クエリ最適化なし**: WHERE 有無での分岐だけ

## 参考資料

- [Writing a SQL database from scratch (notes)](https://notes.eatonphil.com/database-basics.html) — Go で SQL DB を作る連載。この章と同じ3段構成
- [CMU 15-445](https://15445.courses.cs.cmu.edu/) — クエリ実行・最適化まで含むDBの決定版講義
- [SQLite: The Architecture Of SQLite](https://www.sqlite.org/arch.html) — 実物の tokenizer→parser→VDBE のパイプライン
