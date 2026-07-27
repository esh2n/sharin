# chain/substrate — Substrate/Polkadot 風ランタイムの最小実装

ブロックチェーンのロジック(ランタイム)を、チェーン上の差し替え可能なコードとして持つ設計を
決定的な Go でモデル化する。3 本柱:

1. **pallet 合成 (FRAME)**(`pallet.go`): ランタイムは system・balances・staking などの pallet の
   合成。各 pallet は dispatchable な呼び出しと weight を持ち、共有ストレージを読み書きする。
2. **ランタイム = 状態遷移関数**(`runtime.go`): extrinsic を pallet に dispatch して状態を進める。
   1 ブロックの仕事量は weight で予算化し、上限を超える extrinsic は弾く(gas の一般化)。
3. **forkless upgrade**(`pallet.go` の system.set_code): ランタイムを取引 1 本で差し替える。
   spec_version が上がり pallet 集合が入れ替わっても、ストレージ(残高など)は引き継がれる。
   ノードの更新もハードフォークも要らない。

## 実行

```sh
go test -race -cover ./chain/substrate/
```

`-race`・`go vet` クリーン、カバレッジ 98.5%。実 Wasm 実行・SCALE エンコード・署名・合意は
使わず、ランタイム差し替えと weight の意味論だけを取り出している。教科書の該当章は
`docs/parts/substrate.md`。
