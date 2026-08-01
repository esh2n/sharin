<script setup>
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# Cloud Monitoring で読み替える

<Summary>
時系列の整列と集約で作った2段は、Cloud Monitoring の API に同じ構造で載っている。Align は perSeriesAligner、Reduce は crossSeriesReducer で、整列が先という順序の制約まで対応する。だから自作して確かめた「分位点を先に取ると遅い1台が消える」が、そのまま実物の罠になる。画面の各欄も、製品ごとの指標も、その対応として読む。
</Summary>

## この章で読むもの

この章はコードを書かない。[メトリクスとヒストグラム](/parts/metrics)と[時系列の整列と集約](/parts/timeseries)で作ったものが、実物のどの名前に当たるかを突き合わせる。

そうする理由は 2 つある。1 つは、自作したときに確かめた性質が、そのまま実物の落とし穴になっているからだ。「分位点は平均できない」も「平均は張り付いた 1 台を隠す」も、画面の設定を 1 つ変えるだけで踏める。もう 1 つは、名前を知らないと探せないからで、`ALIGN_DELTA` という語を知っていれば公式ドキュメントに辿り着けるが、「分布のまま揃える」では検索できない。

操作の手順(どこをクリックするか)は書かない。画面は変わるが、下にある 2 段の構造は変わらないからだ。

順に見ていく。

1. **2 段はそのまま API になっている**: 名前も、順序の制約も対応する。書き方が 3 通りあっても構造は同じ
2. **製品ごとに「読み方の型」がある**: Pub/Sub の 2 つ組のように、片方だけでは判断できない指標の組がある
3. **指標に無いものは別の場所にある**: ログから作るか、内部表を直接引く

## ① 2 段はそのまま API になっている

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

制約も対応している。`ALIGN_RATE` と `ALIGN_DELTA` は `CUMULATIVE` と `DELTA` にしか使えず、`ALIGN_PERCENTILE_99` は分布値にしか使えない。系列をまたぐ集約をするには、`perSeriesAligner` が `ALIGN_NONE` でないことが要る。**整列が先だという順序が、API の制約として書かれている**。

### Metrics Explorer の画面は、この 2 段でできている

指標を選んでグラフにする画面には欄がいくつも並ぶが、やっていることは 5 段しかない。

<FigureBox caption="Metrics Explorer の欄と、この教科書の言葉の対応。中央の 2 つが整列と集約で、その前後に絞り込みと描画が付く">

```
  ① 指標を選ぶ        metric type + resource type
                      「何の数字か」と「誰の数字か」を決める
        ▼
  ② 絞り込む          filter(ラベルの条件)
                      系列の本数を減らす。潰す前の母集団が決まる
        ▼
  ③ 時間方向に揃える   perSeriesAligner + alignmentPeriod   ← 自作した Align
                      系列ごとに、窓の中を1点へ
        ▼
  ④ 系列方向にまとめる crossSeriesReducer + groupByFields   ← 自作した Reduce
                      残す軸を groupByFields に書き、それ以外を潰す
        ▼
  ⑤ 描く              線 / 積み上げ / ヒートマップ
                      潰し終わった後の話。ここで情報は増えない
```

</FigureBox>

読み違えが起きるのは ③ と ④ の間だ。④ で `REDUCE_MEAN` を選ぶのは自然に見えるが、③ で既に `ALIGN_PERCENTILE_99` を選んでいれば、それは「台ごとの p99 を平均する」になる。[時系列の整列と集約](/parts/timeseries)で測ったとおり、遅い 1 台がここで消える。

`groupByFields` の効き方も同じ形になる。ここに `instance_id` を入れておけば台ごとに線が分かれ、入れなければ全部が 1 本に潰れる。**潰したくない軸を書く欄**であって、見たい軸を選ぶ欄ではない、と読むほうが間違えにくい。

### 書き方は 3 通りあるが、構造は同じ

コンソールの GUI は `perSeriesAligner` と `crossSeriesReducer` を選ばせる形になっていて、上の 2 段がそのまま画面に出ている。

