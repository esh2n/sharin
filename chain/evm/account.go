// Package evm はアカウントモデルと、その上で動くスタックマシン(EVM 風)の最小実装。
//
// UTXO モデル(chain/utxo)が「残高を持たず、未使用出力の集合で表す」のに対し、
// アカウントモデルは真逆——口座ごとに残高と状態(ストレージ)を直接持つ。
// スマートコントラクトは「状態を持つプログラム」なので、状態を素直に置けるこの
// モデルが向いていた。Ethereum がこちらを選んだ理由がそこにある。
//
// このパッケージが扱う中核は 3 つ:
//   - State: アドレス → アカウント(残高・nonce・ストレージ・コード)の写像
//   - VM: バイトコードを stack で実行し、gas を消費するインタプリタ
//   - 状態遷移: 実行が成功すれば State を書き換え、失敗(revert/out-of-gas)なら
//     巻き戻す。ただし gas は失敗しても消費される
package evm

// #region account

// Address は口座の宛先。実物は 20 バイトのハッシュだが、ここでは読みやすさ優先で文字列。
type Address string

// Account は 1 つの口座。残高に加えて、コントラクトなら nonce・コード・ストレージを持つ。
// ストレージは「スロット番号 → 値」の永続 KV で、コントラクトの状態(変数)はここに載る。
type Account struct {
	Nonce   uint64
	Balance uint64
	Code    []byte            // 空なら EOA(ただの口座)、非空ならコントラクト
	Storage map[uint64]uint64 // 永続ストレージ(SLOAD/SSTORE が読み書きする)
}

// clone はアカウントの深いコピー(スナップショット用)。
func (a *Account) clone() *Account {
	st := make(map[uint64]uint64, len(a.Storage))
	for k, v := range a.Storage {
		st[k] = v
	}
	code := append([]byte(nil), a.Code...)
	return &Account{Nonce: a.Nonce, Balance: a.Balance, Code: code, Storage: st}
}

// IsContract はコード(=実行可能なプログラム)を持つ口座かどうか。
func (a *Account) IsContract() bool { return len(a.Code) > 0 }

// State はアドレス → アカウントの写像。これがチェーンの「世界の状態」。
type State struct {
	accounts map[Address]*Account
}

// NewState は空の状態を作る。
func NewState() *State {
	return &State{accounts: map[Address]*Account{}}
}

// get は既存アカウントを返す(無ければ nil)。
func (s *State) get(addr Address) *Account { return s.accounts[addr] }

// GetOrCreate はアカウントを返し、無ければ空の口座を作る。
func (s *State) GetOrCreate(addr Address) *Account {
	if a := s.accounts[addr]; a != nil {
		return a
	}
	a := &Account{Storage: map[uint64]uint64{}}
	s.accounts[addr] = a
	return a
}

// Balance はアドレスの残高(無ければ 0)。
func (s *State) Balance(addr Address) uint64 {
	if a := s.accounts[addr]; a != nil {
		return a.Balance
	}
	return 0
}

// Storage はコントラクトのスロット値を返す(状態照会・デモ表示用)。
func (s *State) Storage(addr Address, slot uint64) uint64 {
	if a := s.accounts[addr]; a != nil {
		return a.Storage[slot]
	}
	return 0
}

// snapshot は状態全体の深いコピー。実行失敗時に巻き戻すために取る。
func (s *State) snapshot() *State {
	cp := &State{accounts: make(map[Address]*Account, len(s.accounts))}
	for addr, a := range s.accounts {
		cp.accounts[addr] = a.clone()
	}
	return cp
}

// restore はスナップショットで状態を差し替える(revert)。
func (s *State) restore(snap *State) { s.accounts = snap.accounts }

// #endregion account
