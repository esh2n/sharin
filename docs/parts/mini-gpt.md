<script setup>
import GptScaleDemo from '../components/GptScaleDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import FlowRow from '../components/figures/FlowRow.vue'

const dataflow = [
  { label: 'トークン列', note: '[1, 2, 3]' },
  { label: '埋め込み+位置', note: '(seq, d)', state: 'hot' },
  { label: 'ブロック ×N', note: 'attention + FFN', state: 'hot' },
  { label: 'LayerNorm', note: '(seq, d)' },
  { label: 'logits', note: '(seq, 語彙)' },
]
</script>

# mini-GPT

> 実装: [`llm/gpt/`](https://github.com/esh2n/sharin/tree/main/llm/gpt) / 実行: `go test ./llm/gpt/`

<Summary>
LLM 編の集大成。attention の1ヘッドを Transformer ブロックに仕立て、それを重ねて、実際に「次のトークンを予測する」forward pass を完成させる。埋め込み → ブロック → logits が動けば、あとは llm-sampling 編に繋ぐだけでテキストが生成される。そして最後に——あなたが書いたこの200行と GPT-4 を隔てているものは何か、フロンティアモデルの地図を広げる。
</Summary>

## この章で作るもの

[attention](./attention) で作った1ヘッドを、実際の GPT の形——**Transformer ブロック**に
組み上げ、それを重ねて mini-GPT の forward pass を完成させる。
[行列演算](./tensor)から始まった LLM 編が、ここで1つに合流する。

この章の肝は3つ。

- Transformer ブロック = **マルチヘッド attention + フィードフォワード + 残差 + LayerNorm**
- 埋め込み → ブロック×N → logits で「各位置での次トークンの予測」ができる
- あなたのこの mini-GPT と GPT-4 の違いは、本質的には**規模**。中身は同じ Transformer

## 全体のデータフロー

<FigureBox caption="mini-GPT の forward pass。トークン列を埋め込みでベクトルにし、ブロックを重ねて、最後に語彙分布(logits)を出す">
  <FlowRow :steps="dataflow" />
</FigureBox>

<<< ../../llm/gpt/gpt.go#forward{go}

## 埋め込みと位置

最初に、トークンID を**ベクトル**に変える(埋め込み表を引く)。そして**位置埋め込み**を足す。
なぜ位置が要るか——[attention](./attention) は、実は**トークンの順序を見ない**。
"cat sat" も "sat cat" も、位置情報がなければ attention には同じに見える。
だから「何番目のトークンか」をベクトルに足し込んで、順序を教える。

## Transformer ブロック: 混ぜて、変換する

1つのブロックは2つの部分でできている。

- **マルチヘッド attention**: [attention](./attention) を複数ヘッド並列に。各ヘッドが
  別々の関係(近くの語、主語述語、照応…)を捉える。トークン間で情報を**混ぜる**係
- **フィードフォワード**: 各トークンの中で、GELU 付きの全結合で非線形**変換**する係

どちらも **残差接続**(入力を出力に足す)と **LayerNorm** で包む。残差は「元の情報を
失わずに少しずつ足していく」仕組みで、これがないと深いネットワークは学習できない。
[行列演算](./tensor)で作った LayerNorm と GELU がここで働く。

<<< ../../llm/gpt/gpt.go#config{go}

`DModel % NHeads == 0` が要る(各ヘッドが DModel/NHeads 次元を担当する)のがマルチヘッドの割り付け。

## テキスト生成: 1語ずつ、自己回帰

forward pass は「各位置での次トークンの logits」を返す。生成は、
**最後の位置の logits から次トークンを選び、それを入力に足して、また forward** する。
これを繰り返すのが自己回帰(autoregressive)生成。

<<< ../../llm/gpt/gpt.go#generate{go}

ここで選ぶ部分(argmax = greedy)を、[llm-sampling](./llm-sampling) の
temperature や top-p に差し替えれば、まさに ChatGPT がやっている生成になる。
**LLM 編が一周した**——logits の出し方(この章)と、logits からの選び方(llm-sampling 編)の
両方を、自分の手で書いた。

テストでは因果性(末尾トークンを変えても過去の位置の logits は動かない)も固定している。
これが「左から右へ、既に書いた分だけを見て次を書く」を保証する。

## ここから GPT-4 まで: フロンティアモデルの地図

あなたが書いたこの mini-GPT は、GPT-2/3/4 と**同じ Transformer**。
では何が「フロンティアモデル」を分けるのか。3つの軸で見る。

### 軸1: 規模(スケール)

一番大きな違いは、身も蓋もないが**大きさ**。同じアーキテクチャを、桁違いに大きくする。

**試す**: 次元・層数・語彙を動かすと、あなたの mini-GPT のパラメータ数が変わる。
GPT-2/3/4 と並べると、その差が対数スケールでも歴然。

<GptScaleDemo />

| モデル | パラメータ数 | 学習データ | 年 |
|---|---|---|---|
| あなたの mini-GPT | 数万〜数百万 | なし(乱数) | — |
| GPT-2 | 15億 | 40GB | 2019 |
| GPT-3 | 1750億 | 570GB | 2020 |
| GPT-4 級 | 推定 数兆(非公開) | 数TB〜 | 2023〜 |

「大きくすれば賢くなる」が予測可能な法則(**スケール則**, scaling laws)として効くことが
分かったのが、この数年の LLM 爆発の起点。パラメータ・データ・計算量を増やすほど、
性能が滑らかに上がり続ける。

### 軸2: アーキテクチャの改良

土台は Transformer のままだが、大規模化に伴う改良が積まれている。

- **位置エンコーディングの進化**: この章の「足す」方式から、RoPE(回転位置埋め込み)へ。
  長い文脈でも位置関係を保てる。長コンテキスト(100万トークン)を可能にした立役者
- **Mixture of Experts (MoE)**: FFN を「専門家」に分け、トークンごとに一部だけ使う。
  総パラメータは巨大でも、1回の計算は一部だけ——大きくても速い(Mixtral, GPT-4 も MoE と噂)
- **Attention の効率化**: FlashAttention(メモリ効率)、GQA(KV を共有してメモリ削減)、
  KV cache(生成時に過去の K,V を使い回す。この章にはまだ無い高速化)
- **正規化・活性化の改良**: RMSNorm, SwiGLU など。細かいが積み重ねが効く

### 軸3: 学習の段階(ここが「フロンティア」の核心)

実は、性格を決めるのは事前学習だけではない。**フロンティアモデルは学習が3段階**ある。

<FigureBox caption="フロンティアモデルの学習パイプライン。ただ次の単語を予測する事前学習の上に、人の意図に沿わせる後段の学習が乗る">

```
1. 事前学習          膨大なテキストで「次の単語」を予測。世界の知識と言語を獲得
   (pretraining)     → この段階は「もっともらしい続き」を書くだけ。指示には従わない

2. 教師ありFT         「指示 → 良い応答」の例で微調整。指示に従う形を覚える
   (SFT)

3. 人間からの学習      人間の好みで応答を評価し、その報酬で調整(RLHF / DPO)。
   (RLHF)            「役に立つ・無害・正直」に寄せる。ChatGPT が"アシスタント"な理由
```

</FigureBox>

素の GPT-3(事前学習だけ)は、質問しても「質問を続けて書く」ようなモデルだった。
それを「指示に従うアシスタント」にしたのが後段の学習(SFT + RLHF)。
**ChatGPT を ChatGPT たらしめているのは、アーキテクチャより学習の後半**。
Claude の Constitutional AI もこの後段の工夫。

### 軸4: マルチモーダルとエージェント

さらに最近のフロンティアは、テキストの外へ広がっている。

- **マルチモーダル**: 画像・音声・動画も同じ Transformer に流し込む(トークン化して混ぜる)。
  GPT-4V, Gemini, Claude はテキスト以外も"読める"
- **道具の利用・エージェント**: モデルが外部ツール(検索・コード実行・API)を呼ぶ。
  この教科書を書いている Claude Code も、その一形態
- **長い推論**: 答える前に"考える"ステップを挟む(o1, o3 系の推論モデル)

## 結局、あなたは何を作ったのか

まとめると、あなたが LLM 編で自分の手で書いたものは:

- [tensor](./tensor): 行列演算(numpy に逃げず)
- [attention](./attention): self-attention(Transformer の心臓)
- この章: それを重ねた GPT の forward pass
- [llm-sampling](./llm-sampling): logits からトークンを選ぶ

これは **GPT-4 と同じ骨格**。フロンティアモデルは、この骨格を「桁違いに大きくし、
膨大なデータで事前学習し、人の意図に沿わせる後段学習を施し、テキストの外まで広げた」もの。
魔法ではない——あなたが今書いたものの、スケールと磨き上げの先にある。

## 簡略化したこと

- **未学習(重みは乱数)**: だから生成は意味をなさない。「動く forward pass」の理解が目的。
  意味のある生成には学習(backward + 最適化 + 大量データ)が要る
- **KV cache なし**: 生成のたび全系列を再計算。実物は過去の K,V を使い回す
- **BPE トークナイザは別**: ここはトークンID列を直接受ける
- **RoPE/MoE/FlashAttention なし**: 上記「アーキテクチャの改良」は未実装。素の GPT-2 構造

## 参考資料

- [nanoGPT (Karpathy)](https://github.com/karpathy/nanoGPT) — 学習まで含む最小 GPT。次の一歩
- [The Illustrated GPT-2](https://jalammar.github.io/illustrated-gpt2/) — GPT-2 の内部を図で
- [Scaling Laws for Neural Language Models](https://arxiv.org/abs/2001.08361) — スケール則の原論文
- [Training language models to follow instructions (InstructGPT)](https://arxiv.org/abs/2203.02155) — RLHF で ChatGPT になった論文
