# quant — 重みの量子化

学習済みの重みを少ないビットの整数に写してメモリを縮める量子化を最小構成で実装する。

## 肝

- **対称量子化**: `scale = max|x| / (2^(bits-1)-1)`、`q = round(x/scale)`。0 を厳密にコード 0 へ写す。重み向き。誤差が量子化ステップの半分以内に収まることをテストで固定
- **非対称量子化**: `[min,max]` を `[0, 2^bits-1]` に写す。片側に偏った分布(活性値)で全コード域を使え、対称より誤差が小さい
- **per-tensor vs per-channel**: 行ごとにスケールが違う行列では、行単位で scale を決める per-channel が per-tensor に勝つ(テストで固定)。実物の重み量子化は per-channel が標準
- **メモリ会計**: fp32=4byte → int8=1byte(1/4) → int4=0.5byte(1/8)。`MemoryBytes` / `CompressionRatio`

## 使い方

```go
q := quant.QuantizeSymmetric(weights, 8)   // int8 対称
back := q.Dequantize()                       // 復元(丸め誤差あり)
a := quant.QuantizeAsymmetric(acts, 8)       // 偏った分布向け
pc := quant.QuantizeMatrixPerChannel(rows, 4) // 行ごとscale
quant.MemoryBytes(70e9, 4)                    // 4-bit 70B の実バイト数
```

## 簡略化したこと

- **round-to-nearest のみ**: 実物の GPTQ/AWQ は誤差を後続に伝播させて補正する。ここは素朴な最近傍丸め
- **group-wise なし**: 実物は 1 行をさらに 64〜128 要素のグループに分けて scale を持つ。ここは行単位まで
- **量子化行列積なし**: int8 のまま積を取る高速化は実装せず、復元して比較する形に留めた
- **外れ値処理なし**: LLM.int8() の外れ値を fp16 に残す混合精度は章で言及のみ

## 章

教科書: [量子化](https://sharin-2a1.pages.dev/parts/quantization)

実行: `go test ./llm/quant/`
