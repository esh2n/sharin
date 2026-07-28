<script setup>
import MvccDemo from '../components/MvccDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# mvcc(多版・スナップショット分離・write skew)

<Summary>
ロックで読み書きを直列化すると、読むだけのトランザクションが書き込みを待たせ、書き込みが読み手を止める。MVCC は上書きせずに版を積み、各トランザクションは開始時点で見える版だけを読む。読み手はロックを取らない。並行書き込みは先勝ち(first-committer-wins)で lost update を防ぐが、別々のキーに書く write skew はすり抜ける。それを止めるのが直列化可能の検証だ。
</Summary>

## この章で作るもの

[ミニSQL 編](/parts/mini-sql)まででデータの置き方と引き方を作ったが、複数のトランザクションが同時に読み書きする話はまだだった。素朴な解はロックで、読みにも書きにもロックを取れば正しくなる。だが読むだけの長いトランザクション(集計など)が走っている間、書き込みが全部止まる。

PostgreSQL や MySQL(InnoDB)が採る答えが MVCC(多版型同時実行制御)だ。上書きを捨て、書き込みを「新しい版の追加」にする。読み手は自分の開始時点で見える版を選ぶだけなので、ロックを取らず、書き手を待たせない。この章では版の連なり・スナップショット読み・コミット時の検証を作り、スナップショット分離(SI)がどこまで守れて、何を取りこぼすか(write skew)まで確かめる。

<FigureBox caption="MVCC の読み書き。書き込みは上書きでなく版の追加。各トランザクションは開始時点のタイムスタンプ(snap)を持ち、CommitTS がそれ以下の版の中で最新のものを読む。T1(snap=1)は古い版を、T3(snap=4)は新しい版を見る">

```
   balance の版の連なり:   [ "100" @0 ]──[ "150" @3 ]
                                ▲              ▲
   T1 (snap=1) ── Get ──────────┘              │      100 が見える
   T3 (snap=4) ── Get ─────────────────────────┘      150 が見える

   書き込みはバッファ → Commit 時に検証 → 新しい版として追加(上書きしない)
```

</FigureBox>

肝は3つ:

1. **版とスナップショット読み**: key → 版のリスト。開始時点の TS で見える版を選ぶ。読み手はロック無し
2. **first-committer-wins**: 同じキーへの並行書き込みは先勝ち。後コミットは必ず気づかされる(lost update 防止)
3. **write skew と直列化可能**: 別々のキーに書く異常は SI をすり抜ける。読み集合の検証まで足すと止まる

## ① 版とスナップショット読み

ストアは「キー → 版のリスト」で持つ。版は値とコミット時刻(CommitTS)の組で、時刻順に積まれる。読みの全ては `visible` に集約される。スナップショット TS 以下の版のうち最新を返すだけだ:

<<< ../../db/mvcc/mvcc.go#store{go}

トランザクションは `Begin` で開始時点の時計を写し取る(スナップショット TS)。以降なにを読んでも、この時点の世界が見える。書き込みはコミットまでバッファに留め、ストアには触れない:

<<< ../../db/mvcc/mvcc.go#txn{go}

`Get` がロックを取らないことが MVCC の価値そのものだ。後から誰がコミットしても、自分のスナップショットより新しい版は `visible` が自然に無視する。長い集計トランザクションが走っていても、書き込みは版を積み続けられる。読み手と書き手が互いを待たせない。

## ② コミット: first-committer-wins

スナップショット読みだけでは、古い値を前提にした上書きを防げない。残高 100 を見た 2 本が「+50 だから 150」「-30 だから 70」をそれぞれ書くと、後の方が先の更新を消してしまう(lost update)。これをコミット時の検証で止める:

<<< ../../db/mvcc/mvcc.go#commit{go}

検証はシンプルで、自分が書こうとしているキーに、自分のスナップショットより後のコミットが既にあれば敗北する。先にコミットした方が勝つので first-committer-wins と呼ぶ。負けた側はエラーを受け取り、新しいスナップショットで読み直してやり直す。「気づかず消える」が「必ず気づかされる」に変わるのが要点だ。

## ③ write skew: SI がすり抜ける異常

first-committer-wins は同じキーへの衝突しか見ない。ここに穴がある。古典例が当直医のシフトだ。規則は「最低 1 人は当直」。alice と bob が当直中で、2 人が同時に「相手が残るなら自分は抜けよう」と考える。

T1 は両方のキーを読んで「2 人いる」と確認し、`oncall:alice=no` を書く。T2 も両方を読んで「2 人いる」と確認し、`oncall:bob=no` を書く。書くキーが別々なので書き込み競合は起きず、SI では両方コミットできてしまう。結果、当直はゼロ。それぞれは正しい判断なのに、直列に実行したら決して起きない状態になる。これが write skew だ。

防ぐには、読んだ値が自分のコミットまでの間に書き換えられていないかまで見ればよい:

<<< ../../db/mvcc/ssi.go#ssi{go}

T2 が読んだ `oncall:alice` は、T2 のスナップショット後に T1 が書き換えている。T2 の「2 人いる」という判断は古い世界のものだったので、コミットを中止する。やり直した T2 は「1 人しかいない」と分かり、抜けるのを諦める。規則が守られる。

