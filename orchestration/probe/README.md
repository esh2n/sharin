# probe — ヘルスチェック(readiness / liveness / startup)

外から定期的に叩いて、Pod がリクエストを受けられるか、まだ生きているかを確かめる。仕組みは同じで、失敗したときの扱いだけが違う。

## 肝

- **検査は同じ、扱いが違う**: readiness も liveness も同じ `Probe` 型。落ちたとき、readiness は転送先から外すだけ、liveness は再起動する
- **readiness は取り返しがつく**: 外れている間に回復すれば、自分で戻る。Pod は生き続ける
- **liveness は取り返しがつかない**: 再起動すると経過も検査の状態も初期化される。遅いだけの Pod に使うと殺し続ける
- **連続回数がたまたまを吸収する**: 1 回の失敗で判定を変えない。瞬断で健全な Pod を外したり再起動したりしないため
- **起動用の検査は分離のための道具**: これが通るまで他の2つは動かない。起動が遅いことと、動いてから壊れたことを区別する
- **決定性**: 中身の健康状態は `Behavior` の台本で時刻から決まる。乱数なし

## 効果の固定(テスト)

- `TestNoReadinessDropsDuringStartup` / `TestReadinessGatesTraffic`: 温まった Pod と起動中の Pod が並ぶとき、readiness の有無で 3 件落ちるか 0 件かが変わる
- `TestReadinessDoesNotRestart`: readiness が落ちても再起動せず、回復すれば自分で戻る
- `TestAggressiveLivenessRestartLoop`: 一時的に詰まっただけの Pod を、厳しい liveness は殺し続け、緩い liveness は自力回復させる
- `TestSlowStartupKilledByLiveness`: 起動が liveness の猶予より遅いと、立ち上がる前に殺され続けて永久に起動しない
- `TestStartupProbeProtectsSlowStart`: 起動用の検査を足すと、同じ遅い起動でも殺されない
- `TestThresholdAbsorbsSingleFailure`: 1 回の失敗では判定が変わらない

## 使い方

```go
s := probe.New(probe.Config{
    Readiness: probe.Probe{Period: 1, FailureThreshold: 1, SuccessThreshold: 1},
    Liveness:  probe.Probe{Period: 1, FailureThreshold: 5, SuccessThreshold: 1},
    Startup:   probe.Probe{Period: 1, FailureThreshold: 30, SuccessThreshold: 1},
},
    probe.Behavior{},                                    // 最初から応答できる
    probe.Behavior{StartupTicks: 10, HangAt: 20, HangFor: 3}, // 起動が遅く、後で一時的に詰まる
)
for i := 0; i < 30; i++ {
    s.Tick(2) // 1 周期に 2 件のリクエスト
}
s.Safe()      // 1 件も落とさなかったか
s.Restarts()  // 再起動の合計
s.Log         // 何が起きたかの記録
```

## 簡略化したこと

- **検査の中身なし**: HTTP GET / TCP / exec の区別は扱わない。応答できるかの真偽だけ
- **タイムアウトなし**: 実物は検査自体にタイムアウトがあり、その満了も失敗として数える
- **1 コンテナ**: 実物は Pod 内の全コンテナが ready でないと Pod は ready にならない
- **再起動の間隔なし**: 実物は再起動を繰り返すと指数バックオフで間隔が伸びる(CrashLoopBackOff)
- **論理時刻**: 実時間でなく tick で数える

## 章

教科書: [ヘルスチェック(probe)](https://sharin-2a1.pages.dev/parts/probe)

実行: `go test ./orchestration/probe/`
