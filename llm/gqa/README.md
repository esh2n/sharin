# gqa — attention 変種(MHA / MQA / GQA)

attention のヘッド構成の変種を 1 つの実装で表す。K/V をヘッド間で共有することで、生成時の KV キャッシュを縮める。

## 肝

- **3 方式は同じ計算**: MHA(共有なし)/ MQA(全ヘッドで 1 組)/ GQA(グループごと 1 組)の違いは「Q ヘッドがどの K/V を引くか」の対応表 `KVHeadFor` だけ。テストでは KV 重みを揃えると 3 方式の出力が一致することを固定している
- **KV キャッシュの式**: `2 × 系列長 × KVヘッド数 × ヘッド次元`。NHeads が現れないので、KV ヘッドを 32 → 8 に減らせばキャッシュは 1/4 になる
- **causal 性**: 未来のトークンを書き換えても過去の出力は 1 ビットも動かない(テストで固定)

## 使い方

```go
a, _ := gqa.New(gqa.Config{DModel: 4096, NHeads: 32, NKVHeads: 8}) // GQA
out := a.Forward(x)                    // (seq, DModel)
a.KVHeadFor(5)                         // Q ヘッド 5 が引く KV ヘッド番号
cfg.KVCacheFloats(8192)                // 生成時にキャッシュする float 数
```

## 簡略化したこと

- **出力射影(Wo)なし**: ヘッド連結までを実装。実物は連結後にもう 1 つ射影が入る
- **MLA 未実装**: DeepSeek の低ランク圧縮(latent への射影)は章で解説のみ
- **学習なし**: 重みは決定的な擬似乱数。構造の検証が目的

## 章

教科書: [attention変種(MQA/GQA)](https://sharin-2a1.pages.dev/parts/attention-variants)

実行: `go test ./llm/gqa/`
