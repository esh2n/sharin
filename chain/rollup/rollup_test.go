package rollup

import (
	"errors"
	"testing"
)

func initial() *L2State {
	return NewL2State(map[string]uint64{"alice": 100, "bob": 0, "attacker": 0})
}

// L2 の実行結果が state root に反映され、正直なバッチが素直に確定する。
func TestOptimisticHonestBatchFinalizes(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)

	b, _ := seq.Propose([]Tx{{From: "alice", To: "bob", Amount: 30}})
	if err := r.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if r.Records()[0].Status != Pending {
		t.Fatalf("optimistic commit should be Pending")
	}
	// challenge 期間が過ぎると確定する。
	r.Tick(10)
	r.Finalize()
	if r.Records()[0].Status != Final {
		t.Fatalf("should finalize after challenge period")
	}
	if r.CanonicalRoot() != b.PostRoot {
		t.Fatalf("canonical root should be batch post root")
	}
}

// 不正バッチ(残高を水増し)を optimistic で通し、fraud proof で覆す。
func TestOptimisticFraudChallenged(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)

	// attacker の残高を勝手に 1000 にした嘘の状態を主張する。
	fake := NewL2State(map[string]uint64{"alice": 100, "bob": 0, "attacker": 1000})
	b, witness := seq.ProposeFraud([]Tx{{From: "alice", To: "bob", Amount: 10}}, fake)
	if err := r.Commit(b); err != nil {
		t.Fatalf("optimistic accepts without verifying: %v", err)
	}
	if r.Records()[0].Status != Pending {
		t.Fatalf("fraud batch should sit as Pending until challenged")
	}

	// 監視者が witness を提示して告発。
	fraud, err := r.Challenge(0, witness)
	if err != nil {
		t.Fatalf("challenge error: %v", err)
	}
	if !fraud {
		t.Fatalf("fraud should be proven")
	}
	if r.Records()[0].Status != Reverted {
		t.Fatalf("fraud batch should be reverted")
	}
	if r.Slashed() != 50 {
		t.Fatalf("bond should be slashed, got %d", r.Slashed())
	}
	// canonical root は genesis へ戻る。
	if r.CanonicalRoot() != initial().Root() {
		t.Fatalf("canonical should roll back to genesis")
	}
}

// fraud 告発が後続バッチも巻き戻す。
func TestFraudRevertsSubsequentBatches(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)

	fake := NewL2State(map[string]uint64{"alice": 100, "bob": 0, "attacker": 999})
	b0, w0 := seq.ProposeFraud([]Tx{{From: "alice", To: "attacker", Amount: 5}}, fake)
	_ = r.Commit(b0)
	// 嘘の状態の上に、正直な後続を積む。
	b1, _ := seq.Propose([]Tx{{From: "attacker", To: "bob", Amount: 100}})
	_ = r.Commit(b1)

	fraud, err := r.Challenge(0, w0)
	if err != nil || !fraud {
		t.Fatalf("fraud on batch0 should be proven: %v", err)
	}
	if r.Records()[1].Status != Reverted {
		t.Fatalf("batch built on fraud should also revert")
	}
}

// 正直なバッチへの challenge は失敗する(誣告は通らない)。
func TestChallengeOnHonestFails(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)
	b, witness := seq.Propose([]Tx{{From: "alice", To: "bob", Amount: 30}})
	_ = r.Commit(b)

	fraud, err := r.Challenge(0, witness)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fraud {
		t.Fatalf("honest batch should not be provable as fraud")
	}
	if r.Records()[0].Status == Reverted {
		t.Fatalf("honest batch must not be reverted")
	}
	if r.Slashed() != 0 {
		t.Fatalf("nothing should be slashed")
	}
}

