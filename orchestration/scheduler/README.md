# scheduler — Pod をどのノードに置くか決める

調整ループが作った Pending Pod を、どのマシンに載せるか決める。filter(置けるか)と score(どこが良いか)の 2 段階。Kubernetes の kube-scheduler の核。

## 肝

- **可否と優劣を分ける**: filter は置けないノードを落とすだけ、score は残った候補に順位をつけるだけ。この分離があるから、配置方針を変えても「置けない場所に置く」ことは起こらない
- **要求(request)で予約する**: 実際の使用量でなく、Pod が申告した要求のぶんを確保する。申告が実態とずれると、詰めすぎか空けすぎになる
- **戦略は score だけを変える**: `Spread` は空きの大きいノードを、`BinPack` は埋まるノードを高く評価する。filter は共通なので、置ける場所は戦略によらない
- **落ちた理由を残す**: どのノードがなぜ落ちたかを `Verdict` で返す。Pod が Pending のままな理由を説明できる
- **決定性**: 同点はノード名の辞書順。何度走らせても同じ結果

## 効果の固定(テスト)

- `TestSpreadDistributes` / `TestBinPackConcentrates`: 同じ 3 Pod・同じ 3 ノードで、戦略だけを変えると均等配置と 1 台集中に分かれる
- `TestFilterTaintToleration`: 汚れの付いたノードには、許容する Pod しか置けない
- `TestUnschedulableStaysPending`: どこにも置けなければ配置せず、理由を残す
- `TestBindReservesRequest`: 配置すると要求のぶんだけ空きが減る
- `TestScheduleAllOrderMatters`: 先に大きい Pod を置くと、後の小さい Pod が入らなくなる

## 使い方

```go
ns := []*scheduler.Node{
    scheduler.NewNode("node-a", scheduler.Resources{CPU: 2000, Mem: 2048}),
    scheduler.NewNode("node-b", scheduler.Resources{CPU: 2000, Mem: 2048}),
    scheduler.NewNode("gpu-1", scheduler.Resources{CPU: 4000, Mem: 8192}).Taint("hardware", "gpu"),
}
s := scheduler.New(scheduler.Spread)          // BinPack にすると詰め込む
r := s.Schedule(scheduler.Pod{
    Name: "web-1",
    Req:  scheduler.Resources{CPU: 500, Mem: 512},
}, ns)
r.Scheduled()  // 配置できたか
r.Node         // 配置先
r.Verdicts     // 各ノードの可否と、落ちた理由
r.Scores       // 残った候補の点数(高い順)
```

## 簡略化したこと

- **predicates は 2 つだけ**: 実物は node affinity、port の衝突、volume の制約、topology spread など多数ある
- **preemption なし**: 実物は優先度の低い Pod を追い出して場所を空けることがある
- **1 つずつ処理**: 実物はキューを持ち、置けなかった Pod を再試行する
- **taint の effect なし**: 実物の taint は NoSchedule / PreferNoSchedule / NoExecute を持つ。ここは NoSchedule 相当のみ

## 章

教科書: [スケジューラ(Podの配置)](https://sharin-2a1.pages.dev/parts/pod-scheduler)

実行: `go test ./orchestration/scheduler/`
