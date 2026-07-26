# evm — アカウントモデルとスタックVM、そして gas

[utxo 編](https://sharin-2a1.pages.dev/parts/utxo)とは対の**アカウントモデル**(口座 → 残高 + 状態)と、
その上でコントラクトを動かす**スタックマシン**を Go で作る。gas 計量と、失敗時に状態を
巻き戻す**リバート**——EVM の心臓部を最小構成で。ブロックチェーン編のパーツ。

教科書の章: [evm](https://sharin-2a1.pages.dev/parts/evm)

## これは何か

UTXO は残高を持たず、未使用出力の集合で表した。だが**スマートコントラクト**は「状態を持つ
プログラム」だ——カウンタ、残高表、投票結果。状態を素直に置くには、口座ごとに残高と
ストレージを持つ**アカウントモデル**が向く。Ethereum がこちらを選んだ理由がそこにある。

コントラクトはバイトコードで、EVM という**スタックマシン**が 1 命令ずつ実行する
([bytecode 編](https://sharin-2a1.pages.dev/parts/bytecode)・[wasm 編](https://sharin-2a1.pages.dev/parts/wasm)の直系)。
決定的に違うのは 2 点:

- **gas**: 各命令に価格が付き、実行は前もって渡された gas を消費しながら進む。尽きたら
  強制終了。無限ループや重い計算を「有料化」して止める仕組み(停止性の担保)
- **リバート**: 実行が失敗(REVERT / out-of-gas / 不正命令)したら、その呼び出しでの
  **状態変更は全部巻き戻る**。ただし **gas は消費されたまま**——「計算させた対価は払う」

```
   alice ──Call(value, gas)──▶ contract
     1. 状態のスナップショットを取る
     2. 残高を移す(value)
     3. バイトコードを stack で実行、命令ごとに gas を引く
   成功 → 状態を確定、GasUsed = 使った分
   失敗 → 状態を巻き戻す、でも GasUsed は消費済み
```

## 肝は3つ

1. **アカウントモデル**: 口座 → 残高 + nonce + ストレージ。状態(コントラクトの変数)を直接持つ
2. **gas 計量**: 命令ごとに価格。前渡し gas を消費しながら実行し、尽きたら止める(有料化で停止性を担保)
3. **リバート**: 失敗すると状態は巻き戻るが gas は消費される。スナップショット + 巻き戻しで実装

## ファイル

- `account.go` — `Account`(nonce/balance/code/storage) と `State`。`snapshot`/`restore` で巻き戻し
- `opcode.go` — 命令定義(実 EVM に寄せたバイト値)と gas 単価表。SSTORE が飛び抜けて高い
- `vm.go` — スタックマシン本体。fetch-decode-execute、gas 消費、JUMPDEST 解析、二項演算
- `evm.go` — `EVM.Deploy` / `EVM.Call`。value 送金 + コード実行 + スナップショット/巻き戻し
- `asm.go` — `Disassemble`。バイトコードを "pc: NAME [operand]" に開く

## 設計メモ

- **word は uint64**: 本物の EVM は 256bit ワード。ここは読みやすさ優先で 64bit(桁あふれは wrap)
- **JUMPDEST 解析**: PUSH のデータ・バイトを飛び先にしないよう、事前に有効な JUMPDEST 位置を
  集める。「PUSH1 0x5b」の 0x5b は命令ではなくデータなので飛べない
- **gas は失敗でも消費**: `GasUsed = gasLimit - 残り`。out-of-gas は残りを 0 にするので全消費。
  これが「攻撃者に無限ループを走らせても、gas 上限で必ず止まり、対価は取る」を成立させる
- **CALLER は 1 ワードに畳む**: アドレス → uint64(fnv)。所有者チェック等の比較に使う簡略化

## 簡略化したこと

- **256bit でなく 64bit ワード / メモリ命令なし**: MLOAD/MSTORE・CALLDATA・LOG は省略
- **コントラクト間 CALL なし**: 1 段の呼び出しのみ(再入・委譲呼び出しは扱わない)
- **gas 還付・EIP ごとの価格改定なし**: 単価は固定の教材値
- **状態ルートなし**: 状態のハッシュ(Merkle Patricia Trie)は扱わない。状態は素の map

## 動かす

```bash
go test ./chain/evm/ -race -cover
go vet ./chain/evm/
```

## 参考

- Gavin Wood, "Ethereum: A Secure Decentralised Generalised Transaction Ledger"(Yellow Paper) — EVM の定義
- [utxo 編](https://sharin-2a1.pages.dev/parts/utxo)(対のモデル)・[bytecode 編](https://sharin-2a1.pages.dev/parts/bytecode)
  (自前スタックVM)と合わせて読むと、実行系の系譜がつながる
