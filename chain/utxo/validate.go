package utxo

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

// #region validate

// 検証で返しうる理由。呼び出し側が原因を見分けられるよう、番兵エラーで公開する。
var (
	ErrNoOutputs    = errors.New("utxo: 取引に出力がない")
	ErrNonPositive  = errors.New("utxo: 出力額は正でなければならない")
	ErrUnknownInput = errors.New("utxo: input が指す UTXO が存在しない(使用済みか未知)")
	ErrDoubleSpend  = errors.New("utxo: 同一 UTXO を同じ取引内で二重に使っている")
	ErrBadSignature = errors.New("utxo: 署名が所有者の鍵と一致しない")
	ErrInsufficient = errors.New("utxo: 入力合計が出力合計に足りない")
)

// Validate は取引が台帳に対して正しいかを確かめる。正しければ nil を返す。
//
// 通常取引で見るのは 4 点:
//  1. input が指す UTXO が「未使用として存在」するか
//  2. 同じ UTXO を同じ取引内で二度使っていないか(取引内の二重支払い)
//  3. その UTXO の所有者による署名が有効か(=消費する権利があるか)
//  4. 入力合計 >= 出力合計 か(差額は手数料)
//
// coinbase(input なし)は 1〜3 が無く、「出力が正の額を持つ」ことだけ確かめる。
func Validate(tx Tx, s *UTXOSet) error {
	if len(tx.Outputs) == 0 {
		return ErrNoOutputs
	}
	for _, out := range tx.Outputs {
		if out.Amount <= 0 {
			return ErrNonPositive
		}
	}

	if tx.IsCoinbase() {
		return nil // 無から生む出力。上限などの経済ルールは本章では簡略化
	}

	sig := tx.signingBytes()
	seen := map[OutPoint]bool{}
	inSum := 0
	for _, in := range tx.Inputs {
		if seen[in.Prev] {
			return fmt.Errorf("%w: %s#%d", ErrDoubleSpend, in.Prev.TxID, in.Prev.Index)
		}
		seen[in.Prev] = true

		prev, ok := s.Get(in.Prev)
		if !ok {
			return fmt.Errorf("%w: %s#%d", ErrUnknownInput, in.Prev.TxID, in.Prev.Index)
		}
		// 公開鍵は Input ではなく消費先 UTXO の Owner を使う——鍵のすり替えを許さない。
		if !ed25519.Verify(prev.Owner, sig, in.Sig) {
			return fmt.Errorf("%w: %s", ErrBadSignature, AddressOf(prev.Owner))
		}
		inSum += prev.Amount
	}

	outSum := 0
	for _, out := range tx.Outputs {
		outSum += out.Amount
	}
	if inSum < outSum {
		return fmt.Errorf("%w: 入力 %d < 出力 %d", ErrInsufficient, inSum, outSum)
	}
	return nil
}

// Fee は入力合計 − 出力合計(=手数料)を返す。coinbase では意味を持たないので 0。
// 取引が Validate を通る前提。存在しない input があれば error。
func Fee(tx Tx, s *UTXOSet) (int, error) {
	if tx.IsCoinbase() {
		return 0, nil
	}
	inSum := 0
	for _, in := range tx.Inputs {
		prev, ok := s.Get(in.Prev)
		if !ok {
			return 0, fmt.Errorf("%w: %s#%d", ErrUnknownInput, in.Prev.TxID, in.Prev.Index)
		}
		inSum += prev.Amount
	}
	outSum := 0
	for _, out := range tx.Outputs {
		outSum += out.Amount
	}
	return inSum - outSum, nil
}

// #endregion validate
