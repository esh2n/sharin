<script setup>
import OpenModelsDemo from '../components/OpenModelsDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# オープンモデル(Llama / Mistral / DeepSeek / Qwen)

<Summary>
GPT-4 や Claude が構成を非公開にした一方、重みを公開するオープンモデルが猛追している。Llama が公開の流れを作り、Mistral が小型高効率、DeepSeek が学習効率と MLA・MoE、Qwen が多言語と幅広いサイズを持ち込んだ。この章では部品編で作った RoPE・GQA・RMSNorm・SwiGLU・MoE が各社でどう選ばれているかを対応表で読み、公開ゆえ中身を検証できる強みを追う。
</Summary>

## この章で読むもの

[GPT系譜](/parts/gpt-lineage) で見たとおり、フロンティアの GPT-4 以降は構成すら非公開になった。その空白を埋めるように伸びたのがオープンモデルだ。重みが公開され、自分のマシンで動かせて、改造もファインチューニングもできる。そして技術レポートが読めるので、部品編で 1 つずつ実装してきた RoPE・GQA・RMSNorm・SwiGLU・MoE が、実物でどう組み合わされているかを確認できる。この章は部品編の答え合わせでもある。

<FigureBox caption="オープンモデルの系統。Llama がライセンス公開の流れを作り、各系列が別々の軸(効率・学習・多言語)で分岐した。土台の Transformer と主要部品はほぼ共通に収束している">

```
Transformer (2017)
   └─ decoder-only ── オープン系(重みが公開・改造可)
        ├─ Llama(Meta)       公開の流れを作った基準点
        ├─ Mistral(仏)       小型高効率、SWA / MoE(Mixtral)
        ├─ DeepSeek(中)      学習効率、MLA + 細粒度 MoE、R1 で推論
        └─ Qwen(Alibaba)     多言語、0.5B〜72B の幅広い展開
   共通の部品: RoPE / GQA / RMSNorm / SwiGLU(+ 一部 MoE)
```

</FigureBox>

順に見ていく。

1. **部品はほぼ共通に収束**: 各社独自に見えて、[RoPE](/parts/rope) + [GQA](/parts/attention-variants) + [RMSNorm/SwiGLU](/parts/rmsnorm-swiglu) という土台はオープン系でほぼ一致する
2. **差は独自の 1 手**: Mistral の SWA、DeepSeek の MLA + 細粒度 MoE のように、各社が 1〜2 箇所で独自の選択をする
3. **公開ゆえに検証できる**: 技術レポートと重みが出るので、構成も学習法も外部が再現・検証できる。非公開モデルとの決定的な違い

## ① 部品は収束している

まず驚くべき事実から。オープンモデルは会社もサイズも違うのに、基本部品はほぼ同じセットに落ち着いている。トークナイザは [BPE](/parts/bpe)、位置は [RoPE](/parts/rope)、attention は [GQA](/parts/attention-variants)、正規化は [RMSNorm](/parts/rmsnorm-swiglu)、活性化は SwiGLU。[mini-GPT](/parts/mini-gpt) の GPT-2 構成(絶対位置・MHA・LayerNorm・GELU)からの改良が、独立に開発された各系列で同じ結論に至っている。

これは部品編で 1 つずつ見た「なぜその改良か」の答えが、実物でも支持されているということだ。RoPE は長コンテキストのため、GQA は KV キャッシュのため、RMSNorm は計算の軽さのため、SwiGLU は品質のため。どれも規模を上げる過程で効く改良で、規模を追う各社が同じ道具に行き着いた。

## ② 各系列の独自の 1 手

共通の土台の上で、各社は 1〜2 箇所に独自性を出す。

- **Llama(Meta、2023〜)**: オープンモデルの基準点。特殊な独自技術ではなく、標準部品を丁寧に組んで大量のデータ(Llama 3 は 15 兆トークン)で学習する路線。[学習パイプライン](/parts/llm-training) の Chinchilla 最適を大きく超えるデータ量で、推論の安い小型を賢くする。ライセンス付き公開という形を定着させた功績が大きい
- **Mistral(フランス、2023〜)**: 小型高効率。sliding window attention(SWA、各トークンが直近 W トークンだけを見て計算を抑える)を採り、7B で当時の 13B 級に並んだ。Mixtral 8x7B で [MoE](/parts/moe) をオープンに持ち込んだのもここ
- **DeepSeek(中国、2024〜)**: 学習効率と独自アーキテクチャ。[attention変種](/parts/attention-variants)で触れた MLA(KV を圧縮)と、細粒度 MoE + 共有 expert を組み、V3 は総量 671B・アクティブ 37B。[推論モデル](/parts/reasoning-models) の R1 で推論モデルもオープン化した
- **Qwen(Alibaba、2023〜)**: 多言語と展開の幅。0.5B から 72B、さらに MoE 版まで揃え、中国語・多言語で強い。エッジからサーバまでサイズを選べることを武器にする

独自の 1 手はあっても、土台を共有しているから互いの改良を取り込みやすい。MLA が良いと分かれば他社が試し、MoE の構成が共有される。公開が改良の伝播を速めている。

