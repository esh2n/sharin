# tls — TLS 風ハンドシェイク(暗号系の総まとめ)

これまでの暗号部品(鍵交換・署名・ハッシュ・AEAD)を 1 本の流れに束ね、安全な通信路を張る。中心は「鍵交換に証明書認証を足すと中間者が防げる」を、署名偽造が弾かれる様子で示すこと。

## 束ねる部品

- **crypto/dh** — 一時鍵で共有秘密を作る(前方秘匿性)
- **crypto/rsa** — 証明書とハンドシェイクの署名(相手認証)
- **crypto/hash** — トランスクリプトのハッシュと鍵導出(HMAC)
- **crypto/cipher** — 導出鍵で本文を暗号化+認証(AEAD)

## 流れ

1. **ClientHello**: クライアントが一時 DH 公開鍵と乱数を送る
2. **ServerHello**: サーバが一時 DH 公開鍵・証明書・ハンドシェイク署名を返す。署名は「この DH 公開鍵は確かに証明書の持ち主が出した」ことを証す
3. **Finish**: クライアントが 3 つを検証 — 証明書が信頼 CA の署名を持つ / 主体名が一致 / ハンドシェイク署名が有効。すべて通れば鍵を導く
4. **Record**: 導出鍵で本文を AEAD(Seal/Open)で守る

## 効果の固定(テスト)

- `TestHandshakeEstablishesSharedKey`: 正規の接続で両者が同じ鍵に到達し、本文が往復
- `TestRejectsUntrustedCA`: 攻撃者が自前 CA で発行した偽証明書を弾く
- `TestMITMForgedKeyRejected`: 中間者が DH 公開鍵をすり替えても、署名を偽造できず ErrBadSig
- `TestTamperedRecordRejected`: 確立後の本文の改ざんを AEAD が検知

## 使い方

```go
ca := tls.NewCA(61, 53)
serverPub, serverPriv := rsa.GenKeyPair(67, 71)
cert := ca.Issue("example.com", serverPub)
server := tls.NewServer(cert, serverPriv)

client := tls.NewClient("example.com", ca.Pub, dh.NewRand(1))
sh, serverSess := server.Respond(client.Hello(), dh.NewRand(2))
clientSess, err := client.Finish(sh) // err != nil なら接続拒否
sealed := clientSess.Seal([]byte("GET /"))
msg, ok := serverSess.Open(sealed)
```

## 簡略化したこと

- **玩具の部品**: RSA も DH も小さな数。安全性は保証しない(構造を見るもの)
- **TLS 1.3 の一部**: 実物のメッセージ・拡張・0-RTT・証明書チェーンは省略
- **固定 nonce/random**: 実運用はレコードごと・接続ごとに変える
- **鍵導出は簡略**: 実物は HKDF で複数の鍵(方向別)を導く

## 章

教科書: [TLSハンドシェイク](https://sharin-2a1.pages.dev/parts/tls-handshake)

実行: `go test ./crypto/tls/`
