# dh — Diffie–Hellman 鍵交換

盗聴されている公開路だけを使って、二人にしか分からない共有秘密を作る。剰余べき乗の一方向性(離散対数問題)を根拠に、鍵配布の問題を解く。あわせて、素の DH が中間者攻撃に無力で認証が要ることも示す。

## 肝

- **離散対数の一方向性**: `g^a mod p` は簡単だが、結果から a を逆算するのは p が大きいと現実的に解けない
- **共有秘密の導出**: Alice は秘密 a から `g^a`、Bob は秘密 b から `g^b` を公開。互いの公開値を自分の秘密でべき乗すると、どちらも `g^(ab) mod p` に一致する
- **盗聴者は作れない**: `g^a` と `g^b` は見えても、a も b も分からないので `g^(ab)` を作れない。素朴な合成(積・和・べき乗)では一致しない
- **剰余べき乗は二乗法**: 指数のビットぶんの掛け算で計算(素朴に exp 回掛けるのとは桁違い)
- **素の DH は MITM に無力**: 間に割り込む中間者が両者と別々に鍵交換でき、認証なしでは気づけない。だから TLS は証明書で相手を認証する

## 効果の固定(テスト)

- `TestBothSidesDeriveSameSecret`: 公開鍵だけ交換して両者が同じ `g^(ab)` に到達
- `TestPassiveEavesdropperCannotDerive`: 公開値の素朴な合成では共有秘密に一致しない
- `TestMITMWithoutAuth`: 中間者が Alice とも Bob とも鍵を共有し、両者を騙せる

## 使い方

```go
pr := dh.Params{P: bigPrime, G: big.NewInt(5)}
a, A := pr.Generate(dh.NewRand(seed)) // 秘密 a と公開鍵 A
// A を相手に送り、相手の公開鍵 B を受け取る
shared := pr.Shared(a, B) // g^(ab) mod p(相手も同じ値を得る)
```

## 簡略化したこと

- **小さな素数**: 教科書用に 2^31-1 など。実物は 2048bit 以上、または楕円曲線(ECDH)
- **静的パラメータ**: P・G は固定。実物は名前付き群(RFC 7919)や X25519 を使う
- **鍵導出関数なし**: 共有秘密はそのまま。実物は KDF に通して対称鍵にする
- **認証は別章**: MITM を防ぐ相手認証は [TLS](/parts/crypto)・証明書の話

## 章

教科書: [鍵交換(Diffie–Hellman)](https://sharin-2a1.pages.dev/parts/key-exchange)

実行: `go test ./crypto/dh/`