MQL は独自の言語で、`| align rate(1m) | every 1m | group_by [resource.zone], mean(val())` のようにパイプで繋ぐ。整列と集約が別の演算子として現れるので、順序が目に見える。

PromQL も使える。Google Cloud Managed Service for Prometheus を使うと、Prometheus のデータモデルと PromQL のまま書いて、保存先が Cloud Monitoring になる。`rate(http_requests_total[1m])` の `rate` が整列、`sum by (zone) (...)` が集約に当たる。名前が違うだけで、やはり 2 段になっている。

どれを選んでも構造は同じなので、2 段を押さえておけば読み替えられる。

覚えておくとよい数字が 1 つあって、Cloud Monitoring の保持期間は 6 週間になる。それより長く見たいなら、どこかへ書き出しておく必要がある。

## ② 製品ごとに「読み方の型」がある

### Spanner

Spanner でまず見るのは CPU になる。指標は 2 つある。

`spanner.googleapis.com/instance/cpu/utilization_by_priority` は優先度別の使用率で、`priority` ラベルが `high` / `medium` / `low` に分かれる。`high` がユーザのリクエスト、`low` がバックグラウンドの作業になる。この分解が要るのは、合計だけ見ると「使い切っているように見えるが、実は低優先度の作業が埋めているだけ」という状態を区別できないからだ。低優先度の作業は高優先度に譲るので、そこが埋まっていること自体は問題ではない。

`spanner.googleapis.com/instance/cpu/smoothed_utilization` が均した使用率で、自動スケールの判断はこちらを見る。

閾値は構成で違う。リージョナルなら 65%、マルチリージョンなら 45% を上限として警告を出す。マルチリージョンのほうが低いのは、フェイルオーバーしたときに残った側で捌けるだけの余裕が要るからだ。長期の目安としては、24 時間の移動平均が 90% を下回っていること。

ここで「平均は張り付いた 1 台を隠す」が効いてくる。この閾値は個々のインスタンスに対するものなので、複数インスタンスを平均して見てはいけない。`REDUCE_MAX` で見るか、`groupByFields` に `instance_id` を入れて分けて見る。

レイテンシは `spanner.googleapis.com/api/request_latencies` で、これは分布値になる。だから「分位点は平均できない」がそのまま当てはまる。`ALIGN_PERCENTILE_99` してから `REDUCE_MEAN` すると、遅い 1 つが薄まる。`ALIGN_DELTA` で分布のまま揃えてから `REDUCE_PERCENTILE_99` を当てる。

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

書き込みが特定のキーに集中しているなら、そもそも主キーの設計を疑うことになる。連番や時刻を主キーの先頭に置くと、書き込みが常に末尾の 1 つの分割に集まる。[ID生成](/parts/id-generation)で見た「ソート可能な ID は索引に優しい」の裏返しで、分散した書き込みが欲しい場面では同じ性質が仇になる。

### Pub/Sub

Pub/Sub は、指標を 2 つ組で見る設計になっている。

`pubsub.googleapis.com/subscription/num_undelivered_messages` が未処理の件数(GAUGE、INT64)、`subscription/oldest_unacked_message_age` が最も古い未処理メッセージの経過秒数(GAUGE、INT64)。

なぜ両方要るのか。件数だけでは、それが問題かどうか判断できない。毎秒 10 万件を捌く系で 100 万件溜まっているのは 10 秒ぶんでしかなく、健全な範囲になる。逆に、件数が 3 件で止まっていても、その 3 件が 6 時間前から居るなら、それは処理できないメッセージが詰まっているということになる。

読み分けはこうなる。

- 件数も経過時間も一緒に増える: 処理が追いついていない。購読側を増やす
- 件数は横ばいで経過時間だけ伸びる: 特定のメッセージが処理できずに残っている。デッドレターの出番
- どちらも急に落ちた: 期限切れで消えたか、シークされた可能性がある

警告は、メッセージの保持期間より手前で鳴るようにしておく。保持を超えれば消えるので、鳴ってから気づいても間に合わない。

