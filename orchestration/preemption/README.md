# preemption — PriorityClass と場所の奪い方

置けるノードが無ければ Pod は Pending のまま残る。だがそれで困る Pod がある。
本番のアプリと夜間のバッチが同じクラスタに載っているとき、空きが無いからといって
本番を待たせるわけにはいかない。バッチを退かせばよい。

優先度は2つの意味を持つ。1つは待ち行列の順序、もう1つは場所を奪う権利になる。
このパッケージは後者を実装し、誰を何個追い出すかの決め方を見せる。

## 使い方

```go
c := preemption.New(
    []preemption.NodeSpec{
        {Name: "node-a", Cap: scheduler.Resources{CPU: 1000, Mem: 1000}},
        {Name: "node-b", Cap: scheduler.Resources{CPU: 1000, Mem: 1000}},
    },
    map[string]int{"log": 1}, // App ごとの最低稼働数
)
c.Place(preemption.Pod{Name: "batch", App: "batch", Priority: 50, Req: r(800)}, "node-a")
c.Place(preemption.Pod{Name: "log", App: "log", Priority: 10, Req: r(800)}, "node-b")
c.Submit(preemption.Pod{Name: "api", App: "api", Priority: 100, Req: r(900)})
c.Run()

for _, e := range c.Evictions {
    fmt.Println(e.By, "→", e.Victim, e.Violates)
}
// api → batch false
// batch → log true
```

保護のある `log` を避けて、優先度の高い `batch` のほうが犠牲になる。
追い出された `batch` は行列に戻り、行き場を求めて `log` を追い出す。
そのときは他に選択肢が無いので、保護を破って進む。

## 犠牲の選び方

ノード1台の中では、こう決める:

1. 自分より優先度の低い Pod を全部外す。それでも入らないなら、このノードは候補にしない
2. 入るなら、外したものを優先度の高い順に戻す。戻しても入るなら、その Pod は外さなくてよかった

低いものから順に外していく素朴なやり方だと、外さなくてよいものまで外してしまう。

候補ノードが複数あるときは、この順で選ぶ:

1. 保護を破る数が少ない
2. 犠牲の優先度がなるべく低い
3. 犠牲の数が少ない
4. ノード名順(同点を決定的にするため)

## API

| 関数・メソッド | 役割 |
|---|---|
| `New(specs, budgets) *Cluster` | ノードと保護の設定からクラスタを作る |
| `(*Cluster) Place(p, node)` | すでに動いている Pod を直接載せる |
| `(*Cluster) Submit(p)` | Pod を待ち行列に入れる |
| `(*Cluster) Run()` | 行列が空になるまで、優先度順に置く・奪う |
| `(*Cluster) Placement()` | Pod 名 → ノード名 |
| `(*Cluster) Pending()` | どこにも置けなかった Pod 名 |
| `(*Cluster) Evictions` | 追い出しの記録(誰が・誰を・どこから・保護を破ったか) |

## 決定性

実時間も乱数も使わない。行列は優先度と投入順で全順序が決まり、同点はすべて名前順で
決める。`Run` の周回数には上限があり、玉突きが循環しても必ず止まる。

[`scheduler`](../scheduler/) パッケージの `Filter` を、奪わずに置けるかの判定に
そのまま使っている。実物でも preemption は通常のスケジューリングが失敗した後に走るので、
同じ判定を通ってから来ることになる。

## テスト

```
go test -race -cover ./orchestration/preemption/
```

カバレッジ 100%。
