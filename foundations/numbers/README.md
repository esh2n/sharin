# numbers — 2の補数とバイト順

負の数をビットでどう表すかには何通りも答えがある。素直なのは符号のビットを1本立てて
残りを絶対値にする形で、人間が読むにはこれがいちばん分かりやすい。だが実際の計算機は
これを使わない。ほぼすべてが2の補数を使う。

## なぜ2の補数か

**引き算を足し算にできるから**になる。`a - b` を `a + (-b)` として、同じ回路で計算できる。
符号を見て加算と減算を切り替える必要が無い。表し方の美しさではなく、回路が1つで済むという
都合で選ばれている。

```go
a := numbers.Encode(3, 8, numbers.Twos)
b := numbers.Encode(10, 8, numbers.Twos)
numbers.Sub(a, b, 8).Bits                  // -7
numbers.Add(a, numbers.Neg(b, 8), 8).Bits  // 同じ
```

## 3つの表し方の違い

| | 0 の表し方 | 8ビットの範囲 | 引き算 |
|---|---|---|---|
| `Twos`(2の補数) | **1通り** | -128 .. 127(非対称) | 足し算と同じ回路 |
| `Ones`(1の補数) | 2通り | -127 .. 127 | 繰り上がりの回り込みが要る |
| `SignMag`(符号と絶対値) | 2通り | -127 .. 127 | 符号を見て分岐が要る |

0 が2通りあると、同じ数なのにビットが違うので比較のたびに特別扱いが要る。
2の補数はそれが無いぶん、負の側が1つ多くなる。

## 代償: 絶対値が取れない数がある

```go
min := numbers.Encode(-128, 8, numbers.Twos)
numbers.Neg(min, 8) == min // true ← 符号を反転しても自分自身
```

いちばん小さい数の符号を反転した結果は範囲に収まらないので、行き場がない。
`abs(x)` が負を返しうる、という歪みはここから来る。

## はみ出しは2種類ある

同じ加算でも、符号なしと見るか符号つきと見るかで壊れる条件が違う。

```go
numbers.Add(200, 100, 8) // Carry: true,  Overflow: false
numbers.Add(100, 100, 8) // Carry: false, Overflow: true
```

回路は同じで、どちらの旗を見るかだけが違う。

## バイト順は取り決め

| | 並び | 取り柄 |
|---|---|---|
| `PutLittle` | 下位のバイトから | 途中で止めても壊れない。先頭1バイト = 下位8ビット |
| `PutBig` | 上位のバイトから | 人間が読む順と同じ。通信で使う順序 |

```go
le := numbers.PutLittle(0x12345678, 32) // [78 56 34 12]
numbers.GetLittle(le[:1])               // 0x78 ← 下位8ビット
numbers.GetLittle(le[:2])               // 0x5678 ← 下位16ビット
numbers.GetBig(le)                      // 0x78563412 ← 壊れる
```

正しさの問題ではなく取り決めなので、境界を越えるとき(ファイルに書く、通信で送る)は
必ず明示する。

## API

| 関数 | 役割 |
|---|---|
| `Encode(v, w, kind)` / `Decode(bits, w, kind)` | 数とビット列の変換 |
| `Zeros(w, kind)` / `Range(w, kind)` | 0 の表し方の数、表せる範囲 |
| `Add` / `Sub` / `Neg` | 加算器。`Result` に `Carry` と `Overflow` |
| `SignExtend` / `ZeroExtend` | 幅を広げる。符号を保つかどうか |
| `ShiftRight(bits, n, w, arithmetic)` | 算術シフトと論理シフト |
| `PutLittle` / `PutBig` / `GetLittle` / `GetBig` | バイト順 |

## テスト

```
go test -race -cover ./foundations/numbers/
```

カバレッジ 100%。幅 4 と 8 の全ビットパターンで往復を確かめている。
