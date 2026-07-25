# llm/gpt

mini-GPT の forward pass を tensor 編の行列演算だけで自作。LLM 編の集大成。

## 肝

- attention 編の1ヘッドを、Transformer ブロック(マルチヘッド + FFN + 残差 + LayerNorm)に
  仕立て、それを重ねる。埋め込み → ブロック×N → logits で「次トークンの予測」ができる
- 出力 logits を llm-sampling 編に渡せば実際にテキストが生成される(自己回帰)
- あなたが作ったこの200行と GPT-4 の違いは、本質的には**規模**。同じ Transformer

## 簡略化したこと

- 重みは乱数(未学習)。だから生成は意味をなさない。「動く forward pass」の理解が目的
- BPE トークナイザは別(ここはトークンID列を直接受ける)
- KV cache なし(生成のたびに全系列を再計算。実物は過去の K,V を使い回して高速化)
- 学習(backward・最適化)なし。推論のみ

本文: [教科書の章](../../docs/parts/mini-gpt.md) / 実行: `go test ./llm/gpt/`
