// Package utxo は UTXO(未使用トランザクション出力)モデルの最小実装。
//
// ブロックチェーンの「送金」の正体は、残高を書き換えることではない。
// 取引(Tx)は過去の出力(TxOutput)を input で「消費」し、新しい出力を生む。
// まだ誰にも消費されていない出力の集まり = UTXO セットが、その瞬間の「残高」を表す。
//
// 所有権は署名で示す: 出力には受取人の公開鍵(Owner)が刻まれ、それを消費するには
// その鍵の持ち主による署名が要る。二重支払いは「同じ出力を 2 度消費できない」
// (UTXO セットから消える)ことで防ぐ。これが銀行のような中央台帳なしに、
// 「誰が何を持っているか」と「勝手に使えないこと」を同時に成り立たせる仕掛け。
package utxo

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// #region tx

// OutPoint は「どの取引の何番目の出力か」を指す参照。UTXO セットの鍵になる。
type OutPoint struct {
	TxID  string // 参照先取引の ID
	Index int    // その取引の Outputs の何番目か
}

// TxOutput は「誰にいくら」を表す出力。Owner は受取人の公開鍵(=アドレス)。
// この出力を将来消費できるのは、Owner に対応する秘密鍵の持ち主だけ。
type TxOutput struct {
	Amount int
	Owner  ed25519.PublicKey
}

// TxInput は過去の出力を 1 つ消費する。Sig は「その出力の所有者による署名」で、
// 消費する権利を証明する。検証時の公開鍵は Input には持たせず、消費先 UTXO の
// Owner を使う——鍵をすり替えて他人の出力を奪えないようにするため。
type TxInput struct {
	Prev OutPoint
	Sig  []byte
}

// Tx は 1 つの取引。Inputs が空のものを coinbase と呼び、無から出力を生む
// (マイニング報酬 + 手数料の回収)。Memo は coinbase を一意にするためのタグ
// (実際のチェーンではブロック高など)で、同額 coinbase が同じ ID になるのを防ぐ。
type Tx struct {
	Inputs  []TxInput
	Outputs []TxOutput
	Memo    string
}

// IsCoinbase は input を持たない取引(=無から出力を生む)かどうか。
func (tx Tx) IsCoinbase() bool { return len(tx.Inputs) == 0 }

// signingBytes は署名と ID の対象になる正規バイト列。
// 署名(Sig)そのものは含めない——署名する前に確定している必要があるし、
// Sig を含めないことで ID が署名に左右されず、参照が安定する。
func (tx Tx) signingBytes() []byte {
	var b []byte
	b = append(b, tx.Memo...)
	b = append(b, 0)
	for _, in := range tx.Inputs {
		b = append(b, in.Prev.TxID...)
		b = binary.BigEndian.AppendUint32(b, uint32(in.Prev.Index))
	}
	b = append(b, 0)
	for _, out := range tx.Outputs {
		b = binary.BigEndian.AppendUint64(b, uint64(out.Amount))
		b = append(b, out.Owner...)
	}
	return b
}

// ID は取引を一意に指す短いハッシュ(消費参照に使う)。
// signingBytes から作るので、署名前でも決まる。
func (tx Tx) ID() string {
	sum := sha256.Sum256(tx.signingBytes())
	return hex.EncodeToString(sum[:])[:16]
}

// #endregion tx
