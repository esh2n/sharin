# initcontainer — Pod の中の起動順序

Pod の中には複数のコンテナが入る。設定を取ってくる処理、通信を代理するプロキシ、
そして本体。この間にも順序があるが、宣言しなければ並行に立ち上がる。
並行ということは、速いほうが先になるということだ。

本体の起動が速く、プロキシの起動が遅ければ、プロキシ抜きで本体が動く時間ができる。
終了のときは向きが逆で、プロキシが先に消えると本体の処理が外に出られない。
このパッケージは、順序を宣言しない書き方と宣言する書き方を並べて、その差を数える。

## 使い方

```go
p := initcontainer.New([]initcontainer.Spec{
    {Name: "config-fetch", Kind: initcontainer.Init, Boot: 2},
    {Name: "proxy", Kind: initcontainer.Sidecar, Boot: 3, Drain: 1, Proxy: true},
    {Name: "web", Kind: initcontainer.App, Boot: 1, Drain: 3},
})
for !p.Ready() {
    p.Tick()
}
p.Terminate()
for !p.Finished() {
    p.Tick()
}
fmt.Println(p.Exposed) // 0
```

`Kind` を `Sidecar` から `App` に変えるだけで、同じ3つのコンテナが並行に立ち上がり、
`Exposed` は 4 になる。起動側で 2 tick、停止側で 2 tick。

## 3つの枠

| Kind | 起動 | 停止 |
|---|---|---|
| `Init` | 宣言順に1つずつ。完了するまで次も本体も始まらない | 完了済みなので何もしない |
| `Sidecar` | Init 枠に並ぶが、稼働に入ったら次へ進ませる | 本体がすべて終わってから止まる |
| `App` | Init 枠を通り切ってから、まとめて並行に起動 | 停止要求で一斉に止まり始める |

## API

| 関数・メソッド | 役割 |
|---|---|
| `New(specs []Spec) *Pod` | 宣言から Pod を組み立てる。宣言順を保つ |
| `(*Pod) Tick()` | 論理時刻を1つ進める |
| `(*Pod) Ready() bool` | 本体がすべて稼働しているか |
| `(*Pod) Terminate()` | 停止を始める。二度呼んでも一度しか効かない |
| `(*Pod) Finished() bool` | 全コンテナが終了したか |
| `(*Pod) Exposed` | 本体が生きているのに出入口が居なかった tick 数 |
| `(*Pod) Containers() []*Container` | 宣言順のコンテナ一覧 |

## 決定性

実時間も乱数も使わない。起動と停止にかかる時間は `Spec.Boot` / `Spec.Drain` に、
失敗の回数は `Spec.Fails` に台本として与える。時計は `Tick` で1つずつ進む論理時刻なので、
同じ宣言なら必ず同じ結果になる。

`Fails` を使うと、失敗した Init が後続をすべて止めたまま再試行を繰り返す様子を再現できる。
待ち時間は 1, 2, 4, 8 と倍に伸び、8 で止まる。

## テスト

```
go test -race -cover ./orchestration/initcontainer/
```

カバレッジ 100%。
