# rope — 位置エンコーディングと RoPE

attention は順序を見ないので、位置情報を外から注入する。絶対位置の古典(Sinusoidal)と、現在の主流 RoPE(rotary position embedding)を実装する。

## 肝

- **Sinusoidal**: 位置ごとに sin/cos のパターンを作り、埋め込みに足す絶対位置方式。学習不要
- **RoPE**: Q と K を次元ペアごとに角度 pos·freq で回転させる。回転どうしの内積は角度の差だけで決まるので、attention スコアが相対位置 (m−n) のみに依存する。テストで「両方の位置を同じだけずらしても内積が変わらない」ことを固定している
- **周波数スペクトル**: 先頭ペアほど高周波(近距離を細かく)、末尾ほど低周波(遠距離を大まかに)。freqs[i] = 10000^(−2i/dim)
- **位置補間**: 位置を factor で割ってから回すと、学習レンジを超えた長系列でも角度が既知の範囲に収まる(長コンテキスト化の最も素朴な形)

## 使い方

```go
r, _ := rope.New(64)               // head 次元(偶数)
q2 := r.Apply(q, 9)                // 位置 9 の回転を Q に
k2 := r.Apply(k, 5)                // 位置 5 の回転を K に
// dot(q2, k2) は相対位置 4 だけで決まる
long := r.ApplyInterpolated(q, 8000, 4) // 位置補間で 1/4 に圧縮
pe := rope.Sinusoidal(seqLen, dim)      // 古典の絶対位置テーブル
```

## 簡略化したこと

- **head 次元の 1 ベクトルに適用**: 実物は (batch, head, seq, dim) のテンソルにまとめて適用する
- **NTK-aware / YaRN なし**: 位置補間のみ。周波数ごとに補間量を変える改良は章で言及
- **複素数表現なし**: 実装は実数ペアの 2 次元回転(数学的には同じもの)

## 章

教科書: [位置エンコーディングとRoPE](https://sharin-2a1.pages.dev/parts/rope)

実行: `go test ./llm/rope/`
