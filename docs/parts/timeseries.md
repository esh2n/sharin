<script setup>
import TimeSeriesDemo from '../components/TimeSeriesDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 時系列の整列と集約

> 実装: [`observability/timeseries/`](https://github.com/esh2n/sharin/tree/main/observability/timeseries) / 実行: `go test ./observability/timeseries/`

<Summary>
同じメトリクスは Pod の数だけ別々の系列として届く。1本の線にするには、まず時間方向に揃え、次に系列方向にまとめる。どちらも数を1つに潰す操作なので混同されやすいが、順序が結果を変える。先に分位点にしてから平均すると、遅い1台が速い多数に薄められて消える。分布のまま足してから取れば消えない。同じデータで違う結論が出る。
</Summary>

## この章で作るもの

[メトリクスとヒストグラム](/parts/metrics)の章では、1台のプロセスが値をどう持つかを見た。カウンタは増える一方の数を持ち、ヒストグラムはバケットに数を持つ。だが、画面に線が出るまでにはもう2段ある。

まず、系列がたくさんある。同じ `request_latencies` でも、Pod が 30 個あれば 30 本の別々の時系列として届く。ゾーンやリビジョンやレスポンスコードでも分かれるので、実際には数百本になる。

そして、点の時刻が揃っていない。各 Pod は自分の都合で値を送るので、同じ「10 秒 05 分」の点はどこにも無い。

この2つを片付けないと線が引けない。片付け方が2段あって、時間方向に揃えるのが整列(alignment)、系列方向にまとめるのが集約(reduction)になる。

<FigureBox caption="時間方向に揃えてから、系列方向にまとめる。どちらも数を潰す操作だが、潰す向きが違う">

```
  届く生データ(点の時刻はばらばら)

  pod-a  ・ ・  ・   ・ ・ ・    ・
  pod-b    ・  ・ ・   ・    ・ ・  ・
  pod-c  ・   ・    ・ ・  ・   ・

         ①整列(aligner)      時間方向に潰す。窓ごとに1点へ
         ↓

  pod-a  ┃    ┃    ┃    ┃      窓の中を mean / max / delta / p99 …
  pod-b  ┃    ┃    ┃    ┃
  pod-c  ┃    ┃    ┃    ┃

         ②集約(reducer)      系列方向に潰す。残すラベルを決める
         ↓

  all    ●    ●    ●    ●      同じ時刻の3点を mean / max / sum / p99 …
```

</FigureBox>

肝は3つ:

1. **種類が読み方を決める**: 累計は差を取らないと読めない。読み方はデータ自身が持っている
2. **分位点は平均できない**: 潰す順序を間違えると、遅い1台が消える
3. **平均も窓も、何かを隠す**: 平均は刺さった1本を、広い窓は短いスパイクを

## ① 種類が読み方を決める

同じ「数」でも、3つの種類がある:

<<< ../../observability/timeseries/timeseries.go#model{go}

`Gauge` はその瞬間の値で、CPU 使用率やキューの長さがこれになる。そのまま読める。

`Delta` は前回からの増分で、区間の合計に意味がある。

`Cumulative` が厄介で、計測開始からの累計になっている。そのまま描くと、ひたすら右上がりの線しか見えない。リクエスト数が増えているのか減っているのかは、傾きを見なければ分からない。

だから差を取る:

<<< ../../observability/timeseries/timeseries.go#aligner{go}

`AlignDelta` が窓ぶんの増分、`AlignRate` がそれを時間で割った秒あたりの値になる。ここまでは素直だが、1つ罠がある。

プロセスが再起動すると、累計は 0 から数え直す。素朴に引き算すると、そこで大きな負の値が出る。「リクエストが毎秒マイナス 1200 件」という線が引かれることになる。だから、前の窓より小さくなっていたら数え直しとみなして、現在値そのものを増分として扱う。テストで、数え直しを挟んでも負の値が出ないことを固定した。

分布値を持つ系列に `AlignDelta` を使うと、窓の中のヒストグラムを足し合わせた分布が出る。ここで分布のまま残るのが大事で、その理由が次になる。

## ② 分位点は平均できない

集約はこうなる:

<<< ../../observability/timeseries/timeseries.go#reducer{go}

`Reduce` は `groupBy` に挙げたラベルだけを残し、残りが同じ系列を1本にまとめる。ラベルを1つも残さなければ全体で1本になり、`zone` だけ残せばゾーンの数だけ線が出る。集約とは「どのラベルを捨てるか」を決めることになっている。

ここで、この章でいちばん間違えられるところが出てくる。

3台の Pod があって、2台は速く、1台だけが遅いとする。遅い台では 1 割のリクエストが 900 ミリ秒かかっている。全体の p99 を知りたい。

素朴には、各台の p99 を出してから平均すればよさそうに見える。だが平均は「3台ぶんの真ん中」を出すので、遅い1台の値が速い2台に薄められる。テストで、この誤ったやり方では 318 ミリ秒、正しいやり方では 835 ミリ秒になることを固定した。2.6 倍違う。

どちらもバケットの中を線形補間した推定値なので、真の値である 900 ミリ秒とは少しずれる。だがずれの向きが違う。正しいほうは推定の誤差ぶんだけ低く出ているだけで、誤ったほうは仕組みとして低く出ている。台数を増やせば増やすほど、誤ったほうはさらに低くなる。

正しい順序は、分布のまま足してから分位点を取ることになる。ヒストグラムはバケットごとに足せるので、3台ぶんを足せば全体の分布が手に入る。そこから 99 パーセンタイルを取れば、全体の 99 パーセンタイルになる。この足し算ができることが、[ヒストグラム](/parts/metrics)を使う理由そのものだった。

順序を式にすると、こうなる。

- 間違い: `ALIGN_PERCENTILE_99` してから `REDUCE_MEAN`(分位点を平均している)
- 正しい: `ALIGN_DELTA` で分布のまま揃えてから `REDUCE_PERCENTILE_99`

重みの問題も同じところから来る。1万件を捌いた Pod と、10 件しか来ていない Pod を系列として等しく扱うと、10 件のほうが半分の重みを持ってしまう。分布のまま足せば件数がそのまま重みになる。テストで、100 件と 1 件を混ぜたときに件数の多い側へ寄ることを固定した。

なぜ整列が先なのかも、ここで分かる。点の時刻が揃っていないまま集約すると、たまたま同じ時刻に点があった系列だけが足され、無かった系列は抜ける。値が変わっていなくても、線は上下する。テストで、10 秒ごとに送る系列と 30 秒ごとに送る系列を揃えずに足すと値が揺れ、先に窓へ揃えると揺れなくなることを固定した。

## ③ 平均も窓も、何かを隠す

10 台のうち1台だけが 98% まで張り付いているとする。残り9台は 10%。平均は 18.8% になる。閾値 65% で警告を出す設定にしていても、平均で見ている限り一度も鳴らない。テストで、平均は一度も 65 を超えず、最大なら超えることを固定した。

これは平均が間違っているのではなく、平均という問いに対して正しく答えているだけになる。「全体としてどれくらい使っているか」を知りたいなら平均が正しいし、「困っているところがあるか」を知りたいなら最大が正しい。同じデータに違う問いを投げている。

窓の長さも同じ形の話になる。10 秒だけ 610 まで跳ねる系列を、10 秒窓の平均で見れば 610 のまま見える。600 秒窓の平均で見ると 20 まで落ちて、スパイクは消える。テストで、同じデータが窓の長さだけで見え方を変えること、同じ広い窓でも最大で取れば残ることを固定した。

長い窓には理由がある。窓が短いと点の数が増えて重く、細かい揺れで警告が鳴りやすい。だから運用では窓を広げたくなる。広げると、広げたぶんだけ短い事象が見えなくなる。どちらが正しいということはなく、何を見たいかで決まる。

### 動かす

下のデモは、同じ生データに違う整列と集約を当てる。左が設定で、右が結果の線になる。分位点を先に取るか後に取るかを切り替えると、同じデータから違う数字が出る。窓の長さを動かすと、スパイクが消えたり戻ったりする。

<TimeSeriesDemo />

## GCP での読み方

ここまでの2段は、Cloud Monitoring がそのまま持っている概念になる。API の `aggregation` には `perSeriesAligner` と `crossSeriesReducer` があり、名前も役割も対応している。

### 自作したものとの対応

| この章 | Cloud Monitoring |
|---|---|
| `Align(s, AlignRate, 60)` | `perSeriesAligner: ALIGN_RATE`, `alignmentPeriod: 60s` |
| `AlignDelta` | `ALIGN_DELTA` |
| `AlignP99` | `ALIGN_PERCENTILE_99` |
| `Reduce(all, ReduceMean, "zone")` | `crossSeriesReducer: REDUCE_MEAN`, `groupByFields: ["resource.label.zone"]` |
| `ReduceP99` | `REDUCE_PERCENTILE_99` |
| `Kind` の 3 種 | `metricKind: GAUGE / DELTA / CUMULATIVE` |
| 分布を持つ点 | `valueType: DISTRIBUTION` |

制約も対応している。`ALIGN_RATE` と `ALIGN_DELTA` は `CUMULATIVE` と `DELTA` にしか使えず、`ALIGN_PERCENTILE_99` は分布値にしか使えない。系列をまたぐ集約をするには、`perSeriesAligner` が `ALIGN_NONE` でないことが要る。整列が先だという順序が、API の制約として書かれている。

覚えておくとよい数字が1つあって、Cloud Monitoring の保持期間は 6 週間になる。それより長く見たいなら、どこかへ書き出しておく必要がある。

### Spanner

Spanner でまず見るのは CPU になる。指標は2つある。

`spanner.googleapis.com/instance/cpu/utilization_by_priority` は優先度別の使用率で、`priority` ラベルが `high` / `medium` / `low` に分かれる。`high` がユーザのリクエスト、`low` がバックグラウンドの作業になる。この分解が要るのは、合計だけ見ると「使い切っているように見えるが、実は低優先度の作業が埋めているだけ」という状態を区別できないからだ。低優先度の作業は高優先度に譲るので、そこが埋まっていること自体は問題ではない。

`spanner.googleapis.com/instance/cpu/smoothed_utilization` が均した使用率で、自動スケールの判断はこちらを見る。

閾値は構成で違う。リージョナルなら 65%、マルチリージョンなら 45% を上限として警告を出す。マルチリージョンのほうが低いのは、フェイルオーバーしたときに残った側で捌けるだけの余裕が要るからだ。長期の目安としては、24 時間の移動平均が 90% を下回っていること。

ここで ③ の話が効いてくる。この閾値は個々のインスタンスに対するものなので、複数インスタンスを平均して見てはいけない。`REDUCE_MAX` で見るか、`groupByFields` に `instance_id` を入れて分けて見る。

レイテンシは `spanner.googleapis.com/api/request_latencies` で、これは分布値になる。だから ② の話がそのまま当てはまる。`ALIGN_PERCENTILE_99` してから `REDUCE_MEAN` すると、遅い1つが薄まる。`ALIGN_DELTA` で分布のまま揃えてから `REDUCE_PERCENTILE_99` を当てる。

そして Spanner の遅さは、たいてい CPU ではなくロック待ちから来る。これは Monitoring の指標では追いきれないので、内部表を直接見る。

```sql
SELECT
  interval_end,
  row_range_start_key,
  lock_wait_seconds,
  sample_lock_requests
FROM spanner_sys.lock_stats_top_minute
ORDER BY lock_wait_seconds DESC
LIMIT 10;
```

`row_range_start_key` にぶつかっている行キーの範囲が出るので、どのキーが熱いかが分かる。`sample_lock_requests` には、どの列にどのモードのロックを取ろうとしたかが最大 20 件まで入っている。保持は 1 分表が 6 時間、10 分表が 4 日、1 時間表が 30 日になる。障害の後で見に行くなら、この保持期間の内側に居るかを先に確かめる。

書き込みが特定のキーに集中しているなら、そもそも主キーの設計を疑うことになる。連番や時刻を主キーの先頭に置くと、書き込みが常に末尾の1つの分割に集まる。Key Visualizer で見ると、その集中が縞模様として見える。

### Pub/Sub

Pub/Sub は、指標を2つ組で見る設計になっている。

`pubsub.googleapis.com/subscription/num_undelivered_messages` が未処理の件数(GAUGE、INT64)、`subscription/oldest_unacked_message_age` が最も古い未処理メッセージの経過秒数(GAUGE、INT64)。

なぜ両方要るのか。件数だけでは、それが問題かどうか判断できない。毎秒 10 万件を捌く系で 100 万件溜まっているのは 10 秒ぶんでしかなく、健全な範囲になる。逆に、件数が 3 件で止まっていても、その 3 件が 6 時間前から居るなら、それは処理できないメッセージが詰まっているということになる。

読み分けはこうなる。

- 件数も経過時間も一緒に増える: 処理が追いついていない。購読側を増やす
- 件数は横ばいで経過時間だけ伸びる: 特定のメッセージが処理できずに残っている。デッドレターの出番
- どちらも急に落ちた: 期限切れで消えたか、シークされた可能性がある

警告は、メッセージの保持期間より手前で鳴るようにしておく。保持を超えれば消えるので、鳴ってから気づいても間に合わない。

配信側は `subscription/ack_message_count`(DELTA、INT64)と `subscription/push_request_latencies`(DELTA、分布)を見る。push の場合、受け側がエラーを返すと Pub/Sub は指数的に間隔を空けるので、エラー率が上がった瞬間に滞留の増え方が急になる。エラー率と滞留を並べて置くと、原因と結果が同じ画面に載る。

比較的新しいものとして、配信レイテンシの健全性スコアがある。シーク、否定応答、期限切れ、応答遅延、低い利用率の5つを 10 分の窓で見て、それぞれ健全かどうかを 0 と 1 で採点する。個別の指標を並べる前の入口として使える。

### GKE と Cloud Run

コンテナ側で見るのは、要求に対する使用率になる。`kubernetes.io/container/cpu/limit_utilization` は上限に対する使用率、`request_utilization` は要求に対する使用率で、意味が違う。[ResourceQuota と LimitRange](/parts/quota) の章で見たとおり、要求は予約で、上限は天井になる。要求に対して 200% 使っていても、上限まで余裕があれば問題は起きない。上限に張り付いていれば絞られている。

メモリは絞られずに殺されるので、`memory/limit_utilization` が 100% に近づいているかを別に見る。

ここでも ③ が効く。Pod ごとの使用率を平均で見ると、1つだけ張り付いている Pod が消える。`REDUCE_MAX` を並べて置くか、`groupByFields` に `pod_name` を入れる。

Cloud Run なら `run.googleapis.com/request_latencies`(分布)と `request_count`(DELTA)、それに `container/instance_count` を見る。インスタンス数が上限に張り付いていれば、遅さの原因は自分のコードではなく上限のほうになる。

### BigQuery

BigQuery は、Monitoring の指標と内部表の両方から見る形になる。

指標側では `bigquery.googleapis.com/slots/allocated` と `slots/total_allocated_for_reservation` を見る。予約したスロットに対してどれだけ使っているか、待たされているジョブがどれだけあるかが分かる。

だが、どのクエリが重いかは指標では分からない。そこは `INFORMATION_SCHEMA.JOBS` を直接引く。

```sql
SELECT
  job_id,
  user_email,
  total_bytes_billed / POW(1024, 3) AS gb_billed,
  total_slot_ms,
  TIMESTAMP_DIFF(end_time, start_time, MILLISECOND) AS elapsed_ms,
  SAFE_DIVIDE(total_slot_ms, TIMESTAMP_DIFF(end_time, start_time, MILLISECOND)) AS avg_slots,
  query
FROM `region-us`.INFORMATION_SCHEMA.JOBS
WHERE creation_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
  AND job_type = 'QUERY'
ORDER BY total_slot_ms DESC
LIMIT 20;
```

`total_slot_ms` を経過時間で割ると、そのクエリが平均して何スロットを同時に使っていたかが出る。これが実質的な重さになる。処理バイト数だけで見ると、読んだ量は少ないのにジョインで中間結果が膨れ上がり、長時間スロットを占め続ける、といったクエリを見落とす。`job_stages` を開けば、どの段で何レコード読んだか、どこで待ったかまで見える。

そして federated query が絡んでくる。`EXTERNAL_QUERY` を使うと、Cloud SQL や Spanner、AlloyDB に対して、その DB の方言で書いたクエリを投げて、結果を一時テーブルとして受け取れる。

```sql
SELECT
  s.tenant_id,
  s.plan,
  b.query_count
FROM EXTERNAL_QUERY(
  'projects/p/locations/us-central1/connections/spanner-conn',
  'SELECT tenant_id, plan FROM tenants'
) AS s
JOIN (
  SELECT
    JSON_VALUE(labels, '$.tenant') AS tenant_id,
    COUNT(*) AS query_count
  FROM `region-us`.INFORMATION_SCHEMA.JOBS
  WHERE creation_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY)
  GROUP BY tenant_id
) AS b
USING (tenant_id);
```

これが効くのは、メトリクスに載っていない文脈を結合できるからだ。「スロットを食っているのはどのテナントか」までは `INFORMATION_SCHEMA` で分かるが、「そのテナントはどのプランで、どの営業担当が持っているか」は業務 DB にしかない。コピーを作らずに突き合わせられる。

制約もはっきりしている。読み取り専用で、DML も DDL も通らない。外部 DB の方言で書くので BigQuery の関数は使えない。BigQuery の中だけで完結するクエリより遅く、リージョンをまたぐと1日 1 TB の上限がある。定期的に大量を突き合わせるなら、素直にコピーしたほうが速い。

外部テーブルや BigLake との違いも押さえておく。外部テーブルは BigQuery の構文のまま外のデータを引けるが、`EXTERNAL_QUERY` は相手の方言で書く。相手側でフィルタと集約を済ませてから持ってこられるので、転送量を抑えたいときはこちらが効く。

Monitoring の指標そのものを BigQuery に持ってくることもできる。保持が 6 週間しかないので、それを超えて分析したいときは API で取り出して書き出しておく。Cloud Scheduler で定期的に叩く構成が公式の設計例として示されている。書き出してしまえば、この章の整列と集約を SQL で書くことになる。窓は `TIMESTAMP_TRUNC`、集約は `GROUP BY`。やっていることは同じで、名前が変わるだけになる。

### クエリ言語

Cloud Monitoring には書き方が3つある。

コンソールの GUI は `perSeriesAligner` と `crossSeriesReducer` を選ばせる形になっていて、この章の2段がそのまま画面に出ている。

MQL は独自の言語で、`| align rate(1m) | every 1m | group_by [resource.zone], mean(val())` のようにパイプで繋ぐ。整列と集約が別の演算子として現れるので、順序が目に見える。

PromQL も使える。Google Cloud Managed Service for Prometheus を使うと、Prometheus のデータモデルと PromQL のまま書いて、保存先が Cloud Monitoring になる。`rate(http_requests_total[1m])` の `rate` が整列、`sum by (zone) (...)` が集約に当たる。名前が違うだけで、やはり2段になっている。

どれを選んでも構造は同じなので、この章の2段を押さえておけば読み替えられる。

### ログから作る指標

指標が用意されていないものは、ログから作れる。ログベースの指標には2種類あって、条件に当たったログの件数を数えるカウンタ型と、ログのフィールドから値を抜いて分布にする分布型がある。分布型にしておけば、後から分位点を正しく取れる。

ここでも ② の順序が効く。ログの1行ごとに「95 パーセンタイル」を持たせることはできないので、値そのものを分布に入れておく必要がある。抽出の時点で平均にしてしまうと、後から分位点は取り戻せない。

## 設計の観点

- **潰す前に何を知りたいか決める**: 平均も最大も分位点も、それぞれ違う問いへの正しい答えになる。問いを決めずに選ぶと、答えを誤読する
- **潰せる形のまま運ぶ**: 分布は足せるが、分位点は足せない。潰すのはいちばん最後にする
- **種類は捨てない**: 累計を「ただの数」として扱うと、差を取り忘れるか、再起動で負が出る
- **窓は見たいものの長さで決める**: 10 秒の事象を見たいなら窓は 10 秒より短くする。長い窓は軽いが、短い事象を消す
- **平均と最大を並べる**: どちらか一方だけを置いたダッシュボードは、必ず片方の見落としを持つ
- **[ヒストグラム](/parts/metrics)を前提にする**: 足せる形で持っておくことが、後の自由度になる

## 対照と実例

| 見たいこと | 整列 | 集約 | 間違えるとどうなるか |
|---|---|---|---|
| 秒あたりのリクエスト数 | `ALIGN_RATE` | `REDUCE_SUM` | 累計のまま描くと右上がりの線しか出ない |
| 全体の p99 レイテンシ | `ALIGN_DELTA`(分布のまま) | `REDUCE_PERCENTILE_99` | 先に p99 にすると遅い1台が薄まる |
| 困っているインスタンスの有無 | `ALIGN_MAX` | `REDUCE_MAX` | 平均だと1台の張り付きが消える |
| 全体としての使用量 | `ALIGN_MEAN` | `REDUCE_MEAN` | 最大だと1台の瞬間値に引っ張られる |
| エラー率 | `ALIGN_RATE` | `REDUCE_SUM` を分子と分母それぞれに | 系列ごとの比率を平均すると件数の重みが消える |
| 短いスパイクの検出 | `ALIGN_MAX` + 短い窓 | `REDUCE_MAX` | 長い窓の平均だと平滑化されて見えない |

裏どり:

- **Cloud Monitoring の集約**: `perSeriesAligner` が先、`crossSeriesReducer` が後。系列をまたぐ集約には `ALIGN_NONE` 以外の整列が必要
- **保持期間**: 指標は 6 週間。それより長く見るには API で取り出して BigQuery などに書き出す
- **Spanner の閾値**: リージョナル 65%、マルチリージョン 45%。24 時間移動平均は 90% 未満
- **Spanner のロック**: `SPANNER_SYS.LOCK_STATS_TOP_MINUTE` などの内部表。1 分表は 6 時間、1 時間表は 30 日の保持
- **Pub/Sub の2つ組**: `num_undelivered_messages`(件数)と `oldest_unacked_message_age`(経過時間)は別のことを言う。両方置く
- **federated query**: `EXTERNAL_QUERY` は Cloud SQL、Spanner、AlloyDB に対応。読み取り専用で、相手の方言で書く。リージョンをまたぐと1日 1 TB
- **BigQuery の重さ**: `total_slot_ms` を経過時間で割ると平均同時スロット数になる。処理バイト数だけでは重さを測れない

## 簡略化したこと

- **時刻は整数**: 実物はナノ秒精度のタイムスタンプと、区間の始点と終点を持つ
- **欠測を埋めない**: 実物は点が無い窓の扱い(補間するか空けるか)を選べる
- **分位点は補間**: バケットの中を線形補間するので厳密値ではない。[ヒストグラム](/parts/metrics)の章と同じ性質になる
- **数値への分位点は近似順位**: 分布値でない系列への `AlignP99` は、窓の中の値を並べて位置で取る
- **アラートなし**: 閾値、継続時間、通知は扱わない。線を出すところまで
- **書き出しなし**: 指標をどこかへ持っていく話は文章だけで、実装していない

## 参考資料

- [Filtering and aggregation](https://cloud.google.com/monitoring/api/v3/aggregation) — 整列と集約の順序、種類ごとの制約
- [Aligner / Reducer の一覧](https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.alertPolicies#Aligner) — enum の全一覧と適用条件
- [Spanner のモニタリング](https://cloud.google.com/spanner/docs/monitoring-cloud) — CPU の閾値と優先度別の見方
- [Spanner のロック統計](https://cloud.google.com/spanner/docs/introspection/lock-statistics) — 内部表の列と保持期間
- [Pub/Sub のモニタリング](https://cloud.google.com/pubsub/docs/monitoring) — 滞留の2つ組と警告の考え方
- [BigQuery の federated query](https://cloud.google.com/bigquery/docs/federated-queries-intro) — `EXTERNAL_QUERY` と制約
- [INFORMATION_SCHEMA.JOBS](https://cloud.google.com/bigquery/docs/information-schema-jobs) — スロットとバイト数の列
- [Cloud Monitoring metric export](https://cloud.google.com/architecture/monitoring-metric-export) — 6 週間を超える保存の設計例
- 実装: [observability/timeseries](https://github.com/esh2n/sharin/tree/main/observability/timeseries)
