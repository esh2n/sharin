package substrate

import "fmt"

// pallet.go は FRAME(ランタイムを機能モジュール = pallet の合成で作る枠組み)と、
// forkless upgrade(ランタイムをチェーン上で差し替える)をモデル化する。

// #region types

// Call は extrinsic の中身(どの pallet のどの dispatchable を、どんな引数で呼ぶか)。
// 教育用に引数は固定フィールドで持つ(本物は SCALE エンコードされた任意のバイト列)。
type Call struct {
	Pallet string
	Method string
	From   string
	To     string
	Amount uint64
	Code   *RuntimeCode // system.set_code 用の新ランタイム
}

// Extrinsic はブロックに含まれる 1 件の取引(署名は簡略化して省く)。
type Extrinsic struct {
	Call Call
}

// Event は dispatch が発火する記録。
type Event struct {
	Pallet  string
	Message string
}

// Dispatch は 1 つの dispatchable の本体。共有ストレージ(rt)と呼び出しを受けて状態を進める。
type Dispatch func(rt *Runtime, c Call) ([]Event, error)

// Pallet は機能モジュール。dispatchable の集合と、それぞれの weight を持つ。
// ランタイムはこの Pallet を複数合成して作る(FRAME の考え方)。
type Pallet struct {
	Name    string
	methods map[string]Dispatch
	weights map[string]uint64
}

// RuntimeCode は「チェーンに載るランタイムそのもの」。spec_version と pallet 集合からなる。
// これを差し替えるのが forkless upgrade——ノードもフォークも要らず、ロジックだけ入れ替わる。
type RuntimeCode struct {
	SpecVersion uint32
	Pallets     []Pallet
}

// #endregion types

// balKey / stakeKey はストレージのキー命名(pallet ごとに名前空間を分ける)。
func balKey(who string) string   { return "balances:" + who }
func stakeKey(who string) string { return "staking:" + who }

// #region balances

// balancesPallet は残高を扱う pallet。transfer は from → to へ amount を移す。
// dispatch は check-then-write で書く——先に検査し、通ってから状態を触るので、
// 失敗しても状態は汚れない(weight だけは消費される)。
func balancesPallet() Pallet {
	return Pallet{
		Name:    "balances",
		weights: map[string]uint64{"transfer": 100},
		methods: map[string]Dispatch{
			"transfer": func(rt *Runtime, c Call) ([]Event, error) {
				if rt.Get(balKey(c.From)) < c.Amount {
					return nil, fmt.Errorf("%w: %s", ErrInsufficient, c.From)
				}
				rt.Set(balKey(c.From), rt.Get(balKey(c.From))-c.Amount)
				rt.Set(balKey(c.To), rt.Get(balKey(c.To))+c.Amount)
				return []Event{{Pallet: "balances", Message: fmt.Sprintf("transfer %d: %s → %s", c.Amount, c.From, c.To)}}, nil
			},
		},
	}
}

// stakingPallet は v2 で追加される新機能。bond は残高を staking へ移す。
// v1 には存在しないので、v1 で staking.bond を呼ぶと ErrUnknownPallet になる。
// forkless upgrade 後に初めて使えるようになる——「フォークせず機能を足す」を体現する。
func stakingPallet() Pallet {
	return Pallet{
		Name:    "staking",
		weights: map[string]uint64{"bond": 150},
		methods: map[string]Dispatch{
			"bond": func(rt *Runtime, c Call) ([]Event, error) {
				if rt.Get(balKey(c.From)) < c.Amount {
					return nil, fmt.Errorf("%w: %s", ErrInsufficient, c.From)
				}
				rt.Set(balKey(c.From), rt.Get(balKey(c.From))-c.Amount)
				rt.Set(stakeKey(c.From), rt.Get(stakeKey(c.From))+c.Amount)
				return []Event{{Pallet: "staking", Message: fmt.Sprintf("bond %d: %s", c.Amount, c.From)}}, nil
			},
		},
	}
}

// #endregion balances

// #region upgrade

// systemPallet はランタイム自身を管理する pallet。set_code が forkless upgrade の入口。
// 新ランタイムコードを受け取り、spec_version が今より新しいことだけを確かめて差し替える。
// これ自体がただの dispatchable なので、アップグレードは「取引 1 本」で起きる。
func systemPallet() Pallet {
	return Pallet{
		Name:    "system",
		weights: map[string]uint64{"set_code": 200},
		methods: map[string]Dispatch{
			"set_code": func(rt *Runtime, c Call) ([]Event, error) {
				if c.Code == nil {
					return nil, ErrNoCode
				}
				if c.Code.SpecVersion <= rt.specVersion {
					return nil, fmt.Errorf("%w: %d <= %d", ErrStaleUpgrade, c.Code.SpecVersion, rt.specVersion)
				}
				old := rt.specVersion
				rt.applyCode(*c.Code) // pallet 集合を差し替え。ストレージ(状態)は保たれる
				return []Event{{Pallet: "system", Message: fmt.Sprintf("runtime upgraded: spec %d → %d", old, rt.specVersion)}}, nil
			},
		},
	}
}

// RuntimeV1 は初期ランタイム。system と balances だけを持つ。
func RuntimeV1() RuntimeCode {
	return RuntimeCode{SpecVersion: 1, Pallets: []Pallet{systemPallet(), balancesPallet()}}
}

// RuntimeV2 は staking を足したアップグレード版。set_code でこれを載せると、
// 既存の残高はそのままに、staking.bond が新たに使えるようになる。
func RuntimeV2() RuntimeCode {
	return RuntimeCode{SpecVersion: 2, Pallets: []Pallet{systemPallet(), balancesPallet(), stakingPallet()}}
}

// #endregion upgrade
