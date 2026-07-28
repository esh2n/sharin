# statefulset — 「どの Pod も同じ」を捨てる

序数のついた安定した名前、作る順と消す順の保証、Pod と一対一で結びついたボリューム。データベースのクラスタのように、1台1台が別物であるワークロードを扱う。

## 肝

- **名前は序数から決まる**: `db-0` `db-1` `db-2`。作り直しても同じ序数には同じ名前が戻る。他のメンバーが設定に書いた宛先が生き続ける
- **作るのは 0 から、消すのは後ろから**: しかも一度に1つずつ。手前が ready になるまで次へ進まない
- **順序の代償**: 途中の1つが ready にならないと、それ以降は永久に作られない。並列に進むレプリカとは正反対
- **ボリュームは Pod より長生きする**: Pod を消しても残り、同じ序数の Pod が作り直されると再接続される。データが残るのはこの寿命の差による
- **縮小してもボリュームは消えない**: 意図された動きだが、使っていないボリュームの費用は払い続ける
- **決定性**: 序数順にしか動かないので、何度走らせても同じ

## 効果の固定(テスト)

- `TestStableNames`: 消して作り直しても同じ序数には同じ名前が戻る
- `TestCreatesInOrderOneAtATime`: 手前が ready になるまで次を作らない
- `TestScalesDownInReverseOrder`: 大きい序数から消える
- `TestBrokenOrdinalBlocksTheRest`: 途中が詰まると以降が止まり、直すと進む
- `TestVolumeOutlivesPod`: Pod を消してもデータが残り、作り直すと同じボリュームに繋がる
- `TestVolumeSurvivesScaleDown`: 縮小してもボリュームは残り、戻すと同じデータが繋がる
- `TestDeletePVCLosesData`: ボリュームを明示的に消したときに初めてデータが失われる

## 使い方

```go
s := statefulset.New(statefulset.Config{Name: "db", Replicas: 3, StartupTicks: 2})
s.Run(30)          // db-0 → db-1 → db-2 の順に立ち上がる

s.Write(1, "shard-1 のデータ")
s.DeletePod(1)     // Pod は消えるがボリュームは残る
s.Run(30)          // db-1 が作り直され、同じボリュームに繋がる
s.Read(1)          // "shard-1 のデータ"

s.SetBroken(1, true) // db-1 が ready にならない状況を作る
s.Scale(0)           // Pod は全部消えるが、ボリュームは残る
s.DeletePVC(1)       // ここで初めてデータが消える
```

## 簡略化したこと

- **更新なし**: 実物は序数の大きいほうから逆順に入れ替える。`partition` で段階的に進めることもできる
- **ネットワーク識別子なし**: 実物は headless Service で `db-0.db` という安定した名前が引ける
- **ボリュームは1つ**: 実物は `volumeClaimTemplates` で複数のボリュームを持てる
- **podManagementPolicy は OrderedReady のみ**: 実物は `Parallel` にして順序を捨てることもできる
- **論理時刻**: 実時間でなく周期で数える

## 章

教科書: [StatefulSetとPVC](https://sharin-2a1.pages.dev/parts/statefulset)

実行: `go test ./orchestration/statefulset/`