### 動かす

下のデモは、この筋書きをそのままブラウザで動かしている。「スナップショット + 先勝ち」では、版が積み上がる様子と、古いスナップショットが古い版を見続けること、後コミットが first-committer-wins で敗北することが見える。「write skew」では同じ当直医の筋書きを SI / Serializable で流し、SI では当直ゼロの異常が通り、Serializable では読み集合の検証が止めることを対比できる。

<MvccDemo />

## 設計の観点: 分離レベルの選び方

- **なぜロックでなく多版か**: 読み書きの相互ブロックを消すため。読むだけの長いトランザクション(分析・バックアップ)が書き込みを止めない。代償は版の蓄積で、古い版の掃除(vacuum / purge)が要る
- **分離レベルの階段**: read committed は文の実行ごとに最新スナップショット(同じトランザクション内で読みが変わりうる)。snapshot / repeatable read は開始時点固定(この章の SI)。serializable は直列実行と同じ結果を保証する。上がるほど異常が減り、中止(リトライ)が増える
- **SI が防ぐもの・防がないもの**: dirty read・non-repeatable read・lost update(先勝ち)は防ぐ。write skew と、範囲に対するファントムは防がない。「ほとんどの異常が消えるので serializable と誤解されやすい」のが SI の危うさ
- **write skew の実害**: 当直シフト、残高の合計制約(2 口座の合計 ≥ 0)、重複予約など「複数行にまたがる不変条件」で起きる。対処は serializable にするか、明示ロック(SELECT FOR UPDATE)や制約の 1 行への集約で衝突を「同じキー」に寄せる
- **本物の SSI**: この章の読み集合検証は保守的で、偽陽性(本当は直列化できるのに中止)がある。PostgreSQL の SSI は rw-antidependency の環が閉じたときだけ中止する、より精密な検出を行う(Cahill らの手法)

この章の要点は「MVCC は上書きせず版を積み、開始時点のスナップショットで読む。読み手はロック無し。lost update は first-committer-wins で防ぐが、write skew は SI をすり抜ける。serializable は読んだ値への後発書き込み(rw 依存)まで検証して止める。古い版の掃除が運用課題」に尽きる。

## メリット・デメリットと実例

| 分離レベル | 防げる異常 | すり抜ける異常 | 実例 |
|---|---|---|---|
| read committed | dirty read | non-repeatable read 以降すべて | PostgreSQL 既定、Oracle 既定 |
| snapshot / repeatable read | + non-repeatable read, lost update | write skew、ファントム | PostgreSQL RR、MySQL InnoDB 既定 |
| serializable(SSI) | すべて | (中止とリトライが増える) | PostgreSQL serializable |
| 2 相ロック(ロック直列化) | すべて | (読み書きが互いに待つ) | 旧来の DB、SELECT FOR UPDATE |

裏どり:

- **PostgreSQL**: タプルに xmin/xmax(作成・削除したトランザクション ID)を持つ MVCC。serializable は SSI(Cahill らの手法、2008)を実装した初の主要 DB。不要版の掃除が VACUUM
- **MySQL InnoDB**: undo ログから古い版を再構成する方式の MVCC。既定は repeatable read
- **TiDB / CockroachDB**: 分散 KV の上の MVCC。タイムスタンプを [id-generation 編](/parts/id-generation)のような発番で全順序にする。first-committer-wins はこの章と同じ構図
- **write skew の命名**: Berenson らの論文(1995)が ANSI の分離レベル定義の穴として整理した。当直医の例は Cahill の SSI 論文と Kleppmann *Designing Data-Intensive Applications* で広まった

## 簡略化したこと

- **削除・範囲クエリなし**: キー単位の Get/Put のみ。削除版(墓石)と、範囲に新しい行が現れるファントムは扱わない
- **版の掃除なし**: 古い版は積んだまま。実物は「どのスナップショットからも見えない版」を vacuum / purge で回収する
- **読み集合検証は保守的**: 本物の SSI(rw-antidependency の環検出)より偽陽性が多い。仕組みの説明は設計の観点の節で
- **単一プロセス・論理時計**: 分散でのタイムスタンプ発番・クロックスキューは扱わない([distributed-intro 編](/parts/distributed-intro)の領域)
- **永続化なし**: クラッシュ耐性は [wal 編](/parts/wal)の主題。ここではメモリ上の意味論に絞る

## 参考資料

- Berenson et al., ["A Critique of ANSI SQL Isolation Levels"](https://www.microsoft.com/en-us/research/wp-content/uploads/2016/02/tr-95-51.pdf)(1995) — snapshot isolation と write skew の整理
- Cahill, Röhm, Fekete, ["Serializable Isolation for Snapshot Databases"](https://courses.cs.washington.edu/courses/cse444/08au/544M/READING-LIST/fekete-sigmod2008.pdf)(2008) — PostgreSQL SSI の元論文
- Martin Kleppmann, *Designing Data-Intensive Applications* 7 章 — 分離レベルと write skew の決定版解説
- 実装: [db/mvcc](https://github.com/esh2n/sharin/tree/main/db/mvcc)
