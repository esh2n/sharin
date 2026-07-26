# utxo — UTXO モデルと「送金」の正体

ブロックチェーンの送金は、残高を書き換える操作ではない。過去の**未使用出力(UTXO)**を
署名で消費し、新しい出力を生む——現金の支払いと同じだ。その仕組みを Go でモデル化する。
ブロックチェーン編のパーツ。

教科書の章: [utxo](https://sharin-2a1.pages.dev/parts/utxo)

## これは何か

銀行の台帳は「口座 → 残高」を持ち、送金でその数字を増減する。UTXO モデルは残高を
**どこにも保存しない**。あるのは「まだ誰にも使われていない出力」の集まり(UTXO セット)
だけ。残高は、自分あての UTXO を数え上げて初めて分かる。

取引(`Tx`)は 2 つの側面を持つ:

- **input**: 過去の出力を 1 つ指して「消費」する。その出力の所有者による**署名**を添える
- **output**: 「誰に・いくら」。受取人の公開鍵(Owner)を刻む

送金とは、自分あての UTXO を必要額まで集めて input にし、相手への output と、余りを
自分に戻す**お釣り**の output を作ること。入力合計と出力合計の差が**手数料**になる。

```
   coinbase ─▶ [out: 50 → Alice]                 UTXO: {A:50}
   Alice→Bob 30:
     in  = Alice の 50 を消費(Alice 署名)
     out = [30 → Bob][20 → Alice(お釣り)]         UTXO: {Bob:30, A:20}
                                                  ↑ 元の 50 は消え、二度と使えない
```

**二重支払い**は「同じ出力を 2 度消費できない」ことで防ぐ。一度 input で使われた
出力は UTXO セットから消えるので、後から同じものを指す取引は「存在しない input」
として弾かれる。**所有権**は署名で守る: 消費するには、その出力に刻まれた公開鍵に
対応する秘密鍵の署名が要る。

## 肝は3つ

1. **残高は状態でなく集計**: どこにも残高は無い。UTXO セットを数えて初めて分かる
2. **送金 = 消費と生成**: 過去の出力を input で消し、output で新しく作る。差が手数料
3. **正しさは署名と一意消費**: 所有者だけが消費でき(署名)、同じ出力は二度使えない(UTXO セット)

## ファイル

- `tx.go` — `OutPoint` / `TxInput` / `TxOutput` / `Tx`。取引 ID と署名対象(`signingBytes`)
- `wallet.go` — `Wallet`。seed から決定的に作る ed25519 鍵ペア。`Sign` / `Address`
- `utxo.go` — `UTXOSet`。`Apply`(input 削除 + output 追加)・`Balance`(集計)
- `validate.go` — `Validate`(存在・二重支払い・署名・収支)・`Fee`
- `transfer.go` — `Coinbase`・`BuildTransfer`(UTXO 収集 → お釣り → 署名)

## 設計メモ

- **署名は標準 ed25519**: 署名アルゴリズムそのものは [crypto 編](https://sharin-2a1.pages.dev/parts/crypto)
  の主題。ここでは標準ライブラリを使い、**UTXO の構造**に集中する。seed 指定で決定的化
- **公開鍵は input でなく UTXO 側**: 検証には消費先 UTXO の Owner を使う。input に鍵を
  持たせて他人の出力を奪う「鍵のすり替え」を許さないため
- **ID は署名を含めない**: `signingBytes` から ID を作るので、署名前でも参照が確定する。
  coinbase は `Memo`(ブロック高相当)で一意化し、同額 coinbase の ID 衝突を避ける
- **決定的**: 鍵も取引も seed/Memo から一意に決まる。テストとデモを毎回同じに再現できる

## 簡略化したこと

- **ブロック・PoW は別章**: 鎖への取り込みと採掘は [blockchain 編](https://sharin-2a1.pages.dev/parts/blockchain)。
  ここは 1 つの UTXO セットに取引を順に適用する台帳として扱う
- **Script は署名検証に固定**: Bitcoin Script の一般性(P2SH・マルチシグ等)は扱わず、
  「所有者の署名」に固定(P2PKH 相当)
- **経済ルールは省略**: coinbase の報酬上限・halving・難易度は扱わない
- **mempool・手数料市場なし**: 取引選択や手数料入札は扱わない

## 動かす

```bash
go test ./chain/utxo/ -race -cover
go vet ./chain/utxo/
```

## 参考

- Satoshi Nakamoto, "Bitcoin: A Peer-to-Peer Electronic Cash System"(2008) — UTXO と
  二重支払い防止の原典
- [blockchain 編](https://sharin-2a1.pages.dev/parts/blockchain)(鎖 + PoW)と合わせると、
  「取引 → ブロック → 鎖」の全体像がつかめる
- 次章 [evm](https://sharin-2a1.pages.dev/parts/evm) は、UTXO とは対の**アカウントモデル**を扱う
