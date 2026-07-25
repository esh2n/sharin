# llm/attention

self-attention を1ヘッド、tensor 編の行列演算だけで自作。Transformer の心臓。

## 肝

- attention は「各トークンが他のどのトークンにどれだけ注目するか」を計算し、
  注目度で情報を混ぜる。式は softmax(Q·Kᵀ / √d)·V —— まさに行列積の塊
- Q(query=問い)・K(key=見出し)・V(value=中身)の3つに集約される
- GPT の因果マスク: トークンは未来のトークンに注目してはいけない。
  上三角を -Inf にして softmax で 0 にする

## 簡略化したこと

- 1ヘッドのみ(実物はマルチヘッド = 複数の注目を並列に)
- 重みは学習でなく決定的な擬似乱数/恒等で初期化(学習は範囲外)
- 位置エンコーディングなし(トークンの順序情報。mini-GPT 編で足す)
- 出力射影 Wo なし(マルチヘッドを束ねる行列)

本文: [教科書の章](../../docs/parts/attention.md) / 実行: `go test ./llm/attention/`
