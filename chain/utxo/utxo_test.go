package utxo

import (
	"errors"
	"testing"
)

// coinbase で Alice に配り、送金して UTXO セットがどう推移するかを一通り確認する。
func TestCoinbaseAndBalance(t *testing.T) {
	s := NewUTXOSet()
	alice := NewWallet("alice")

	cb := Coinbase(alice.Pub, 50, "block-1")
	if err := Validate(cb, s); err != nil {
		t.Fatalf("coinbase should be valid: %v", err)
	}
	s.Apply(cb)

	if got := s.Balance(alice.Pub); got != 50 {
		t.Fatalf("alice balance = %d, want 50", got)
	}
	if s.Len() != 1 {
		t.Fatalf("utxo count = %d, want 1", s.Len())
	}
}

func TestTransferSpendsAndMakesChange(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	s.Apply(Coinbase(alice.Pub, 50, "block-1"))

	tx, err := BuildTransfer(s, alice, bob.Pub, 30, 0)
	if err != nil {
		t.Fatalf("build transfer: %v", err)
	}
	if err := Validate(tx, s); err != nil {
		t.Fatalf("transfer should validate: %v", err)
	}
	s.Apply(tx)

	if got := s.Balance(bob.Pub); got != 30 {
		t.Fatalf("bob balance = %d, want 30", got)
	}
	if got := s.Balance(alice.Pub); got != 20 { // お釣り
		t.Fatalf("alice balance = %d, want 20 (change)", got)
	}
	// coinbase 出力は消費され、Bob への 30 と Alice のお釣り 20 が残る。
	if s.Len() != 2 {
		t.Fatalf("utxo count = %d, want 2", s.Len())
	}
}

func TestFeeIsInputMinusOutput(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	s.Apply(Coinbase(alice.Pub, 50, "block-1"))

	tx, err := BuildTransfer(s, alice, bob.Pub, 30, 5)
	if err != nil {
		t.Fatalf("build transfer: %v", err)
	}
	fee, err := Fee(tx, s)
	if err != nil {
		t.Fatalf("fee: %v", err)
	}
	if fee != 5 {
		t.Fatalf("fee = %d, want 5", fee)
	}
	// 50 = 30(Bob) + 15(お釣り) + 5(手数料)。手数料ぶんは出力に現れない。
	s.Apply(tx)
	if got := s.Balance(alice.Pub); got != 15 {
		t.Fatalf("alice change = %d, want 15", got)
	}
}

func TestDoubleSpendAcrossTxRejectedByLedger(t *testing.T) {
	s := NewUTXOSet()
	alice, bob, carol := NewWallet("alice"), NewWallet("bob"), NewWallet("carol")
	s.Apply(Coinbase(alice.Pub, 50, "block-1"))

	// Alice が同じ UTXO を元に 2 つの取引を作る。最初は通る。
	tx1, _ := BuildTransfer(s, alice, bob.Pub, 40, 0)
	if err := Validate(tx1, s); err != nil {
		t.Fatalf("tx1 should validate: %v", err)
	}
	s.Apply(tx1)

	// 2 つ目は同じ coinbase 出力を使うが、もう UTXO セットにない。
	tx2 := Tx{
		Inputs:  []TxInput{{Prev: OutPoint{TxID: Coinbase(alice.Pub, 50, "block-1").ID(), Index: 0}}},
		Outputs: []TxOutput{{Amount: 40, Owner: carol.Pub}},
	}
	tx2.Inputs[0].Sig = alice.Sign(tx2)
	if err := Validate(tx2, s); !errors.Is(err, ErrUnknownInput) {
		t.Fatalf("double spend should be ErrUnknownInput, got %v", err)
	}
}

func TestDoubleSpendWithinTx(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	cb := Coinbase(alice.Pub, 50, "block-1")
	s.Apply(cb)
	op := OutPoint{TxID: cb.ID(), Index: 0}

	// 同じ input を 2 度並べる。
	tx := Tx{
		Inputs:  []TxInput{{Prev: op}, {Prev: op}},
		Outputs: []TxOutput{{Amount: 90, Owner: bob.Pub}},
	}
	sig := alice.Sign(tx)
	tx.Inputs[0].Sig, tx.Inputs[1].Sig = sig, sig
	if err := Validate(tx, s); !errors.Is(err, ErrDoubleSpend) {
		t.Fatalf("want ErrDoubleSpend, got %v", err)
	}
}

