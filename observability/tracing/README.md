# tracing — 分散トレーシングの中核

1 リクエストが複数サービスを渡り歩くとき、どこで時間を使ったかを 1 本の木で見えるようにする。span・trace_id・親子関係・境界を越える伝播を最小構成で実装する。

## 肝

- **span**: 1 サービスでの 1 区間の処理。trace_id・span_id・親 span_id・名前・開始/終了を持つ
- **trace_id の共有**: 1 リクエストの全 span が同じ trace_id を持つ。これで散らばった span を 1 つに束ねる
- **親子関係**: 子 span は親の span_id を ParentID に記録。呼び出しの木構造を再現できる
- **伝播(Inject/Extract)**: サービス境界をヘッダ("traceid-spanid")で越える。別プロセスの span も同じトレースに繋がる(W3C Trace Context の traceparent の最小形)
- **クリティカルパス**: 全体の所要時間を決めているのは、各段で最も遅く終わる子の連なり。ここを速くしないと全体は縮まない
- **決定性**: ID は注入した生成器(NewIDGen)、時刻は論理時計(Advance)。実乱数・実時計を使わずテストが再現的

## 効果の固定(テスト)

- `TestCriticalPath`: gateway→handler→billing が全体時間を決めていることを特定
- `TestPropagationStitchesServices`: Inject/Extract を挟んでも、別プロセスの span が同じ trace_id と正しい親に繋がる

## 使い方

```go
t := tracing.New(tracing.NewIDGen(seed))
root := t.StartRoot("gateway")
child := t.Start(root, "auth"); t.End(child)
header := tracing.Inject(root)     // 境界を越えて送る
ctx, _ := tracing.Extract(header)  // 別サービスで復元
// ...
root2 := tracing.BuildTree(t.Spans())
path := tracing.CriticalPath(root2) // 全体時間を決める鎖
```

## 簡略化したこと

- **論理時計**: 実時刻でなく Advance で進める整数。決定性のため
- **属性・イベントなし**: span に付ける tag / log は持たない
- **サンプリングは別章**: どのトレースを保存するかは [trace-sampling](/parts/trace-sampling)
- **ヘッダは最小形**: 実物は W3C traceparent(version-traceid-spanid-flags)

## 章

教科書: [分散トレーシング](https://sharin-2a1.pages.dev/parts/tracing)

実行: `go test ./observability/tracing/`
