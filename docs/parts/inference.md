<script setup>
import InferenceDemo from '../components/InferenceDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 推論高速化(KVキャッシュ / speculative decoding)

> 実装: [`llm/kvcache/`](https://github.com/esh2n/sharin/tree/main/llm/kvcache) / 実行: `go test ./llm/kvcache/`

<Summary>
生成は 1 トークンずつ進み、各ステップで過去全トークンの K と V が要る。毎回作り直すと合計コストは系列長の二次だが、位置ごとに不変なので保存して使い回せば線形に落ちる。これが KV キャッシュ。もう 1 つの柱が speculative decoding で、軽いドラフトに先読みさせて本命が一括検証し、出力を一切変えずにパス数を減らす。両方を実装し、その保証をテストで固定する。
</Summary>

## この章で作るもの

[mini-GPT](/parts/mini-gpt) の生成ループは、1 トークン足すたびに系列全体を forward し直していた。動きはするが、生成が進むほど 1 ステップが重くなる。実物の推論サーバがこの形で動いていたら、チャットの応答は後半ほど遅くなっていくはずだ。実際にはそうならない。この章では、その理由である 2 つの仕組みを作る。

1 つ目の KV キャッシュは「同じ計算を二度しない」だ。位置 i のトークンの K と V は、その位置のトークンが決まった瞬間から不変で、後から何を生成しても変わらない(因果マスクのおかげで未来は過去に影響しない)。なら一度計算して保存すればいい。2 つ目の speculative decoding は「重いモデルの呼び出し回数そのものを減らす」で、軽いモデルの下書きを重いモデルが一括採点する。

<FigureBox caption="KV キャッシュ。位置ごとの K/V は不変なので、各ステップで作るのは新トークンの 1 列だけ。キャッシュ無しでは毎ステップ全列を作り直すことになる">

```
キャッシュ無し                        キャッシュ有り
step1  [K/V][K/V][K/V]                [K/V][K/V][K/V]  ← プロンプトを一度だけ
step2  [K/V][K/V][K/V][K/V]           [ 再利用      ][K/V]  ← 新トークンだけ
step3  [K/V][K/V][K/V][K/V][K/V]      [ 再利用           ][K/V]
        毎回ぜんぶ作り直し(二次)        1 位置 1 回(線形)
```

</FigureBox>

先に押さえることが3つある。

1. **K/V は位置ごとに不変**: 因果マスクにより過去の計算は未来に依存しない。だから保存が成立する
2. **速さの代償はメモリ**: キャッシュは系列長 × KV ヘッド数に比例して太る。[attention 変種](/parts/attention-variants)の GQA はこのメモリを減らす話だった
3. **speculative は出力を変えない**: ドラフトの質は速度にだけ効き、生成結果は本命単独と完全一致する。品質と速度の分離が設計の核心

## ① KV キャッシュ: 二次を線形にする

効果を実測できるよう、K/V の射影回数を数えられるおもちゃの attention モデルを作り、キャッシュ無し / 有りの 2 つの生成ループを書く:

<<< ../../llm/kvcache/kvcache.go#cache{go}

<<< ../../llm/kvcache/kvcache.go#generate{go}

テストが固定しているのは 2 点だ。まず、生成列はキャッシュの有無で 1 トークンも変わらない。キャッシュは近似ではなく、同じ計算の重複を省いているだけだからだ。次に射影回数で、プロンプト 3 トークンから 20 トークン生成すると、キャッシュ無しは 250 回、有りは 22 回になる。系列が伸びるほどこの差は開く。

代償はメモリで、そのサイズの式と削減策(GQA/MQA)は [attention 変種](/parts/attention-variants)で扱った。実務ではさらに、リクエストごとに長さの違うキャッシュを GPU メモリにどう詰めるかが問題になり、OS のページングを模した PagedAttention(vLLM)が標準になっている。断片化を防いでメモリ利用率を上げる話で、[バッファプール](/parts/buffer-pool)や [OS](/parts/os) で見た資源管理の考え方がそのまま再登場する。

## ② speculative decoding: 下書きと一括検証

KV キャッシュを入れても、生成が 1 トークンずつ逐次であることは変わらない。1 トークンごとに巨大モデルの全層を 1 回通す必要があり、このパス数が応答速度を決める。speculative decoding はここに切り込む。

仕組みは校閲に似ている。軽いドラフトモデルが γ トークンぶん下書きを書き、本命モデルが 1 回のパスでまとめて検証する。Transformer は複数位置を並列に採点できるので、γ 個の検証は 1 個の生成とほぼ同じコストで済む。decoder-only の学習が速いのと同じ並列性を、推論の検証側で使い直していることになる:

<<< ../../llm/kvcache/kvcache.go#speculative{go}

一致した分は一気に採用し、最初の不一致は本命の訂正で置き換える。全部一致なら同じパスからボーナスの 1 トークンも得られる。

重要な保証が 1 つある。出力は本命単独の生成と完全に一致する。テストでは、完全一致するドラフト・半分ほど一致するドラフト・ほぼ外すドラフトの 3 通りで、出力列が本命 greedy と同一であることを固定した。ドラフトの質が効くのはパス数だけで、良いドラフトなら 1 パスで最大 γ+1 トークン、最悪でも 1 パス 1 トークン(本命単独と同等)に収まる。速くしても品質が落ちない、という珍しい性質はここから来る。

### 動かす

下のデモは両方をそのまま動かす。「KVキャッシュ」は生成を 1 ステップずつ進めながら、キャッシュ無し / 有りの射影回数が二次と線形で開いていく様子を数える。「speculative」はドラフトの下書き → 本命の一括検証 → 採用と訂正の 1 ラウンドずつを追い、ドラフトの精度を切り替えるとパス数だけが変わって出力が変わらないことを確かめられる。

<InferenceDemo />

## 設計の観点

- **なぜキャッシュが成立するか**: 因果マスクで過去の K/V が未来に依存しないから。双方向の encoder では成立しない。アーキテクチャの選択が推論の効率を決めている例
- **prefill と decode の 2 相**: プロンプト全体の K/V を一括で作る prefill は並列でき計算律速、1 トークンずつの decode はメモリ帯域律速。推論サーバはこの 2 相を別々に最適化する(投機は decode 側の技法)
- **バッチングとの関係**: decode はメモリ律速なので、複数リクエストを同時に流すと GPU の計算が余っている分だけ得になる。continuous batching(vLLM)が標準
- **speculative の損益分岐**: 得 = 一致率 × γ、損 = ドラフトの計算 + 不一致時の捨て。一致率が低い難しいテキストでは逆効果もある。ドラフトは本命の 1/10 以下のサイズが目安
- **greedy とサンプリングの違い**: サンプリング時は「一致 = 受理」でなく受理確率 min(1, p/q) の棄却サンプリングを使うと、出力分布そのものが本命と一致することが証明できる(原論文の主定理)

## メリット・デメリットと実例

| 技法 | 効果 | 代償 | 実例 |
|---|---|---|---|
| KV キャッシュ | ステップコスト二次 → 線形 | メモリ(系列長に比例) | 全推論エンジン(必須の前提) |
| PagedAttention | キャッシュのメモリ利用率向上 | 実装の複雑さ | vLLM |
| speculative decoding | パス数削減(2〜3 倍の報告) | ドラフトの計算・保守 | Gemini / GPT 系 API の内部、llama.cpp |
| self-speculative 系 | ドラフト不要で先読み | 追加ヘッドの学習 | Medusa、EAGLE |

裏どり:

- **vLLM / PagedAttention(2023)**: KV キャッシュを固定サイズブロックに割ってページ管理し、スループットを数倍にした論文と OSS。推論サーバの事実上の標準になった
- **speculative decoding(2023)**: Leviathan ら(Google)と Chen ら(DeepMind)がほぼ同時に発表。出力分布を変えない証明つきの高速化として広まった
- **llama.cpp**: ローカル推論でもドラフト付き生成を実装。小型の同系列モデルをドラフトに使う構成が定番
- **Medusa(2024)**: 本命モデル自身に予測ヘッドを足してドラフトを不要にする変種。投機の考え方が派生を生んでいる例

## 簡略化したこと

- **greedy 限定**: サンプリング分布を保つ棄却サンプリング版は設計の観点で触れるに留めた
- **おもちゃのモデル**: 1 ヘッド・要素積射影。射影回数の計測が目的で、多層・多ヘッドでも位置あたりの不変性は同じ
- **prefill の並列化なし**: プロンプトの K/V も逐次に数えている
- **メモリ管理なし**: PagedAttention のブロック割当は解説のみ

## 参考資料

- Leviathan et al., [Fast Inference from Transformers via Speculative Decoding](https://arxiv.org/abs/2211.17192)(2023) — 投機的デコードの原論文
- Chen et al., [Accelerating LLM Decoding with Speculative Sampling](https://arxiv.org/abs/2302.01318)(2023) — DeepMind の同時発表版
- Kwon et al., [Efficient Memory Management for LLM Serving with PagedAttention](https://arxiv.org/abs/2309.06180)(2023) — vLLM
- [llama.cpp の speculative 実装](https://github.com/ggml-org/llama.cpp) — ローカルでの実例
- 実装: [llm/kvcache](https://github.com/esh2n/sharin/tree/main/llm/kvcache)
