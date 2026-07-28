# clusterautoscaler — ノードそのものを増減させる

水平オートスケーラは Pod を増やす。だが置き場所がなければ Pod は Pending のまま残る。足りないのがノードなら、ノードを増やすしかない。増やす判断も減らす判断も、[スケジューラ](../scheduler/)に委ねる。

## 肝

- **増やす前に確かめる**: 新しいノードを1台仮に置き、その Pending Pod が載るかを `scheduler.Filter` で判定する。載らないなら足さない。1台に収まらない要求のために無限に増やす事故を防ぐ
- **減らす前に確かめる**: そのノードを除いた世界を作り、全 Pod を置き直せるかを試す。1つでも置けなければ消さない。使用率が低いだけでは消す理由にならない
- **判断はスケジューラのもの**: この層に配置の知識はない。前章の scheduler パッケージをそのまま部品として使う
- **ノードの起動は遅い**: Pod は秒、ノードは分。起動中は Pod を置けず、Pending はただ待つ。この待ち時間がこの層の性質を決める
- **Pending があるうちは減らさない**: 増やす側と減らす側が同時に動くと、足しては消しての繰り返しになる
- **決定性**: 置き直しは要求の大きい順、同点は名前順。何度走らせても同じ結果

## 効果の固定(テスト)

- `TestScalesUpForPendingPods`: Pending が出るとノードを足し、起動を待ってから置ける
- `TestDoesNotScaleUpForImpossiblePod`: 1台に収まらない要求では増やさない
- `TestScalesDownWhenPodsFitElsewhere`: Pod が減って集約できるようになったら1台に寄せる
- `TestKeepsNodeWhenPodsCannotMove`: 使用率が低くても行き先がなければ消さない
- `TestDoesNotScaleDownWhilePending`: 置けていない Pod があるうちは減らさない
- `TestDeterministic`: 同じ入力なら何度走らせても同じ台数に落ち着く

## 使い方

```go
a := clusterautoscaler.New(clusterautoscaler.Config{
    NodeCap:       scheduler.Resources{CPU: 2000, Mem: 2048},
    MinNodes:      1,
    MaxNodes:      5,
    BootTicks:     3,  // ノードが使えるようになるまで
    ScaleDownUtil: 40, // この使用率を下回ったら縮小の候補
    Strategy:      scheduler.BinPack,
})
for i := 0; i < 6; i++ {
    a.Submit(scheduler.Resources{CPU: 500, Mem: 512})
}
for i := 0; i < 10; i++ {
    a.Tick()
}
a.Nodes()     // 使えるノード
a.Booting()   // 起動中のノード数
a.Pending()   // 置き場所が見つからない Pod
a.Remove("pod-5") // Pod を消すと、空きに応じて集約が起こる
```

## 簡略化したこと

- **ノードは全部同じ**: 実物はノードグループごとに種類が違い、どの種類を足すかも判断の対象になる
- **退避なし**: 縮小のとき Pod を移すが、実際には[終了処理](../lifecycle/)を通して安全に落とす必要がある
- **PodDisruptionBudget なし**: 実物は自発的な退避の下限を守る仕組みがある
- **待ち時間なし**: 実物は「使用率が低い状態が一定時間続いたら」縮小する。ここは即座に判断する
- **論理時刻**: 実時間でなく周期で数える

## 章

教科書: [Cluster Autoscaler](https://sharin-2a1.pages.dev/parts/cluster-autoscaler)

実行: `go test ./orchestration/clusterautoscaler/`
