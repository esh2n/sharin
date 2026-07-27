<script setup>
import RollupDemo from '../components/RollupDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# rollup(Layer2 と fraud proof / validity proof)

<Summary>
L1 は安全な代わりに遅く、手数料も高い。ロールアップは取引の実行を L2 に逃がし、結果の要約(state root)と取引データだけを L1 に記録して速くする。問題は、L1 が再実行しないなら sequencer の嘘をどう見抜くか。答えは 2 つあり、事後に暴く optimistic(fraud proof)と、事前に証明する zk(validity proof)で、確定の速さと検証コストが逆になる。
</Summary>

## この章で作るもの

[evm 編](/parts/evm)で見たとおり、コントラクトの実行は全ノードがなぞる。だから L1 のスループットには天井がある。速くするには「みんなが実行する量」を減らすしかない。ロールアップは実行を 1 台の sequencer(L2)に集約し、L1 は結果の root を記録する監査役に徹する。

問題は、監査役が再実行しないなら不正をどう見抜くか。これが本章の主題で、答えは 2 通りある。optimistic は受理してから事後に fraud proof で罰する。zk は commit のときに validity proof を要求する。

<FigureBox caption="ロールアップの 2 系統。L2 で実行し、バッチ(PrevRoot→PostRoot + 取引)を L1 に投稿する。Optimistic は楽観的に受理し challenge 期間内の fraud proof で覆す。ZK は validity proof を commit 時に検証して即確定する">

```
   L2 (sequencer が実行)          L1 (再実行しない、root を記録)
   ┌─────────────────┐           ┌──────────────────────────────┐
   │ txs を適用        │  batch    │ Optimistic:                   │
   │ PrevRoot→PostRoot │ ───────▶ │   受理(Pending)→ 期間内に       │
   │ (+ 取引データ)     │           │   fraud proof 無ければ Final    │
   └─────────────────┘           │   不正なら巻き戻し + 保証金没収    │
                                  │ ZK:                            │
                                  │   validity proof を検証 → 即Final │
                                  │   証明無効なら commit 時に拒否    │
                                  └──────────────────────────────┘
```

</FigureBox>

肝は3つ:

1. **L1 は再実行しない**: 実行は L2、L1 は state root の列を記録するだけ。取引データも L1 に載せる(data availability)——載せないと誰も検証できない
2. **Optimistic = 事後に暴く**: 楽観的に受理し、challenge 期間内の fraud proof で覆す。安いが遅く、正直な監視者が前提
3. **ZK = 事前に証明**: validity proof を commit 時に検証して即確定。証明生成は重いが速く、監視者不要

## ① L2 の実行と state root

まず L2 側。sequencer は取引を適用して状態を進める。L1 が記録するのは状態そのものではなく、その**要約(state root)**——全残高を決定的に畳んだハッシュだ。L1 は短い root だけ持てばよく、残高の全体は持たない:

<<< ../../chain/rollup/state.go#state{go}

`Root` が鍵だ。L1 はこの 16 文字を記録するだけで「L2 が今どういう状態と主張しているか」を指させる。そして `Execute` は「正直に実行したらどうなるか」を返す。fraud proof も validity proof も、最終的にはこの正直な結果と sequencer の主張を突き合わせる作業に帰着する。

## ② バッチと 2 種類の「正しさの担保」

sequencer が L1 に投稿する単位が `Batch` だ。「PrevRoot から始めて、これらの Txs を実行し、PostRoot になった」という主張。取引データを丸ごと載せるのは、**誰でも再実行して検証できるようにする**ため(data availability):

<<< ../../chain/rollup/batch.go#batch{go}

`Proof` が ZK ロールアップの validity proof を模したものだ。本物は「PostRoot が正しい結果である」ことを状態を明かさずに検証できる暗号学的証明だが、その暗号自体は [crypto 編](/parts/crypto)の領域なのでここでは自作しない。代わりに「正直な prover だけが、正しい PostRoot に対して有効な証明を作れる」という性質だけをモデル化する。嘘の PostRoot を主張すると `Prove` は `Valid=false` を返す。これが後で ZK の門番になる。

## ③ L1 コントラクト: 受理・告発・確定

L1 側が本章の中心だ。`Commit` はモードで挙動が分かれる。Optimistic は**検証せずに Pending で受理**(だから安い)。ZK は **proof を検証して有効なら即 Final、無効なら拒否**。そして Optimistic だけが `Challenge`(fraud proof)を持つ:

<<< ../../chain/rollup/rollup.go#rollup{go}

`Challenge` の中身が fraud proof の本質だ。L1 は状態を持たないので、告発者が **witness(そのバッチの開始状態そのもの)**を提示する。L1 は「witness が PrevRoot と一致するか」「witness に Txs を適用した正しい PostRoot が主張値と一致するか」を確かめ、食い違えば不正確定となる。そのバッチ以降を全部巻き戻し、sequencer の保証金を没収する。この没収(slashing)が、不正のコストを攻撃者に負わせる経済的な抑止になる。

