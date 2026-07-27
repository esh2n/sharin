# cipher — 対称暗号とモード

8 バイトの Feistel ブロック暗号を自作し、その使い方(ECB / CBC / CTR)と、暗号化だけでは足りず認証が要る(encrypt-then-MAC)ことを実装する。中心は「ECB は模様を漏らす」「CTR は改ざんできる、だから MAC が要る」を攻撃コードで示すこと。

## 肝

- **Feistel 構造**: ブロックを左右に分け、片側だけ鍵で混ぜるのを繰り返す。混ぜる関数 F が可逆でなくても全体は必ず戻せる(DES と同じ骨格)
- **ECB の弱点**: 各ブロックを独立に暗号化。同じ平文ブロック → 同じ暗号文ブロック。繰り返し模様が透ける(有名な「ECB ペンギン」)
- **CBC**: 暗号化前に直前の暗号文と XOR。IV が違えば同じ平文でも別の暗号文。模様が消える
- **CTR**: 鍵で作った擬似乱数列と平文を XOR。暗号化=復号、詰め物不要、長さそのまま
- **暗号化 ≠ 認証**: CTR はビットを反転すると平文の同じ位置が反転する(可鍛性)。攻撃者が中身を狙って書き換えられる
- **AEAD(encrypt-then-MAC)**: 暗号化してから nonce‖暗号文に HMAC。復号前に tag を検証し、改ざんを弾く

## 効果の固定(テスト)

- `TestECBLeaksPatterns`: 同一平文ブロックが ECB では同一暗号文、CBC ではばらける
- `TestCTRMalleable`: 鍵なしで暗号文を書き換え、復号後の平文を "balance=0000100" → "0000900" に改変(検知されない)
- `TestSealOpenDetectsTampering`: 同じ改ざんを Seal/Open は tag 検証で弾く

## 使い方

```go
c := cipher.NewCipher(key)
ecb := c.EncryptECB(plain)          // 使ってはいけない例
cbc := c.EncryptCBC(plain, iv)      // IV は毎回変える
ct := c.CTR(plain, nonce)           // nonce は使い回さない
sealed := c.Seal(key, plain, nonce) // 暗号化+認証(これを使う)
pt, ok := c.Open(key, sealed, nonce)
```

## 簡略化したこと

- **自作の弱い暗号**: 8 バイトブロック・16 段の玩具。実物は AES(128bit ブロック・S-box)。安全性は保証しない
- **nonce/IV 管理は呼び出し側**: 実運用は乱数生成と使い回し防止が必須。CTR の nonce 再利用は致命的
- **GCM でなく HMAC**: 本物の AEAD は GCM(GHASH)や ChaCha20-Poly1305。ここは encrypt-then-MAC で概念を示す
- **パディングオラクル**: CBC の詰め物検証を突く攻撃は概念のみ。だから復号前に認証する AEAD を使う

## 章

教科書: [対称暗号とモード](https://sharin-2a1.pages.dev/parts/symmetric-cipher)

実行: `go test ./crypto/cipher/`
