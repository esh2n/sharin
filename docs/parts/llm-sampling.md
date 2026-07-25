<script setup>
import SamplingDemo from '../components/SamplingDemo.vue'
import Summary from '../components/Summary.vue'
</script>

# LLM Sampling

> 実装: [`llm-sampling/`](https://github.com/esh2n/sharin/tree/main/llm-sampling) / 実行: `go test ./llm-sampling/`

<Summary>
LLM が次の1トークンを選ぶところを、モデルが吐く生スコア(logits)から自分で計算する。softmax で確率にして、temperature で尖らせたり平らにしたり、top-p などで候補を絞って、最後に1つ抽選する。ChatGPT の応答の「性格」は、実はモデル本体と同じくらいこの選び方で決まっている。
</Summary>

::: info 「サンプリング」の同名別分野に注意
この章は **LLM がテキストを生成するときのトークンサンプリング**の話。
分散トレーシングの head-based / tail-based sampling(「全リクエストを記録できない中で
どのトレースを残すか」)は別問題なので、[Trace Sampling](./trace-sampling) で独立して扱う。
:::

## この章で作るもの

LLM の出力層で行われているサンプリングを、logits から自分で計算する。

1. **softmax** — logits を確率分布に変換する(数値安定化つき)
2. **temperature** — 分布を尖らせる / 平らにする
3. **top-k / top-p / min-p** — 分布の尻尾を切り落とす3つの流儀
4. **greedy / sample** — 分布から1トークン決める

各節にはその場で動かせるデモを置いてある。今回のデモは Worker すら不要で、
すべてブラウザ内の計算(Go 版と同じロジックの JS ミラー)で動いている。

この章の肝は3つ。

- LLM の「生成」とは、**logits を確率分布に変えて1トークン抽選する行為の繰り返し**である
- temperature は softmax 直前の**割り算1つ**。魔法のパラメータではない
- top-k / top-p / min-p はどれも「**尻尾を -Inf に落として renormalize する**」フィルタで、
  切る基準(順位 / 累積確率 / 最大確率との比)だけが違う

## 前提: トークン・語彙・logits

- **トークン**: LLM が文章を扱う最小単位。単語より少し細かい(「食べました」が
  「食べ」「まし」「た」のように割れる)。分割の仕組み自体は llm 編(BPE)で作る
- **語彙(vocabulary)**: モデルが知っている全トークンの一覧。GPT 系で5万〜25万種類
- **logits**: 語彙の各トークンに対する「次に来る確からしさ」の生スコア(実数の列)。
  確率ではない(合計1でも0〜1でもない)。これを確率に直すのが softmax で、この章の出発点

## モデルの出力は「次のトークンの点数表」

Transformer が1ステップで出すのは、語彙の全トークンに対する **logits**(実数の点数の列)だけ。
「明日の天気は」まで読んだモデルなら、「晴れ」に 4.0、「曇り」に 3.3、「猫」に -1.0、
のような点数を全語彙ぶん並べる。文章が流れるように出てくるのは、
この点数表から1個選んで文脈に足し、また点数表を出す、を繰り返しているだけ。

つまり ChatGPT の応答の「性格」は、モデル本体と同じくらい
**点数表から1個選ぶ方法**(=サンプリング)に支配されている。

## softmax: logits を確率にする

<<< ../../llm-sampling/sampling.go#softmax{go}

見どころは `maxLogit` を引いている部分。logits が 1000 を超えると `exp(1000)` が
overflow して全部 `+Inf` になるが、softmax は「差」しか見ないので、
全要素から最大値を引いてから exp しても結果は変わらない。
テストでは `logits = [1000, 1000]` を入れてこの安定化を検証している。

もう1つの仕掛けが `-Inf`。`exp(-Inf) = 0` なので、
**logit を -Inf にする = そのトークンの確率を0にして renormalize する**ことになる。
この後の top-k / top-p / min-p はすべてこの性質を使ったフィルタとして書ける。

## temperature: 分布の尖りを操作する

<<< ../../llm-sampling/filters.go#temperature{go}

やっていることは割り算1つだが、softmax が exp(指数関数)を通すため効果は劇的で、

- **t → 0**: 差が無限に拡大され、最大 logit のトークンが確率1に近づく(greedy と同じ)
- **t = 1**: モデルが出した分布そのまま
- **t → 大**: 差が消えて一様分布に近づく(でたらめ)

**試してみる**: スライダーを 0.1 まで下げると「晴れ」がほぼ100%になり、
3.0 まで上げると「猫」や「無」にも現実的な確率がつく。
何回か抽選して、低温では同じ結果ばかり、高温ではばらつくことを確認してほしい。

<SamplingDemo :controls="['temperature']" />

なお `t = 0` は0除算なので、実装では受け付けずに greedy(argmax)を使う。

<<< ../../llm-sampling/sampling.go#greedy{go}

## top-k: 上位k個しか見ない

温度を上げて多様性を出すと、副作用として「猫」のような明らかにおかしいトークンにも
確率が漏れる。そこで**抽選の前に候補を絞る**のがフィルタ系の役割。
最も素朴な top-k は、logit の上位 k 個だけ残して残りを -Inf に落とす。

<<< ../../llm-sampling/filters.go#topk{go}

**試してみる**: k を下げていくと下位のトークンから順に消え、
残った候補だけで確率が再分配(renormalize)される。k=1 は greedy と同じ。

<SamplingDemo :controls="['topk']" />

top-k の弱点は k が**固定**なこと。分布が尖っている(答えがほぼ決まっている)場面では
k=40 は緩すぎ、分布が平らな(どう続けてもいい)場面では k=40 は厳しすぎる、
ということが同じ文章の中で起きる。

## top-p: 累積確率で切る(nucleus sampling)

top-k の「個数固定」問題への答えが top-p。確率の大きい順に足していき、
累積が p に達するまでの最小の集合(nucleus)だけを残す。

<<< ../../llm-sampling/filters.go#topp{go}

**試してみる**: p を下げると尻尾から消えていくのは top-k と似ているが、
残る**個数**が分布次第で変わるのが違い。temperature と組み合わせたとき、
尖った分布では1〜2個、平らな分布では多数が残る「適応的な」切り方になる。

<SamplingDemo :controls="['topp']" />

## min-p: 最大確率との比で切る

比較的新しい流儀。「最大確率の minP 倍未満のトークンは切る」という相対閾値で、
top-p の累積計算より単純なのに、分布の尖り具合に適応する性質は保たれる。

<<< ../../llm-sampling/filters.go#minp{go}

**試してみる**: min-p = 0.1 なら「1位の10分の1未満の泡沫候補は切る」という意味になる。
top-p と見比べると、切れ方が「累積」ではなく「個々の確率」で決まることがわかる。

<SamplingDemo :controls="['minp']" />

## 抽選: 確率分布から1個選ぶ

フィルタを通した最終分布から実際に1トークン選ぶのが inverse CDF 法。
[0,1) の乱数を引き、累積確率が乱数を超えた位置のトークンを返す。

<<< ../../llm-sampling/sampling.go#sample{go}

テストでは乱数生成器を「固定値を返す関数」に差し替えて、
境界(累積0.2ちょうど、など)で正しいトークンが選ばれることを確認している。
時計を注入した rate-limiter 編と同じ、**非決定的なものを外から注入してテスト可能にする**定石。

## 全部つなげる

実際の推論エンジン(llama.cpp など)では、これらが1本のパイプラインになっている:

```
logits → ÷temperature → top-k → top-p → min-p → softmax → 抽選
```

**試してみる**: 全部のつまみを同時に動かして、
「temperature で形を作り、フィルタで尻尾を切り、最後に抽選する」流れを確認してほしい。

<SamplingDemo :controls="['temperature', 'topk', 'topp', 'minp']" />

## 4方式の比較

| 方式 | 何で切るか | 特徴 | 実例 |
|---|---|---|---|
| greedy | 切らない(argmax) | 決定的。同じ入力から常に同じ出力 | temperature=0 指定。コード生成・分類など再現性重視の用途 |
| temperature | 切らない(形を変える) | 尖り/多様性のトレードオフの主役 | ほぼすべての LLM API の基本パラメータ |
| top-k | 順位(固定k個) | 単純。分布の形に適応しない | Hugging Face transformers の既定(k=50) |
| top-p | 累積確率 | 分布の形に適応する。長年の実務標準 | OpenAI / Anthropic API の `top_p` |
| min-p | 最大確率との比 | 実装が単純で適応的。近年の推論エンジンで採用増 | llama.cpp・vLLM 等の OSS 推論エンジン |

## 簡略化したこと

- **logits は手作りの8語彙**: 実モデルでは数万〜数十万次元。ただし計算は完全に同じ
- **履歴依存の補正なし**: repetition penalty / frequency penalty は「過去に出したトークンの
  logit を下げる」処理で、本質は同じ引き算・割り算
- **1トークンずつの抽選のみ**: beam search のような複数候補の探索は扱わない
- **デモの乱数は Math.random**: 実務では seed 固定で再現性を作る(Go 版は rng 注入済み)

## 参考資料

- [The Curious Case of Neural Text Degeneration](https://arxiv.org/abs/1904.09751) — top-p(nucleus sampling)の原典。greedy/beam が繰り返しに陥る観察も面白い
- [Min-p Sampling](https://arxiv.org/abs/2407.01082) — min-p の提案論文
- [llama.cpp sampling](https://github.com/ggml-org/llama.cpp/tree/master/common) — 実務のパイプラインでフィルタがどう直列されているかが読める
