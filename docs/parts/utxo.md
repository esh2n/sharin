<script setup>
import UtxoDemo from '../components/UtxoDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# utxo(UTXO モデルと「送金」の正体)

<Summary>
Bitcoin をはじめ多くのチェーンは、口座の残高という数字をどこにも持たない。あるのは「まだ誰にも使われていない出力(UTXO)」の集まりだけで、残高は自分あての出力を数え上げて初めて分かる。送金は残高を書き換えるのではなく、過去の出力を消費して新しい出力を生む操作だ。所有権は出力に刻まれた公開鍵への署名で守り、二重支払いは一度使った出力が集合から消えることで防ぐ。
</Summary>

## この章で作るもの

[blockchain 編](/parts/blockchain)では、ハッシュチェーンと Proof of Work で「改竄しにくい追記ログ」を作った。だがそこには肝心の取引が無く、ブロックの中身はただの文字列だった。この章はその中身、「A が B に送る」の正体を作る。

問いは 2 つある。中央の銀行なしに「誰が何を持っているか」をどう決めるか。そして「他人の金を勝手に使えない」ようにどうするか。UTXO モデルの答えは、残高を持たないことだった。

<FigureBox caption="UTXO モデルの送金。coinbase が Alice に 50 を与える。Alice が Bob に 30 送るとき、その 50 を input で消費し、Bob への 30 と自分へのお釣り 20 を output に作る。元の 50 は UTXO セットから消え、二度と使えない。これが二重支払いを防ぐ">

```
   coinbase ──▶ out#0: 50 → Alice            UTXO セット: { (cb,0): 50→A }

   Alice → Bob 30:
      input : (cb,0) を消費          ← Alice の署名で「使ってよい」を証明
      output: [ 30 → Bob ][ 20 → Alice(お釣り) ]

   適用後                                    UTXO セット: { (tx,0): 30→B,
                                                          (tx,1): 20→A }
                                              ↑ (cb,0) は消えた = もう使えない
   残高 = 自分あて UTXO の合計:  Bob=30,  Alice=20
```

</FigureBox>

順に見ていく。

1. **残高は状態でなく集計**: どこにも残高は保存されない。UTXO セットを数え上げて初めて分かる
2. **送金 = 消費と生成**: 過去の出力を input で消し、新しい output を生む。入力合計と出力合計の差が手数料
3. **正しさは署名と一意消費**: 所有者だけが消費でき(署名)、同じ出力は二度使えない(UTXO セットから消える)

## ① 取引の形: input と output

取引は「過去の出力を消費し、新しい出力を生む」ものだ。だから 2 つの部品でできている。**output** は「誰にいくら」、すなわち受取人の公開鍵(Owner)と金額を刻む。**input** は過去の出力を 1 つ指す参照(`OutPoint`)と、それを消費してよいことを示す**署名**を持つ:

<<< ../../chain/utxo/tx.go#tx{go}

ここで効いているのが、input が公開鍵を**持たない**ことだ。検証時に使う鍵は、消費先 UTXO の `Owner` から取る。もし input に鍵を持たせたら、攻撃者は「自分の鍵とその署名」を添えて他人の出力を指し、奪えてしまう。鍵を出力側に固定することで、「その出力の所有者だけが消費できる」を強制する。

`ID` を署名(`Sig`)から独立させているのにも理由がある。署名する前に取引 ID が決まっていないと、「何に署名するのか」が循環してしまう。`signingBytes` は Sig を含めないので、ID は署名前に確定し、input はそれを安定して参照できる。

## ② UTXO セット: 残高を持たない台帳

台帳の実体は「未使用出力の集合」だ。取引を適用するとは、input が指す出力を**消し**、output を**加える**こと。残高はこの集合を数えるだけで、状態としては存在しない:

<<< ../../chain/utxo/utxo.go#utxoset{go}

`Apply` の `delete` が二重支払い防止の核心だ。一度消費された `OutPoint` は集合から消える。だから後から同じ出力を指す取引が来ても、「そんな UTXO は無い」と弾かれる。二重支払いは集合から消えることそのもので防がれ、特別なフラグも履歴走査も要らない。

`Balance` が「集計」なのも見どころだ。銀行なら口座の数字を読むだけだが、ここでは全 UTXO を走査して自分あてを足す。残高は保存された事実ではなく、その瞬間の集合から**導かれる**。

## ③ 検証: 署名・一意消費・収支

取引を台帳に適用してよいかは、`Validate` が決める。通常取引で見るのは 4 点。input が未使用として存在するか、同じ input を取引内で二度使っていないか、所有者の署名が有効か、入力合計が出力合計以上か(差が手数料):

<<< ../../chain/utxo/validate.go#validate{go}

`ed25519.Verify(prev.Owner, sig, in.Sig)` が所有権の門番だ。公開鍵は必ず**消費先 UTXO の Owner** を使う。coinbase(input なし)は「無から出力を生む」特別な取引で、署名も収支チェックも無い。マイニング報酬と手数料の回収がここから入る。

