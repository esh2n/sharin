<script setup>
import LayersDemo from '../components/LayersDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# RMSNormとSwiGLU

> 実装: [`llm/layers/`](https://github.com/esh2n/sharin/tree/main/llm/layers) / 実行: `go test ./llm/layers/`

<Summary>
mini-GPT のブロックは LayerNorm と GELU でできていた。実物のオープンモデルはほぼ共通して RMSNorm と SwiGLU に置き換えている。前者は正規化から平均を引く工程を省いた簡略化、後者は FFN に入力ごとの通過量を決めるゲートを足した拡張だ。この章では両方を実装し、性質の違いとゲートの挙動をテストで固定して、小さな改良が層数ぶん積み重なる意味を確かめる。
</Summary>

## この章で作るもの

[Transformer全体像](/parts/transformer)で見たとおり、ブロックの骨格は attention と FFN の分業で決まっている。だが骨格の間に挟まる部品、正規化と活性化にも世代交代があった。[tensor](/parts/tensor) で作った LayerNorm と GELU は GPT-2 世代の構成で、Llama 以降のオープンモデルは RMSNorm と SwiGLU にほぼ収束している。

どちらの置き換えも 1 箇所あたりの差は小さい。だが正規化は 1 ブロックに 2 回、FFN は 1 回、それが 32〜100 層積み重なり、その全体を兆単位のトークンで学習する。小さな計算の節約と品質の改善が、規模に掛け算されて効いてくる。この章はその 2 つを実装し、性質の違いを恒等式として確かめる。

<FigureBox caption="ブロック内の世代交代。骨格(attention と FFN の分業)は同じまま、正規化は平均減算を省いた RMSNorm に、FFN はゲート付きの SwiGLU に置き換わった">

```
GPT-2 世代                Llama 以降
LayerNorm                 RMSNorm
 = (x−平均)/標準偏差        = x / RMS(x)      ← 平均を引く工程が消えた
FFN                       SwiGLU FFN
 = GELU(x·W1)·W2           = (SiLU(x·W1) ⊙ x·W3)·W2   ← ゲートが増えた
```

</FigureBox>

肝は3つ:

1. **RMSNorm = 平均を引かない LayerNorm**: RMS で割るだけ。シフト不変性を捨てる代わりに平均の計算と減算が消える
2. **SwiGLU = 値とゲートの積**: 入力から値とゲートを別々に作り、ゲートがチャネルごとの通過量を決める
3. **小差 × 層数 × 学習規模**: どちらも単体では僅差。積み重なる場所にある部品だから置き換える価値が出る

## ① RMSNorm: 引き算を省く

LayerNorm は各行から平均を引き、標準偏差で割る。RMSNorm は平均を引かず、RMS(二乗平均平方根)で割るだけだ:

<<< ../../llm/layers/layers.go#rmsnorm{go}

2 つの正規化の違いは、不変性の違いとしてテストに固定してある。まずどちらもスケール不変で、入力を 2 倍しても出力は変わらない(分母も 2 倍になって打ち消す)。学習中に重みが育って表現の絶対量が変わっても、後段に流れるスケールが暴れないという、正規化の本来の役目はここにある。

分かれるのはシフトだ。LayerNorm は平均を引くので、全要素に +2 しても出力が変わらない。RMSNorm は変わる。つまり RMSNorm は「平均のずれを整える」仕事を放棄している。それでも学習の安定性と最終品質にほぼ差が出ないことが実験で分かり、平均計算と減算が層数 × 2 箇所ぶん消える方が得だと判断された。Llama、Mistral、Qwen、DeepSeek はすべて RMSNorm だ。

もう 1 つ、置く場所の話がある。GPT-2 以降の標準は Pre-LN(attention や FFN の前に正規化)で、mini-GPT もこの形だった。原論文の Post-LN(後に正規化)は深くすると学習が不安定になりやすく、勾配が残差経路を素通りできる Pre-LN が主流になった。RMSNorm への置き換えはこの配置をそのまま引き継いでいる。

## ② SwiGLU: FFN にゲートを足す

通常の FFN は「広げて、曲げて、戻す」の 1 本道だった。SwiGLU は入力から 2 本の経路を作る:

<<< ../../llm/layers/layers.go#swiglu{go}

値側(x·W3)は通常の線形変換で、ゲート側 SiLU(x·W1) がチャネルごとに 0〜1 前後の係数を作って掛かる。ゲートが強い負を出せば SiLU はほぼ 0 になり、値側が何を出していてもそのチャネルは閉じる。テストでは重みを手で置いて、閉じたゲートが出力を消し、開いたゲートが値を素通しすることを固定した。

この形は [container](/parts/container) や [event-loop](/parts/event-loop) で見たような構造の工夫ではなく、純粋に実験の勝者だ。GLU 変種を並べて比較した論文(Shazeer 2020)で SwiGLU が安定して勝ち、PaLM と Llama が採用して標準になった。論文自身が「なぜ効くかの説明は持ち合わせていない」と書いており、理屈より測定が先行した部品として面白い位置にある。

実務上の注意が 1 つ。行列が W1、W3、W2 の 3 枚になるので、同じ中間次元ならパラメータが 1.5 倍になる。実物は中間次元を 4d から約 8d/3 に狭めて、FFN 全体のパラメータ数を GELU 版と揃えている。Llama の中間次元が 11008(= 4096 × 8/3 を 256 の倍数に丸めた値)のような半端な数なのはこの釣り合いの跡だ。

### 動かす

下のデモで両方の性質を確かめられる。「正規化」は同じ入力ベクトルを ×2、+2 と動かし、LayerNorm と RMSNorm の出力がどこで一致しどこで割れるかをバーで見る。「ゲート」は SwiGLU の 1 チャネルについて、ゲート側の入力を動かすと値側の出力がどれだけ通るかを追う。

<LayersDemo />

## アーキテクチャ面接の観点

- **なぜ平均を引かなくてよいか**: 正規化の主目的はスケールの安定。シフトの補正は、続く線形層のバイアスや残差の流れで実質吸収される。「効いていた理由」が思い込みだった部品は削れる、という教訓
- **正規化のコストは帯域**: 計算量としては軽いが、行全体を読む reduce が層数 × 2 回走る。GPU ではメモリ帯域を食う操作で、fused kernel 化の定番対象。RMSNorm は reduce が 1 種類(二乗和)で済む
- **Pre-LN と Post-LN**: Pre-LN は勾配が残差経路を素通りするので深くても安定。Post-LN は表現力でわずかに勝るという報告もあるが、warmup 頼みの不安定さで廃れた
- **SwiGLU のパラメータ会計**: 3 枚行列 × 中間次元 8d/3 ≒ 2 枚 × 4d。「何を増やして何を削って釣り合わせたか」まで言えると設計を理解している証拠になる
- **測定が理屈に先行する**: SwiGLU 採用の根拠は比較実験の勝利で、理論的必然ではない。アーキテクチャの細部は「実験で勝った方を残す」進化を続けている

## メリット・デメリットと実例

| 部品 | 得るもの | 失うもの | 実例 |
|---|---|---|---|
| LayerNorm | シフトも整う | 平均計算のコスト | GPT-2/3、BERT、mini-GPT |
| RMSNorm | reduce 1 種で軽い | シフト補正 | Llama 系、Mistral、Qwen、DeepSeek |
| GELU FFN | 構造が単純 | ゲートなし | GPT-2/3、BERT |
| SwiGLU FFN | 品質(実験で一貫) | 行列 1 枚と実装の複雑さ | PaLM、Llama 系、Mistral、Qwen |

裏どり:

- **RMSNorm(2019)**: Zhang & Sennrich。LayerNorm の再中心化は不要でスケール不変性だけが本質、という主張を実験で示した
- **GLU Variants(Shazeer 2020)**: GEGLU/SwiGLU 等のゲート付き FFN を比較し、SwiGLU の優位を示した論文。「説明はできないが効く」と明記している
- **PaLM(2022)**: SwiGLU + RMSNorm + RoPE + MQA という「現行標準セット」を大規模で揃えた早期の例
- **Llama(2023)**: 同セットをオープンモデルに持ち込み、以後のオープン系がほぼ全てこの構成に追随した

## 簡略化したこと

- **gain(γ)なし**: 実物の RMSNorm は正規化後に学習可能なスケール γ を掛ける。恒等式の検証には不要なので省いた
- **学習なし**: SwiGLU の重みは決定的な擬似乱数。ゲートの構造的性質だけを検証した
- **fused kernel の話は言及のみ**: 帯域律速の最適化は実装しない
- **mini-GPT への組み込みなし**: [mini-GPT](/parts/mini-gpt) は GPT-2 構成のまま。差し替え位置は本文で示した

## 参考資料

- Zhang & Sennrich, [Root Mean Square Layer Normalization](https://arxiv.org/abs/1910.07467)(2019) — RMSNorm 原論文
- Shazeer, [GLU Variants Improve Transformer](https://arxiv.org/abs/2002.05202)(2020) — SwiGLU の比較実験
- Xiong et al., [On Layer Normalization in the Transformer Architecture](https://arxiv.org/abs/2002.04745)(2020) — Pre-LN vs Post-LN
- [Llama: Open and Efficient Foundation Language Models](https://arxiv.org/abs/2302.13971)(2023) — 現行標準セットの採用例
- 実装: [llm/layers](https://github.com/esh2n/sharin/tree/main/llm/layers)
