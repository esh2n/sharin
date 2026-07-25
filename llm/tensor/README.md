# llm/tensor

LLM を Go で自作するための最小の行列演算。numpy に逃げず全部書く。

## 肝

- Transformer の forward pass は、結局この数本の関数(matmul, softmax, layernorm, GELU)の
  組み合わせ。「attention は行列積の塊」を実感するための土台
- 表現は「float32 スライス + 行数・列数」の2次元行列に絞る(実物は N 次元テンソル)
- softmax は llm-sampling 編と同じ。ここでは attention の重み計算に使う

## 簡略化したこと

- 2次元行列のみ(バッチ・多次元は畳まない)
- float32 の素朴なループ(SIMD・BLAS・GPU なし。実物との速度差の源泉)
- LayerNorm は正規化のみ(学習された gain/bias は掛けない)
- 自動微分なし(推論=forward のみ。学習は範囲外)

本文: [教科書の章](../../docs/parts/tensor.md) / 実行: `go test ./llm/tensor/`
