# kvcache — 推論高速化(KV キャッシュと speculative decoding)

LLM の生成を速くする 2 本柱を最小構成で実装する。

## 肝

- **KV キャッシュ**: K/V は位置ごとに不変なので、一度作って保存すれば新トークンぶんだけ作ればよい。射影回数が二次 → 線形になることを、おもちゃの attention モデルの実測カウンタで固定している。生成列はキャッシュの有無で 1 トークンも変わらない
- **speculative decoding**: 軽いドラフトが γ トークン先読みし、本命は 1 パスでまとめて検証。一致した分だけ一気に進み、不一致は本命の訂正で置き換える。生成結果は本命単独の greedy 生成と完全一致し、節約されるのは本命のパス数だけ(テストで固定)
- **ドラフトの質はパス数にだけ効く**: 完全一致ドラフトなら 1 パスで γ+1 トークン、常に外すドラフトでも 1 パス 1 トークン(本命単独と同等)。正しさはどちらでも変わらない

## 使い方

```go
m := kvcache.NewModel(16, 8)
seq, ops := m.GenerateWithCache(prompt, 20)   // ops は K/V 射影回数(線形)
seq2, ops2 := m.GenerateNoCache(prompt, 20)   // 同じ列、ops2 は二次

out, passes := kvcache.Speculative(target, draft, prompt, 24, 4)
// out == kvcache.GenerateGreedy(target, prompt, 24)、passes ≤ 24
```

## 簡略化したこと

- **greedy 限定の speculative**: 実物はサンプリング分布を保つ棄却サンプリング(受理確率 min(1, p/q))。greedy では「一致 = 受理」に退化するのでその形で実装した
- **1 ヘッド・要素積射影のおもちゃモデル**: 射影回数の計測が目的。実物の多層・多ヘッドでも位置あたりの K/V 不変性は同じ
- **メモリ管理なし**: PagedAttention のようなキャッシュのページ管理は章で解説のみ

## 章

教科書: [推論高速化](https://sharin-2a1.pages.dev/parts/inference)

実行: `go test ./llm/kvcache/`
