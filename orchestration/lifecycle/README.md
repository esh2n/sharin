# lifecycle — Pod の終了でリクエストを落とさない

Pod を止めるとき、トラフィックの切り離しとプロセスの停止は別々に走る。順序が逆になると、まだ振られてくるのに受けられない Pod ができる。preStop と猶予期間でそれを塞ぐ。

## 肝

- **切り離しと停止は並行**: 削除を決めると、転送先一覧からの除去と SIGTERM が同時に動き出す。どちらが先に効くかは保証されない
- **転送先一覧は現実に遅れる**: 制御側が管理する一覧は、Pod が実際に死んだことを知らない。この隙間に振られたリクエストが落ちる
- **preStop は伝播を待つための時間**: SIGTERM の前に、切り離しが伝わりきるまで待つ。`PreStop >= Propagation` なら 1 件も落ちない
- **SIGTERM は新規だけを止める**: 処理中のリクエストは続く。止めるのは受け付けだけ
- **猶予が足りないと道連れ**: SIGKILL は処理中を問答無用で切る。`Grace >= Work` が要る
- **決定性**: 論理時刻と round-robin で回すので、同じ設定なら結果は毎回同じ

## 効果の固定(テスト)

- `TestNoPreStopDropsRequests`: preStop なしだとリクエストが落ちる
- `TestPreStopCoversPropagation`: 同じ設定でも preStop が伝播を覆えば 0 件になる
- `TestShortPreStopStillDrops`: preStop が短いほど落ちる件数が増える(対策の効きが連続的)
- `TestShortGraceKillsInflight` / `TestGraceCoversInflight`: 猶予が処理時間を覆うかで結果が変わる
- `TestEndpointRemovalIsDelayed`: 削除直後も転送先には残っている
- `TestNoEndpointsDropsAll`: レプリカ 1 個だと、更新のたびに全部落ちる

## 使い方

```go
s := lifecycle.New(lifecycle.Config{
    Propagation: 3, // 転送先から外れるまでの遅れ(こちらでは短くできない)
    PreStop:     3, // SIGTERM の前に待つ時間。伝播以上に取る
    Grace:       10, // SIGTERM から SIGKILL までの猶予。処理時間以上に取る
    Work:        5, // 1 リクエストの処理時間
}, 2)
s.Tick(4)              // 平常運転(1 tick に 4 件)
s.Terminate("pod-1")   // 削除開始
for i := 0; i < 20; i++ {
    s.Tick(4)
}
s.Safe()     // 1 件も落とさなかったか
s.Dropped    // 落ちた件数
s.Log        // 何が起きたかの記録
```

## 簡略化したこと

- **probe なし**: readiness probe による転送先の出し入れは扱わない。ここでは削除だけを起点にする
- **1 コンテナ**: サイドカーの終了順序(実物では順序の保証が課題になる)は扱わない
- **接続の再利用なし**: keep-alive された接続が終了後も残る問題は扱わない
- **論理時刻**: 実時間でなく tick で数える。秒数への対応づけは設定しだい

## 章

教科書: [Podの終了(graceful shutdown)](https://sharin-2a1.pages.dev/parts/pod-lifecycle)

実行: `go test ./orchestration/lifecycle/`
