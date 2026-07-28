# rbac — 誰が何をしてよいか

前章の NetworkPolicy は通信を絞った。こちらは API を絞る。似た形だが、既定の向きが逆で、何も書かなければ何も通らない。

## 肝

- **既定は全拒否**: 通信とは逆。何も書かなければ何もできない。書いたものだけが通る
- **役割と付与を分ける**: 許可の束に名前をつけ、それを誰に与えるかを別に書く。役割を直せば持つ全員に一度に効く
- **足し算しかない**: 拒否は書けない。持っている役割のどれか1つが許せば通る
- **広い許可は打ち消せない**: ワイルドカードを1つ与えると、狭い役割を足しても取り消せない
- **通らないものは書いていないだけ**: 何が通るかを知りたければ、書いてあるものを全部見ればよい
- **決定性**: 役割も判定表も名前順

## 効果の固定(テスト)

- `TestDeniedByDefault`: 何も与えなければ通らず、役割を定義しただけでも効かない
- `TestOnlyWhatIsWritten`: 書いた資源と操作の組み合わせだけが通る
- `TestRolesAreAdditive`: 役割は複数持て、許可は足し算になる
- `TestEditingRoleAffectsEveryone`: 役割の中身を直すと、持つ全員に一度に効く
- `TestWildcardGrantsEverything` / `TestCannotDenyByAddingRole`: 広い許可は強く、後から打ち消せない
- `TestMatrix`: 誰に何を与えているかを一望できる

## 使い方

```go
a := rbac.New()
a.AddRole(&rbac.Role{Name: "viewer", Rules: []rbac.PolicyRule{
    {Resources: []string{"pods", "services"}, Verbs: []rbac.Verb{rbac.Get, rbac.List}},
}})
a.Bind("viewer", "alice", "bob") // ここで初めて効く

a.Can("alice", "pods", rbac.Get).Allowed    // true
a.Can("alice", "pods", rbac.Delete).Allowed // false(書いていない)
a.Can("carol", "pods", rbac.Get).Allowed    // false(与えていない)

a.Matrix([]string{"alice", "bob"}, []string{"pods", "secrets"}, rbac.Get)
```

## 簡略化したこと

- **namespace の区別なし**: 実物は Role(namespace 内)と ClusterRole(全体)を分ける
- **resourceNames なし**: 特定の名前の資源だけを許す指定は扱わない
- **subresource なし**: `pods/log` のような下位資源は扱わない
- **集約なし**: 実物はラベルで複数の ClusterRole を1つにまとめられる
- **利用者の種類なし**: 人とサービスアカウントの区別は扱わない

## 章

教科書: [RBAC](https://sharin-2a1.pages.dev/parts/rbac)

実行: `go test ./orchestration/rbac/`
