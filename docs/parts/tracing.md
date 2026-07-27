<script setup>
import TracingDemo from '../components/TracingDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 分散トレーシング

> 実装: [`observability/tracing/`](https://github.com/esh2n/sharin/tree/main/observability/tracing) / 実行: `go test ./observability/tracing/`

<Summary>
1つのリクエストは、いくつものサービスを渡り歩く。全体が800ミリ秒かかったとき、どこで時間を使ったかは1台のログを見ても分からない。分散トレーシングはリクエストに1つのtrace_idを振り、各サービスの処理をspanとして記録する。spanは親spanのidを持ち、境界はヘッダで伝える。spanと伝播を実装し、集めたspanを木に組み立てて、全体の時間を決めるクリティカルパスを特定する。
</Summary>

## この章で作るもの

[メトリクス](/parts/metrics)は「システム全体でどれくらい遅いか」を教えてくれる。p99 が 800ms だと分かった。だが「なぜ」は教えてくれない。1 つのリクエストがゲートウェイ・認証・在庫・課金と 4 つのサービスを渡り歩くとき、その 800ms がどこで使われたのか。各サービスのログを別々に見ても、同じリクエストのログを突き合わせる手がかりがない。1 秒間に何千ものリクエストが流れる中で、この 1 本を追うのは干し草の山から針を探すようなものだ。

分散トレーシングはこれを解く。リクエストが入ってきた瞬間に、ただ 1 つの trace_id を振る。以後、そのリクエストがどのサービスに渡ろうと、この trace_id が付いて回る。各サービスは自分の処理を span として記録し、span には trace_id と「誰に呼ばれたか(親の span_id)」を書く。サービスの境界を越えるときは、trace_id と今の span_id をヘッダに詰めて渡す。あとで同じ trace_id の span を集めて親子で組み立てれば、1 リクエストの全行程が 1 本の木になる。

<FigureBox caption="1 リクエストのトレース。全 span が同じ trace_id を共有し、親子で繋がる。横軸は時間で、どの span がいつ走ったかが見える">

```
trace_id=1
gateway   ├──────────────────────────────────┤  0-100
auth      │  ├───┤                                10-30
handler   │      ├─────────────────────────┤      30-95
inventory │        ├──┤                            35-50
billing   │           ├──────────────────┤        50-90  ← 遅い
          0   20   40   60   80   100 (ms)
```

</FigureBox>

肝は3つ:

1. **trace_id で束ねる**: リクエストに 1 つの trace_id を振り、全 span が共有する。散らばった記録を 1 本に束ねる鍵
2. **親子で木にする**: 各 span は親の span_id を持つ。集めて組み立てると呼び出しの木が復元できる
3. **境界を伝播する**: サービスを越えるとき trace_id と span_id をヘッダで渡す。別プロセスの span も同じトレースに繋がる

## ① span と ID: 記録の単位

まず span を定義する。1 つのサービスでの 1 区間の処理だ。自分の span_id、リクエスト全体で共通の trace_id、そして親の span_id を持つ:

<<< ../../observability/tracing/tracing.go#model{go}

ID は決定的な生成器から採る。実物はランダムな 128bit などを使うが、ここでは連番にしてテストを再現可能にした。0 を「親なし」の印に予約し、根の span(リクエストの入口)は ParentID が 0 になる。SpanContext は、この後で境界を越えて運ぶ最小の情報だ。trace_id で「どのリクエストか」、span_id で「誰が親になるか」を伝える。この 2 つさえ渡れば、受け取った側は自分の span を正しい位置に繋げられる。

## ② span を開始・終了する

Tracer が span を開始し、終了時に完了一覧へ集める。根の span は StartRoot で、子は親の SpanContext を渡して Start する。子は trace_id を親から受け継ぎ、親の span_id を自分の ParentID に書く:

<<< ../../observability/tracing/tracing.go#tracer{go}

ここに伝播(Inject / Extract)も入れた。あるサービスが別のサービスを HTTP で呼ぶとき、SpanContext をヘッダ文字列にして(Inject)リクエストに載せる。受け取った側はヘッダから復元し(Extract)、それを親として自分の span を始める。プロセスが別でも、ID 生成器が別でも、trace_id と親の span_id さえ伝われば span は 1 つのトレースに繋がる。実物はこれを W3C Trace Context の `traceparent` ヘッダで標準化していて、ここではその最小形を書いた。

## ③ 木に組み立て、クリティカルパスを見る

集めた span はただの平らな一覧だ。ここから親子関係で木を組み立て直す。ParentID が指す先を親として繋げば、呼び出しの構造が戻ってくる:

<<< ../../observability/tracing/tracing.go#tree{go}

木ができたら、次は「どこが遅いか」を突き止める。ここで見るのがクリティカルパスだ。親 span は、子(並列に走ることもある)がすべて終わって初めて自分を終われる。だから全体の所要時間を決めているのは、各段で最も遅く終わる子の連なりだ。根から「いちばん遅く終わる子」を辿っていくと、全体時間の責任を負う鎖が出る。サンプルのトレースなら gateway → handler → billing。billing が遅い限り、認証や在庫をいくら速くしても全体は縮まない。最適化すべき場所を、勘でなくこの鎖が指す。

テストでこれを固定した。5 つの span を組み、trace_id が全 span で共有されること、親子リンクが正しいこと、クリティカルパスが gateway → handler → billing になることを確かめた。さらに Inject / Extract を挟んでも、別プロセスの span が同じ trace_id と正しい親に繋がることも固定した。

### 動かす

下のデモは 1 リクエストのトレースを時間軸で描く。各 span を横棒(いつからいつまで走ったか)で並べ、クリティカルパスを強調する。遅いサービスを切り替えると、クリティカルパスがどう変わり、どこを直せば全体が縮むかが見える。

<TracingDemo />

## 設計の観点

- **trace_id の生成場所**: リクエストの最も外側(ゲートウェイやロードバランサ)で 1 度だけ振る。内側で振り直すとトレースが分断される。既に trace_id 付きで来たら引き継ぐ
- **伝播の標準**: 独自ヘッダでなく W3C Trace Context(`traceparent`)を使う。異なるベンダの計装同士でも繋がる。ここでは仕組みを見るため最小形にした
- **サンプリングとの関係**: 全リクエストの全 span を保存すると量が多すぎる。どれを残すかは [trace-sampling](/parts/trace-sampling) の話。トレーシング(記録の仕組み)とサンプリング(保存の取捨)は別の層
- **クリティカルパスで狙いを定める**: 遅い span を闇雲に直すのでなく、クリティカルパス上の span を直す。パス外の span を速くしても全体は縮まない
- **オーバーヘッド**: 計装は各リクエストにわずかな負荷を足す。span の生成・伝播・送信は軽く保ち、重い属性は必要な span だけに付ける

## 対照と実例

| 観測手段 | 答えられる問い | 答えられない問い |
|---|---|---|
| メトリクス | 全体でどれくらい遅い/多いか | この 1 本がなぜ遅いか |
| ログ | この瞬間に何が起きたか | 複数サービスに跨る流れ |
| トレース | 1 リクエストがどこで時間を使ったか | 全体の統計傾向 |

この 3 つ(メトリクス・ログ・トレース)を可観測性の三本柱と呼ぶ。

裏どり:

- **Dapper (Google)**: 分散トレーシングの原典。trace_id・span・親子・サンプリングの基本設計を示した論文。以降の実装はほぼこの子孫
- **W3C Trace Context**: `traceparent` ヘッダの標準。ベンダ非依存で trace_id と span_id を伝播する
- **OpenTelemetry**: 現在の事実上の標準。span / context propagation / exporter を統一 API で提供。この章の Tracer / Inject / Extract はその最小版
- **Jaeger / Zipkin / Tempo**: 集めた span を保存し、木とタイムラインで可視化するバックエンド

## 簡略化したこと

- **論理時計**: 実時刻でなく Advance で進める整数。決定性のため
- **属性・イベントなし**: span に付ける tag / log は扱わない
- **ID は連番**: 実物はランダムな 128bit trace_id / 64bit span_id
- **サンプリングなし**: 全 span を保存する。取捨は [trace-sampling](/parts/trace-sampling) で
- **並行実行なし**: 論理時計を手で進めるだけで、実際のゴルーチンは走らせない

## 参考資料

- [Dapper, a Large-Scale Distributed Systems Tracing Infrastructure (Google)](https://research.google/pubs/pub36356/) — 分散トレーシングの原典
- [W3C Trace Context](https://www.w3.org/TR/trace-context/) — `traceparent` 伝播の標準
- [OpenTelemetry: Traces](https://opentelemetry.io/docs/concepts/signals/traces/) — 現行標準の span / 伝播モデル
- 実装: [observability/tracing](https://github.com/esh2n/sharin/tree/main/observability/tracing)
