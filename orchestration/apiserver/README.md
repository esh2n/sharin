# apiserver — 写しを持つ。ただし写しは古くなる

この編のコントローラはずっと「現状を数える」と言ってきた。実際に見ていたのは手元の写しで、それを保つのが informer になる。

## 肝

- **一度だけ全件、以降は差分**: 毎回全件は重く、差分だけでは始点が無い。最初に一度だけ全件を取る
- **読みは写しから**: API サーバに触らないので、何度読んでも安い。コントローラが頻繁に調整できる理由
- **写しは古くなる**: watch が切れている間の変更は届かない。読み手は古い状態を見て判断することがある
- **履歴は無限に持てない**: 古い版から追いつけなくなったら、全件を取り直すしかない
- **だから level-triggered が要る**: 取りこぼしても次に数え直せば追いつく設計でなければ、この土台の上では動けない
- **決定性**: 一覧は名前順、版は単調増加

## 効果の固定(テスト)

- `TestListsFromCacheNotServer`: 100回読んでも全件読み出しは増えない
- `TestDisconnectedCacheGoesStale`: 切れている間の変更は届かず、消えたものが写しに残る
- `TestReconnectCatchesUp`: 張り直すと、途中の経過は見ないが最終の姿には追いつく
- `TestCompactedHistoryForcesResync`: 履歴が捨てられていると全件を取り直す
- `TestRecentHistoryAvoidsResync`: 履歴が残っていれば差分で足りる
- `TestReaderSeesStaleState`: 写しと真実がずれている状態を、読み手が見ることがある

## 使い方

```go
s := apiserver.NewStore()
s.Put("Pod", "web-1", "running")

i := apiserver.NewInformer(s, "Pod")
i.Start()   // ここで一度だけ全件を読む
i.List()    // 以降は写しから読むので安い

s.Put("Pod", "web-2", "running")
i.Stale()   // true(まだ届いていない)
i.Sync()    // 差分を反映
i.Stale()   // false

i.Disconnect()
s.Delete("Pod", "web-1")
i.List()    // web-1 がまだ居るように見える(写しが古い)
i.Reconnect() // 張り直して追いつく
```

## 簡略化したこと

- **etcd を作らない**: 実物は etcd が保存と watch を担い、API サーバはその前段に立つ
- **合意なし**: 実物の etcd は Raft で複数台に複製する。[Raft の章](../../distributed/raft/)が該当する
- **通信なし**: watch は HTTP のストリームで流れる。ここでは関数を呼ぶだけ
- **並行なし**: 複数の informer が同時に動く状況は扱わない
- **workqueue なし**: 実物は変更を受けてキューに積み、重複をまとめてから reconcile を呼ぶ

## 章

教科書: [APIサーバとinformer](https://sharin-2a1.pages.dev/parts/apiserver)

実行: `go test ./orchestration/apiserver/`
