# metrics — カウンタ・ゲージ・ヒストグラム

システムの健康を数字で見るための 3 つの型を最小構成で実装する。中心はヒストグラムで、平均が隠すテールレイテンシ(p99)をバケットから推定し、マシン間で合算できることを固定テストで確かめる。

## 肝

- **カウンタ**: 増える一方の値(処理総数、エラー総数)。`Add` に負を渡すとパニック
- **ゲージ**: 上下する値(同時接続数、キュー長、メモリ使用量)。Set/Add/Sub
- **ヒストグラム**: 値を上限つきバケットに数える(Prometheus 方式)。末尾に暗黙の +Inf バケット
- **平均は嘘をつく**: 速い大多数に引っ張られ、遅い少数(テール)が埋もれる。ユーザが怒るのは遅い方
- **分位点の推定**: 生の値を保存せず、バケット内を線形補間して p50/p99 を出す。バケットが細かいほど誤差が小さい
- **合算性(Merge)**: 同じ bounds のバケットは足し合わせられる。各台の p99 を平均しても全体の p99 にはならないが、バケットを足せば全体の p99 が正しく出る

## 効果の固定(テスト)

- `TestMeanHidesTail`: 980 件が 4ms、20 件(2%)が 800ms。平均は約 20ms だが p99 は数百 ms。平均だけ見るとテールを見逃す
- `TestMergeCombinesDistributions`: 速い台と遅い台のバケットを Merge し、合算後に中央値は速い側、p99 は遅い側から正しく出る

## 使い方

```go
var reqs metrics.Counter
reqs.Inc()

var conns metrics.Gauge
conns.Add(1); defer conns.Sub(1)

h := metrics.NewHistogram([]float64{10, 50, 100, 500}) // ms の上限
h.Observe(latencyMs)
p99 := h.Quantile(0.99)
h.Merge(otherNodeHistogram) // 全台を合算してから Quantile
```

## 簡略化したこと

- **並行安全でない**: 実物は atomic やロックで保護。ここでは単一ゴルーチン前提
- **ラベルなし**: Prometheus の {method="GET"} のような次元は持たない
- **固定バケット**: 動的にバケット境界を変える指数バケット等は扱わない
- **エクスポート形式なし**: /metrics テキスト出力やスクレイプは範囲外

## 章

教科書: [メトリクスとヒストグラム](https://sharin-2a1.pages.dev/parts/metrics)

実行: `go test ./observability/metrics/`
