# retry — リトライと指数バックオフ

一時的な失敗を、待ち時間を指数的に伸ばしながら再送するリトライを最小構成で実装する。

## 肝

- **指数バックオフ**: 待ち時間を `base·mult^attempt` で伸ばし、max で頭打ち。過負荷の相手に追い打ちをかけない
- **ジッター**: 多数のクライアントが同時に失敗すると全員同じ間隔で再送して衝突(リトライ嵐)。待ちに乱数の揺らぎを足して散らす。none / full([0,raw]) / equal(half+[0,half])
- **恒久的失敗は再送しない**: `Permanent` でラップしたエラー(400 番台など)は即諦める。待っても直らないものを再送しない
- **試行の間だけ待つ**: 最後の失敗後は待たない。TotalDelay で待ち合計を確認
- **決定性**: ジッターは注入した擬似乱数源(NewRand)。実 rand を使わずテストが再現的

## 使い方

```go
p := retry.Policy{MaxAttempts: 5, BaseDelay: 100, MaxDelay: 10000, Multiplier: 2, Jitter: retry.JitterFull}
res := p.Do(func() error { return call() }, retry.NewRand(seed))
// res.Attempts, res.TotalDelay, res.Err
return retry.Permanent(err) // これは再送されない
```

## 簡略化したこと

- **論理時間**: 実際の sleep でなく待ち時間を数えるだけ。決定性のため
- **リトライ判定は Permanent マークのみ**: 実物はステータスコードやエラー型で細かく分岐
- **予算(retry budget)なし**: 全体のリトライ率に上限をかける仕組みは扱わない
- **サーキットブレーカーとの統合は別**: 組み合わせて使う([circuit-breaker](/parts/circuit-breaker))

## 章

教科書: [リトライとバックオフ](https://sharin-2a1.pages.dev/parts/retry-backoff)

実行: `go test ./resilience/retry/`
