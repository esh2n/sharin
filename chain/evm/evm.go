package evm

import (
	"errors"
	"fmt"
	"hash/fnv"
)

// #region evm

// ErrInsufficientBalance は value を送るだけの残高が送り主に無いとき。
var ErrInsufficientBalance = errors.New("evm: 残高が送金額に足りない")

// Result は 1 回の呼び出しの結果。
//
// 最重要なのは Reverted と GasUsed の組み合わせだ: 実行が失敗(revert / out-of-gas)
// しても GasUsed は消費される——「計算させた分の対価は払う」。一方で状態変更は
// 巻き戻る。つまり「gas は減るが、ストレージや残高は元通り」。ここが EVM の肝。
type Result struct {
	Success  bool     // 正常終了したか
	Reverted bool     // REVERT / out-of-gas / 実行時エラーで巻き戻したか
	GasUsed  uint64   // 消費した gas(失敗しても発生する)
	Stack    []uint64 // 終了時のスタック(RETURN 値の確認・デモ表示用)
	Err      error    // 失敗理由
}

// EVM は状態を持ち、その上で呼び出し(Call)と配備(Deploy)を実行する。
type EVM struct {
	state *State
}

// NewEVM は与えた状態に対して動く EVM を作る。
func NewEVM(state *State) *EVM { return &EVM{state: state} }

// State は現在の世界の状態を返す(残高・ストレージの照会用)。
func (e *EVM) State() *State { return e.state }

// Deploy は from が code を持つコントラクト口座を配備する。
// アドレスは from と nonce から決定的に導く(実 EVM と同じ考え方)。
func (e *EVM) Deploy(from Address, code []byte) Address {
	acc := e.state.GetOrCreate(from)
	addr := contractAddress(from, acc.Nonce)
	acc.Nonce++
	c := e.state.GetOrCreate(addr)
	c.Code = append([]byte(nil), code...)
	return addr
}

// Call は from から to へ value を送り、to がコントラクトならコードを実行する。
//
// 手順: (1)状態のスナップショットを取る (2)残高を移す (3)コードを実行する。
// 途中で失敗したら状態をスナップショットへ巻き戻す。ただし gas は消費されたまま。
func (e *EVM) Call(from, to Address, value, gasLimit uint64) Result {
	snap := e.state.snapshot()

	fromAcc := e.state.GetOrCreate(from)
	toAcc := e.state.GetOrCreate(to)

	// 残高不足は実行前に弾く(gas も使っていないので巻き戻すだけ)。
	if fromAcc.Balance < value {
		e.state.restore(snap)
		return Result{Reverted: true, Err: fmt.Errorf("%w: 残高 %d < value %d", ErrInsufficientBalance, fromAcc.Balance, value)}
	}
	fromAcc.Balance -= value
	toAcc.Balance += value

	// 受け手がコントラクトでなければ、ただの送金で終わり。
	if !toAcc.IsContract() {
		return Result{Success: true, GasUsed: 0}
	}

	m := &vm{
		code:   toAcc.Code,
		gas:    gasLimit,
		self:   toAcc,
		caller: from,
		value:  value,
		jumpOK: analyzeJumpdests(toAcc.Code),
	}
	err := m.run()
	gasUsed := gasLimit - m.gas

	if err != nil {
		// 失敗: 状態を巻き戻す。だが gas は消費済みとして報告する。
		e.state.restore(snap)
		return Result{Reverted: true, GasUsed: gasUsed, Stack: m.stack, Err: err}
	}
	return Result{Success: true, GasUsed: gasUsed, Stack: m.stack}
}

// contractAddress は配備者アドレスと nonce から決定的にコントラクトアドレスを導く。
func contractAddress(from Address, nonce uint64) Address {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", from, nonce)
	return Address(fmt.Sprintf("0xC%08x", h.Sum64()&0xffffffff))
}

// #endregion evm
