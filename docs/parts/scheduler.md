<script setup>
import SchedulerDemo from '../components/SchedulerDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# スケジューラ(M:N スケジューラと work-stealing)

<Summary>
Go が数百万の goroutine を数個の OS スレッドで回す仕組み(M:N スケジューラと work-stealing)をモデル化する。キューを 1 本にすればロックが詰まり、コアごとに分ければ偏ったとき暇なコアが遊ぶ。Go は P ごとにローカルキューを持たせてロック無しで回し、空いた P が最も混む P からキューの半分を横取りする。偏りが自動で均され、全コアが働き続ける。
</Summary>

## この章で作るもの

[os 編](/parts/os)では CPU 1 個・実行キュー 1 本の協調スケジューラを作った。だが実機はコアが複数ある。素朴に「1 本のキューを全コアで共有」すると、そのロックの奪い合いがボトルネックになる。かといって「コアごとに独立キュー」だと、仕事が一部のコアに偏ったとき、他のコアが手持ち無沙汰になる。Go ランタイムの答えが **M:N スケジューラ + work-stealing** だ。

<FigureBox caption="GMP モデルと work-stealing。各 P(プロセッサ)は自分のローカル実行キューを持つ。仕事は生成した P に溜まりがちで偏る。ローカルが空になった P は、まずグローバルキューを見て、無ければ他の最も忙しい P からキューの半分を横取りする。これで偏りが自動的に均される">

```
        P0                    P1                    P2
   ┌─ local ─┐           ┌─ local ─┐           ┌─ local ─┐
   │ G G G G │           │ (空)    │  ◀─steal──│ (空)    │
   └────┬────┘           └─────────┘   half    └─────────┘
        │ 偏り!                ▲___________________|
        └── P1 が半分を横取り ──┘   暇な P が忙しい P から仕事を盗む

        ┌──────────── global run queue(全 P 共有)────────────┐
        │  ローカルが溢れたぶん / 新規のあふれ                │
        └────────────────────────────────────────────────────┘
```

</FigureBox>

順に見ていく。

1. **P ごとのローカルキュー**: 仕事は生成した P のローカルキューに積む。ロック無しで速いが偏りやすい
2. **work-stealing で均す**: 空になった P は、最も混んでいる他の P からキューの半分を盗む
3. **グローバルキューとあふれ**: ローカルが溢れたら半分をグローバルへ退避し、暇な P はそこからも引く

## ① G と P: 走らせる仕事と、走らせる文脈

まず登場人物を定義する。G は goroutine で、走らせたい仕事(ここでは `work` tick を消費し切ったら終わる)。P はプロセッサで、スケジューリングの文脈として自分だけのローカル実行キューを持つ。M(OS スレッド)は「P 1 つにつき常に 1 つある」と割り切って省く(本章の主題は P 間の仕事の均し方だから):

<<< ../../foundations/scheduler/goroutine.go#gp{go}

`P` が `local`(ローカルキュー)を持つのが要。G の出し入れがローカルに閉じるので、コアごとにロック無しで回せる。これがマルチコアでスケールする土台だ。`ran`(実行 tick)は、あとで「負荷が均せたか」を見る指標になる。

## ② スケジューラ: ローカルに積み、あふれたらグローバルへ

`Scheduler` は複数の P と、全 P が共有するグローバルキューを持つ。`Go` は goroutine を生成して P0 のローカルキューに積み、「1 つの goroutine から生まれた仕事は同じ P に溜まる」を模す。ローカルが上限に達したら、半分をグローバルへ退避する(spill):

<<< ../../foundations/scheduler/scheduler.go#scheduler{go}

なぜ P0 に固めるのか。実際の Go でも、ある goroutine が `go f()` で生んだ子は、親が乗っている P のローカルキューに入る。だから仕事は自然と偏る。この偏りを次の work-stealing が均す。それを見せるために、あえて偏った積み方をしている。

## ③ Step と work-stealing: 暇な P が仕事を盗む

心臓部。`Step` は 1 ラウンド進める。全 P が index 順に「必要なら仕事を確保 → 1 量子走らせる」を 1 回ずつ行う(ラウンド制で並行実行を決定的に表す)。仕事の確保がポイントだ。ローカルが空なら、まずグローバルキューを見て、それも空なら `steal` で他の最も混んでいる P からキューの半分を横取りする:

<<< ../../foundations/scheduler/scheduler.go#step{go}

`steal` が均衡の要だ。盗むのが半分なのは、盗みすぎず・盗まなさすぎずの勘所だ。全部盗むと今度は相手が空になって盗み返しが起き、少ししか盗まないと何度も盗みに行く羽目になる。半分なら、盗んだ側も盗まれた側も次の仕事を持てる。`quantum` を使い切った G をローカル末尾へ戻すのは、os 編の yield に当たる協調的な切り替え点だ。

### 動かす

