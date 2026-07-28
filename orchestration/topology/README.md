# topology — 何に対して散らすのか

スケジューラの章では散らすか詰めるかを選んだ。だが何に対して散らすのかを決めていなかった。ノードに均等でも、同じ区画に偏っていれば、その区画が落ちたとき全部失う。

## 肝

- **数えるのは区画ごと**: ノードごとではない。ノードに均等でも区画に偏っていれば偏り(skew)は大きい
- **偏りの許容量を宣言する**: 最も多い区画と最も少ない区画の差の上限。この幅が散らばりの強さになる
- **置いた後を見る**: 今の状態でなく、置いた後どうなるかで候補を絞る
- **filter の側に立つ**: 願いでなく制約。守れないなら置かない、という選択ができる
- **守れないときの振る舞いを選べる**: 置かない(可用性優先)か、置く(配置優先)か
- **決定性**: 同数の区画は名前順、区画内は載っている数の少ないノード順

## 効果の固定(テスト)

- `TestSpreadsAcrossZones`: 制約が厳しいと区画に均等に散る
- `TestSkewCountsZonesNotNodes`: ノードごとに均等でも、区画では偏っている
- `TestSurvivesZoneLoss`: 散らせば1区画を失っても大半が残り、寄せていれば全部失う
- `TestNodeLossDiffersFromZoneLoss`: 1台の障害と1区画の障害は被害が違う
- `TestDoNotScheduleRefuses` / `TestScheduleAnywayPlacesAnyway`: 守れないときの2つの振る舞い

## 使い方

```go
c := topology.New(topology.Constraint{
    MaxSkew: 1,
    When:    topology.DoNotSchedule, // 守れないなら置かない
})
for _, z := range []string{"zone-a", "zone-b", "zone-c"} {
    c.AddNode(z+"-1", z)
    c.AddNode(z+"-2", z)
}
c.PlaceN("web", 6)

c.Skew()           // 0(区画ごとに均等)
c.WorstZoneLoss()  // 4(1区画を失っても4個残る)
c.LoseNode("zone-a-1") // 5(1台なら5個残る)
```

## 簡略化したこと

- **区画は1段のみ**: 実物はノード・ラック・ゾーン・地域を同時に指定できる
- **セレクタなし**: 実物はラベルで対象の Pod を絞る。ここでは全 Pod が対象
- **minDomains なし**: 何区画以上に散らすかの下限は扱わない
- **nodeAffinity との併用なし**: 置けるノードの絞り込みは[スケジューラ](../scheduler/)の担当として分けている
- **再配置なし**: すでに置いた Pod を偏りのために動かすことはしない

## 章

教科書: [topology spread(区画をまたいで散らす)](https://sharin-2a1.pages.dev/parts/topology)

実行: `go test ./orchestration/topology/`
