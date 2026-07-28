<script setup>
import AttentionDemo from '../components/AttentionDemo.vue'
import Summary from '../components/Summary.vue'
</script>

# Attention

> 実装: [`llm/attention/`](https://github.com/esh2n/sharin/tree/main/llm/attention) / 実行: `go test ./llm/attention/`

<Summary>
Transformer の心臓。"attention is all you need" の attention を、前章の行列演算だけで1ヘッド組む。各トークンが「他のどのトークンにどれだけ注目するか」を計算し、注目度に応じて情報を混ぜる。式にすると softmax(Q·Kᵀ/√d)·V で、まさに行列積の塊だ。GPT が文脈を理解する仕組みと、"未来を見ない"因果マスクが、行列の絵で腑に落ちる。
</Summary>

## この章で作るもの

[行列演算](./tensor)で作った MatMul と SoftmaxRows **だけ**を使って、
self-attention を1ヘッド組む。ここが Transformer の本体で、
「文脈を読む」とはどういう計算なのかが見える。

この章の肝は3つ。

- attention は「各トークンが**他のどのトークンにどれだけ注目するか**」を計算して情報を混ぜる
- それは3つの行列 **Q(問い)・K(見出し)・V(中身)** に集約される
- GPT は**因果マスク**で「未来のトークンを見ない」。これが「次の単語を予測する」を成立させる

## Q・K・V: 問い、見出し、中身

各トークンは、自分のベクトルから3つの役割のベクトルを作る。

- **Q(query)**: 「私は何を探している?」。そのトークンが出す問い合わせ
- **K(key)**: 「私は何を持っている?」。各トークンが出す見出し
- **V(value)**: 「私の中身はこれ」。各トークンが出す実体

トークン i が誰に注目するかは、**i の Q と、全トークンの K の内積**で決まる。
Q と K がよく合う(内積が大きい)トークンほど、強く注目する。図書館で例えると、
Q は「探している本のテーマ」、K は「各本の背表紙」、内積は「どれだけ合致するか」。

<<< ../../llm/attention/attention.go#head{go}

## 式: softmax(Q·Kᵀ / √d)·V

attention の全体はこの1式。前章の道具でそのまま書ける。

<<< ../../llm/attention/attention.go#attention{go}

順に見ると:

1. **Q·Kᵀ**: 全トークン対の「注目スコア」を一気に計算(行列積)
2. **/√d**: 次元が大きいとスコアが大きくなりすぎて softmax が尖るので割る
3. **softmax**: 各行を確率に(トークン i が各トークンに配る注目の合計が1)
4. **·V**: その確率で V を混ぜる。強く注目した相手の中身が多く混ざる

`rawScores` の `√dHead` で割るところ、`SoftmaxRows` で確率にするところ、
最後の `MatMul(weights, v)` で混ぜるところが、[行列演算](./tensor)で作った関数だけで
できているのが分かる。**attention は行列積の塊**、を実装で確かめられた。

## 因果マスク: 未来を見ない

GPT は「次の単語を予測する」モデル。だから予測するとき、**まだ書いていない未来の
単語を見てはいけない**(見えたらカンニング)。これを保証するのが因果マスク。

注目スコアの行列で、「トークン i が トークン j > i(未来)を見る」マスは
上三角にあたる。そこを **-∞** にすると、softmax を通ったとき 0 になり、
未来への注目が消える。

**試す**: 因果マスクの ON/OFF を切り替えると、行列の上三角(未来への注目)が
消える/現れるのが見える。マスク ON では、各トークンは自分と過去にしか注目しない。
一番上のトークンは過去がないので、自分に100%注目している。

<AttentionDemo />

行 = 注目する側(query)、列 = 注目される側(key)。マスありの下三角だけの形が、
GPT がテキストを左から右へ生成できる理由そのもの。

## なぜこれで「文脈を理解」できるのか

attention の1回で、各トークンは「文中の関連する他のトークンの情報」を自分に混ぜ込む。
"it" が何を指すか、"bank" が金融か川岸か。そういう文脈依存の意味は、
関連トークンに注目して情報を集めることで決まる。これを何層も重ねると、
だんだん高度な意味が組み上がる。それが Transformer。

- 1層目: 隣接語や単純な係り受け
- 深い層: 主語と動詞の一致、遠く離れた照応、文全体の意図

## 次の章へ: mini-GPT

attention 1ヘッドができた。実物の Transformer ブロックは、これに

- **マルチヘッド**(複数の attention を並列に。別々の関係を捉える)
- **位置エンコーディング**(トークンの順序情報。attention 自体は順序を見ないので)
- **フィードフォワード層**([行列演算](./tensor)の GELU 付き全結合)
- **残差接続 + LayerNorm**

を足したもの。これを重ねて、最後にトークン分布([LLM Sampling](./llm-sampling)の logits)を
出すのが GPT の forward pass。次章でこれを組み、実際にテキストを生成する。

## 簡略化したこと

- **1ヘッドのみ**: 実物はマルチヘッド(8〜96個の attention を並列)
- **重みは乱数/恒等**: 実物は学習で得た Wq/Wk/Wv。ここは仕組みの可視化が目的
- **位置エンコーディングなし**: attention は順序を見ないので実物は別途足す(次章)
- **出力射影 Wo なし**: マルチヘッドを束ねる行列

## 参考資料

- [Attention Is All You Need (2017)](https://arxiv.org/abs/1706.03762) — Transformer の原論文
- [The Illustrated Transformer](https://jalammar.github.io/illustrated-transformer/) — Q/K/V の図解の決定版
- [nanoGPT (Karpathy)](https://github.com/karpathy/nanoGPT) — 最小の GPT 実装。次章の参照
