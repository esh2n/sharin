# quota — 合計に上限を置く

スケジューラは1つずつ「置けるか」を見た。1つ1つが正しくても、合計が問題になることがある。ResourceQuota は区画ごとの総量に上限を置き、LimitRange は書き忘れが総量の勘定を素通りしないようにする。

## 肝

- **判断の単位が総量になる**: 個々の要求でなく、区画の合計を見る。1つずつ正しくても合計で止まる
- **既定値を入れてから数える**: 順序が逆だと、要求を書き忘れた Pod が 0 として通り、上限が意味を失う
- **1つあたりの上限は別に効く**: 総量に余裕があっても、大きすぎる要求は通さない
- **断った分は数えない**: 拒否したものが総量を消費しない
- **消せば戻る**: 使っていた分が返り、また入れられる
- **決定性**: Pod は名前順。乱数も時計も使わない

## 効果の固定(テスト)

- `TestTotalIsWhatMatters`: 上限に達すると、どれだけ小さくても入らない
- `TestDefaultsAreCountedInTotal`: 既定値が無いと、書き忘れが 0 として通り続ける
- `TestPerPodMaxAppliesEvenWithRoom`: 総量に余裕があっても1つあたりの上限は効く
- `TestDefaultThenCheck`: 既定値を入れた後で上限に照らす
- `TestRejectionDoesNotConsume`: 断った分は総量に数えられない
- `TestRemovingFreesQuota`: 消せば分が戻り、また入る

## 使い方

```go
n := quota.New("team-a",
    quota.ResourceQuota{Hard: quota.Resources{CPU: 2000, Mem: 2048}, MaxPods: 6},
    quota.LimitRange{
        Default: quota.Resources{CPU: 200, Mem: 256},  // 書いていなければこれ
        Max:     quota.Resources{CPU: 1000, Mem: 1024}, // 1つあたりの上限
    })

n.Admit("web-1", quota.Resources{CPU: 400, Mem: 400}) // 受け入れ
n.Admit("no-req", quota.Resources{})                  // 既定値が入って 200m で数える
n.Admit("huge", quota.Resources{CPU: 1500, Mem: 1500}) // 1つあたりの上限で拒否

n.Used()  // 合計
n.Free()  // 残り
n.Remove("web-1") // 消すと分が戻る
```

## 簡略化したこと

- **資源は CPU とメモリのみ**: 実物はストレージや、オブジェクトの個数(Service や Secret の数)にも上限を置ける
- **requests と limits を分けない**: 実物は両方に別々の上限を置ける
- **scope なし**: 優先度クラスごとに別の上限を置く仕組みは扱わない
- **Min なし**: 実物の LimitRange は下限も指定できる
- **実装は判定のみ**: 実際の適用は admission の段で行われる

## 章

教科書: [ResourceQuotaとLimitRange](https://sharin-2a1.pages.dev/parts/quota)

実行: `go test ./orchestration/quota/`