対して ZK の `Commit` は `!b.Proof.Valid` を commit の瞬間に弾く。不正はそもそも入れない。Optimistic が「入れてしまってから暴く」のと対照的だ。

### 動かす

下のデモは、この対比を**そのままブラウザで**動かしている。sequencer が正直なバッチと、残高を水増しした不正バッチを投稿する。右の切り替えで **Optimistic / ZK** を比べられる——Optimistic では不正が一旦通り、challenge で暴かれて巻き戻る(保証金も没収)。ZK では不正が commit 時に弾かれ、そもそも通らない。どちらが「事後」でどちらが「事前」かが見えるはずだ。

<RollupDemo />

## 設計の観点: なぜ 2 系統あるか

- **なぜ L1 は再実行しないのか**: 再実行したら L1 の負荷が減らず、スケールしない。ロールアップの価値は「L1 を監査役に徹させ、実行を 1 か所に集約する」点にある。だから正しさの担保を「再実行以外の方法」で用意する必要が生まれる
- **Optimistic の前提と弱点**: 「1 人でも正直な監視者がいて、challenge 期間内に fraud proof を出す」ことに安全性が乗っている。だから確定まで待つ(実際は約 7 日)。監視者がいない・検閲されると危うい。長所は commit が安いこと(検証しないから)
- **ZK の前提と弱点**: 暗号的に正しさが保証されるので監視者不要・即ファイナリティ。弱点は証明生成が計算的に重く、対応できる計算に制約があること(汎用 EVM の ZK 化は難所だった)
- **data availability が要**: 取引データが L1 に載っていないと、fraud proof も再構築もできない。「root だけ載せてデータは別」にすると、データを隠されて出金不能になりうる(data withholding)。EIP-4844 の blob はこの DA コストを下げる仕組み
- **なぜ保証金を没収するか**: 不正が「バレても損しない」なら、何度でも試せる。bond の slashing で「不正の期待値をマイナス」にして抑止する。fraud proof は「暴く仕組み」、slashing は「割に合わなくする仕組み」

この章の要点は「ロールアップは実行を L2 に逃がし L1 は root を記録するだけ。正しさの担保が optimistic(fraud proof + challenge 期間 + slashing、安いが遅く監視者前提)と zk(validity proof、即確定・監視者不要だが証明が重い)に分かれる。どちらも data availability が前提」に尽きる。

## メリット・デメリットと実例

| 方式 | 確定の速さ | commit コスト | 前提 | 実例 |
|---|---|---|---|---|
| Optimistic rollup | 遅い(challenge 期間) | 安い(検証しない) | 正直な監視者が 1 人以上 | Arbitrum、Optimism、Base |
| ZK rollup | 速い(即) | 高い(証明生成) | 証明系の健全性 | zkSync、StarkNet、Polygon zkEVM、Scroll |
| Validium | 速い | 高い | DA を L1 外に置く(信頼要) | StarkEx(一部) |

裏どり:

- **Arbitrum / Optimism / Base**: optimistic rollup。fraud proof と challenge 期間(約 7 日)を持ち、出金にその待ち時間がかかる。Arbitrum の対話的 fraud proof は「争点を二分探索で 1 命令まで絞る」洗練版
- **zkSync / StarkNet / Polygon zkEVM / Scroll**: zk rollup。validity proof で即ファイナリティ。汎用 EVM の ZK 化(zkEVM)は長く難所とされ、各社が実装を競った
- **EIP-4844(proto-danksharding)**: ロールアップのボトルネックだった DA コストを、専用の blob 領域で大幅に下げた(2024)。「data availability が要」を地で行く改善
- **The DAO 以降の設計思想**: 「信頼せず検証する」を、実行そのものでなく root + 証明/告発で成立させたのがロールアップ。L1 の安全性を借りつつスケールする

## 簡略化したこと

- **ZK 暗号は模型**: SNARK/STARK は自作せず「正しい主張にだけ有効な証明が付く」性質のみ再現
- **対話的 fraud proof の二分探索なし**: 実際の optimistic は 1 手ずつ争うが、ここはバッチ全体を一度に再実行する簡略版
- **単一 sequencer**: 分散 sequencer・強制 include・検閲耐性は扱わない
- **状態は残高のみ / DA は前提**: 一般的なコントラクト状態は [evm 編](/parts/evm)に、DA レイヤの中身は扱わない

## 参考資料

- Vitalik Buterin, ["An Incomplete Guide to Rollups"](https://vitalik.eth.limo/general/2021/01/05/rollup.html) — optimistic と zk の対比の定番
- Ethereum.org, ["Layer 2"](https://ethereum.org/en/layer-2/) と ["Rollups"](https://ethereum.org/en/developers/docs/scaling/) — 概観
- [evm 編](/parts/evm)(L2 が実行する中身)・[blockchain 編](/parts/blockchain)(L1 の鎖)と合わせて読むとレイヤの分担が見える
- 実装: [chain/rollup](https://github.com/esh2n/sharin/tree/main/chain/rollup)
