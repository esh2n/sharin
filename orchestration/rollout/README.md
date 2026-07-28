# rollout — 止めずに入れ替える(ローリング更新)

動いているものを、止めずに新しい版へ入れ替える。何個多く作ってよいか(maxSurge)と、何個まで減ってよいか(maxUnavailable)の2つの幅が、入れ替えの速さと保たれる容量を決める。

## 肝

- **2つの幅がすべてを決める**: maxSurge は速さ、maxUnavailable は容量の下限。広げれば速く、狭めれば安全
- **進行の条件は readiness**: 新しい版が ready にならなければ古い版は消えない。壊れた版をデプロイしても途中で止まり、全滅しない
- **maxUnavailable が被害の上限**: 壊れた版を出したとき、止まるまでにどこまで容量が落ちるかを、この値だけが決める
- **ready でないものは常に消せる**: 消しても ready な数は減らないため。これが無いと壊れた版から戻れなくなる
- **ロールバックは特別な操作ではない**: 目標をもう一度書き換えるだけ。同じ仕組みが逆向きに走る
- **どちらも 0 なら進めない**: 作ることも消すこともできない。`Deadlocked` で検出できる

## 効果の固定(テスト)

- `TestMaxUnavailableZeroKeepsCapacity`: maxUnavailable=0 なら入れ替え中も容量が目標を割らない
- `TestMaxSurgeZeroReducesCapacity`: maxSurge=0 なら先に消すしかなく、maxUnavailable のぶん容量が落ちる
- `TestWiderWindowFinishesSooner`: 幅を広げるほど速く終わり、そのぶん容量は落ちる
- `TestBrokenReleaseStallsInsteadOfWiping`: 壊れた版では古い版が消えず、容量が保たれたまま止まる
- `TestMaxUnavailableBoundsTheDamage`: 同じ壊れた版でも、maxUnavailable の値がそのまま被害の深さになる
- `TestRollbackUsesTheSameMechanism`: 止まった後で元の版を宣言し直すと、同じ仕組みで戻る

## 使い方

```go
r := rollout.New(rollout.Config{
    Replicas:       4,
    MaxSurge:       1, // 一時的に 5 個まで持ってよい
    MaxUnavailable: 0, // ready な数は 4 を割らない
}, rollout.Release{Version: 1, StartupTicks: 2})

r.Deploy(rollout.Release{Version: 2, StartupTicks: 2}) // 目標を書き換えるだけ
r.Run(50)

r.Done()             // 全部が新しい版になったか
r.Stalled()          // 進めない状態で止まっているか
r.MinAvailableSeen   // 入れ替え中に観測した ready 数の最小値
r.History            // 1周期ごとの内訳
```

## 簡略化したこと

- **ReplicaSet を作らない**: 実物は版ごとに ReplicaSet を作り、その replicas を上下させて入れ替える。ここは Pod を直接扱う
- **版は2つまで**: 入れ替え中にさらに別の版をデプロイする場合の扱いは簡略化している
- **終了処理なし**: 消す側の preStop や猶予期間は[別章](https://sharin-2a1.pages.dev/parts/pod-lifecycle)。ここでは即座に消える
- **progressDeadline なし**: 実物は一定時間進まないと失敗として記録する。ここは止まったままになる
- **論理時刻**: 実時間でなく周期で数える

## 章

教科書: [ローリング更新](https://sharin-2a1.pages.dev/parts/rollout)

実行: `go test ./orchestration/rollout/`
