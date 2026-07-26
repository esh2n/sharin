package utxo

import (
	"crypto/ed25519"
	"errors"
	"sort"
)

// #region transfer

// ErrCannotAfford は送金額 + 手数料に足る UTXO を送り主が持っていないとき。
var ErrCannotAfford = errors.New("utxo: 残高が送金額 + 手数料に足りない")

// Coinbase は input を持たない取引を作る。無から amount を to に与える。
// memo で一意にする(同額 coinbase の ID 衝突を避ける)。マイニング報酬の入口。
func Coinbase(to ed25519.PublicKey, amount int, memo string) Tx {
	return Tx{
		Outputs: []TxOutput{{Amount: amount, Owner: to}},
		Memo:    memo,
	}
}

// BuildTransfer は from が to へ amount を送る取引を組み立てて署名する。
//
// UTXO モデルの送金は「差額を引く」のではなく、from が持つ未使用出力を
// 必要額に届くまで集めて input にし、to への出力と、余りを from へ戻す
// 「お釣り」出力を作る——現金の支払いと同じ。fee は入力合計と出力合計の差。
func BuildTransfer(s *UTXOSet, from *Wallet, to ed25519.PublicKey, amount, fee int) (Tx, error) {
	if amount <= 0 {
		return Tx{}, ErrNonPositive
	}
	need := amount + fee

	// from が所有する UTXO を決定的な順で集める。
	var mine []OutPoint
	for _, op := range s.Outpoints() {
		out, _ := s.Get(op)
		if string(out.Owner) == string(from.Pub) {
			mine = append(mine, op)
		}
	}
	sort.Slice(mine, func(i, j int) bool {
		oi, _ := s.Get(mine[i])
		oj, _ := s.Get(mine[j])
		if oi.Amount != oj.Amount {
			return oi.Amount > oj.Amount // 大きい出力から使い、input 数を抑える
		}
		if mine[i].TxID != mine[j].TxID {
			return mine[i].TxID < mine[j].TxID
		}
		return mine[i].Index < mine[j].Index
	})

	var inputs []TxInput
	gathered := 0
	for _, op := range mine {
		out, _ := s.Get(op)
		inputs = append(inputs, TxInput{Prev: op})
		gathered += out.Amount
		if gathered >= need {
			break
		}
	}
	if gathered < need {
		return Tx{}, ErrCannotAfford
	}

	outputs := []TxOutput{{Amount: amount, Owner: to}}
	if change := gathered - need; change > 0 {
		outputs = append(outputs, TxOutput{Amount: change, Owner: from.Pub}) // お釣り
	}

	tx := Tx{Inputs: inputs, Outputs: outputs}
	// 全 input を同じ送り主が持つので、本体に 1 度署名して各 input に載せる。
	sig := from.Sign(tx)
	for i := range tx.Inputs {
		tx.Inputs[i].Sig = sig
	}
	return tx, nil
}

// #endregion transfer
