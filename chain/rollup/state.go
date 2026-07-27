// Package rollup は Layer2 ロールアップの最小実装。
//
// L1(メインチェーン)は安全だが遅く高い。ロールアップは「実行を L2 に逃がし、
// 結果だけを L1 に固める」ことでスループットを上げる。核心は 1 つ——
// L1 はトランザクションを再実行しない。L2 が出した状態(の要約=state root)を
// ただ記録するだけ。では、L2 が嘘の結果を出したらどう気づくのか。
//
// 答えが 2 系統ある:
//   - Optimistic: とりあえず信じて記録し、一定期間(challenge window)だけ
//     「異議申立て」を受け付ける。誰かが再実行して不正を証明(fraud proof)すれば
//     巻き戻す。1 人でも正直な監視者がいれば安全。
//   - ZK: バッチに「計算が正しいことの証明(validity proof)」を添える。L1 は
//     証明を検証するだけで正しさを確信できる。異議期間は要らず即確定。
//
// このパッケージは L2 の実行(state.go)、バッチ(batch.go)、L1 側のロールアップ
// コントラクト(rollup.go)を組み、両系統の違いを決定的に再現する。
package rollup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// #region state

// L2State は L2 のアカウント残高。ここでは口座 → 残高の単純な写像に絞る。
type L2State struct {
	Balances map[string]uint64
}

// NewL2State は初期残高から L2 状態を作る。
func NewL2State(initial map[string]uint64) *L2State {
	b := make(map[string]uint64, len(initial))
	for k, v := range initial {
		b[k] = v
	}
	return &L2State{Balances: b}
}

// clone は状態の深いコピー。バッチ適用を副作用なく試すために使う。
func (s *L2State) clone() *L2State {
	return NewL2State(s.Balances)
}

// Root は状態の要約(state root)。全残高を決定的に畳んだハッシュ。
// L1 が記録するのはこの短い値だけで、残高そのものは持たない——ここがロールアップの肝。
func (s *L2State) Root() string {
	keys := make([]string, 0, len(s.Balances))
	for k := range s.Balances {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%d;", k, s.Balances[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Tx は L2 の送金。from が to へ amount を送る。
type Tx struct {
	From   string
	To     string
	Amount uint64
}

// apply は 1 つの取引を状態に反映する。残高不足なら false(この取引はスキップ扱い)。
// 正直な実行はこの規則に従う。不正な sequencer は、これに従わない state root を主張する。
func (s *L2State) apply(tx Tx) bool {
	if s.Balances[tx.From] < tx.Amount {
		return false
	}
	s.Balances[tx.From] -= tx.Amount
	s.Balances[tx.To] += tx.Amount
	return true
}

// Execute は取引列を順に適用した「正直な結果状態」を返す(元の状態は変えない)。
// fraud proof と validity proof は、どちらも「この正直な結果」と主張値を突き合わせる。
func Execute(pre *L2State, txs []Tx) *L2State {
	next := pre.clone()
	for _, tx := range txs {
		next.apply(tx)
	}
	return next
}

// #endregion state
