<script setup>
import LlmComponentDemo from '../components/LlmComponentDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# LLMアーキテクチャ図鑑

<Summary>
「Transformer」「GPT」「Claude」——名前は聞くが、何がどう違うのか。この章はコードを書かず、地図を広げる。まず"そもそも Transformer とは何か・なぜ生まれたか"を RNN の限界から説き起こし、次に LLM を構成する部品(トークナイザ・attention・正規化…)をカタログにし、最後に各社モデル(GPT / Claude / Gemini / Llama 系)が何を選んでいるかを並べる。実装した mini-GPT が、この広い地図のどこにいるのかが分かる。
</Summary>

## この章について

[行列演算](./tensor)→[attention](./attention)→[mini-GPT](./mini-gpt) で
Transformer を部品から組んだ。でも「そもそも Transformer とは何で、なぜそれが主流なのか」
「Claude や Gemini は GPT と何が違うのか」——その全体像はまだ地図がない。

この章は**コードを書かない解説の章**。作ったものを、より広い文脈に置き直す。

## そもそも Transformer とは何か

Transformer は2017年の論文 "Attention Is All You Need" で登場した、
**文章を扱うニューラルネットの設計**。今の LLM はほぼ全部これがベース。
なぜこれが天下を取ったのか——その前の主流だった **RNN** の限界を知ると分かる。

### RNN の限界: 逐次処理

Transformer の前は **RNN / LSTM** が主流だった。RNN は文章を**単語を1つずつ順番に**
読む。「私は」を読んで状態を更新、「猫が」を読んで更新…と、前の単語の処理が
終わらないと次に進めない。

<FigureBox caption="RNN と Transformer の違い。RNN は1語ずつ順番(逐次)。Transformer は全語を一度に(並列)。GPU は並列計算が得意なので、Transformer は桁違いに速く大量に学習できる">

```
RNN:         [私は] → [猫が] → [好き]     1つ終わってから次(逐次)
              状態    状態     状態         遅い。長い文で最初を忘れる

Transformer: [私は] [猫が] [好き]          全部を一度に見る(並列)
                ╲    │    ╱                attention で互いに直接注目
                 全単語が全単語を見る       速い。遠い語も直接繋がる
```

</FigureBox>

RNN には2つの弱点があった:

1. **遅い**: 逐次処理なので、GPU の並列計算力を活かせない。大規模化できない
2. **長距離が苦手**: 文が長いと、最初の方の情報が薄れる(状態を1本のベクトルに
   押し込むので)

Transformer の [attention](./attention) はこれを両方解く。全単語が全単語を
**一度に・直接**見るので、並列計算できて速く、遠く離れた語も直接繋がる。
「文章を並列で処理できるようになった」——これが LLM 爆発の技術的な起点。

## Transformer の3系統

同じ Transformer でも、使い方で3系統に分かれる。

| 系統 | 何をする | 代表 | 用途 |
|---|---|---|---|
| **decoder-only** | 左から右へ次の語を予測(因果マスク) | GPT, Llama, Claude | **生成**(チャット、文章作成) |
| **encoder-only** | 文全体を一度に理解(マスクなし) | BERT | 分類・検索(埋め込み) |
| **encoder-decoder** | 理解して変換 | T5, 初期の翻訳 | 翻訳・要約 |

[mini-GPT](./mini-gpt) で作ったのは **decoder-only**。因果マスクで「未来を見ない」
のがそれ。今のチャット LLM はほぼ全部 decoder-only——1つの仕組みで、
質問応答も翻訳も要約もコード生成も、全部「次の語を予測する」に還元できると分かったから。

## LLMコンポーネント図鑑

Transformer 本体は同じでも、各部品には選択肢がある。mini-GPT で使った素朴な部品と、
実物が使う改良版を並べる。

- **トークナイザ**: 文字列をトークンに切る。**BPE**(GPT系)、**SentencePiece**(多言語)。
  [llm-sampling](./llm-sampling) 編で触れた「トークン」を作る部分
- **位置エンコーディング**: mini-GPT は「位置ベクトルを足す」絶対方式。実物は **RoPE**
  (回転位置埋め込み)——長い文脈でも位置関係を保て、長コンテキストの立役者
- **attention の変種**:
  - **MHA**(マルチヘッド): mini-GPT のこれ。各ヘッドが独立の Q/K/V
  - **MQA/GQA**: K,V を複数ヘッドで共有。メモリ(KV cache)を大幅削減。長文生成が現実的に
  - **MLA**: DeepSeek の圧縮版。さらにメモリ効率が良い