配信側は `subscription/ack_message_count`(DELTA、INT64)と `subscription/push_request_latencies`(DELTA、分布)を見る。push の場合、受け側がエラーを返すと Pub/Sub は指数的に間隔を空けるので、エラー率が上がった瞬間に滞留の増え方が急になる。エラー率と滞留を並べて置くと、原因と結果が同じ画面に載る。この間隔の空け方は[リトライとバックオフ](/parts/retry-backoff)で作ったものと同じ形になる。

比較的新しいものとして、配信レイテンシの健全性スコアがある。シーク、否定応答、期限切れ、応答遅延、低い利用率の 5 つを 10 分の窓で見て、それぞれ健全かどうかを 0 と 1 で採点する。個別の指標を並べる前の入口として使える。

### GKE と Cloud Run

コンテナ側で見るのは、要求に対する使用率になる。`kubernetes.io/container/cpu/limit_utilization` は上限に対する使用率、`request_utilization` は要求に対する使用率で、意味が違う。[ResourceQuota と LimitRange](/parts/quota)の章で見たとおり、要求は予約で、上限は天井になる。要求に対して 200% 使っていても、上限まで余裕があれば問題は起きない。上限に張り付いていれば絞られている。

メモリは絞られずに殺されるので、`memory/limit_utilization` が 100% に近づいているかを別に見る。

ここでも平均の話が効く。Pod ごとの使用率を平均で見ると、1 つだけ張り付いている Pod が消える。`REDUCE_MAX` を並べて置くか、`groupByFields` に `pod_name` を入れる。

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

## ③ 指標に無いものは別の場所にある

### ログから作る

指標が用意されていないものは、ログから作れる。ログベースの指標には 2 種類あって、条件に当たったログの件数を数えるカウンタ型と、ログのフィールドから値を抜いて分布にする分布型がある。分布型にしておけば、後から分位点を正しく取れる。

ここでも順序が効く。ログの 1 行ごとに「95 パーセンタイル」を持たせることはできないので、値そのものを分布に入れておく必要がある。**抽出の時点で平均にしてしまうと、後から分位点は取り戻せない**。[メトリクスとヒストグラム](/parts/metrics)で「足せる形のまま持つ」と言ったのは、この段でも同じように効く。

### 別のデータと突き合わせる

`EXTERNAL_QUERY` を使うと、Cloud SQL や Spanner、AlloyDB に対して、その DB の方言で書いたクエリを投げて、結果を一時テーブルとして受け取れる。

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

制約もはっきりしている。読み取り専用で、DML も DDL も通らない。外部 DB の方言で書くので BigQuery の関数は使えない。BigQuery の中だけで完結するクエリより遅く、リージョンをまたぐと 1 日 1 TB の上限がある。定期的に大量を突き合わせるなら、素直にコピーしたほうが速い。

外部テーブルや BigLake との違いも押さえておく。外部テーブルは BigQuery の構文のまま外のデータを引けるが、`EXTERNAL_QUERY` は相手の方言で書く。相手側でフィルタと集約を済ませてから持ってこられるので、転送量を抑えたいときはこちらが効く。

### 保持期間を超えて見る

Monitoring の指標そのものを BigQuery に持ってくることもできる。保持が 6 週間しかないので、それを超えて分析したいときは API で取り出して書き出しておく。Cloud Scheduler で定期的に叩く構成が公式の設計例として示されている。

書き出してしまえば、整列と集約を SQL で書くことになる。窓は `TIMESTAMP_TRUNC`、集約は `GROUP BY`。やっていることは同じで、名前が変わるだけになる。

## 設計の観点

- **名前が違っても 2 段は同じ**: GUI でも MQL でも PromQL でも、整列してから集約するという構造は変わらない。1 つ覚えれば残りは読み替えで済む
- **制約は都合ではなく意味から来る**: `ALIGN_RATE` が累計にしか使えないのは実装の手抜きではなく、増える一方の数からしか変化率を出せないからになる。自作すると同じ制約に自分で行き当たる
- **潰したくない軸を書く**: `groupByFields` は「見たい軸を選ぶ欄」ではなく「潰さずに残す軸を書く欄」になる。この読み方をすると、書き忘れたときに何が消えるかが分かる
- **2 つ組で見る型を知っておく**: 件数と経過時間、要求に対する使用率と上限に対する使用率。片方だけでは判断できない組が製品ごとにある
- **指標の粒度には限界がある**: ロック待ちも、クエリ単位のスロット消費も、指標には出てこない。内部表を引く経路を先に知っておく
- **保持期間が分析の上限になる**: 監視の道具は直近を見るためのもので、季節性や年次比較には向かない。長く見たいなら書き出す前提で設計する

