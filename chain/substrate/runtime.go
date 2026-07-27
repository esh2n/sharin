// Package substrate は Substrate/Polkadot 風のランタイムを最小構成でモデル化する。
//
// Substrate の設計思想は「ブロックチェーンのロジック(ランタイム)を、チェーンの状態として
// 持つ」ことにある。普通のチェーンでは、取引を処理する規則(送金・手数料・合意)はノードの
// ソフトに焼き込まれ、規則を変えるには全ノードが更新して**ハードフォーク**する必要がある。
// Substrate はランタイムを Wasm バイナリとしてチェーンに載せ、ノードはそれを読んで実行する。
// だから規則の入れ替えは「チェーン上のランタイムを差し替える取引」1 本で済み、ノードの更新も
// フォークも要らない(forkless upgrade)。
//
// ランタイムは pallet(機能モジュール)の合成でできている(FRAME)。各 pallet は
// dispatchable な呼び出し(extrinsic の中身)を持ち、共有ストレージ(状態トライ)を読み書きする。
// このパッケージは、ランタイム(状態遷移関数)・pallet 合成・weight による資源計量・
// そして forkless upgrade を、決定的な Go で組む。
package substrate

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownPallet  = errors.New("substrate: そのような pallet は無い")
	ErrUnknownMethod  = errors.New("substrate: その dispatchable は無い")
	ErrWeightExceeded = errors.New("substrate: ブロックの weight 上限を超える")
	ErrInsufficient   = errors.New("substrate: 残高が足りない")
	ErrNoCode         = errors.New("substrate: set_code に新ランタイムコードが無い")
	ErrStaleUpgrade   = errors.New("substrate: spec_version が現在以下(アップグレードにならない)")
)

// #region runtime

// Runtime は状態遷移関数そのもの。共有ストレージ(状態トライ)と、それを操作する pallet の
// 集合(=ランタイムコード)を持つ。ブロックの実行とは、extrinsic をこの Runtime に順に
// 適用してストレージを進めることに等しい。
type Runtime struct {
	specVersion uint32
	storage     map[string]uint64 // 状態トライの最小版(pallet:key → 値)
	pallets     map[string]Pallet // 差し替え可能なロジック = ランタイムコード
	weightLimit uint64            // 1 ブロックが使える weight の上限
	used        uint64            // 現ブロックで消費した weight
	events      []Event
}

// NewRuntime は初期ランタイムコードと weight 上限、初期状態(genesis)から Runtime を作る。
func NewRuntime(code RuntimeCode, weightLimit uint64, genesis map[string]uint64) *Runtime {
	rt := &Runtime{
		storage:     map[string]uint64{},
		weightLimit: weightLimit,
	}
	for k, v := range genesis {
		rt.storage[k] = v
	}
	rt.applyCode(code)
	return rt
}

// SpecVersion は現在のランタイムのバージョン。forkless upgrade で単調に上がる。
func (rt *Runtime) SpecVersion() uint32 { return rt.specVersion }

// Get / Set はストレージ(状態トライ)への最小アクセス。pallet はこれ越しに状態を触る。
func (rt *Runtime) Get(key string) uint64 { return rt.storage[key] }
func (rt *Runtime) Set(key string, v uint64) {
	if v == 0 {
		delete(rt.storage, key)
		return
	}
	rt.storage[key] = v
}

// Events は現ブロックで発火したイベント。
func (rt *Runtime) Events() []Event { return rt.events }

// Weight は現ブロックの消費 weight と上限。
func (rt *Runtime) Weight() (used, limit uint64) { return rt.used, rt.weightLimit }

// applyCode はランタイムコード(pallet 集合)を差し替え、spec_version を更新する。
// ストレージ(状態)には触れない——ここが forkless upgrade の肝。ロジックだけ入れ替わり、
// 残高などの状態はそのまま引き継がれる。
func (rt *Runtime) applyCode(code RuntimeCode) {
	rt.specVersion = code.SpecVersion
	rt.pallets = make(map[string]Pallet, len(code.Pallets))
	for _, p := range code.Pallets {
		rt.pallets[p.Name] = p
	}
}

// Execute は 1 つの extrinsic を dispatch する。pallet → method を引き、weight 予算を
// 確認してから実行する。weight は「試みた分」として実行前に確保する(失敗しても消費される)。
func (rt *Runtime) Execute(ex Extrinsic) ([]Event, uint64, error) {
	p, ok := rt.pallets[ex.Call.Pallet]
	if !ok {
		return nil, 0, fmt.Errorf("%w: %s", ErrUnknownPallet, ex.Call.Pallet)
	}
	d, ok := p.methods[ex.Call.Method]
	if !ok {
		return nil, 0, fmt.Errorf("%w: %s.%s", ErrUnknownMethod, ex.Call.Pallet, ex.Call.Method)
	}
	w := p.weights[ex.Call.Method]
	if rt.used+w > rt.weightLimit {
		return nil, 0, ErrWeightExceeded // ブロックに入らない(実行しない)
	}
	rt.used += w // 試みた時点で weight を消費(失敗しても返らない)
	evs, err := d(rt, ex.Call)
	if err != nil {
		return nil, w, err // 失敗: weight は消費済み、状態は各 dispatch が check-then-write で汚さない
	}
	rt.events = append(rt.events, evs...)
	return evs, w, nil
}

// #endregion runtime

// #region block

// Result は 1 extrinsic の実行結果(ブロック実行の記録)。
type Result struct {
	Call   Call
	Ok     bool
	Err    string
	Weight uint64
	Events []Event
}

// ExecuteBlock は extrinsic 列を 1 ブロックとして実行する。ブロックの頭で weight メータと
// イベントをリセットし、各 extrinsic を順に dispatch した結果を返す。
func (rt *Runtime) ExecuteBlock(exs []Extrinsic) []Result {
	rt.used = 0
	rt.events = nil
	out := make([]Result, 0, len(exs))
	for _, ex := range exs {
		evs, w, err := rt.Execute(ex)
		r := Result{Call: ex.Call, Weight: w}
		if err != nil {
			r.Err = err.Error()
		} else {
			r.Ok = true
			r.Events = evs
		}
		out = append(out, r)
	}
	return out
}

// #endregion block
