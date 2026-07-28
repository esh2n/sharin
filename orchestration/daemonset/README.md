# daemonset — 数でなく場所を宣言する

調整ループは「3個であれ」と数を宣言し、スケジューラが場所を選んだ。DaemonSet は逆で、場所を宣言し、数はそこから導かれる。

## 肝

- **数を宣言しない**: 宣言するのは「どこに要るか」。必要数は対象ノードの数から導かれる
- **ノードの増減に自動で追随する**: 増えれば置かれ、減れば消える。数を書き換える必要がない
- **集合の差を埋める**: 調整ループが数を比べたのに対し、こちらは対象ノードの集合と Pod の載っているノードの集合を比べる
- **スケジューラの出番がない**: どこに置くかを選ぶ余地が無い。そのノードに置かなければ意味がないから
- **汚れの扱いが逆**: 監視や収集は、他の Pod が避けるノードにも置かれてほしい。だから広く許容する
- **決定性**: ノードも Pod も名前順

## 効果の固定(テスト)

- `TestOnePerNode`: 対象すべてに1つずつ置かれる
- `TestFollowsNodeAdditions` / `TestFollowsNodeRemovals`: ノードの増減に自動で追随する
- `TestUnreadyNodeLeavesTargets`: ready でなくなれば対象から外れ、戻れば置き直される
- `TestSelectorNarrowsTargets` / `TestLabelChangeMovesTargets`: ラベルで対象が決まり、変えれば追随する
- `TestTaintsAndTolerations`: 許容していなければ汚れたノードには置かれない
- `TestConvergesAfterBulkChange`: まとめて増減しても1回の調整で追いつく

## 使い方

```go
s := daemonset.New(daemonset.Spec{
    Name:        "log-agent",
    Selector:    map[string]string{},      // すべてのノード
    Tolerations: []string{"*"},            // どんな汚れも許容する
})
s.AddNode("node-1", true, nil)
s.AddNode("node-2", true, nil)
s.AddNode("infra-1", true, nil, "dedicated=infra")

s.Desired()   // 3(対象ノードの数がそのまま必要数)
s.Reconcile() // 3台すべてに1つずつ作る

s.AddNode("node-3", true, nil)
s.Reconcile() // node-3 にだけ作る。数はどこにも書いていない
```

## 簡略化したこと

- **更新なし**: 実物は DaemonSet の更新もローリングで行える
- **資源の確認なし**: 実物は置く前に空き容量を見る。足りなければ Pending になる
- **優先度なし**: 実物の DaemonSet Pod は高い優先度を持ち、他を追い出せる
- **taint の effect なし**: NoSchedule / NoExecute の区別は扱わない
- **1つの集合のみ**: 複数の DaemonSet が同じノードに同居する場合は扱わない

## 章

教科書: [DaemonSet](https://sharin-2a1.pages.dev/parts/daemonset)

実行: `go test ./orchestration/daemonset/`
