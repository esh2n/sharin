package substrate

import (
	"errors"
	"testing"
)

func genesis() map[string]uint64 {
	return map[string]uint64{
		balKey("alice"): 1000,
		balKey("bob"):   0,
	}
}

func transfer(from, to string, amt uint64) Extrinsic {
	return Extrinsic{Call: Call{Pallet: "balances", Method: "transfer", From: from, To: to, Amount: amt}}
}

func TestTransferDispatch(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	evs, w, err := rt.Execute(transfer("alice", "bob", 300))
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if w != 100 {
		t.Fatalf("weight want 100 got %d", w)
	}
	if len(evs) != 1 || evs[0].Pallet != "balances" {
		t.Fatalf("event want balances got %+v", evs)
	}
	if rt.Get(balKey("alice")) != 700 || rt.Get(balKey("bob")) != 300 {
		t.Fatalf("残高 want 700/300 got %d/%d", rt.Get(balKey("alice")), rt.Get(balKey("bob")))
	}
}

func TestUnknownPalletAndMethod(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	if _, _, err := rt.Execute(Extrinsic{Call: Call{Pallet: "nope", Method: "x"}}); !errors.Is(err, ErrUnknownPallet) {
		t.Fatalf("unknown pallet want ErrUnknownPallet got %v", err)
	}
	if _, _, err := rt.Execute(Extrinsic{Call: Call{Pallet: "balances", Method: "nope"}}); !errors.Is(err, ErrUnknownMethod) {
		t.Fatalf("unknown method want ErrUnknownMethod got %v", err)
	}
}

func TestInsufficientDoesNotCorruptState(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	_, w, err := rt.Execute(transfer("alice", "bob", 5000)) // 残高超過
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("want ErrInsufficient got %v", err)
	}
	// 失敗しても weight は消費される(試みた対価)。
	if w != 100 {
		t.Fatalf("失敗時 weight want 100 got %d", w)
	}
	if used, _ := rt.Weight(); used != 100 {
		t.Fatalf("used weight want 100 got %d", used)
	}
	// 状態は汚れない(check-then-write)。
	if rt.Get(balKey("alice")) != 1000 || rt.Get(balKey("bob")) != 0 {
		t.Fatalf("失敗後の状態が変わった: %d/%d", rt.Get(balKey("alice")), rt.Get(balKey("bob")))
	}
}

func TestWeightLimit(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 250, genesis()) // transfer=100 なので 2 件で 200、3 件目で超過
	res := rt.ExecuteBlock([]Extrinsic{
		transfer("alice", "bob", 10),
		transfer("alice", "bob", 10),
		transfer("alice", "bob", 10),
	})
	if !res[0].Ok || !res[1].Ok {
		t.Fatalf("先頭 2 件は通るはず: %+v", res)
	}
	if res[2].Ok {
		t.Fatal("3 件目は weight 超過で弾かれるはず")
	}
	used, limit := rt.Weight()
	if used != 200 || limit != 250 {
		t.Fatalf("weight want 200/250 got %d/%d", used, limit)
	}
	// 弾かれた 3 件目のぶんは適用されない: 20 だけ移動。
	if rt.Get(balKey("bob")) != 20 {
		t.Fatalf("bob want 20 got %d", rt.Get(balKey("bob")))
	}
}

func TestBlockResetsWeight(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 250, genesis())
	rt.ExecuteBlock([]Extrinsic{transfer("alice", "bob", 10), transfer("alice", "bob", 10)})
	if used, _ := rt.Weight(); used != 200 {
		t.Fatalf("1 ブロック目 used want 200 got %d", used)
	}
	// 次ブロックで weight メータはリセットされる。
	rt.ExecuteBlock([]Extrinsic{transfer("alice", "bob", 10)})
	if used, _ := rt.Weight(); used != 100 {
		t.Fatalf("2 ブロック目 used want 100 got %d", used)
	}
}

func setCode(code RuntimeCode) Extrinsic {
	return Extrinsic{Call: Call{Pallet: "system", Method: "set_code", Code: &code}}
}

