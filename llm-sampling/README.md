# llm-sampling

LLM のトークンサンプリング。トレーシングの head/tail-based sampling は
別パーツ(trace-sampling)で扱う。

## 肝

- LLM の「生成」とは、logits を確率分布に変えて1トークン抽選する行為の繰り返しである
- temperature は softmax 直前の割り算1つ。t<1 で尖り(確定的)、t>1 で平ら(多様)になる
- top-k / top-p / min-p はどれも「尻尾を -Inf に落として renormalize する」フィルタで、
  切る基準(順位 / 累積確率 / 最大確率との比)だけが違う

## 簡略化したこと

- logits は手作りの小語彙(実モデルの出力ではない)
- repetition penalty や frequency penalty などの履歴依存の補正は扱わない
- beam search のような「複数候補を保持する」探索は扱わない

本文: [教科書の章](../docs/parts/llm-sampling.md) / 実行: `go test ./llm-sampling/`
