# layers — RMSNorm と SwiGLU

mini-GPT が使った LayerNorm / GELU を、実物のオープンモデルが使う RMSNorm / SwiGLU に置き換える部品を実装する。

## 肝

- **RMSNorm**: LayerNorm から「平均を引く」工程を省き、RMS で割るだけ。スケール不変だがシフト不変ではない(テストで両方固定)。品質差がほぼ出ないと分かり、計算が軽いので Llama 以降の標準
- **SwiGLU**: FFN を `(SiLU(x·W1) ⊙ x·W3)·W2` のゲート付きにする。ゲートが強い負ならチャネルが閉じ、値側が何であれ出力はほぼ 0(テストで固定)。行列が 1 枚増えるぶん中間次元を約 2/3 に狭めて釣り合いを取るのが実物の流儀
- **SiLU**: x·σ(x)。GELU とよく似た滑らかな片側通過

## 使い方

```go
y := layers.RMSNorm(x, 1e-5)
f := layers.NewSwiGLUFFN(dModel, dHidden)
out := f.Forward(x)
```

## 簡略化したこと

- **gain(γ)なし**: 実物の RMSNorm は正規化後に学習可能なスケール γ を掛ける
- **学習なし**: 重みは決定的な擬似乱数。構造とゲートの性質の検証が目的

## 章

教科書: [RMSNormとSwiGLU](https://sharin-2a1.pages.dev/parts/rmsnorm-swiglu)

実行: `go test ./llm/layers/`
