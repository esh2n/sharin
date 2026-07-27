# hash — ハッシュ・HMAC・パスワードハッシュ

暗号学的ハッシュを Merkle–Damgård 構成で自作し、その上に HMAC とパスワードハッシュを組む。中心は「素朴な H(secret‖msg) は長さ拡張攻撃で偽造できる、だから HMAC が要る」を実際に攻撃コードで示すこと。

## 肝

- **ハッシュ**: 任意長 → 固定長の一方向関数。同じ入力=同じ指紋、1 ビット違えば総崩れ(なだれ効果)、指紋から入力は戻せない
- **Merkle–Damgård**: ブロックごとに内部状態を混ぜていく構成。末尾に長さを書く(強化)。この構成の副作用が長さ拡張攻撃
- **長さ拡張攻撃**: H(secret‖msg) と長さが分かれば、secret を知らずに末尾を継ぎ足した正しい指紋を作れる。素朴な MAC が破れる
- **HMAC**: `H((k⊕opad)‖H((k⊕ipad)‖m))`。内側を外側で包むので拡張が効かない。鍵付き認証符号の定番
- **パスワードハッシュ**: 速いハッシュ 1 回は総当たりに弱い。塩でレインボーテーブルを無効化、反復で 1 回の検証を重くする
- **定数時間比較**: 指紋の照合は早期 return せず全バイト見る(タイミングで正誤を漏らさない)

## 効果の固定(テスト)

- `TestLengthExtension`: 秘密なしで forged tag が本物と一致(攻撃成功)
- `TestHMACResistsExtension`: 同じ拡張が HMAC には効かない
- `TestPasswordSaltMatters`: 塩が違えば同じパスワードでも別の指紋

## 使い方

```go
d := hash.Sum([]byte("message"))            // 指紋
tag := hash.HMAC(key, msg)                  // 認証符号(H(k‖m) の代わりにこれ)
ok := hash.Equal(tag, hash.HMAC(key, msg))  // 定数時間で照合
stored := hash.HashPassword(pw, salt, 10000) // 塩+反復で保存用
hash.VerifyPassword(stored, pw, salt, 10000)
```

## 簡略化したこと

- **自作の弱いハッシュ**: 128bit・8 ラウンド。実物は SHA-256 等。衝突耐性は保証しない(構成を見るための玩具)
- **反復は素朴**: 実物は bcrypt/scrypt/argon2 でメモリ困難性も持たせる。ここは単純反復
- **SHA-3 は別構成**: スポンジ構成は長さ拡張に強い。この章は Merkle–Damgård の穴を見るのが目的

## 章

教科書: [ハッシュとHMAC](https://sharin-2a1.pages.dev/parts/hashing)

実行: `go test ./crypto/hash/`
