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
  │    行列演算 → attention → mini-GPT → sampling
  │
  ├─ Transformerの部品(GPT-2 からの改良)
  │    位置: 絶対 → RoPE
  │    attention: MHA → GQA/MQA(→ MLA)
  │    正規化・活性化: LayerNorm/GELU → RMSNorm/SwiGLU
  │    FFN: dense → MoE
  │
  ├─ モデルを支える部品
  │    トークナイザ(BPE) / 推論高速化(KVキャッシュ・投機) / 学習(事前学習→SFT→RLHF)
  │
  └─ モデルの系譜
       GPT系 → 推論モデル(o1/R1)
       Claude(Constitutional AI) / Gemini(マルチモーダル)
       オープン(Llama/Mistral/DeepSeek/Qwen)
       非Transformer: Mamba/SSM・拡散LM
```

</FigureBox>

## 章の一覧

**基礎スパイン**（自分の手で Transformer を組む）

- [行列演算](/parts/tensor) — numpy に頼らず行列積・softmax・LayerNorm を作る
- [Attention](/parts/attention) — self-attention の 1 ヘッド
- [mini-GPT](/parts/mini-gpt) — ブロックを重ねた forward pass
- [LLM Sampling](/parts/llm-sampling) — logits からトークンを選ぶ

**Transformerの部品**（GPT-2 構成からの改良）

- [Transformer全体像](/parts/transformer) — RNN の限界、3 系統、ブロックの分業
- [位置エンコーディングとRoPE](/parts/rope) — 足す位置から回す位置へ
- [attention変種(MQA/GQA)](/parts/attention-variants) — KV キャッシュを縮める共有
- [RMSNormとSwiGLU](/parts/rmsnorm-swiglu) — 正規化と活性化の世代交代
- [MoE](/parts/moe) — FFN を専門家に分ける

**モデルを支える部品**

- [BPEトークナイザ](/parts/bpe) — 文字列をトークンに切る入口
- [推論高速化](/parts/inference) — KV キャッシュと speculative decoding
- [学習パイプライン](/parts/llm-training) — 事前学習 → SFT → RLHF/DPO

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
- **部品**: GPT-2 相当の素朴な構成(絶対位置・MHA・LayerNorm・GELU・dense FFN)。RoPE・GQA・RMSNorm・SwiGLU・MoE の改良は、それぞれ [Transformerの部品](/parts/rope)の各章で実装した
- **学習**: なし(重みは乱数)。実物は事前学習 + SFT + RLHF の 3 段([学習パイプライン](/parts/llm-training))
- **規模**: 数万〜数百万パラメータ。フロンティアは兆単位

土台の Transformer は同じで、そこから先は規模・部品の磨き・学習・データの差でしかない。特別な仕掛けが 1 つあるわけではなく、この教科書で 1 つずつ実装した改良の積み重ねの先に、今のモデルがある。

## 参考資料

- [Attention Is All You Need (2017)](https://arxiv.org/abs/1706.03762) — すべての起点
- [The Illustrated Transformer](https://jalammar.github.io/illustrated-transformer/) — 図解の定番
- [A Survey of Large Language Models](https://arxiv.org/abs/2303.18223) — LLM 全体像のサーベイ
