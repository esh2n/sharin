# pdb — アプリの側から下限を宣言する

更新も集約もノードの入れ替えも、それぞれ自分の都合しか見ていない。同時に起これば全部消える。だからアプリの側から「常に2つ以上」と宣言し、消す側に許可を求めさせる。

## 肝

- **消す側でなくアプリが宣言する**: 「一度に何個まで止めてよいか」でなく「常に何個残っていること」を書く。消す仕組みが増えても宣言は変えなくてよい
- **守れるのは自発的な退避だけ**: ノードが落ちる分は止められない。止められるものと止められないものを、はっきり分けている
- **ready でない Pod は頭数に入らない**: 作り直し中のものは数えない。だから前の退避が立ち上がるまで、次は断られる。この待ちが実質的な速度制限になる
- **厳しすぎると運用が止まる**: 下限とレプリカ数が同じだと、1つも退避できない。更新も集約もノードの入れ替えも全部止まる
- **drain は退避の連続**: 断られたぶんはノードに残る。空にするには何度も呼ぶ

## 効果の固定(テスト)

- `TestEvictDeniedWhenAtFloor`: 下限を割る退避は断られ、代わりが立ち上がると通る
- `TestFloorEqualToReplicasBlocksEverything`: 下限 = レプリカ数だと1つも動かせない
- `TestCrashIgnoresBudget`: 落ちる分は宣言に関係なく消える。下限 3 を宣言していても 1 個まで減る
- `TestDrainProgressesOverTime`: drain は一度に終わらず、立ち上がりを待って進む
- `TestEvictingUnreadyPodIsAllowed`: ready でない Pod は消しても頭数が減らないので通る
- `TestBudgetOnlyAppliesToMatchingPods`: 条件に合わない Pod は縛られない

## 使い方

```go
c := pdb.New(pdb.Config{StartupTicks: 3, ReplaceNode: "node-c"})
for i := 1; i <= 3; i++ {
    c.AddPod("web-"+itoa(i), "node-a", map[string]string{"app": "web"}, true)
}
c.AddBudget("web-pdb", 2, map[string]string{"app": "web"}) // 常に2つ以上

c.Evict("web-1")        // 通る(残り2つ)
c.Evict("web-2")        // 断られる(下限を割る)
for i := 0; i < 3; i++ { c.Tick() } // 代わりが立ち上がる
c.Evict("web-2")        // 通る

c.Crash("web-3")        // 宣言に関係なく消える
c.Drain("node-a")       // 退避できたぶんだけ進む
```

## 簡略化したこと

- **minAvailable のみ**: 実物は `maxUnavailable` でも書ける。割合(`50%`)での指定も扱わない
- **退避の作法なし**: 実際の退避は[終了処理](../lifecycle/)を通る。ここでは即座に消える
- **1つの宣言だけを見る**: 実物も同じだが、複数の宣言が同じ Pod にかかる場合の扱いは簡略化している
- **作り直しは固定のノードへ**: 本来は[スケジューラ](../scheduler/)が置き場所を決める
- **論理時刻**: 実時間でなく周期で数える

## 章

教科書: [PodDisruptionBudget](https://sharin-2a1.pages.dev/parts/pdb)

実行: `go test ./orchestration/pdb/`
