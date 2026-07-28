# ingress — 入口を1つに束ねて、中で振り分ける

Service はクラスタの中の宛先だった。外からの入口を Service ごとに出すと、口がサービスの数だけできる。Ingress は入口を1つに束ね、ホスト名とパスで中へ振り分ける。

## 肝

- **扱う単位が上がる**: IP とポートでなく、ホスト名とパス。層が上がったぶん、パスで分けるといったことができる
- **入口が1つで済む**: 証明書もドメインも1つ。サービスが増えても入口は増えない
- **特定度で決まる**: 長いパスが勝ち、ホスト指定が勝つ。書いた順は結果に影響しない
- **振り分け先は Service**: ここから先は[Service](../service/)の仕事。層がきれいに分かれている
- **当たらなければ既定へ**: 既定も無ければ見つからない。どこにも落ちない状態を作らない
- **決定性**: 同じ特定度は Service 名の順。何度並べ直しても同じ

## 効果の固定(テスト)

- `TestRoutesByPath`: 同じホストの下で `/` は web、`/api` は api へ
- `TestLongerPathWins`: 両方当たるときは長いパスを採る
- `TestOrderOfAdditionDoesNotMatter`: 書いた順を変えても結果が同じ
- `TestHostRuleBeatsWildcard`: ホストを指定した規則が優先される
- `TestFallbackAndNotFound`: 当たらなければ既定へ、既定も無ければ見つからない
- `TestRulesAreSortedBySpecificity`: 評価順が特定度の高い順に並ぶ

## 使い方

```go
i := ingress.New()
i.Add(ingress.Rule{Host: "shop.example", Path: "/",
    Backend: ingress.Backend{Service: "web", Port: 80}})
i.Add(ingress.Rule{Host: "shop.example", Path: "/api",
    Backend: ingress.Backend{Service: "api", Port: 8080}})
i.Default = &ingress.Backend{Service: "notfound", Port: 80}

i.Route("shop.example", "/")           // web:80
i.Route("shop.example", "/api/users")  // api:8080(長いパスが勝つ)
i.Route("other.example", "/")          // notfound:80(既定へ)
i.Rules()                              // 評価される順に並んだ規則
```

## 簡略化したこと

- **TLS なし**: 実物は入口で証明書を持ち、TLS を終端する。入口を束ねる主な動機の1つ
- **前方一致のみ**: 実物は `Exact` / `Prefix` / 実装依存の正規表現を選べる
- **書き換えなし**: パスの書き換えやヘッダの追加は扱わない
- **ヘルスチェックなし**: 振り分け先の生死は[Service](../service/)側の話として扱う
- **実装は判定のみ**: 実際の受信と転送は Ingress コントローラが行う。ここでは行き先の計算だけ

## 章

教科書: [Ingress](https://sharin-2a1.pages.dev/parts/ingress)

実行: `go test ./orchestration/ingress/`
