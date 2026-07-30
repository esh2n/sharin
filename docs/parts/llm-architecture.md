<script setup>
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# LLM全体マップ

<Summary>
LLM 編は部品の実装から各社モデルの系譜まで多くの章に分かれている。この章はその見取り図で、Transformer という 1 つの土台から、部品の改良と学習・モデルの系統がどう枝分かれするかを 1 枚にまとめ、各章への入口にする。実装した mini-GPT が全体のどこに位置し、そこからフロンティアまでの距離が規模・部品・学習・データの差でできていることを確認する。
</Summary>

## この章の役割

LLM 編は章数が多い。骨格を組む基礎スパイン、Transformer の部品、モデルを支える部品、各社モデルの系譜。どこから読んでも構わないが、全体の中でその章がどこにあるかは掴んでおきたい。この章はそのための見取り図で、詳細は各章に譲り、ここでは接続だけを示す。

## 全体の見取り図

<FigureBox caption="LLM 編の全体像。2017 年の Transformer を土台に、部品の改良(大きく・長く・速く)と、モデルの系統(各社の後段学習と設計思想)が枝分かれする">

```
Transformer (2017)  ── decoder-only が生成の主流
  │
  ├─ 基礎スパイン(自分の手で組む)
  │    入口(BPE)→ 出口(sampling)→ 行列演算 → attention → mini-GPT
  │
  ├─ Transformerの部品(GPT-2 からの改良)
  │    位置: 絶対 → RoPE
  │    attention: MHA → GQA/MQA(→ MLA)
  │    正規化・活性化: LayerNorm/GELU → RMSNorm/SwiGLU
  │    FFN: dense → MoE
  │
  ├─ モデルを支える部品
  │    速く: 推論高速化(KVキャッシュ・投機)
  │    小さく: 量子化 → GPTQ → 枝刈り
  │    育てる: 学習パイプライン / LoRA
  │    外へ: 埋め込み検索 / パッチ化 / エージェントの枠組み
  │
  └─ モデルの系譜
       GPT系 → 推論モデル(o1/R1)
       Claude(Constitutional AI) / Gemini(マルチモーダル)
       オープン(Llama/Mistral/DeepSeek/Qwen)
       非Transformer: Mamba/SSM・拡散LM
```

</FigureBox>

## 章の一覧

**基礎スパイン**(自分の手で Transformer を組む)

- [BPEトークナイザ](/parts/bpe) — 文字列をトークン ID 列に切る入口
- [LLM Sampling](/parts/llm-sampling) — logits からトークンを選ぶ出口
- [行列演算](/parts/tensor) — numpy に頼らず行列積・softmax・LayerNorm を作る
- [Attention](/parts/attention) — self-attention の 1 ヘッド
- [mini-GPT](/parts/mini-gpt) — ブロックを重ねた forward pass

**Transformerの部品**(GPT-2 構成からの改良)

- [Transformer全体像](/parts/transformer) — RNN の限界、3 系統、ブロックの分業
- [位置エンコーディングとRoPE](/parts/rope) — 足す位置から回す位置へ
- [attention変種(MQA/GQA)](/parts/attention-variants) — KV キャッシュを縮める共有
- [RMSNormとSwiGLU](/parts/rmsnorm-swiglu) — 正規化と活性化の世代交代
- [MoE](/parts/moe) — FFN を専門家に分ける

**モデルを支える部品**

- [推論高速化](/parts/inference) — KV キャッシュと speculative decoding
- [学習パイプライン](/parts/llm-training) — 事前学習 → SFT → RLHF/DPO
- [量子化](/parts/quantization) — 整数に写して 1/4〜1/8 に縮める
- [誤差を配る量子化(GPTQ)](/parts/gptq) — 丸めた誤差を残りの重みに肩代わりさせる
- [枝刈り](/parts/pruning) — どれを 0 にするかを、消す損の見積もりで選ぶ
- [LoRA](/parts/lora) — 元の重みを凍結し、低ランクの補正だけ学習する
- [埋め込みとベクトル検索](/parts/embedding-search) — 意味の近さで引く、RAG の土台
- [マルチモーダル入口(パッチ化)](/parts/patchify) — 画像を 16×16 に切ってトークンにする
- [エージェントの枠組み](/parts/agent-harness) — プロンプト・ループ・グラフの分かれ目

**モデルの系譜**

