# csi — 要求と実体を分け、3段で繋ぐ

[statefulset](../statefulset/) の章では、ボリュームは Pod より長生きだと書いた。
その「消えない置き場」を用意している下の層が、このパッケージになる。

## 要求と実体

使う側が書くのは要求(`Claim`)だけで、それがどのディスクになるかは知らない。
要求に合う実体(`Volume`)を用意するのは `Driver` の仕事になる。作り方の型は `Class` にまとめる。

## 3段に分かれている

| 段 | 単位 | 誰の都合 |
|---|---|---|
| `Request` / provision | 要求ごと | 保存の担当。どこにどう作るか |
| `Attach` | **ノード** | ノードにディスクを見せる |
| `Mount` | **Pod** | Pod のファイルシステムに出す |

分かれているのは、担当も失敗の仕方も違うからになる。そしてこの分かれ方が、
いちばん有名な誤解を生む。

```go
d.Attach("data", "node-a", "")
d.Mount("data", "node-a", "web-1") // ok
d.Mount("data", "node-a", "web-2") // ok ← 同じノードなら共有できる
d.Attach("data", "node-b", "")     // 失敗 ← 別のノードには繋げない
```

**`ReadWriteOnce` は「1つのノードから」であって「1つの Pod から」ではない。**
繋ぐのがノード単位だからで、同じノードに載った Pod は何個でも共有できる。

## 外れないと引き継げない

```go
d.Mount("data", "node-a", "web")
d.Detach("data")      // 失敗: まだ 1 個の Pod から見えている
d.ForceDetach("data") // ノードが応答しないときの最後の手段
```

ノードが死んだとき、外れたことを確かめられないので別のノードで起動できない。
これが引き継ぎの遅さの正体になる。

## 配置が先か、確保が先か

| Binding | いつ作るか | 区画があると |
|---|---|---|
| `Immediate` | 要求が来た時点 | 作った区画から出られなくなる |
| `WaitForFirstConsumer` | 使う Pod の置き場所が決まってから | Pod の区画に合わせて作れる |

[topology](../topology/) がある環境では、先に作ると繋がらない組み合わせができる。
だから「待つ」という選択肢が要る。

## API

| メソッド | 役割 |
|---|---|
| `New(classes...) *Driver` | 作り方の型を登録 |
| `Request(claim) (bool, string)` | 要求を受ける。待つ設定ならまだ作らない |
| `Attach(claim, node, zone) (bool, string)` | ノードに繋ぐ。待つ設定ならここで作る |
| `Mount(claim, node, pod)` / `Unmount(claim, pod)` | Pod から見せる・隠す |
| `Detach(claim)` / `ForceDetach(claim)` | ノードから外す |
| `DeleteClaim(claim)` | 要求を消す。実体の行方は `Reclaim` が決める |

## 決定性

実時間も乱数も使わない。実体の名前は連番、一覧は名前順。同じ手順なら必ず同じ結果になる。

## テスト

```
go test -race -cover ./orchestration/csi/
```

カバレッジ 99.1%。