## ③ 公開という性質そのもの

オープンモデルの本質は性能より公開の効果にある。

- **検証できる**: 学習データの構成、アーキテクチャ、評価が論文で読める。ベンチマークの数字を外部が再現でき、[GPT系譜](/parts/gpt-lineage) の「GPT-4 は何パラメータか答えが無い」という状況の逆になる
- **改造できる**: ファインチューニング(LoRA など)、量子化、蒸留を自分でかけられる。[推論モデル](/parts/reasoning-models) の R1 蒸留版のように、派生が公式・非公式に大量に生まれる
- **自分で動かせる**: 手元やプライベート環境で推論でき、データを外に出せない用途(医療、機密)で選ばれる
- **費用の下押し**: 高性能なオープンモデルの存在が、クローズド API の価格に競争圧力をかける

一方で「オープン」の程度には幅がある。重みは公開でも学習データやコードは非公開なことが多く、商用利用に制限が付くライセンス(Llama の許諾)もある。完全なオープンソース(データ・コードまで公開、例: OLMo)とは区別される。公開と一口に言っても、何がどこまで開いているかはモデルごとに違う。

### 動かす

下のデモは各社モデルの部品選択を対応表で見る。列(部品)や行(モデル)を選ぶと、部品編の各章で作ったものが実物でどう採用されているかが浮かび、オープン系の共通土台と、GPT-4/Claude/Gemini の非公開部分の差が一目で分かる。

<OpenModelsDemo />

## 設計の観点

- **なぜ部品が収束するか**: 規模を上げる過程で効く改良は限られており、独立に探索しても同じ最適に行き着く。差別化はアーキテクチャの細部より、データの質と量、後段学習に移っている
- **オープンを選ぶ基準**: データを外に出せない、細かく作り込みたい、費用を抑えたい、という要求ではオープンが有利。最高性能や運用の手離れではクローズド API が有利。トレードオフはプロダクトの制約で決まる
- **サイズ展開の意味**: Qwen の 0.5B〜72B のような幅は、[キャパシティ見積もり](/parts/capacity-estimation) 的な要求(レイテンシ、費用、精度)ごとに最適点を選べる価値。1 サイズしかないモデルは選択肢を狭める
- **蒸留と派生のエコシステム**: 公開モデルは無数の派生を生む。R1 の思考データで小型を鍛える([推論モデル](/parts/reasoning-models))ような伝播は、クローズドでは起きにくい
- **「オープン」の解像度**: 重み公開 ≠ オープンソース。ライセンス、データ公開、コード公開の 3 軸で見ないと、実際に何ができるかを見誤る

## 対照と実例

| 系列 | 独自の 1 手 | 強み | 公開の程度 |
|---|---|---|---|
| Llama | 標準部品 + 大量データ | 基準点・エコシステム最大 | 重み公開、許諾ライセンス |
| Mistral | SWA、Mixtral で MoE | 小型高効率 | Apache 2.0(一部)、重み公開 |
| DeepSeek | MLA + 細粒度 MoE、R1 | 学習効率・推論 | 重み・論文とも公開 |
| Qwen | 幅広いサイズ、多言語 | 展開の幅・中国語 | 重み公開 |
| OLMo(参考) | 完全公開 | データ・コードまで再現可 | フルオープンソース |

裏どり:

- **Llama 2 / 3**: [Llama 2](https://arxiv.org/abs/2307.09288)(2023)、[Llama 3](https://arxiv.org/abs/2407.21783)(2024)。GQA 採用と学習レシピの公開
- **Mistral 7B / Mixtral**: [Mistral 7B](https://arxiv.org/abs/2310.06825)(2023)、[Mixtral of Experts](https://arxiv.org/abs/2401.04088)(2024)。SWA と MoE
- **DeepSeek-V3**: [Technical Report](https://arxiv.org/abs/2412.19437)(2024)。MLA + 細粒度 MoE の全容
- **Qwen2 / OLMo**: [Qwen2](https://arxiv.org/abs/2407.10671)(2024)、[OLMo](https://arxiv.org/abs/2402.00838)(2024、完全オープンの対照例)

## 簡略化したこと

- **各社の全世代は追わない**: 系列ごとの独自性が見える範囲に絞り、世代ごとの数値比較はしない
- **ライセンスの詳細は概略**: 商用可否や再配布条件はモデル・版で異なる。3 軸(重み・データ・コード)の枠組みだけ示した
- **微調整・量子化の実装はこの章では扱わない**: [LoRA](/parts/lora)と[量子化](/parts/quantization)で別に実装したので、ここでは各社が何を選んだかだけを見る
- **ベンチマーク数値なし**: 順位は頻繁に入れ替わるため、構造と設計思想に絞った

## 参考資料

- [Llama 3 Herd of Models](https://arxiv.org/abs/2407.21783)(2024)
- [Mixtral of Experts](https://arxiv.org/abs/2401.04088)(2024)
- [DeepSeek-V3 Technical Report](https://arxiv.org/abs/2412.19437)(2024)
- [OLMo: Accelerating the Science of Language Models](https://arxiv.org/abs/2402.00838)(2024) — 完全オープンの意義
