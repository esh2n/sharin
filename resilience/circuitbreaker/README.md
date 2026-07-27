# circuitbreaker — サーキットブレーカー

落ちている依存先への呼び出しを遮断して連鎖障害を止めるサーキットブレーカーを最小構成で実装する。

## 肝

- **3状態の状態機械**: closed(通す・失敗を数える)/ open(即失敗 = fail fast)/ half-open(1本だけ試す)
- **連続失敗で開く**: closed で FailureThreshold 回連続失敗したら open。成功で連続失敗カウントをリセット
- **タイムアウトで半開**: open から OpenTimeout 経過で half-open。1本だけ通して回復を確かめる
- **半開の判定**: 連続 SuccessThreshold 成功で close、失敗したら即 open に戻る。試行は同時1本だけ(テストで固定)
- **決定性**: 論理時計(Advance で進める)。実時計を使わないのでテストが決定的

## 使い方

```go
b := circuitbreaker.New(circuitbreaker.Config{
    FailureThreshold: 5, SuccessThreshold: 2, OpenTimeout: 30,
})
err := b.Call(func() error { return callDependency() })
// open なら fn を呼ばず ErrOpen を即返す(fail fast)
b.Advance(30) // 論理時計を進める(実物は実時計)
```

## 簡略化したこと

- **連続失敗のみ**: 実物は失敗率(直近ウィンドウの割合)や slow call 率でも開く。ここは連続カウント
- **論理時計**: 実物は実時計。決定性のため Advance で進める形にした
- **並行制御は最小**: 単一 goroutine 前提。実物は原子カウンタやロックで並行安全にする
- **バルクヘッド・タイムアウトは別**: 同時実行数の隔離や呼び出しタイムアウトは扱わない(組み合わせて使う)

## 章

教科書: [サーキットブレーカー](https://sharin-2a1.pages.dev/parts/circuit-breaker)

実行: `go test ./resilience/circuitbreaker/`