- [GPT系譜](/parts/gpt-lineage) — スケールに賭けた歴史
- [推論モデル(o1/R1)](/parts/reasoning-models) — 推論時間という第 2 の軸
- [Claude(Constitutional AI)](/parts/claude-lineage) — 基準を明文化した後段学習
- [Gemini(マルチモーダル)](/parts/gemini-lineage) — 全部をトークンにして混ぜる
- [オープンモデル](/parts/open-models) — 公開ゆえに検証できる系譜
- [Mamba / SSM](/parts/ssm) — attention の二乗に挑む線形時間モデル

## あなたの mini-GPT は全体のどこにいるか

[mini-GPT](/parts/mini-gpt) で作ったのは、decoder-only の GPT-2 相当だ。全体マップの中で位置を確かめると、そこからフロンティアまでの距離が 4 つの差でできていることが分かる。

- **系統**: decoder-only(GPT 系)。今のチャット LLM の主流に乗っている
- **部品**: GPT-2 相当の素朴な構成(絶対位置・MHA・LayerNorm・GELU・dense FFN)。[RoPE](/parts/rope)・[GQA](/parts/attention-variants)・[RMSNorm と SwiGLU](/parts/rmsnorm-swiglu)・[MoE](/parts/moe) の改良は、それぞれの章で実装した
- **学習**: なし(重みは乱数)。実物は事前学習 + SFT + RLHF の 3 段([学習パイプライン](/parts/llm-training))
- **規模**: 数万〜数百万パラメータ。フロンティアは兆単位

土台の Transformer は同じで、そこから先は規模・部品の磨き・学習・データの差でしかない。特別な仕掛けが 1 つあるわけではなく、この教科書で 1 つずつ実装した改良の積み重ねの先に、今のモデルがある。

## 設計の観点

- **1つの土台に、改良が集まる構造**: 2017 年の設計が今も土台であり続けているのは、部品ごとに独立して差し替えられる形をしているからになる。位置の入れ方も、attention の共有の仕方も、FFN の分け方も、本体を作り直さずに交換できる
- **改良は、詰まっている場所に出る**: 系列長の2乗が詰まれば attention の変種と推論の工夫が、パラメータ量が詰まれば MoE と量子化が出てくる。**どこが上限かを知ると、次に何が来るかが読める**
- **能力の差はアーキテクチャの外にある**: 各社モデルの違いは、土台よりも学習データと後段の学習に出る。図の枝分かれで下のほうが太いのは、そのため
- **入口と出口は交換可能**: トークン化を差し替えれば画像も音声も同じ本体に流せる。**中心を変えずに扱える対象を増やす**のが、この設計の伸びしろになっている
- **読む順にも土台がある**: 部品の章は全体像の骨格のどこか1か所を深掘りする形で並んでいる。先に配置を頭に入れておくと、各章がどの穴を埋めているかが分かる

裏どり:

- **decoder-only に収束した**: 3系統(生成・理解・変換)のうち、チャット LLM はほぼ全部が decoder-only になった。生テキストがそのまま教師データになり、全位置が同時に教師信号になり、学習と生成の形が一致する。この3つが揃うのがこの系統だけだからになる
- **標準セットが存在する**: RoPE + GQA + RMSNorm + SwiGLU(+ MoE)という組み合わせが、独立に開発された各社のモデルでほぼ一致している。**別々に探索して同じ答えに行き着いた**ことが、それぞれの改良の妥当性の傍証になっている
- **非 Transformer の挑戦は続いている**: [Mamba / SSM](/parts/ssm) は系列長の2乗そのものを線形に置き換えようとし、拡散言語モデルは自己回帰という生成の形を変えようとする。どちらもまだ主流を置き換えてはいないが、**どこが本質的な制約かを示す役**を果たしている
- **非公開化が進んだ**: GPT-2 までは重みも構成も公開されていたが、GPT-4 以降は規模すら公開されていない。この空白を埋める形でオープンモデルが伸びたという経緯が、系譜の章の背景になっている
- **この編で実装したのは推論だけ**: 逆伝播も最適化も扱っていないので、ここで作ったものは重みが与えられれば動く forward pass になる。学習まで含めた最小実装としては nanoGPT が定番の次の一歩になる

## 参考資料

- [Attention Is All You Need (2017)](https://arxiv.org/abs/1706.03762) — すべての起点
- [The Illustrated Transformer](https://jalammar.github.io/illustrated-transformer/) — 図解の定番
- [A Survey of Large Language Models](https://arxiv.org/abs/2303.18223) — LLM 全体像のサーベイ