### 送金を組み立てる

送金は、自分あての UTXO を必要額まで集め、相手への output と自分へのお釣りを作り、署名する。この「集めてお釣りを作る」が現金の支払いそのものだ:

<<< ../../chain/utxo/transfer.go#transfer{go}

### 動かす

下のデモは、この筋書きを**そのままブラウザで**動かしている。「1手すすめる」で coinbase → 送金 → お釣り → 二重支払いの試み、と進む。UTXO セットが取引ごとにどう入れ替わるか、残高がどう集計されるか、そして二度目の消費がなぜ弾かれるかが見えるはずだ。

<UtxoDemo />

## 設計の観点: UTXO vs アカウント

- **なぜ残高を持たないか**: 「口座の数字を書き換える」には、その口座への並行アクセスを直列化する必要がある。UTXO は各出力が**独立に一度だけ**消費されるので、異なる UTXO を使う取引は互いに衝突せず**並列検証**しやすい。プライバシーも上がる(毎回アドレスを変えられる)
- **二重支払いをどう防ぐか**: 「消費 = 集合から削除」で、同じ出力は二度使えない。ネットワーク全体では「どの取引を先に鎖へ取り込むか」の合意([PoW](/parts/blockchain) や PoS)が、競合する二重支払いのどちらを正とするかを決める
- **UTXO の弱点**: 残高照会が「自分あて UTXO の走査」になり、状態(スマートコントラクトの変数)を素直に持てない。だから Ethereum は**アカウントモデル**(口座 → 残高 + ストレージ)を選んだ。次章の evm で扱う主題
- **お釣りと手数料**: 送金は input 合計を output で使い切る。余りはお釣りとして自分に戻し、意図的に残した差が手数料としてマイナーに渡る。「お釣りを作り忘れる」と全額が手数料になる(実際に起きた事故がある)
- **なぜ署名対象から Sig を外すか**: 署名前に ID が確定しないと循環する。また Sig を ID に含めると、署名を作り替えて別 ID にする**トランザクション展性(malleability)**が起き、未確認取引を参照する仕組み(Lightning など)が壊れる

## メリット・デメリットと実例

| モデル | 並列性 | 状態の持ちやすさ | プライバシー | 実例 |
|---|---|---|---|---|
| UTXO | ◎(独立出力を並列検証) | ✕(残高は集計、変数を持てない) | ○(アドレス使い捨て) | Bitcoin、Litecoin、Cardano(EUTXO) |
| アカウント | △(同口座は直列化) | ◎(残高 + ストレージ) | △(残高が紐づく) | Ethereum、多くの L1 |
| EUTXO(拡張) | ◎ | ○(出力にデータ + 検証子) | ○ | Cardano |

裏どり:

- **Bitcoin**: UTXO モデルの原典。ノードは「UTXO セット」を chainstate として保持し、これが実質の台帳。取引検証は各 input の UTXO 存在と署名(Script)チェック
- **Ethereum がアカウントを選んだ理由**: スマートコントラクトは「状態(変数)」を持つ。UTXO で状態を表すのは不自然なので、口座 → 残高 + ストレージのアカウントモデルにした。代わりに nonce で二重実行(リプレイ)を防ぐ
- **Cardano の EUTXO**: UTXO の並列性を保ちつつ、出力にデータと検証スクリプトを載せてコントラクトを可能にした拡張。UTXO とアカウントの中間
- **お釣り事故**: 「お釣り output を付け忘れて巨額を手数料にした」取引が現実に複数ある。UTXO は差額を明示的にお釣りへ戻す必要があるため、実装ミスが金額の消失に直結する

## 簡略化したこと

- **ブロック・PoW は別章**: 取引を鎖へ取り込む採掘と合意は [blockchain 編](/parts/blockchain)。ここは 1 つの UTXO セットに取引を順に適用する台帳として扱う
- **Script は署名に固定**: Bitcoin Script の一般性(マルチシグ・タイムロック・P2SH)は扱わず、「所有者の署名」に固定(P2PKH 相当)
- **署名は標準 ed25519**: 署名アルゴリズム自体は [crypto 編](/parts/crypto)の主題。ここは UTXO の構造に集中する
- **経済ルール省略**: coinbase の報酬上限・halving・難易度調整・mempool・手数料市場は扱わない

## 参考資料

- Satoshi Nakamoto, ["Bitcoin: A Peer-to-Peer Electronic Cash System"](https://bitcoin.org/bitcoin.pdf)(2008) — UTXO と二重支払い防止の原典
- Bitcoin Developer Guide, ["Transactions"](https://developer.bitcoin.org/devguide/transactions.html) — input/output と Script の実際
- Vitalik Buterin, ["A note on UTXOs vs Account/Balance model"](https://ethereum.stackexchange.com/questions/326/what-are-the-pros-and-cons-of-ethereum-balances-vs-utxos) — 2 モデルの比較
- 実装: [chain/utxo](https://github.com/esh2n/sharin/tree/main/chain/utxo)
