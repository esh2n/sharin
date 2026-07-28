# operator — 調整ループを自分の型へ広げる

この編で作ってきた仕組みは、どれも「宣言された状態へ寄せる」の変奏だった。Operator パターンは、その形を Kubernetes の資源でなく自分のドメインへ広げる。運用の手順書が、状態の宣言と差を埋めるループに置き換わる。

## 肝

- **型を1つ足すだけ**: Spec(あるべき姿)と Status(今の姿)を持つ型を宣言し、差を埋めるループを書く。それだけで[調整ループ](../reconcile/)と同じ性質が手に入る
- **手順を状態に書き換える**: 「復元して、繋いで、選び直す」という順序のある手順は、「まだ済んでいない差」の判定として書ける
- **1回に1手だけ**: 全部を一気にやらない。1手打って状態を書き戻せば、途中で落ちても次は現状から再開できる
- **冪等**: 揃った後は何度呼んでも何もしない。だから安心して回し続けられる
- **自己修復が付いてくる**: リーダーが落ちても、イベントを購読していないのに次の調整で選び直す
- **決定性**: リーダーは名前順で選ぶ。乱数なし

## 効果の固定(テスト)

- `TestRestoreHappensBeforeMembers`: 復元が済むまでメンバーを作らない。順序が差の判定で表現されている
- `TestRestoreOnlyOnce`: 復元は一度きり。何度呼んでも繰り返さない
- `TestMissingBackupIsDegraded`: 差を埋められないことを状態で示し、先へ進まない
- `TestReelectsWhenLeaderDies`: リーダーが落ちると選び直し、落ちた分も作り直す
- `TestSelfHealsAfterMultipleLosses`: 全滅からも戻る
- `TestOneStepPerCall`: 1回の呼び出しで1手しか打たない
- `TestIdempotentOnceReady`: 揃った後は何もしない

## 使い方

```go
w := operator.NewWorld()
w.PutBackup("backup-2026-07-28")

r := &operator.Resource{Spec: operator.Spec{
    Name:        "db",
    Members:     3,
    RestoreFrom: "backup-2026-07-28",
}}

o := operator.New()
o.Run(r, w, 30)

r.Status.Phase    // Ready
r.Status.Leader   // "db-1"
r.Status.Restored // true

w.Kill(r.Status.Leader) // リーダーが落ちる
o.Run(r, w, 30)         // 同じループが選び直し、作り直す
```

## 簡略化したこと

- **CRD の登録なし**: 実物は API サーバに型を登録し、`kubectl` から扱えるようになる。ここは Go の型のみ
- **watch なし**: 実物は informer が変更を検知して reconcile を呼ぶ。ここは手で呼ぶ
- **エラーと再試行なし**: 実物は失敗を返すとバックオフして再度キューに入る
- **所有関係なし**: 実物は作った部品に所有者を記録し、親が消えると連鎖して消える
- **1つのリソースのみ**: 実物は同じ型の複数のインスタンスを並行に調整する

## 章

教科書: [Operatorパターン](https://sharin-2a1.pages.dev/parts/operator)

実行: `go test ./orchestration/operator/`