下のデモは、この M:N スケジューラを**そのままブラウザで**動かしている。3 つの P があり、goroutine は全部 P0 に積まれる(偏り)。「1手すすめる」で 1 ラウンド進むと、P1・P2 が空になった瞬間に **P0 から仕事を半分横取りする**様子が見える。各 P の実行量(下のバー)が、偏った投入にもかかわらず**だんだん揃っていく**。work-stealing が負荷を均しているのが分かるはずだ。

<SchedulerDemo />

## 設計の観点: なぜ M:N + work-stealing か

- **1:1 vs M:N**: 「1 goroutine = 1 OS スレッド」(1:1)は単純だが、スレッドは生成が重く(数 KB〜MB のスタック)、数万本で破綻する。M:N は軽量な G を少数の M に多重化するので、goroutine を数十万本作っても安い。C10K を[イベントループ](/parts/event-loop)で解くのと同じ動機を、言語ランタイムに埋め込んだ形
- **なぜローカルキューか**: 単一グローバルキューはスケールしない。全コアが 1 つのロックを奪い合う。P ごとのローカルキューにすれば、大半の操作がロック無しで済む。その代償(偏り)を work-stealing で埋める
- **work-stealing の性質**: 盗む側(暇なコア)がコストを払う。忙しいコアは自分の仕事に集中でき、余ったコアが自律的に仕事を探しに行く。分散設計の定番で、Java の ForkJoinPool・Rust の Tokio・TBB も同じ発想
- **公平性と飢餓**: ローカル優先だとグローバルキューの G が後回しになりうる。実機の Go は約 61 回に 1 回グローバルを優先して見て飢餓を防ぐ。「速さ(ローカル優先)」と「公平さ(たまに全体を見る)」のトレードオフ
- **プリエンプション**: 協調点(関数呼び出し)でしか切り替わらないと、タイトなループが P を占有して他を飢えさせる。Go 1.14 以降は**非同期プリエンプション**(シグナルで割り込む)を足した。os 編で触れた話がここでも効く

## メリット・デメリットと実例

| 方式 | 並列度 | 生成コスト | 負荷分散 | 実例 |
|---|---|---|---|---|
| 1:1(スレッド=タスク) | 出る | 高い(スタック重い) | OS 任せ | pthread、Java 旧来スレッド |
| 単一キュー M:N | ロック競合で頭打ち | 低い | 中央キュー | 素朴なスレッドプール |
| M:N + work-stealing(本章) | 出る | 低い | 自律的に均衡 | Go、Java ForkJoinPool、Rust Tokio、Erlang |
| イベントループ(1 スレッド) | 出ない(1 コア) | 低い | 不要 | Node.js、nginx、Redis |

裏どり:

- **Go ランタイム**: まさに GMP + work-stealing。`runtime/proc.go` の `schedule` / `findrunnable` / `runqsteal` が実物。`GOMAXPROCS` が P の数(並列度)。Go 1.14 で非同期プリエンプションを追加
- **Java ForkJoinPool**: 各ワーカスレッドが自分の deque を持ち、空になったら他から末尾を盗む。`parallelStream()` の裏でも動く、work-stealing の代表例
- **Rust Tokio / Erlang BEAM**: どちらも軽量タスク(async task / プロセス)を少数スレッドに多重化し、work-stealing で均す。M:N は現代の並行ランタイムの共通解
- **一方 Node.js は逆張り**: 1 スレッドの[イベントループ](/parts/event-loop)で多重化する。CPU 並列が要らない I/O 中心の用途では、スケジューラの複雑さを持たないこちらが単純で速い

## 簡略化したこと

- **M(OS スレッド)を省略**: 「P 1 つに M が常に 1 つ」の前提。syscall で M がブロックしたとき P を別 M に渡す handoff・spinning M・M の生成/破棄は扱わない
- **横取りは決定的**: 実機はランダムな P から複数回試みる。ここでは最混雑 P 固定(再現性を優先)
- **グローバルの定期チェック省略**: 実機の「約 61 回に 1 回グローバル優先」は入れない(仕組みは設計の観点の節で説明)
- **仕事は tick 数**: G は実際の計算をせず `work` tick を消費し切ったら終了。チャネル待ち・I/O 待ちでのブロックや goroutine 間の依存は扱わない
- **非同期プリエンプション無し**: 切り替えは量子境界のみ。シグナル割り込みによる強制プリエンプションは省略([os 編](/parts/os)で対比を説明)

## 参考資料

- Dmitry Vyukov, "Scalable Go Scheduler Design Doc" — Go の GMP スケジューラ設計の原典
- Go ランタイムの [`runtime/proc.go`](https://github.com/golang/go/blob/master/src/runtime/proc.go) — `schedule` / `findrunnable` / `runqsteal` の実物
- Blumofe & Leiserson, "Scheduling Multithreaded Computations by Work Stealing"(1999) — work-stealing の理論的原典
- 実装: [foundations/scheduler](https://github.com/esh2n/sharin/tree/main/foundations/scheduler)
