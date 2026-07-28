# autoscaler — 負荷からレプリカ数を決める(HPA)

調整ループは「何個に保つか」を守る。その「何個であるべきか」を負荷から自動で決める。Kubernetes の HorizontalPodAutoscaler の核。

## 肝

- **中心は 1 行の式**: `desired = ceil(current * currentMetric / targetMetric)`。負荷とレプリカ数が比例するという仮定に立つ
- **切り上げる**: 3.2 個必要なら 4 個。足りないより多いほうが安全
- **許容誤差(tolerance)**: 目標付近の小さな揺れは無視する。これがないと増減を繰り返す(flapping)
- **上下は非対称**: 拡大は即座に、縮小は安定化ウィンドウのぶん待つ。負荷は待ってくれないが、縮めすぎは戻せない
- **上下限が式より強い**: 式が何を出しても Min と Max の外には出ない
- **決定性**: 実時間でなく観測回数でウィンドウを数えるので、テストが再現的

## 効果の固定(テスト)

- `TestRatioFormula`: HPA の式そのもの。2 倍の負荷なら 2 倍のレプリカ、端数は切り上げ
- `TestScaleUpIsImmediate`: 使用率が目標を超えたら 1 回で必要数まで跳ぶ
- `TestToleranceIgnoresNoise`: 目標 ±10% の揺れでは動かない
- `TestScaleDownWaitsForStability`: 低い観測が続くまで縮まない
- `TestSpikeInWindowBlocksScaleDown`: ウィンドウ内に 1 度でもバーストがあれば縮まない
- `TestClampToMinMax`: 式が 16 を出しても上限 5 で止まる
- `TestBurstThenSettle`: バーストで速く増え、落ち着いてからゆっくり減る一連の流れ

## 使い方

```go
a := autoscaler.New(autoscaler.Config{
    Target:        50, // 目標使用率 50%
    Min:           1,
    Max:           10,
    Tolerance:     10, // ±10% の揺れは無視
    StabilizeDown: 3,  // 3 回続けて低ければ縮む
})
d := a.Decide(autoscaler.Sample{Replicas: 2, Utilization: 100})
d.To      // 4(式が出した必要数)
d.Raw     // 4(clamp も安定化も通す前の生の値)
d.Reason  // なぜその数にしたか
```

## 簡略化したこと

- **指標は 1 つだけ**: 実物は CPU・メモリ・カスタム指標・外部指標を同時に見て、最大の提案を採る
- **実時間なし**: ウィンドウを観測回数で数える。実物は秒で数える(既定 300 秒)
- **拡大側の制限なし**: 実物は拡大にも速度制限(scaleUp policy)をかけられる
- **Pod の起動時間を無視**: 増やした Pod が効き始めるまでの遅れは模していない
- **ノードは増えない**: Pod が増えても置く先がなければ Pending のまま。ノードを増やすのは Cluster Autoscaler の仕事

## 章

教科書: [水平オートスケール(HPA)](https://sharin-2a1.pages.dev/parts/autoscaler)

実行: `go test ./orchestration/autoscaler/`
