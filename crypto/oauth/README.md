# oauth — 認可コードフロー(PKCE)

第三者アプリにパスワードを渡さず、範囲を絞った権限だけを委譲する。OAuth 2.0 の認可コードフローと PKCE を最小構成で実装する。中心は「横取りしたコードは検証子なしでは使えない」を攻撃コードで示すこと。

## 肝

- **委譲認可**: パスワードを渡す代わりにトークンを渡す。トークンには scope(できること)と期限がある。パスワードは第三者アプリに触れない
- **フロント/バックの分離**: ブラウザ経由(フロントチャネル)に流すのは短命のコードだけ。トークン本体はアプリとサーバの直接通信(バックチャネル)で交換。トークンが履歴やリダイレクト URL に残らない
- **コードは一度きり**: 認可コードは単回使用。再利用は弾く
- **PKCE**: 秘密を持てない公開クライアント向け。検証子(verifier)を秘密に持ち、そのハッシュ(challenge)だけを先に預ける。交換時に検証子の原本を示せた者だけがトークンを得る。コードを横取りされても検証子がなければ無意味
- **署名付きトークン**: HMAC 署名で改ざんを検出。資源サーバは署名・期限・scope を確かめる
- **決定性**: コード生成は注入した生成器、時刻は論理時計(Advance)

## 効果の固定(テスト)

- `TestHappyPath`: パスワードなしでコード→トークン交換、scope に応じたアクセス
- `TestPKCEStolenCodeUseless`: 横取りしたコードは検証子なしで ErrBadVerifier
- `TestCodeSingleUse`: コード再利用は ErrBadCode
- `TestTokenTamperDetected`: トークンの scope 書き換えを署名で検出

## 使い方

```go
as := oauth.NewAuthServer(secret, codeTTL, tokenTTL, seed)
rs := oauth.NewResourceServer(secret)

verifier := "random-verifier"          // クライアントが秘密に持つ
challenge := oauth.Challenge(verifier)  // ハッシュだけ預ける
code := as.Authorize("app", "alice", "photo:read", challenge) // フロント
token, err := as.Exchange("app", code, verifier)              // バック
ok := rs.Validate(token, "photo:read")
```

## 簡略化したこと

- **不透明でなく自己完結トークン**: HMAC 署名の中身入りトークン。実物は JWT や不透明トークン+イントロスペクション
- **OIDC の id_token なし**: 認可(OAuth)に絞り、認証の id_token は扱わない
- **リフレッシュトークンなし**: アクセストークンのみ。更新の仕組みは省略
- **リダイレクト検証は簡略**: redirect_uri の厳密一致など実運用の防御は省く

## 章

教科書: [OAuthと認可フロー](https://sharin-2a1.pages.dev/parts/oauth)

実行: `go test ./crypto/oauth/`
