# networkpolicy — 繋いでよい相手を宣言で絞る

既定では全部が全部に繋がる。方針を1つ向けると、その Pod への通信は既定で拒否に変わり、明示的に許した分だけが通る。許可を足すことが、同時に既定を落とすことになっている。

## 肝

- **既定は全通し**: 方針が1つも無いうちは、どの Pod からどの Pod へも繋がる。そうでないと何も動かないから
- **方針を1つ向けると既定が反転する**: 選ばれた Pod への通信は既定で拒否になる。許可を書いた瞬間に、書かなかった分が閉じる
- **方針は受け側に付く**: `Selector` は守られる側で、`Rules` は入ってくる側。送り側に付けても、その Pod が出す通信は縛られない
- **許可は足し算**: どれか1つの規則が合えば通る。後から方針を足して既存の通信を塞ぐことはできない
- **規則が空なら完全に閉じる**: 何も許可しない方針が、その Pod への通信を全部止める書き方になる
- **決定性**: Pod は名前順、規則は書いた順に評価する

## 効果の固定(テスト)

- `TestOpenByDefault`: 方針が無いうちは全組み合わせが通る
- `TestPolicyFlipsDefaultToDeny`: db に方針を向けると、許可した api だけが通り web は塞がる
- `TestPolicyAppliesToDestination`: 守られるのは受け側。送り側の出ていく通信は縛られない
- `TestRulesAreAdditive` / `TestCannotDenyByAddingPolicy`: 許可は足し算で、後から塞ぐことはできない
- `TestEmptyPolicyDeniesAll`: 規則を持たない方針は全部止める
- `TestThreeTierIsolation`: web → api → db は通り、web → db の飛び越えは塞がる

## 使い方

```go
c := networkpolicy.New()
c.AddPod("web", map[string]string{"app": "web"})
c.AddPod("api", map[string]string{"app": "api"})
c.AddPod("db", map[string]string{"app": "db"})

c.AddPolicy(&networkpolicy.Policy{
    Name:     "db-allow-api",
    Selector: map[string]string{"app": "db"}, // 守られる側
    Rules: []networkpolicy.Rule{
        {From: map[string]string{"app": "api"}, Port: 5432}, // 入ってよい側
    },
})

c.Connect("api", "db", 5432).Allowed // true
c.Connect("web", "db", 5432).Allowed // false(方針で守られ、許可が無い)
c.Connect("web", "api", 8080).Allowed // true(api には方針が無いので既定のまま)

c.Matrix(5432) // 全組み合わせの可否を一望する
```

## 簡略化したこと

- **入ってくる側のみ**: 実物は `Ingress` と `Egress` の両方を書ける。ここは入ってくる側だけ
- **namespace セレクタなし**: 実物は別の namespace からの通信も条件にできる
- **IP 範囲なし**: 実物はクラスタ外の IP 範囲も条件に書ける
- **プロトコルなし**: TCP / UDP の区別は扱わない
- **実装は判定のみ**: 実際の遮断はネットワークプラグインが行う。ここでは可否の計算だけ

## 章

教科書: [NetworkPolicy](https://sharin-2a1.pages.dev/parts/network-policy)

実行: `go test ./orchestration/networkpolicy/`
