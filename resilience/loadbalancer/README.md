# loadbalancer — 負荷分散の選び方

複数の同等なバックエンドに、どのリクエストをどこへ振るか。代表的な 4 方式を最小構成で実装し、なぜ P2C(power of two choices)が効くかを固定テストで確かめる。

## 肝

- **ラウンドロビン**: 順番に回すだけ。状態を見ないので速いが、遅い台にも均等に送る
- **最少接続(least-conn)**: 全台の処理中数を見て最も空いた台へ。偏りは最小だが、全接続数を正確に知る必要があり、分散した振り分け役が同じ「最少」を見て一斉に殺到する群集効果(herd)を起こす
- **P2C(power of two choices)**: 無作為に 2 台だけ選び、軽い方へ。全体を知らずとも最大負荷が劇的に下がる。ランダムの最大負荷が平均+O(log n / log log n)なのに対し、2 択にするだけで O(log log n)まで縮む
- **一貫ハッシュ**: キーのハッシュをリング上に置き、同じキーは常に同じ台へ。台の増減で振り先が動くのは 1/n 程度で済む(素朴な mod n は大半が動く)
- **決定性**: P2C の乱数は注入した擬似乱数源(NewRand)。実 rand を使わずテストが再現的

## 効果の固定(テスト)

- `TestP2CBeatsRandom`: 20 台に 2000 リクエスト。平均 100 に対し、ランダムの最大は 111、P2C の最大は 101。2 択にするだけで偏りがほぼ消える
- `TestConsistentHashMinimalRemap`: 5→6 台に増やしたとき、振り先が変わるキーは 1000 中 252(mod n なら約 830 が動く)

## 使い方

```go
b := loadbalancer.New([]string{"a", "b", "c"}, loadbalancer.P2C, loadbalancer.NewRand(1))
i := b.Pick("")   // 振り先インデックス
b.Acquire(i)      // 処理中カウント +1
defer b.Release(i) // 完了で -1
// 一貫ハッシュは key で振る:
h := loadbalancer.New(ids, loadbalancer.ConsistentHash, nil)
i = h.Pick("user-42")
```

## 簡略化したこと

- **健康チェックなし**: 落ちた台を外す仕組みは扱わない(実物は失敗した台を一時的に候補から外す)
- **重み付けなし**: 台ごとの性能差を反映する重みは持たない
- **論理的な active カウント**: 実際の並行実行でなく Acquire/Release で数えるだけ
- **一貫ハッシュの仮想ノードは固定 64 個**: 実物は台の重みに応じて数を変える

## 章

教科書: [ロードバランサ](https://sharin-2a1.pages.dev/parts/load-balancer)

実行: `go test ./resilience/loadbalancer/`
