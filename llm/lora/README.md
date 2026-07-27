# lora — LoRA(low-rank adaptation)

凍結した重みの隣に低ランクの補正行列を足して、ごく少数のパラメータだけで微調整する LoRA を最小構成で実装する。

## 肝

- **低ランク補正**: `y = x·W + (alpha/r)·x·A·B`。W(d×k)は凍結、学習するのは A(d×r)と B(r×k)だけ。r が小さいほどパラメータが激減(4096×4096 を rank 8 で 0.4% まで)
- **初期は恒等**: B を 0 初期化するので学習開始時の補正は 0。base の挙動を一切変えないことをテストで固定
- **alpha でスケール**: 補正は alpha/rank 倍。alpha を 2 倍にすると base からのズレも 2 倍(テストで固定)
- **マージで追加コストゼロ**: 学習後 `A·B` を W に足し込めば 1 枚の行列に戻る。推論時は通常の行列積と同じ(Merge が Forward と一致することをテストで固定)

## 使い方

```go
l, _ := lora.New(baseWeight, 8, 16.0)  // rank 8, alpha 16
y := l.Forward(x)                        // base + 低ランク補正
l.TrainableParams()                      // A + B のパラメータ数(base 全体の <1%)
merged := l.Merge()                      // 学習後、1枚の行列に畳む(推論用)
```

## 簡略化したこと

- **勾配計算・学習ループなし**: B の更新は `SetB` で差し替えて性質を検証。実物は誤差逆伝播で A・B を学習
- **1 層のみ**: 実物は attention の Q/V 射影など複数の層に LoRA を挿す
- **量子化併用(QLoRA)なし**: 4bit 凍結重み + LoRA は [量子化](/parts/quantization)章と本章の組み合わせ。ここでは別々に
- **DoRA 等の変種なし**: 大きさと向きを分離する改良は章で言及のみ

## 章

教科書: [LoRA](https://sharin-2a1.pages.dev/parts/lora)

実行: `go test ./llm/lora/`
