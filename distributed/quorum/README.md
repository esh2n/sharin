# quorum — 何台に聞けば古い値を見ないか

[replication](../replication/) の章では、書き込みを受けるのはリーダー1台だった。
リーダーが居れば最新は必ずリーダーにあるので、どこから読むかは速さの問題でしかない。

リーダーを置かないと、その拠り所が消える。書き込みは複数の台に散り、
**どの台が最新を持っているかは聞いてみるまで分からない**。

## 台数の引き算

担当が `N` 台、書きで返事を待つのが `W` 台、読みで返事を待つのが `R` 台。

```
N 台のうち W 台が新しい値を持つ。古い台は N - W 台しかない。
R > N - W なら、R 台すべてを古い台で埋められない。
移項して   R + W > N
```

```go
Config{N: 3, R: 2, W: 2}.Overlaps()  // true
Config{N: 3, R: 1, W: 1}.Overlaps()  // false
```

`R=2,W=2,N=3` では、1台が古いまま12回読んでも古い値は0回。
`R=1,W=1,N=3` で同じ手順を踏むと、12回中8回が古い値になる。

## 重なっても、どれが最新かは別に決める

重なりが言うのは「最新を持つ台が返事の中に居る」ことだけ。
返ってきた値のどれが新しいかは版番号が決める。

```go
Newer(a, b Value) Value  // Stamp が大きいほうが勝つ
```

読んだついでに古い台へ書き戻すのが**読み修復**(`ReadRepair`)。
読まれない key はいつまでも古いままなので、実物はこれとは別に定期的な突き合わせを持つ。

## 緩めた quorum は重なりを手放す

担当が落ちているとき、担当外の台を**代役**に立てて預かってもらう。
返事の数には数えるので書き込みは通るが、値は担当の上に無い。

```go
c := quorum.New(quorum.Config{N: 3, R: 2, W: 2, Sloppy: true}, "a", "b", "c", "d", "e")
c.Kill("a"); c.Kill("b")
c.Put("x", "v2")   // ok=true acks=3 stored=[c] subs=[d e]
```

読みは担当にしか聞かないので、`R+W>N` でも古い値が返る(6回中2回)。
`Handoff()` で預かりぶんが担当へ渡ると、重なりが戻って0回になる。

## API

| 関数・メソッド | 役割 |
|---|---|
| `New(cfg, names...) *Cluster` | 台を並べる。リーダーは居ない |
| `Config.Overlaps() bool` | `R + W > N` |
| `(*Cluster) Home(key) []string` | key を担当する N 台([consistenthash](../consistenthash/) と同じ決め方) |
| `(*Cluster) Put(key, data) WriteResult` | N 台へ送り、W 台の返事で成功 |
| `(*Cluster) Get(key) ReadResult` | R 台に聞いて、いちばん新しい値を返す |
| `(*Cluster) Handoff() int` | 代役の預かりぶんを担当へ渡す |
| `(*Cluster) Stale(key) []string` | 最新を持っていない担当(観測用) |
| `Newer(a, b) Value` | 版番号が大きいほうを採る |

## 決定性

実時間も乱数も使わない。版番号は書き込みごとの数え上げ、
「誰が先に返事するか」は読みの回数で順を回すので、同じ操作なら必ず同じ結果になる。

## テスト

```
go test -race -cover ./distributed/quorum/
```

カバレッジ 100.0%。
