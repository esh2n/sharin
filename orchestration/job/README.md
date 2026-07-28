# job — 終わるワークロード

この編で扱ってきたものは、どれも動き続けるものだった。Job は終わる。終わることが正常な結末なので、数え方も失敗の扱いも逆になる。

## 肝

- **成功した数を数える**: ready な数ではない。目標に達したら終わりで、目標を保ち続けるのではない
- **量と速さは別の軸**: `Completions` が仕事の量、`Parallelism` が同時に走らせる数。片方だけ変えても意味が変わらない
- **上限を決めないと永久に再試行する**: `BackoffLimit` は諦める点。決めておかないと、直らない失敗を試し続ける
- **要る数より多くは走らせない**: 残り1個なら、同時実行の設定が5でも1個しか起動しない
- **周期実行には固有の問題がある**: 前の実行が終わらないまま次の時刻が来たとき、重ねるか、飛ばすか、置き換えるか
- **決定性**: 失敗は台本(`FailFirst`)で決める。乱数なし

## 効果の固定(テスト)

- `TestCompletesWhenEnoughSucceed`: 必要な数だけ成功したら終わる
- `TestParallelismShortensTheRun`: 同時実行を増やすと同じ仕事量が短く終わる
- `TestDoesNotOverrunCompletions`: 残り分しか起動しない
- `TestRetriesWithinBackoffLimit` / `TestGivesUpAfterBackoffLimit`: 上限の内なら再試行し、超えたら諦める
- `TestConcurrencyPolicies`: Allow は重ね、Forbid は飛ばし、Replace は置き換える
- `TestForbidSkipsRuns`: Forbid では実行そのものが起きない回がある

## 使い方

```go
j := job.New(job.Config{
    Completions:  6, // 6 個成功したら終わり
    Parallelism:  2, // 同時に 2 個まで
    BackoffLimit: 3, // 失敗 3 回までは再試行
    FailFirst:    2, // 最初の 2 回は失敗する(台本)
})
j.Run(30)
j.Phase      // Complete / Failed
j.Succeeded  // 6
j.Failed     // 2

c := job.NewCron(job.CronConfig{
    Every:  3,
    Policy: job.Forbid, // 前の実行が残っていたら飛ばす
    Job:    job.Config{Completions: 5, Parallelism: 1},
})
for i := 0; i < 12; i++ { c.Tick() }
c.Started / c.Skipped / c.Replaced
```

## 簡略化したこと

- **Pod を持たない**: 実物は Job が Pod を作る。ここでは走っている数だけを数える
- **バックオフの間隔なし**: 実物は再試行のたびに待ち時間が延びる
- **completionMode は既定のみ**: 序数つきの `Indexed` は扱わない
- **時刻はカレンダーでない**: 実物は cron 式で書く。ここは何周期ごとという単純な形
- **後片付けなし**: 実物は `ttlSecondsAfterFinished` で終わった Job を消せる

## 章

教科書: [JobとCronJob](https://sharin-2a1.pages.dev/parts/job)

実行: `go test ./orchestration/job/`
