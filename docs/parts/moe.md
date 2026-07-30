<script setup>
import MoeDemo from '../components/MoeDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# MoE(Mixture of Experts)

> 実装: [`llm/moe/`](https://github.com/esh2n/sharin/tree/main/llm/moe) / 実行: `go test ./llm/moe/`

<Summary>
FFN はモデルのパラメータの大半を占める。MoE はこの FFN を複数の expert に分割し、トークンごとにルータが上位 k 個だけを選んで通す。総パラメータは expert 数ぶん巨大になるのに、1 トークンの計算は k 個ぶんで済む。「大きいのに速い」の正体はこの分離だ。この章ではルータ、expert 群、負荷分散損失を実装し、パラメータの会計と偏り対策までテストで固定する。
</Summary>

## この章で作るもの

[Transformer全体像](/parts/transformer)で見たとおり、パラメータの約 3 分の 2 は FFN にある。モデルを賢くしたければ FFN を太らせたいが、太らせたぶんだけ全トークンの計算が重くなる。全部のトークンに全部のパラメータを使わせる限り、この比例関係からは逃げられない。

MoE はこの前提を崩す。FFN を N 個の expert に分割し、トークンごとに「どの expert に見せるか」をルータが決めて、上位 k 個だけを通す。総パラメータは N 倍に膨らむが、1 トークンが触るのは k 個ぶんだけだ。Mixtral 8x7B なら総量 47B に対しアクティブ 13B、DeepSeek-V3 なら総量 671B に対しアクティブ 37B で、計算コストは小さいモデル並みのまま知識の容量だけを増やしている。

<FigureBox caption="MoE 層。ルータがトークンごとに expert を top-k 選択し、選ばれた expert の出力を重み付きで混ぜる。総パラメータは expert 数に比例し、計算は k に比例する">

```
            ルータ(スコア上位 k を選ぶ)
トークン ──┬── expert 0 (FFN)  ← 選ばれた(重み 0.7)
           ├── expert 1 (FFN)     …計算しない
           ├── expert 2 (FFN)  ← 選ばれた(重み 0.3)
           └── expert 3 (FFN)     …計算しない
出力 = 0.7·E0(x) + 0.3·E2(x)
総量 = 全 expert ぶん(メモリ) / アクティブ = k 個ぶん(計算)
```

</FigureBox>

順に見ていく。

1. **総量とアクティブ量の分離**: メモリは expert 数に比例、計算は top-k に比例。この 2 つが別の変数になる
2. **ルータは学習される小さな線形層**: トークンの埋め込みからスコアを出し、上位 k 個を選んで softmax し直すだけ
3. **偏りは仕組みで防ぐ**: 放っておくとルータは少数の expert に集中して崩壊する。負荷分散の補助損失で均しへ誘導する

## ① パラメータの会計

まず「大きいのに速い」を式にする。総量とアクティブ量を別々に数えると:

<<< ../../llm/moe/moe.go#config{go}

テストでは 8 expert・top-2 の構成で、expert 部分の総量がアクティブの 4 倍になることを固定した。この比が「メモリは 4 倍払うが、計算は据え置き」の内訳になる。逆に言えば MoE の代償はメモリと、その上に乗る運用の複雑さで、GPU に載り切らない総量は複数 GPU への分散(expert 並列)と all-to-all 通信を要求する。

## ② ルーティング: 上位 k を選んで softmax し直す

ルータの実体は小さな線形層だ。トークンの埋め込みから expert ごとのスコアを出し、上位 k 個を選び、選ばれた分だけで softmax を取り直して混合の重みにする:

<<< ../../llm/moe/moe.go#route{go}

forward は各トークンについて、選ばれた expert の FFN だけを計算し、重み付きで混ぜる:

<<< ../../llm/moe/moe.go#forward{go}

境界の確認として、expert 1 個・top-1 の MoE は普通の FFN と完全に一致することをテストで固定している。MoE は FFN の置き換えであって、別の計算を持ち込んだわけではない。トークンごとに「どの重みで計算するか」が変わるだけだ。

ここで面白いのは、expert の分業が人間の設計ではなくルータの学習で生まれることだ。実物の観察では、句読点や数字、特定言語に反応する expert が自然に分化する一方で、人間が期待するような「数学 expert」「コード expert」の明確な分業になるとは限らない。分業の粒度も含めて学習が決める。

## ③ 負荷分散: 崩壊を損失で防ぐ

ルーティングには構造的な罠がある。学習の初期にたまたま良かった expert に多くのトークンが流れると、その expert だけがさらに学習されて強くなり、ますます選ばれる。放っておくと少数の expert に全トークンが集中し、残りは学習されないまま死ぬ。ルータの崩壊と呼ばれる現象だ。

対策は損失に均しの圧力を足すことだ。expert ごとの負荷の割合 f_e から補助損失を作る:

<<< ../../llm/moe/moe.go#balance{go}

均等なら 1、1 つに集中すると N になる値で、テストで両端を固定した。学習ではこれを小さな係数で本来の損失に足し、精度を最大化したいルータに「ただし偏るな」という第 2 の目標を課す。[コンシステントハッシュ](/parts/consistent-hashing) の偏り対策が仮想ノードという構造だったのに対し、こちらは学習信号で均すのが特徴で、DeepSeek-V3 は係数の動的調整だけで補助損失を実質不要にする改良まで進めている。

### 動かす

下のデモは 8 expert・top-2 の MoE をそのまま動かす。トークンを 1 つずつ流すと、ルータのスコアで選ばれる expert が変わり、各 expert の負荷カウンタが積み上がる。ルータを「偏った初期化」に切り替えると、特定の expert に集中して負荷分散損失が跳ね上がる様子が見える。

<MoeDemo />

## 設計の観点

- **何と何を交換しているか**: 計算(FLOPs)を据え置いてメモリと通信を払う。GPU の計算が余りメモリ帯域が逼迫する推論では、この交換が逆に苦しくなる場面もある
- **capacity factor**: 実物は expert ごとに受け入れ上限を置く。あふれたトークンは捨てる(drop)か次候補へ回す。上限がないと 1 バッチ内の偏りで特定 expert の計算が詰まり、並列効率が崩れる
- **expert 並列と all-to-all**: expert を GPU に分散すると、トークンをルーティング先の GPU へ送る all-to-all 通信が毎層走る。MoE の実効速度はネットワーク帯域で決まることが多い
- **推論サーバとの相性**: バッチ内のトークンが expert に散らばるほど、まとめ計算の効率が落ちる。バッチを大きくして expert ごとの塊を作るのが定石
- **なぜ FFN だけ分割するか**: attention は K/V がトークン間で共有される構造上、分割の利得が薄い。パラメータの大半が FFN にあることも理由

## メリット・デメリットと実例

| 構成 | 総量/アクティブ | 特徴 | 実例 |
|---|---|---|---|
| dense FFN | 1 倍 | 単純。運用が楽 | GPT-3、Llama 3 70B |
| MoE(top-2) | 4〜8 倍 | 計算据え置きで容量増 | Mixtral 8x7B(47B/13B) |
| 細粒度 MoE + 共有 expert | 10 倍超 | 小さい expert 多数 + 全員が通る共有部 | DeepSeek-V3(671B/37B) |
| MoE(非公開) | 不明 | GPT-4 が MoE という広く信じられた報告 | GPT-4(未確認)、Gemini 1.5 |

裏どり:

- **Switch Transformer(2021)**: top-1 の単純化と負荷分散損失の定式化で MoE を実用に載せた Google の論文。この章の補助損失はその簡略版
- **Mixtral 8x7B(2023)**: オープンモデルで MoE を普及させた実例。8 expert・top-2、総量 47B・アクティブ 13B で 70B 級 dense に並ぶ性能を示した
- **DeepSeek-V3(2024)**: 細粒度 expert 256 個 + 共有 expert、補助損失なしの負荷分散(バイアス動的調整)。MoE 設計の現在の到達点
- **Shazeer et al.(2017)**: LSTM 時代の Sparsely-Gated MoE。「条件付き計算」の発想自体は Transformer より古い

## 簡略化したこと

- **capacity factor なし**: あふれ処理(drop / 再ルーティング)は実装しない
- **expert 並列なし**: 全 expert が同一プロセスにいる。all-to-all 通信は設計の観点で言及のみ
- **学習なし**: 補助損失は値の計算まで。ルータが実際に均されていく過程は扱わない
- **expert は GELU FFN**: 実物(Mixtral 等)の expert は [SwiGLU](/parts/rmsnorm-swiglu) FFN

## 参考資料

- Fedus et al., [Switch Transformers](https://arxiv.org/abs/2101.03961)(2021) — top-1 ルーティングと負荷分散損失
- Jiang et al., [Mixtral of Experts](https://arxiv.org/abs/2401.04088)(2024) — オープン MoE の代表例
- DeepSeek-AI, [DeepSeek-V3 Technical Report](https://arxiv.org/abs/2412.19437)(2024) — 細粒度 + 共有 expert + 補助損失フリー
- Shazeer et al., [Outrageously Large Neural Networks](https://arxiv.org/abs/1701.06538)(2017) — Sparsely-Gated MoE の原典
- 実装: [llm/moe](https://github.com/esh2n/sharin/tree/main/llm/moe)
