# custommetrics — CPU 以外で伸ばす

[autoscaler](../autoscaler/) は CPU 使用率ひとつでレプリカ数を決めた。式は1行で、
難しいのは周り(揺れと遅れ)だった。だがもう1つ、式の外に問題がある。何で測るか、という問題になる。

CPU 使用率には上限がある。100% を超えられないので、100% に達した後は「どれだけ足りないか」を
教えてくれない。10 倍の負荷でも 100 倍の負荷でも 100% になる。式は現在値と目標の比を取るので、
目標が 50% なら、どれだけ遅れていても倍にしかならない。

待ち行列の長さには上限が無い。1000 件溜まっていれば 1000 と出る。1 個が 10 件持つのが適正なら、
必要なのは 100 個だと、その場で分かる。

## 3つの目標の与え方

| Kind | current の意味 | 式 |
|---|---|---|
| `Utilization` | 1 個あたりの使用率(%) | `ceil(replicas × current / target)` |
| `AverageValue` | 全体の合計 | `ceil(current / target)` |
| `Value` | 全体の値 | `ceil(current / target)` |

`Utilization` だけが現在のレプリカ数を掛ける。だから 0 個のときに計算できない。

## 目標は、どういう定常状態でいたいかの宣言になる

同じ負荷(到着 200 件/tick、1 個が 10 件/tick を捌く)に、2つの目標を当てた結果:

| | 追いつくまで | 滞留の山 | 落ち着き先 |
|---|---|---|---|
| CPU 使用率 50% | 8 tick | 320 件 | **40 個**・行列 0 |
| 待ち行列 10 件/個 | 6 tick | 200 件 | **20 個**・行列 200 |

どちらも指示どおりに動いている。CPU 50% は「半分空けておけ」なので、必要な数の倍を抱える。
1 個 10 件は「10 件並んでいてよい」なので、数は必要なぶんで止まる代わりに行列が残る。

## 0 にするには、値でなく有無を見る

```go
sc := custommetrics.NewScaler(custommetrics.Config{
    Metrics:    []custommetrics.Metric{{Name: "queue", Kind: custommetrics.AverageValue, Target: 10}},
    Min:        0,   // 0 を許す
    Max:        100,
    Activation: 0,   // これを超えたら 0 → 1
    Cooldown:   2,   // 仕事が無い状態がこれだけ続いたら 0 へ
})
sc.Decide(0, map[string]float64{"queue": 1})   // → 1 個起こす
sc.Decide(1, map[string]float64{"queue": 100}) // → 10 個(ここからは普通の式)
```

0 個のときは指標の値を計算できないので、仕事があるかどうかだけを見て 1 個へ起こす。
1 個以上になれば普通の式に戻る。この境目が KEDA と HPA の分担そのものになっている。

## 複数の指標は最大を採る

どれか1つでも足りていなければ足りていないので、いちばん多くを要求する指標に合わせる。
逆に言えば、指標を足すことは減る方向には働かない。1つ足すたびに下がりにくくなる。

## API

| 関数・メソッド | 役割 |
|---|---|
| `Desired(replicas, m, current) int` | 1つの指標から必要な数を出す |
| `DesiredAll(replicas, ms, readings) (int, string)` | 最大を採り、決め手になった指標名も返す |
| `NewScaler(cfg) *Scaler` / `Decide(replicas, readings) Decision` | 0 の扱いを含めた判断 |
| `NewSim(cfg, sc, start) *Sim` / `Tick()` / `Run(n)` | 待ち行列のある系を動かす |
| `(*Sim) StabilizedAt()` | 遅れが解消して以降ずっと追いついている最初の時刻 |
| `(*Sim) PeakBacklog()` / `Backlog()` / `Replicas()` | 観測 |

## 決定性

実時間も乱数も使わない。到着件数は `SimConfig.Arrivals` に台本として与える。
同じ台本なら必ず同じ結果になる。

## テスト

```
go test -race -cover ./orchestration/custommetrics/
```

カバレッジ 100%。
