# reconcile — 調整ループ(Kubernetes の心臓)

宣言した desired state と、実際にある observed state を毎回まるごと見比べて収束させる。作成・スケール・障害回復を 1 つのループが等しく処理する。Kubernetes のコントローラの核。

## 肝

- **宣言的**: 「Pod を 3 個に保て」という状態を宣言する。手順(どう 3 個にするか)は書かない
- **level-triggered**: イベント(Pod が死んだ)に反応せず、毎回まるごと現状を数え直して差を埋める。取りこぼしやコントローラ再起動に強い
- **冪等**: 目標に達していれば何度 Reconcile しても何も起こさない。収束先は常に同じ
- **1 ループが全部やる**: 作成(不足を埋める)・スケール(目標変更)・自己修復(Failed を作り直す)が、同じ「差を数えて埋める」処理で片づく
- **決定性**: Pod 名は連番、Pod 一覧は名前順。テストが再現的

## 効果の固定(テスト)

- `TestSelfHealing`: Pod を落とすと次の Reconcile が作り直す(障害イベントを購読していないのに直る)
- `TestLevelTriggered`: 3 つまとめて落ちても、1 回の Reconcile が数え直して全部復旧
- `TestIdempotent`: 収束後の Reconcile は no-op
- `TestScaleUpDown`: 目標数を変えるだけで過不足を埋める

## 使い方

```go
c := reconcile.New(3)       // 目標 3 レプリカ
cl := reconcile.NewCluster()
c.Reconcile(cl)             // 不足を作る
cl.StartPending()           // 起動
cl.Fail("pod-2")            // 障害
c.Reconcile(cl)             // 気づいて作り直す(自己修復)
c.SetDesired(5)             // 宣言的スケール
c.Reconcile(cl)
```

## 簡略化したこと

- **ReplicaSet 相当のみ**: Deployment のローリング更新や revision 管理は扱わない
- **単一コントローラ**: 実物は多数のコントローラが並行に走り、共有 API サーバの状態を watch する
- **watch でなく明示 Reconcile**: 実物は informer が変更を検知して reconcile を呼ぶ。ここは手で呼ぶ
- **スケジューリングなし**: Pod をどのノードに置くかは別章(scheduler)

## 章

教科書: [調整ループ(reconciliation)](https://sharin-2a1.pages.dev/parts/reconcile)

実行: `go test ./orchestration/reconcile/`
