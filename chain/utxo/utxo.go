package utxo

import (
	"bytes"
	"crypto/ed25519"
	"sort"
)

// #region utxoset

// UTXOSet は「まだ消費されていない出力」の集まり。これがその瞬間の台帳=残高。
// 取引を適用するたびに、消費された出力が消え、新しい出力が加わる。
type UTXOSet struct {
	utxos map[OutPoint]TxOutput
}

// NewUTXOSet は空の UTXO セットを作る。
func NewUTXOSet() *UTXOSet {
	return &UTXOSet{utxos: map[OutPoint]TxOutput{}}
}

// Get は参照先の出力を返す。第 2 返り値が未使用として存在するかどうか。
func (s *UTXOSet) Get(op OutPoint) (TxOutput, bool) {
	out, ok := s.utxos[op]
	return out, ok
}

// Len は未使用出力の個数。
func (s *UTXOSet) Len() int { return len(s.utxos) }

// Balance は特定の所有者が持つ未使用出力の合計額。
// 「残高」という状態はどこにも保存されておらず、UTXO を数え上げて初めて分かる。
func (s *UTXOSet) Balance(owner ed25519.PublicKey) int {
	total := 0
	for _, out := range s.utxos {
		if bytes.Equal(out.Owner, owner) {
			total += out.Amount
		}
	}
	return total
}

// Apply は取引を台帳に反映する: input が指す出力を削除し、output を新規に加える。
// 検証(Validate)を通ったあとに呼ぶ前提。ここが「送金の確定」に当たる。
func (s *UTXOSet) Apply(tx Tx) {
	id := tx.ID()
	for _, in := range tx.Inputs {
		delete(s.utxos, in.Prev) // 消費された出力は二度と使えない=二重支払い防止
	}
	for i, out := range tx.Outputs {
		s.utxos[OutPoint{TxID: id, Index: i}] = out
	}
}

// Outpoints は現在の未使用出力を決定的な順序で返す(表示・列挙用)。
func (s *UTXOSet) Outpoints() []OutPoint {
	ops := make([]OutPoint, 0, len(s.utxos))
	for op := range s.utxos {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].TxID != ops[j].TxID {
			return ops[i].TxID < ops[j].TxID
		}
		return ops[i].Index < ops[j].Index
	})
	return ops
}

// #endregion utxoset