func TestForklessUpgradeAddsFeature(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	// v1 では staking pallet が無い。
	if _, _, err := rt.Execute(Extrinsic{Call: Call{Pallet: "staking", Method: "bond", From: "alice", Amount: 100}}); !errors.Is(err, ErrUnknownPallet) {
		t.Fatalf("v1 staking want ErrUnknownPallet got %v", err)
	}
	// alice が少し使っておく(状態が upgrade をまたいで保たれるか確認するため)。
	if _, _, err := rt.Execute(transfer("alice", "bob", 200)); err != nil {
		t.Fatalf("pre-upgrade transfer: %v", err)
	}
	// forkless upgrade: 取引 1 本で v2 へ。
	evs, _, err := rt.Execute(setCode(RuntimeV2()))
	if err != nil {
		t.Fatalf("set_code: %v", err)
	}
	if rt.SpecVersion() != 2 {
		t.Fatalf("spec_version want 2 got %d", rt.SpecVersion())
	}
	if len(evs) != 1 || evs[0].Pallet != "system" {
		t.Fatalf("upgrade event want system got %+v", evs)
	}
	// 残高はアップグレードをまたいで保たれる(ロジックだけ入れ替わり、状態は不変)。
	if rt.Get(balKey("alice")) != 800 || rt.Get(balKey("bob")) != 200 {
		t.Fatalf("upgrade 後の残高 want 800/200 got %d/%d", rt.Get(balKey("alice")), rt.Get(balKey("bob")))
	}
	// v2 では staking.bond が使えるようになる。
	if _, _, err := rt.Execute(Extrinsic{Call: Call{Pallet: "staking", Method: "bond", From: "alice", Amount: 300}}); err != nil {
		t.Fatalf("v2 staking.bond: %v", err)
	}
	if rt.Get(balKey("alice")) != 500 || rt.Get(stakeKey("alice")) != 300 {
		t.Fatalf("bond 後 want bal500/stake300 got %d/%d", rt.Get(balKey("alice")), rt.Get(stakeKey("alice")))
	}
}

func TestUpgradeRejectsStaleVersion(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	// 同じ spec_version(=1)への set_code は拒否。
	if _, _, err := rt.Execute(setCode(RuntimeV1())); !errors.Is(err, ErrStaleUpgrade) {
		t.Fatalf("stale upgrade want ErrStaleUpgrade got %v", err)
	}
	// nil コードは拒否。
	if _, _, err := rt.Execute(Extrinsic{Call: Call{Pallet: "system", Method: "set_code"}}); !errors.Is(err, ErrNoCode) {
		t.Fatalf("nil code want ErrNoCode got %v", err)
	}
	// v2 に上げたあと v1 へ戻す(ダウングレード)も拒否。
	if _, _, err := rt.Execute(setCode(RuntimeV2())); err != nil {
		t.Fatalf("upgrade to v2: %v", err)
	}
	if _, _, err := rt.Execute(setCode(RuntimeV1())); !errors.Is(err, ErrStaleUpgrade) {
		t.Fatalf("downgrade want ErrStaleUpgrade got %v", err)
	}
}

func TestExecuteBlockRecords(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	res := rt.ExecuteBlock([]Extrinsic{
		transfer("alice", "bob", 100),
		transfer("bob", "carol", 5000), // 失敗(残高不足)
		setCode(RuntimeV2()),
	})
	if !res[0].Ok {
		t.Fatalf("res0 should ok")
	}
	if res[1].Ok || res[1].Err == "" {
		t.Fatalf("res1 should fail with err")
	}
	if !res[2].Ok || res[2].Weight != 200 {
		t.Fatalf("res2 upgrade want ok weight200 got %+v", res[2])
	}
	// ブロックのイベントは成功したものだけ。
	if len(rt.Events()) != 2 { // transfer + upgrade
		t.Fatalf("block events want 2 got %d", len(rt.Events()))
	}
}

func TestSetStorageZeroDeletes(t *testing.T) {
	rt := NewRuntime(RuntimeV1(), 10000, genesis())
	rt.Set("k", 5)
	if rt.Get("k") != 5 {
		t.Fatalf("set/get failed")
	}
	rt.Set("k", 0) // 0 はキー削除(空スロットと同義)
	if rt.Get("k") != 0 {
		t.Fatalf("zero should read as 0")
	}
}
