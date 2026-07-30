<script setup>
import SubstrateDemo from '../components/SubstrateDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# Substrate(ランタイム・pallet 合成・forkless upgrade)

<Summary>
多くのチェーンは規則をノードのソフトに焼き込むので、規則を変えるには全ノードが更新してハードフォークするしかない。Substrate はランタイム(規則)をチェーン上の差し替え可能なコードとして持ち、変更を取引 1 本で行う(forkless upgrade)。ランタイムは pallet の合成でできた状態遷移関数で、extrinsic を dispatch し、1 ブロックの仕事量は weight で予算化する。
</Summary>

## この章で作るもの

これまで作ってきたチェーンでは、取引を処理する規則はコードに焼き込まれていた。[UTXO 編](/parts/utxo)の送金規則も、[EVM 編](/parts/evm)の gas 価格も、変えるには全ノードがソフトを更新して足並みを揃える必要がある。これがハードフォークで、調整も合意も重い。

Substrate の発想は、その規則(ランタイム)自体をチェーンの状態として持つことだ。ノードは焼き込まれたロジックではなく、チェーン上のランタイムを読んで実行する。だから規則を変えるのは「ランタイムを差し替える取引」を 1 本流すだけで済む。ノードの更新もフォークも要らない。これが forkless upgrade で、この章の主眼になる。あわせて、ランタイムを機能モジュール(pallet)の合成で組む枠組み(FRAME)と、資源を計る weight を作る。

<FigureBox caption="ランタイムは状態遷移関数。extrinsic を受けて pallet に dispatch し、共有ストレージ(状態)を進める。ランタイム(ロジック)はチェーン上の差し替え可能なコードで、set_code で入れ替えても状態はそのまま引き継がれる">

```
   extrinsic ──▶ ┌──────── Runtime(状態遷移関数)────────┐
   (取引)         │  pallet を合成: system / balances /… │
                  │     │ dispatch(pallet.method)        │
                  │     ▼                                │
                  │  共有ストレージ(状態トライ)          │
                  └───────────────┬─────────────────────┘
                                  │ system.set_code(新コード)
                  ┌───────────────▼─────────────────────┐
                  │  ロジックだけ差し替え(spec v1→v2)    │
                  │  ストレージ(残高など)はそのまま      │
                  └──────────────────────────────────────┘
```

</FigureBox>

順に見ていく。

1. **pallet 合成(FRAME)**: ランタイムは system・balances などの pallet の合成。各 pallet は dispatchable な呼び出しと weight を持つ
2. **ランタイム = 状態遷移関数**: extrinsic を pallet に dispatch して共有ストレージを進める。1 ブロックの仕事量は weight で予算化する
3. **forkless upgrade**: ランタイムをチェーン上の差し替え可能なコードとして持ち、set_code 取引 1 本で入れ替える。状態は引き継がれ、フォークは要らない

## ① pallet: ランタイムを機能モジュールに分ける

まず部品を定める。`Call` は extrinsic の中身(どの pallet のどの呼び出しを、どんな引数で)。`Pallet` は dispatchable の集合とそれぞれの weight を持つモジュールで、`RuntimeCode` は spec_version と pallet 集合、つまり「チェーンに載るランタイムそのもの」だ:

<<< ../../chain/substrate/pallet.go#types{go}

`RuntimeCode` を値として持てることが、あとで forkless upgrade を可能にする。ランタイムは焼き込まれた固定物ではなく、差し替えられるデータになる。

具体的な pallet が balances だ。dispatch は check-then-write で書く。先に検査し、通ってから状態を触るので、失敗しても状態は汚れない:

<<< ../../chain/substrate/pallet.go#balances{go}

staking pallet は v1 には無く、あとで足す新機能として用意してある。ここが forkless upgrade の見せ場になる。

## ② ランタイム: 状態遷移関数と weight

ランタイムは状態遷移関数そのものだ。共有ストレージ(状態トライ)と pallet 集合を持ち、`Execute` が extrinsic を pallet に振り分ける。振り分ける前に weight の予算を確認し、超えるなら実行しない:

<<< ../../chain/substrate/runtime.go#runtime{go}

`weight` が gas を一般化したものだ。[EVM 編](/parts/evm)の gas が主に計算量に価格を付けたのに対し、weight は計算・ストレージ・呼び出しの重さをまとめて「1 ブロックがこなせる仕事量」として予算化する。`Execute` は実行前に weight を確保するので、失敗した extrinsic も weight を消費する。試みた分の対価を必ず取る、という点は gas と同じだ。

`applyCode` に注目してほしい。pallet 集合を差し替えても、ストレージには触れない。ロジックだけが入れ替わり、残高などの状態はそのまま残る。次の forkless upgrade は、この性質の上に成り立つ。

## ③ forkless upgrade: 取引 1 本でランタイムを変える

ランタイムの差し替えは、特別な儀式ではない。system pallet の `set_code` という、ただの dispatchable だ。新しい `RuntimeCode` を受け取り、spec_version が今より新しいことだけを確かめて差し替える:

<<< ../../chain/substrate/pallet.go#upgrade{go}

`set_code` が普通の呼び出しであることが要になる。ランタイムの更新が、送金と同じ「取引 1 本」として起きる。全ノードがソフトを更新して足並みを揃える必要はない。ノードはチェーン上のランタイムを読んで実行するので、差し替えられた瞬間から新しい規則で動く。v1 には無かった staking.bond が、v2 に上げた直後から使えるようになる。そのとき残高は `applyCode` によって保たれたままだ。

