<script setup>
import AttentionVariantsDemo from '../components/AttentionVariantsDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# attention変種(MQA/GQA/MLA)

> 実装: [`llm/gqa/`](https://github.com/esh2n/sharin/tree/main/llm/gqa) / 実行: `go test ./llm/gqa/`

<Summary>
マルチヘッド attention はヘッドごとに独立の K/V を持つが、生成時にキャッシュする K/V のメモリはヘッド数に比例して膨らみ、長文生成のボトルネックになる。K/V だけをヘッド間で共有するのが MQA と GQA だ。この章では 3 方式を 1 つの実装で書き、違いが「Q ヘッドがどの K/V を引くか」の対応表だけであること、キャッシュが共有数に応じて縮むことを確かめる。
</Summary>

## この章で作るもの

[attention](/parts/attention) で作ったマルチヘッド attention(MHA)は、ヘッドごとに独立の Q/K/V を持っていた。表現力の点ではそれでよいが、実際に文章を生成させると別の問題が現れる。生成は 1 トークンずつ進み、各ステップで過去全トークンの K と V が要る。これを毎回再計算しないよう保存しておくのが KV キャッシュで(詳細は次の推論高速化の章)、そのサイズは次の式で決まる:

```
KVキャッシュ = 2 (KとV) × 系列長 × KVヘッド数 × ヘッド次元 × 層数 × バイト/値
```

MHA では KV ヘッド数 = ヘッド数なので、32 ヘッド × 4096 次元 × 32 層のモデルで 8k トークンを保持すると、1 リクエストだけで GB 級のメモリを食う。同時に多数のリクエストをさばく推論サーバでは、GPU メモリの大半がモデル本体ではなくこのキャッシュに消える。

この式でモデル側から動かせるのは KV ヘッド数だ。Q ヘッドは 32 のまま、K/V だけを共有して減らす。全ヘッドで 1 組まで減らすのが MQA(multi-query attention)、グループごとに 1 組持つ折衷が GQA(grouped-query attention)になる。

<FigureBox caption="3 方式の違いは Q ヘッドと K/V の対応表だけ。MHA は 1 対 1、MQA は全 Q が 1 組を共有、GQA はグループ単位で共有する。KV キャッシュは K/V の組数に比例して縮む">

```
MHA (32Q : 32KV)   Q0→KV0  Q1→KV1  Q2→KV2  Q3→KV3   キャッシュ 1
GQA (32Q :  8KV)   Q0→KV0  Q1→KV0  Q2→KV1  Q3→KV1   キャッシュ 1/4
MQA (32Q :  1KV)   Q0→KV0  Q1→KV0  Q2→KV0  Q3→KV0   キャッシュ 1/32
                   (図は 4 ヘッドに縮めた模式)
```

</FigureBox>

順に見ていく。

1. **3 方式は同じ計算**: 違いは「Q ヘッド h がどの KV ヘッドを引くか」の対応表だけ。attention の式そのものは変わらない
2. **キャッシュの式に NHeads は現れない**: 効くのは KV ヘッド数。そこだけ減らせばキャッシュが線形に縮む
3. **品質と削減の折衷が GQA**: MQA は最大削減だが品質が下がりやすい。グループ共有の GQA が現在の主流

## ① 構成: KV ヘッド数という設計変数

実装は 3 方式を別々に書かず、KV ヘッド数を変数にした一般形として書く。NKVHeads = NHeads で MHA、= 1 で MQA、その間が GQA だ:

<<< ../../llm/gqa/gqa.go#config{go}

`KVCacheFloats` が上の式の実装で、NHeads がどこにも現れないことがそのまま「Q ヘッドを減らさずキャッシュだけ減らせる」理由になっている。テストでは 32:8 の GQA が MHA の 1/4、MQA が 1/32 になることを固定した。

## ② forward: 共有の実体は対応表

計算本体を見ると、K/V の射影は KV ヘッド数ぶんしか行われない。各 Q ヘッドは `KVHeadFor` で自分の共有先を引く:

<<< ../../llm/gqa/gqa.go#forward{go}

MHA との差分はこれだけだ。このことはテストでも確かめている。全 KV ヘッドの重みを同一にすると、MHA・GQA・MQA の出力は完全に一致する。つまり 3 方式の違いは K/V の中身が何通りあるかだけで、attention という計算の形は共有していない場合と寸分違わない。

品質面の直観も同じ場所から出る。ヘッドの多様性のうち「何を探すか」(Q)は全ヘッドぶん残り、「何を持っているか」(K/V)の多様性だけが減る。MQA まで削ると品質低下が測定できるレベルで現れることがあり、Llama 2 70B 以降の主要モデルは 4〜8 ヘッドに 1 組の GQA に落ち着いている。

## ③ MLA: 共有ではなく圧縮する

DeepSeek-V2/V3 はさらに別の路線を取った。MLA(multi-head latent attention)は K/V をヘッド間で共有するのではなく、低ランクの潜在ベクトルに圧縮してキャッシュする。K/V を作る前の中間表現(数百次元)だけを保存し、attention 時にそこから各ヘッドの K/V を復元する。

- **GQA との違い**: GQA は「K/V の種類を減らす」、MLA は「K/V の元を細くする」。MLA はヘッドごとの多様性を保ったままキャッシュを削れる
- **代償**: 復元のための行列積が毎ステップ増える。メモリと引き換えに計算を払う設計で、メモリ律速の推論サーバでは得になる

DeepSeek-V2 の報告では、MLA は MHA 比で KV キャッシュを 93% 削減しつつ品質を上回った。ここでは仕組みの解説に留め、実装は共有系(MQA/GQA)までとした。

### 動かす

下のデモは「対応表」と「キャッシュの式」をそのまま操作できる。MHA / GQA / MQA を切り替えると、8 つの Q ヘッドから K/V への線のつながりが変わり、系列長を伸ばすと KV キャッシュのバーが方式ごとに違う速さで伸びる。同じ系列長でもキャッシュが 1/4、1/8 になることが見て取れる。

<AttentionVariantsDemo />

## 設計の観点

- **なぜ K/V だけ共有するか**: 生成時にキャッシュされるのは K と V だけ(Q は毎ステップ新トークンのものを作れば済む)。だから Q を減らしてもメモリは減らず、K/V を減らせば線形に減る
- **品質への影響の非対称**: Q の多様性は「どこを見るか」の多様性で、これを残せば各ヘッドは共有 K/V から違う場所を引ける。K/V の多様性削減の方が影響が小さいという経験則が GQA の根拠
- **GQA のグループ数の選び方**: 実務では GPU のテンソル並列数と揃えることが多い(KV ヘッド 8 で 8 GPU に 1 つずつ)。ハードウェア構成が設計に染み出す例
- **MLA との使い分け**: 共有(GQA)は実装が軽く既存構造を保つ。圧縮(MLA)は削減率と品質で勝るが、復元計算と実装の複雑さを払う
- **アップトレイン**: 既存 MHA モデルの K/V ヘッドを平均して GQA 化し、少量の追加学習で回復させられる(GQA 論文の手法)。ゼロから学習し直さなくてよい

## メリット・デメリットと実例

| 方式 | KVキャッシュ | 品質 | 実例 |
|---|---|---|---|
| MHA | 1(基準) | 基準 | GPT-2/3、初期 Llama、mini-GPT |
| MQA | 1/NHeads | 低下が出やすい | PaLM、Falcon-7B、Gemini 1.0 Pro 系の一部 |
| GQA | NKVHeads/NHeads | ほぼ維持 | Llama 2 70B / Llama 3、Mistral、Qwen 2 以降 |
| MLA | 数%まで圧縮 | 維持〜向上の報告 | DeepSeek-V2 / V3 / R1 |

裏どり:

- **MQA(Shazeer 2019)**: "Fast Transformer Decoding" で提案。1 人の著者による 4 ページの論文が、のちの推論効率化路線の起点になった
- **GQA(Google 2023)**: MQA の品質低下と MHA のメモリの間を取る論文。既存モデルからのアップトレイン手順も示し、Llama 2 70B が採用して標準化した
- **Llama 3(2024)**: 8B モデルまで GQA(32Q:8KV)を採用。小型でも長コンテキスト運用でキャッシュが効くため
- **DeepSeek-V2(2024)**: MLA の初出。KV キャッシュ 93% 削減の報告とともに、共有ではなく圧縮という第 3 の路線を示した

## 簡略化したこと

- **出力射影(Wo)なし**: ヘッド連結までを実装。実物は連結後にもう 1 枚射影が入る
- **KV キャッシュ自体は未実装**: この章はキャッシュのサイズを決める構造の話。キャッシュを実際に持って生成を速くする実装は次の推論高速化の章
- **MLA は解説のみ**: 低ランク圧縮と復元の実装は行わない
- **RoPE との結合なし**: 実物は Q/K に [RoPE](/parts/rope) を挟む。MLA では RoPE との両立に追加の工夫が要る(decoupled RoPE)

## 参考資料

- Shazeer, [Fast Transformer Decoding: One Write-Head is All You Need](https://arxiv.org/abs/1911.02150)(2019) — MQA 原論文
- Ainslie et al., [GQA: Training Generalized Multi-Query Transformer Models](https://arxiv.org/abs/2305.13245)(2023) — GQA とアップトレイン
- DeepSeek-AI, [DeepSeek-V2](https://arxiv.org/abs/2405.04434)(2024) — MLA の初出
- [Llama 2 Technical Report](https://arxiv.org/abs/2307.09288)(2023) — 70B での GQA 採用
- 実装: [llm/gqa](https://github.com/esh2n/sharin/tree/main/llm/gqa)
