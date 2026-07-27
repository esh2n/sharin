# moe — Mixture of Experts

FFN を複数の expert に分割し、トークンごとにルータが上位 k 個だけを選んで通す MoE 層を最小構成で実装する。

## 肝

- **総量とアクティブ量の分離**: `TotalParams`(メモリに載る総量)は expert 数に比例、`ActiveParams`(1 トークンの計算)は TopK に比例。「大きいのに速い」の正体。テストで会計を固定
- **ルーティング**: ルータのスコア上位 TopK を選び、選ばれた分だけで softmax を取り直して混合重みにする。expert 1 個・top-1 なら普通の FFN と完全一致(テストで固定)
- **負荷分散損失**: N·Σ f_e²(Switch Transformer の簡略版)。均等で 1、1 つに集中で N。学習でルータの崩壊(全部同じ expert に流す)を防ぐために本来の損失へ足す

## 使い方

```go
m, _ := moe.New(moe.Config{DModel: 64, DHidden: 256, NExperts: 8, TopK: 2})
out, stats := m.Forward(x)             // トークンごとに 2 expert だけ計算
loss := moe.LoadBalanceLoss(stats)     // 1(均等)〜 NExperts(集中)
cfg.TotalParams() / cfg.ActiveParams() // ≈ 総量/アクティブの比
```

## 簡略化したこと

- **capacity factor なし**: 実物は expert ごとの受け入れ上限とあふれ処理(drop / 再ルーティング)を持つ
- **専門家並列なし**: 実物は expert を GPU 間に分散し、トークンを all-to-all 通信で送る
- **学習なし**: 補助損失は値の計算のみ。逆伝播はスコープ外
- **共有 expert なし**: DeepSeek 系の「全トークンが必ず通る共有 expert + 選択制 expert」構成は章で解説

## 章

教科書: [MoE](https://sharin-2a1.pages.dev/parts/moe)

実行: `go test ./llm/moe/`
