# search/invertedindex — 転置インデックスと TF-IDF / BM25 の最小実装

全文検索の心臓部を決定的な Go で自作する。3 本柱:

1. **索引構築**(`index.go`): 文書をトークン化し、語 → ポスティングリスト
   (DocID と TF の列)の表を作る。検索時に文書本文へ触れない下ごしらえ。
2. **ブール検索**(`index.go`): AND はポスティングリストの積(昇順マージ走査)、OR は和。
3. **ランキング**(`rank.go`): TF-IDF(TF × IDF の和)と BM25(TF の飽和 + 文書長正規化 +
   平滑化 IDF)。k1=1.2, b=0.75 の標準パラメータ。

## 実行

```sh
go test -race -cover ./search/invertedindex/
```

`-race`・`go vet` クリーン、カバレッジ 100%。TF 飽和(3 回出ても 3 倍にならない)と
文書長正規化(同じ 1 回でも短い文書が上)を性質としてテストで固定している。
教科書の該当章は `docs/parts/inverted-index.md`。
