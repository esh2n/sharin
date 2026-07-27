# rollup — Layer2 と fraud proof / validity proof

L1 は安全だが遅くて高い。**ロールアップ**は実行を L2 に逃がし、結果(state root)だけを
L1 に固めることでスループットを上げる。核心は「L1 は再実行しない」こと——では嘘の結果に
どう気づくか。**Optimistic(fraud proof)** と **ZK(validity proof)** の 2 系統を Go で作る。
ブロックチェーン編のパーツ。

教科書の章: [rollup](https://sharin-2a1.pages.dev/parts/rollup)

## これは何か

[evm 編](https://sharin-2a1.pages.dev/parts/evm)で見たように、コントラクト実行は全ノードが
なぞる。だから L1 は遅く、手数料も高い。**ロールアップ**の発想はこうだ:

- 取引の**実行を L2 に逃がす**(1 台の sequencer が大量にさばく)
- L1 には**結果の要約(state root)と取引データ**だけを投稿する
- **L1 は取引を再実行しない**。ただ root の列を記録する

これでスループットは跳ね上がる。だが致命的な問いが残る——**sequencer が嘘の root を
投稿したら?** 誰も再実行しないなら、残高の水増しも通ってしまう。ここで 2 つの流派に分かれる。

```
   Optimistic: とりあえず信じて記録。challenge 期間だけ異議を受け付ける。
      誰かが再実行 → fraud proof で嘘を暴く → そのバッチ以降を巻き戻し + 保証金没収
      安いが、確定まで待つ(数日)。1 人でも正直な監視者が要る。

   ZK: バッチに「計算が正しい証明(validity proof)」を添える。
      L1 は証明を検証するだけで確信 → 即確定。異議期間なし。監視者も不要。
      証明生成は重いが、ファイナリティが速い。
```

## 肝は3つ

1. **L1 は再実行しない**: 実行は L2、L1 は state root の列を記録するだけ。取引データも載せる(data availability)——載せないと誰も検証できない
2. **Optimistic = 事後に暴く**: 楽観的に受理し、challenge 期間内の fraud proof で不正を覆す。安いが遅い、正直な監視者が前提
3. **ZK = 事前に証明**: validity proof を commit 時に検証し即確定。証明生成は重いが速く、監視者不要

## ファイル

- `state.go` — `L2State`(残高)・`Tx`・`Execute`・`Root`(state root のハッシュ)
- `batch.go` — `Batch`(PrevRoot/PostRoot/Txs/Proof)と `Proof`(validity proof の模型)
- `sequencer.go` — `Sequencer`。`Propose`(正直)/`ProposeFraud`(嘘の PostRoot を主張)
- `rollup.go` — L1 コントラクト。`Commit`・`Challenge`(fraud proof)・`Finalize`・`CanonicalRoot`

## 設計メモ

- **fraud proof は witness 提示で検証**: L1 は状態を持たないので、告発者が「そのバッチの
  開始状態(witness)」を提示する。L1 は (1)witness が PrevRoot と一致 (2)Txs 適用の正しい
  PostRoot が主張値と一致、を確かめる。食い違えば不正確定
- **validity proof は「正直な prover しか作れない」を模す**: 暗号は自作せず、`Prove` が
  嘘の PostRoot には `Valid=false` を返す性質だけを再現。ZK の `Commit` はこれを検証して
  不正を commit 時に弾く
- **巻き戻しは連鎖する**: 不正バッチの上に積まれた後続も一緒に Reverted。canonical root は
  不正バッチの PrevRoot まで戻る
- **保証金の没収(slashing)**: 不正が証明されると sequencer の bond を没収。不正のコストを
  攻撃者に負わせる = 経済的な抑止
- **決定的**: 論理時計(`Tick`)で challenge 期間の経過を表す

## 簡略化したこと

- **ZK 暗号は模型**: SNARK/STARK は自作せず「正しい主張にだけ有効な証明が付く」性質のみ
- **単一 sequencer**: 分散 sequencer・強制 include・検閲耐性は扱わない
- **状態は残高のみ**: 一般的なコントラクト状態は [evm 編](https://sharin-2a1.pages.dev/parts/evm)に譲る
- **data availability は前提**: Txs は L1 に載る前提。DA レイヤ(EIP-4844 blob 等)は扱わない
- **対話的 fraud proof の二分探索**: 実際の optimistic rollup は 1 手ずつ争うが、ここは
  バッチ全体を一度に再実行する簡略版

## 動かす

```bash
go test ./chain/rollup/ -race -cover
go vet ./chain/rollup/
```

## 参考

- Vitalik Buterin, "An Incomplete Guide to Rollups" — optimistic と zk の対比の定番
- Ethereum.org, "Layer 2 / Rollups" — 概観と各実装
- [evm 編](https://sharin-2a1.pages.dev/parts/evm)(L2 が実行する中身)・[blockchain 編](https://sharin-2a1.pages.dev/parts/blockchain)
  (L1 の鎖)と合わせて読むと、レイヤの分担が見える
