<script setup>
import MatMulDemo from '../components/MatMulDemo.vue'
import Summary from '../components/Summary.vue'
</script>

# 行列演算(tensor)

> 実装: [`llm/tensor/`](https://github.com/esh2n/sharin/tree/main/llm/tensor) / 実行: `go test ./llm/tensor/`

<Summary>
LLM を作る、その第一歩。llm-sampling 編では「logits から先」を作ったが、今度は「入力から logits まで」、つまり Transformer 本体に向かう。ただしその前に、numpy に頼らず行列積・softmax・layernorm を自分で書く。派手さはないが、この数本の関数が Transformer のほぼ全てだ。attention は行列積の塊であることを、土台から自分の手で確かめる回。
</Summary>

## この章で作るもの

LLM の forward pass(入力 → logits)を Go で自作していく。その一番下の層、
**行列演算ライブラリ**をまず作る。numpy も PyTorch も使わない。それ自体が
この教科書の主旨(車輪の再発明)であり、「モデルの中で何が起きているか」を
ブラックボックスにしないため。

この章の肝は3つ。

- Transformer の計算は、結局 **matmul・softmax・layernorm・GELU** の数本の組み合わせ
- その中心は**行列積(MatMul)**。attention も全結合層も、突き詰めればこれ
- 表現は「**float32 スライス + 行数・列数**」の2次元行列に絞る(実物は N 次元)

## 中心にあるのは行列積

ニューラルネットの計算のほとんどは行列積。「入力ベクトルに重み行列を掛ける」を
層の数だけ繰り返す。だからまず MatMul を書く。

<<< ../../llm/tensor/tensor.go#matmul{go}

結果の1マスは「左の行列の**行**」と「右の行列の**列**」の内積。
この単純な計算の積み重ねが、GPT の何十億回の演算の正体。

**試す**: 結果 C のマスにマウスを乗せると、掛け合わされる A の行(青)と B の列(緑)が
光り、その内積が値になっているのが見える。

<MatMulDemo />

行数が違えば掛けられない(A の列数 = B の行数でないとダメ)。実装では形が
合わなければ panic する。テンソルの形(shape)の管理は、実際の LLM 実装で
最も間違えやすくデバッグの大半を占める部分。

## 表現: 素朴な2次元行列

<<< ../../llm/tensor/tensor.go#core{go}

`Data[r*Cols + c]` で (r, c) にアクセスする**行優先(row-major)**の1次元配列。
多次元に見える計算も、メモリ上は1本の連続した配列。この「連続したメモリ + 添字計算」が、
[ディスクとページ](./disk-and-pages)のページと同じで、キャッシュ効率に効く。

## softmax: 注目度を確率にする

<<< ../../llm/tensor/tensor.go#softmax{go}

[LLM Sampling](./llm-sampling) で作った softmax と同じ(max 引きの数値安定化も)。
違う使い方をする。あちらは「次のトークンを選ぶ確率」だったが、attention では
「**どのトークンにどれだけ注目するか**」の重みを確率にするのに使う。同じ道具が、
モデルの入口と出口の両方で働く。

## layernorm と GELU: 各層の脇役

<<< ../../llm/tensor/tensor.go#norm{go}

- **LayerNorm**: 各行を平均0・分散1に正規化する。層を深く重ねると値が発散しがちなのを、
  各層の入口で揃えて安定させる。深いネットワークが学習できるようになった立役者の1つ
- **GELU**: 活性化関数。ReLU(負を0にする)の滑らかな版で、GPT/BERT が使う。
  「非線形性」を注入する係で、これがないと何層重ねても1つの行列積に潰れてしまう

## 次の章へ: これで attention が組める

これで Transformer を組む部品が揃った。次は、この行列演算だけを使って
**self-attention**(トークンどうしが互いに注目し合う仕組み)を1ヘッド作る。
`Q·Kᵀ → softmax → ·V` という、まさに「行列積の塊」を、ここで書いた MatMul と
SoftmaxRows で組み立てられる。その次が、それを重ねた mini-GPT の forward pass。

## 実物との距離

- **速度**: ここは float32 の素朴な3重ループ。実物は BLAS / SIMD / GPU で
  何百〜何万倍速い。行列積のアルゴリズムは同じでも、実装の最適化が別世界
- **N 次元**: 実物はバッチ・ヘッド・系列長を含む4次元以上のテンソル。ここは2次元に潰した
- **自動微分**: 学習にはforwardの逆(backward, 勾配計算)が要る。ここは推論(forward)のみ

## 参考資料

- [llm.c](https://github.com/karpathy/llm.c) — GPT-2 を C で書く。この教科書の Go 版の精神的な元
- [Neural Networks: Zero to Hero (Karpathy)](https://karpathy.ai/zero-to-hero.html) — 行列演算から Transformer まで手を動かす講義
- [The Illustrated Transformer](https://jalammar.github.io/illustrated-transformer/) — 次章 attention の予習に最適