- **正規化**: mini-GPT は **LayerNorm**。実物は **RMSNorm**(平均を引かない簡略版。速い)
- **活性化**: mini-GPT は **GELU**。実物は **SwiGLU**(ゲート付き。性能が少し上)
- **MoE**(Mixture of Experts): FFN を複数の「専門家」に分け、トークンごとに一部だけ使う。
  総サイズは巨大でも計算は一部——「大きいのに速い」を実現

**見比べる**: 各社モデルがどの部品を選んでいるかの対応表。オープンモデル(緑)は
公開情報、GPT-4/Claude/Gemini(灰)は中身が非公開。

<LlmComponentDemo />

RoPE・GQA・RMSNorm・SwiGLU は、GPT-2(mini-GPT と同じ素朴な構成)以降に
「大きく・長く・速く」するために生まれた改良。オープンモデルはほぼ共通してこの構成に
収束している。**土台の Transformer は同じ**で、その上の部品の磨き上げが進んでいる。

## 各社モデルの系統

<FigureBox caption="LLM の大まかな系統。2017年の Transformer から分岐し、decoder-only(GPT系)が生成の主流に。近年はオープンモデルが猛追">

```
Transformer (2017)
   ├─ BERT系(encoder)          … 検索・埋め込みで今も現役
   ├─ T5系(encoder-decoder)    … 翻訳・要約
   └─ GPT系(decoder) ─────────── 生成の主流
        ├─ OpenAI: GPT-2/3/4 → o1/o3(推論)
        ├─ Anthropic: Claude(Constitutional AI で安全性)
        ├─ Google: Gemini(マルチモーダル前提)
        └─ オープン: Llama(Meta) / Mistral / DeepSeek / Qwen
                     ↑ 重みが公開され、誰でも動かせる・改造できる
```

</FigureBox>

- **クローズド**(GPT-4 / Claude / Gemini): 最高性能だが中身は非公開。API で使う
- **オープン**(Llama / Mistral / DeepSeek / Qwen): 重みが公開。自分のマシンで動かせる、
  改造できる、ファインチューニングできる。性能はクローズドに肉薄

各社の差は、アーキテクチャそのものより **学習データ・学習方法(RLHF の質)・安全性の
作り込み**([mini-GPT](./mini-gpt) の「学習の3段階」)にある。Claude の
Constitutional AI、OpenAI の RLHF など、後段の工夫が"性格"を決める。

## Transformer 以外の潮流

Transformer が主流だが、挑戦者もいる。

- **Mamba / SSM**(状態空間モデル): attention の「全語が全語を見る」計算は
  文長の2乗のコストがかかる。SSM はそれを線形にする試み。超長文で有利かもしれない
- **拡散モデル**(Diffusion): 画像生成(Stable Diffusion, DALL-E)の主流。
  ノイズから徐々に画像を作る。LLM とは別系統だが、テキストへの応用も研究中
- **マルチモーダル**: テキスト・画像・音声を同じ Transformer に流す。
  Gemini や GPT-4V。「全部をトークンにして混ぜる」のが基本発想

## まとめ: あなたの mini-GPT は地図のどこにいるか

- **系統**: decoder-only(GPT系)。今のチャット LLM の主流
- **部品**: GPT-2 相当の素朴な構成(LayerNorm・絶対位置・MHA・GELU)。
  実物の改良(RoPE・GQA・RMSNorm・SwiGLU・MoE)は入れていない
- **規模**: 数万〜数百万パラメータ。フロンティアは兆単位
- **学習**: なし(乱数)。フロンティアは事前学習 + SFT + RLHF

つまり、あなたが作ったのは「**2019年の GPT-2 の、うんと小さい未学習版**」。
そこから現在のフロンティアまでの距離が、この地図で見える。骨格は同じ——
違いは規模・部品の磨き・学習・データ。魔法はどこにもない。

## 参考資料

- [Attention Is All You Need (2017)](https://arxiv.org/abs/1706.03762) — Transformer 原論文
- [The Illustrated Transformer](https://jalammar.github.io/illustrated-transformer/) — 図解の決定版
- [A Survey of Large Language Models](https://arxiv.org/abs/2303.18223) — LLM 全体像の網羅的サーベイ
- [Llama / Mistral / DeepSeek の技術レポート](https://arxiv.org/abs/2407.21783) — オープンモデルの中身は論文で読める