## 対照と実例

| 見たいこと | 見る場所 | よくある読み違え |
|---|---|---|
| Spanner が詰まっているか | `cpu/smoothed_utilization` を `REDUCE_MAX` | 平均で見て、張り付いた 1 インスタンスを見落とす |
| Spanner が遅い理由 | `spanner_sys.lock_stats_top_minute` | CPU だけ見て、ロック待ちに気づかない |
| Pub/Sub が捌けているか | 未処理件数と最古経過時間の 2 つ組 | 件数だけ見て、少数の詰まりを見逃す |
| Pod が絞られているか | `cpu/limit_utilization` | `request_utilization` と混同して、正常を異常と読む |
| 重い BigQuery クエリ | `INFORMATION_SCHEMA.JOBS` の `total_slot_ms` | 処理バイト数で見て、中間結果が膨れるクエリを見落とす |
| 6 週間より前の傾向 | 書き出した先(BigQuery 等) | Monitoring 上で探して、そもそも無いことに気づかない |

裏どり:

- **API の 2 段は公式に定義されている**: `aggregation` の仕様に `perSeriesAligner`(整列)と `crossSeriesReducer`(集約)が並び、整列なしでは系列をまたぐ集約ができないことが制約として書かれている。**この教科書で作った順序が、そのまま API の前提条件になっている**
- **分位点の制約**: `ALIGN_PERCENTILE_99` は分布値にしか使えない。値が分布のまま保持されていなければ、後から分位点は出せないという性質が、型の制約として現れている
- **Managed Service for Prometheus**: PromQL のまま書いて保存先が Cloud Monitoring になる。`rate` が整列、`sum by` が集約にあたり、語彙が違うだけで同じ 2 段を踏む
- **数値は版と構成で変わる**: 保持 6 週間、Spanner の 65% / 45%、`EXTERNAL_QUERY` の 1 日 1 TB といった値は、本書の執筆時点で公開されていたもの。**閾値や上限は運用の前に公式ドキュメントで確かめる前提で読んでほしい**
- **指標名は非推奨になることがある**: 製品側の世代交代で指標が置き換わることがある。名前で覚えるより、「何を測った数か」と「どの 2 段を通すか」で覚えるほうが持つ

## 簡略化したこと

- **操作手順は書かない**: どの画面のどこを押すかは扱わない。画面は変わるが、下の 2 段は変わらない
- **アラートの設計は扱わない**: 閾値・継続時間・通知経路の設計は別の主題になる
- **課金は扱わない**: 取り込み量や保持による費用は触れない
- **SLO と誤差予算は扱わない**: 指標の読み方までで、目標の立て方には踏み込まない
- **他社の監視基盤は扱わない**: Datadog や New Relic でも 2 段の構造は同じだが、対応表は Cloud Monitoring に絞った
- **収集側の構成は概略**: OpenTelemetry Collector や エージェントの配置は扱わない

## 参考資料

- [Monitoring API: Aggregation](https://cloud.google.com/monitoring/api/v3/aggregation) — `perSeriesAligner` と `crossSeriesReducer` の定義と制約
- [Metrics Explorer](https://cloud.google.com/monitoring/charts/metrics-explorer) — 画面の各欄が何にあたるか
- [Spanner: モニタリング](https://cloud.google.com/spanner/docs/monitoring-cloud) — CPU の閾値と指標の一覧
- [Pub/Sub: モニタリング](https://cloud.google.com/pubsub/docs/monitoring) — 滞留を 2 つ組で見る根拠
- [BigQuery: INFORMATION_SCHEMA.JOBS](https://cloud.google.com/bigquery/docs/information-schema-jobs) — ジョブ単位のスロット消費
- [ログベースの指標](https://cloud.google.com/logging/docs/logs-based-metrics) — カウンタ型と分布型
