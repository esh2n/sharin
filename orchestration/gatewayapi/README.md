# gatewayapi — 入口と規則を分けて、双方の同意で繋ぐ

[Ingress](../ingress/) は入口を1つに束ねて、ホスト名とパスで中へ振り分けた。
振り分けの仕組みとしてはこれで足りている。足りなかったのは、1つのオブジェクトに
全部が入っていることだった。

証明書や公開ホスト名は運用側の持ち物で、`/api` を自分のサービスへ向ける規則は
アプリ側の持ち物になる。同居していると、アプリ側に書かせようとすれば入口ごと
触れてしまうし、運用側が抱えれば規則を足すたびに依頼が要る。

Gateway API はこれを役割で分ける。入口は `Gateway`、振り分けは `HTTPRoute`。
別のオブジェクトなので、[RBAC](../rbac/) で別々に権限を切れる。

## 双方の同意

分けたぶん、繋ぎ方が問題になる。

```go
c := gatewayapi.New()
c.AddGateway(gatewayapi.Gateway{
    Name: "public", Namespace: "infra",
    Listeners: []gatewayapi.Listener{{
        Name: "https", Port: 443, Hostname: "shop.example",
        AllowedFrom: []string{"team-a"},   // 受け入れる側の同意
    }},
})
c.AddRoute(gatewayapi.HTTPRoute{
    Name: "web", Namespace: "team-a",
    ParentRefs: []string{"public"},        // 指名する側の同意
    Hostnames:  []string{"shop.example"},
    Rules: []gatewayapi.Rule{{
        Matches:  []gatewayapi.Match{{PathType: "PathPrefix", Path: "/api"}},
        Backends: []gatewayapi.Backend{{Service: "api", Port: 8080}},
    }},
})

c.Attachments()  // 繋がったか、繋がらなかったなら理由
c.Route(gatewayapi.Request{Gateway: "public", Host: "shop.example", Path: "/api/users"})
```

Route が親を指名し、Gateway がどの名前空間を受け入れるかを宣言する。
両方が揃わなければ繋がらないので、片側だけでは他人の入口へ相乗りすることも、
他人の規則を勝手に取り込むこともできない。既定は「同じ名前空間だけ」で、
開くには明示が要る。

## 優先順位は仕様で決まっている

Ingress の細かい挙動は実装ごとに違い、annotation で補うのが普通だった。
Gateway API は順序を決めている。上から見て、最初に差がついたところで勝負が決まる:

1. ホスト名が完全一致(ワイルドカードより強い)
2. パスが `Exact`(`PathPrefix` より強い。長さは関係ない)
3. パスが長い
4. メソッドの一致
5. ヘッダの一致数
6. クエリの一致数
7. それでも同点なら Route の名前順

## 重み付き分岐

同じ規則に複数の振り分け先を並べ、`Weight` で取り分を決められる。
カナリアが annotation ではなく型で書ける。

```go
Backends: []gatewayapi.Backend{
    {Service: "stable", Port: 80, Weight: 90},
    {Service: "canary", Port: 80, Weight: 10},
}
```

## API

| 関数・メソッド | 役割 |
|---|---|
| `New() *Cluster` | 空のクラスタを作る |
| `(*Cluster) AddGateway(g)` / `AddRoute(r)` | 入口と規則を足す |
| `(*Cluster) Attachments() []Attachment` | 繋がりの判定と、繋がらない理由 |
| `(*Cluster) Route(req) Result` | 1件を振り分ける。勝った規則の特定度も返す |

## 決定性

乱数を使わない。重み付き分岐は規則ごとの通過数を数えて割り当てるので、
同じ順で投げれば必ず同じ結果になり、比率をテストで数えられる。
同点の解決もすべて名前順で、宣言した順は結果に影響しない。

## テスト

```
go test -race -cover ./orchestration/gatewayapi/
```

カバレッジ 100%。