spec_version の単調増加チェックが、古いランタイムへの巻き戻しや同一版の無意味な差し替えを弾く。実際の Substrate では、この version の一致でノードが「ネイティブ実行できるか、チェーン上の Wasm を解釈実行するか」を切り替える。

### 動かす

下のデモは、この筋書きをそのままブラウザで動かしている。左の切り替えで 2 つの場面を比べられる。「extrinsic + weight」では、transfer を dispatch するたびに weight メータが埋まり、上限を超える 3 件目がブロックに入らず弾かれる。「forkless upgrade」では、v1 で staking.bond が弾かれ、set_code で v2 に上げた直後に同じ呼び出しが通る。残高がアップグレードをまたいで保たれることも見えるはずだ。

<SubstrateDemo />

## 設計の観点: なぜ forkless か

- **ハードフォークの何が重いか**: 規則をノードのソフトに焼き込むと、変更には全運用者の協調更新が要る。タイミングがずれるとチェーンが分岐(フォーク)する。Substrate はランタイムをチェーン上に置くことで、この協調を「オンチェーンの取引」に畳み込み、分岐リスクを消す
- **なぜ Wasm か**: 実際のランタイムは Wasm バイナリとしてチェーンに載る([wasm 編](/parts/wasm)で作った、移植可能で検証できるバイトコード)。どのノードも同じ Wasm を同じ結果で実行できるので、決定性が保たれる。本章は Wasm 実行までは踏み込まず、「ロジックが差し替え可能なデータである」ことに集中した
- **weight と gas の違い**: gas は主に計算量への課金、weight は計算 + ストレージ + 呼び出しを含む「ブロックの処理予算」。事前に各 dispatchable の weight を見積もり、ブロックが上限を超えないように詰める。手数料は weight から算出される
- **state migration の必要**: ストレージの形(スキーマ)が変わるアップグレードでは、古い状態を新しい形に移す migration が要る。本章は残高の形を変えないので不要だが、実際は set_code と同時に migration を走らせる。「状態は保たれる」は「形が同じなら」という条件付き
- **ガバナンス**: set_code を誰が呼べるかは重大だ。実際は sudo(単一の管理者)や on-chain 投票(referenda)で権限を絞る。ランタイムを自由に差し替えられる力は、そのまま統治の力になる

## メリット・デメリットと実例

| 論点 | 仕組み | 効果 | 実例 |
|---|---|---|---|
| フォークレス更新 | ランタイムをオンチェーンの Wasm に | 協調更新なしで規則を変える | Polkadot/Kusama の runtime upgrade |
| モジュール合成 | pallet の組み合わせで構築 | 機能の付け外しが宣言的 | FRAME の pallet 群 |
| 資源計量 | weight でブロック予算化 | 処理量の上限と手数料を決める | weights + transaction payment |
| 更新権限 | set_code をガバナンスで制御 | 誰が規則を変えるかを統治する | sudo / 投票(referenda) |

裏どり:

- **Polkadot / Kusama**: Substrate 製。ランタイムを何度も forkless upgrade してきた。Kusama は「先行して壊す」実験場として、活発にアップグレードを試す
- **FRAME と pallet**: `pallet-balances`・`pallet-staking`・`pallet-democracy` などの pallet を合成してランタイムを組む。本章の system/balances/staking はその最小の写し
- **Wasm ランタイム**: ランタイムは Wasm にコンパイルされチェーンに載る。ノードは spec_version を見て、ネイティブに持っていればネイティブ実行、無ければ Wasm を解釈実行する。[wasm 編](/parts/wasm)の「移植可能・検証可能」がここで効く
- **on-chain ガバナンス**: Polkadot の OpenGov では、ランタイム更新を含む提案をトークン保有者の投票で決める。「規則を変える力」を分散した統治に載せている

## 簡略化したこと

- **Wasm 実行なし**: 実ランタイムは Wasm バイナリだが、ここでは pallet を Go の値として持ち、差し替えの意味論だけを取り出す。Wasm 実行は [wasm 編](/parts/wasm)を参照
- **SCALE エンコードなし**: extrinsic の引数は固定フィールド。本物は SCALE コーデックで任意の型をバイト列にする
- **署名・手数料・nonce なし**: extrinsic の署名検証、weight からの手数料算出、リプレイ防止の nonce は扱わない
- **state migration なし**: ストレージの形を変えるアップグレードと、その移行処理は扱わない(設計の観点の節で言及)
- **ガバナンス・合意なし**: set_code の権限制御(sudo / 投票)とブロック生成の合意(BABE/GRANDPA)は範囲外

## 参考資料

- [Substrate / Polkadot SDK docs](https://docs.polkadot.com/) — FRAME・pallet・runtime upgrade の一次資料
- [FRAME overview](https://docs.substrate.io/learn/runtime-development/) — pallet を合成してランタイムを組む枠組み
- [wasm 編](/parts/wasm)(ランタイムが載る Wasm)・[evm 編](/parts/evm)(gas と weight の対比)と合わせて読むと、実行系の設計選択が見える
- 実装: [chain/substrate](https://github.com/esh2n/sharin/tree/main/chain/substrate)