// challenge 期間を過ぎると告発できない。
func TestChallengeAfterWindowRejected(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)
	fake := NewL2State(map[string]uint64{"alice": 100, "bob": 0, "attacker": 500})
	b, witness := seq.ProposeFraud([]Tx{{From: "alice", To: "bob", Amount: 1}}, fake)
	_ = r.Commit(b)

	r.Tick(10) // 期間経過
	r.Finalize()
	if _, err := r.Challenge(0, witness); !errors.Is(err, ErrNotChallengeable) {
		t.Fatalf("want ErrNotChallengeable after window, got %v", err)
	}
}

// ZK: 正直なバッチは proof 検証を通り即 Final。
func TestZKHonestFinalizesImmediately(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(ZK, initial(), 0, 50)
	b, _ := seq.Propose([]Tx{{From: "alice", To: "bob", Amount: 40}})
	if err := r.Commit(b); err != nil {
		t.Fatalf("zk commit of honest batch: %v", err)
	}
	if r.Records()[0].Status != Final {
		t.Fatalf("zk honest batch should be Final immediately")
	}
}

// ZK: 不正バッチは proof が無効なので commit 時に弾かれる(そもそも入らない)。
func TestZKFraudRejectedAtCommit(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(ZK, initial(), 0, 50)
	fake := NewL2State(map[string]uint64{"alice": 100, "bob": 0, "attacker": 777})
	b, _ := seq.ProposeFraud([]Tx{{From: "alice", To: "bob", Amount: 10}}, fake)
	if err := r.Commit(b); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("zk should reject fraud at commit, got %v", err)
	}
	if len(r.Records()) != 0 {
		t.Fatalf("rejected batch must not be recorded")
	}
}

func TestZKRequiresProof(t *testing.T) {
	r := New(ZK, initial(), 0, 50)
	b := Batch{PrevRoot: initial().Root(), PostRoot: initial().Root(), Txs: nil, Proof: nil}
	if err := r.Commit(b); !errors.Is(err, ErrProofRequired) {
		t.Fatalf("want ErrProofRequired, got %v", err)
	}
}

func TestCommitRootMismatch(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)
	b, _ := seq.Propose([]Tx{{From: "alice", To: "bob", Amount: 30}})
	b.PrevRoot = "deadbeef00000000" // 繋がらない root
	if err := r.Commit(b); !errors.Is(err, ErrRootMismatch) {
		t.Fatalf("want ErrRootMismatch, got %v", err)
	}
}

func TestChallengeWrongMode(t *testing.T) {
	r := New(ZK, initial(), 0, 50)
	if _, err := r.Challenge(0, initial()); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("challenge in zk should be ErrWrongMode, got %v", err)
	}
}

func TestChallengeBadWitness(t *testing.T) {
	seq := NewSequencer(initial())
	r := New(Optimistic, initial(), 10, 50)
	b, _ := seq.Propose([]Tx{{From: "alice", To: "bob", Amount: 30}})
	_ = r.Commit(b)
	wrong := NewL2State(map[string]uint64{"alice": 999})
	if _, err := r.Challenge(0, wrong); !errors.Is(err, ErrBadWitness) {
		t.Fatalf("want ErrBadWitness, got %v", err)
	}
}

func TestChallengeIndexOutOfRange(t *testing.T) {
	r := New(Optimistic, initial(), 10, 50)
	if _, err := r.Challenge(5, initial()); err == nil {
		t.Fatalf("out-of-range index should error")
	}
}

func TestStateApplyInsufficient(t *testing.T) {
	s := NewL2State(map[string]uint64{"alice": 5})
	if s.apply(Tx{From: "alice", To: "bob", Amount: 10}) {
		t.Fatalf("insufficient balance should not apply")
	}
	if s.Balances["alice"] != 5 {
		t.Fatalf("balance must be unchanged on failed apply")
	}
}

func TestModeAndStatusStrings(t *testing.T) {
	if Optimistic.String() != "optimistic" || ZK.String() != "zk" {
		t.Fatalf("mode strings")
	}
	if Pending.String() != "pending" || Final.String() != "final" || Reverted.String() != "reverted" {
		t.Fatalf("status strings")
	}
}