func TestBadSignatureRejected(t *testing.T) {
	s := NewUTXOSet()
	alice, bob, mallory := NewWallet("alice"), NewWallet("bob"), NewWallet("mallory")
	cb := Coinbase(alice.Pub, 50, "block-1")
	s.Apply(cb)

	// Mallory が Alice の出力を奪おうとする。署名は Mallory の鍵。
	tx := Tx{
		Inputs:  []TxInput{{Prev: OutPoint{TxID: cb.ID(), Index: 0}}},
		Outputs: []TxOutput{{Amount: 50, Owner: bob.Pub}},
	}
	tx.Inputs[0].Sig = mallory.Sign(tx) // 所有者(Alice)でない鍵で署名
	if err := Validate(tx, s); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("want ErrBadSignature, got %v", err)
	}
}

func TestInsufficientInputs(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	cb := Coinbase(alice.Pub, 30, "block-1")
	s.Apply(cb)

	// 30 しか持たないのに 40 を出力しようとする(署名は正しい)。
	tx := Tx{
		Inputs:  []TxInput{{Prev: OutPoint{TxID: cb.ID(), Index: 0}}},
		Outputs: []TxOutput{{Amount: 40, Owner: bob.Pub}},
	}
	tx.Inputs[0].Sig = alice.Sign(tx)
	if err := Validate(tx, s); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("want ErrInsufficient, got %v", err)
	}
}

func TestCannotAfford(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	s.Apply(Coinbase(alice.Pub, 10, "block-1"))
	if _, err := BuildTransfer(s, alice, bob.Pub, 30, 0); !errors.Is(err, ErrCannotAfford) {
		t.Fatalf("want ErrCannotAfford, got %v", err)
	}
}

func TestValidateRejectsEmptyAndNonPositive(t *testing.T) {
	s := NewUTXOSet()
	bob := NewWallet("bob")
	if err := Validate(Tx{}, s); !errors.Is(err, ErrNoOutputs) {
		t.Fatalf("want ErrNoOutputs, got %v", err)
	}
	if err := Validate(Tx{Outputs: []TxOutput{{Amount: 0, Owner: bob.Pub}}}, s); !errors.Is(err, ErrNonPositive) {
		t.Fatalf("want ErrNonPositive, got %v", err)
	}
}

func TestTransferGathersMultipleUTXOs(t *testing.T) {
	s := NewUTXOSet()
	alice, bob := NewWallet("alice"), NewWallet("bob")
	// Alice に 3 つの小さな出力。合計 30。
	s.Apply(Coinbase(alice.Pub, 10, "block-1"))
	s.Apply(Coinbase(alice.Pub, 10, "block-2"))
	s.Apply(Coinbase(alice.Pub, 10, "block-3"))
	if s.Len() != 3 {
		t.Fatalf("setup utxo count = %d, want 3", s.Len())
	}

	tx, err := BuildTransfer(s, alice, bob.Pub, 25, 0)
	if err != nil {
		t.Fatalf("build transfer: %v", err)
	}
	if len(tx.Inputs) != 3 { // 25 に届くには 3 つ全部要る
		t.Fatalf("inputs = %d, want 3", len(tx.Inputs))
	}
	if err := Validate(tx, s); err != nil {
		t.Fatalf("validate: %v", err)
	}
	s.Apply(tx)
	if got := s.Balance(bob.Pub); got != 25 {
		t.Fatalf("bob = %d, want 25", got)
	}
	if got := s.Balance(alice.Pub); got != 5 {
		t.Fatalf("alice change = %d, want 5", got)
	}
}

func TestIDIsDeterministicAndMemoDistinguishes(t *testing.T) {
	alice := NewWallet("alice")
	a := Coinbase(alice.Pub, 50, "block-1")
	b := Coinbase(alice.Pub, 50, "block-1")
	c := Coinbase(alice.Pub, 50, "block-2")
	if a.ID() != b.ID() {
		t.Fatalf("same content should give same ID")
	}
	if a.ID() == c.ID() {
		t.Fatalf("different memo should give different ID")
	}
}

func TestAddressHelpers(t *testing.T) {
	alice := NewWallet("alice")
	if alice.Address() != AddressOf(alice.Pub) {
		t.Fatalf("address helpers disagree")
	}
	if AddressOf(nil) != "(none)" {
		t.Fatalf("nil address should be (none)")
	}
}
