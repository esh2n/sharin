# clock — 時刻を測らずに順序をつける

[leaderelection](../../orchestration/leaderelection/) の章で、時刻は共有できないが経過は共有できると書いた。
他のノードが書いた時刻は、自分の時計とのずれが分からないので比べられない。

では、複数のノードで起きた出来事に順序をつけたいときはどうするのか。
答えは、**時刻を測るのをやめて出来事を数える**ことになる。

## Lamport の論理時計

自分のところで何か起きたら数を1つ増やす。知らせるときは数を添える。
受け取った側は、自分の数と受け取った数の**大きいほうから続ける**。

```go
a, b := clock.NewLamport("a"), clock.NewLamport("b")
a.Local()          // 1
sent := a.Send()   // 2
b.Local()          // 1
b.Recv(sent)       // max(1, 2) + 1 = 3
```

これだけで「原因は結果より小さい数を持つ」が保証される。

## 約束は片道しかない

| | 言えるか |
|---|---|
| a が b の原因 → `L(a) < L(b)` | **言える** |
| `L(a) < L(b)` → a が b の原因 | **言えない** |

無関係な2つの出来事にも、たまたま順序がつく。並べられることと、意味があることは別になる。

## ベクタークロック

数を1つでなく、ノードの数だけ持つ。受け取ったら**要素ごとに大きいほう**を取る。

```go
clock.Compare(x, y)
// Before / After / Concurrent / Equal
```

すべての要素で `a <= b` かつ1つでも `a < b` があれば `Before`。
どちらでもなければ `Concurrent` で、これが Lamport では出せない答えになる。

| | Lamport | Vector |
|---|---|---|
| 大きさ | 数1つ | 動いたノードの数だけ |
| 全順序 | つけられる | つけられない(同時があるので) |
| 因果の復元 | できない | できる |
| 同時の検出 | できない | **できる** |

## 筋書きを両方の時計で記録する

```go
s := clock.New("a", "b", "c")
s.Local("a", "a で更新")
e, m := s.Send("a", "b へ知らせる")
s.Local("b", "b で更新")       // ← a のことを知らない
s.Recv("b", m, "受けた")

s.Concurrent()          // 同時に起きた組をすべて返す
s.Relation(x, y)        // 2つの時計の答えを並べて返す
```

`Relation` が `("前", Concurrent)` を返す組がある。
Lamport は順序をつけたが、実際には無関係だった、という場面になる。

## API

| 関数・メソッド | 役割 |
|---|---|
| `NewLamport(id)` / `Local` / `Send` / `Recv` | 数え上げ1つ |
| `LamportLess(t1, id1, t2, id2)` | 全順序。同点はノード名で決める |
| `NewVClock(id)` / `Local` / `Send` / `Recv` | ノードごとの数え上げ |
| `Compare(a, b) Ord` | `Before` / `After` / `Concurrent` / `Equal` |
| `New(nodes...) *Sim` | 両方の時計で筋書きを記録する |
| `(*Sim) ByLamport()` / `Concurrent()` / `Relation(a, b)` | 観測 |

## 決定性

実時間も乱数も使わない。ノードもベクタの要素も名前順に並べる。
同じ筋書きなら必ず同じ結果になる。

## テスト

```
go test -race -cover ./distributed/clock/
```

カバレッジ 100%。
