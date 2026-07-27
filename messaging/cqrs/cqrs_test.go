package cqrs

import "testing"

// TestMultipleProjections はこの章の主眼その 1。同じイベント列から、
// 注文状況・顧客ごとの購入額・総売上という別々の読みモデルが同時に作れることを固定する。
func TestMultipleProjections(t *testing.T) {
	var s Store
	w := NewWriteSide(&s)
	r := NewReadSide(&s)

	w.Place("o1", "alice", 1000)
	w.Place("o2", "bob", 500)
	w.Place("o3", "alice", 300)
	w.Pay("o1")
	w.Pay("o3")
	// o2 は未払い、o1・o3 は alice が支払い。

	r.CatchUp()

	// 射影1: 注文状況。
	if r.Status["o1"] != "paid" || r.Status["o2"] != "placed" || r.Status["o3"] != "paid" {
		t.Fatalf("status view wrong: %v", r.Status)
	}
	// 射影2: 顧客ごとの購入額(支払済みのみ)。alice=1300, bob=0。
	if r.Spend["alice"] != 1300 {
		t.Fatalf("alice spend: got %d want 1300", r.Spend["alice"])
	}
	if r.Spend["bob"] != 0 {
		t.Fatalf("bob spend: got %d want 0 (unpaid)", r.Spend["bob"])
	}
	// 射影3: 総売上。1000 + 300 = 1300。
	if r.Revenue != 1300 {
		t.Fatalf("revenue: got %d want 1300", r.Revenue)
	}
}

// TestWriteValidation は書き込み側のコマンド検証を固定する。
func TestWriteValidation(t *testing.T) {
	var s Store
	w := NewWriteSide(&s)

	if err := w.Place("o1", "alice", 100); err != nil {
		t.Fatalf("first place should succeed: %v", err)
	}
	// 同じ ID の再作成は拒否。
	if err := w.Place("o1", "alice", 100); err != ErrExists {
		t.Fatalf("duplicate place should be ErrExists, got %v", err)
	}
	// 無い注文の支払いは拒否。
	if err := w.Pay("nope"); err != ErrNotFound {
		t.Fatalf("pay missing should be ErrNotFound, got %v", err)
	}
	// 支払い後の再支払いは拒否。
	w.Pay("o1")
	if err := w.Pay("o1"); err != ErrNotPayable {
		t.Fatalf("double pay should be ErrNotPayable, got %v", err)
	}
	// 支払済みは取り消せない。
	if err := w.Cancel("o1"); err != ErrPaidCannotCancel {
		t.Fatalf("cancel paid should be ErrPaidCannotCancel, got %v", err)
	}
}

func TestCancelFlow(t *testing.T) {
	var s Store
	w := NewWriteSide(&s)
	r := NewReadSide(&s)
	w.Place("o1", "alice", 100)
	if err := w.Cancel("o1"); err != nil {
		t.Fatalf("cancel placed should succeed: %v", err)
	}
	// 取消後は支払えない。
	if err := w.Pay("o1"); err != ErrNotPayable {
		t.Fatalf("pay cancelled should be ErrNotPayable, got %v", err)
	}
	r.CatchUp()
	if r.Status["o1"] != "cancelled" {
		t.Fatalf("status should be cancelled: %v", r.Status)
	}
	if r.Revenue != 0 {
		t.Fatalf("cancelled order should not add revenue: %d", r.Revenue)
	}
}

// TestEventualConsistency はこの章のもう一つの主眼。書き込み直後、読みモデルは
// まだ古く(遅れ)、CatchUp で追いつくことを固定する。
func TestEventualConsistency(t *testing.T) {
	var s Store
	w := NewWriteSide(&s)
	r := NewReadSide(&s)

	w.Place("o1", "alice", 1000)
	w.Pay("o1")

	// まだ CatchUp していないので読みモデルは空(書き込みに 2 イベント遅れ)。
	if r.Lag() != 2 {
		t.Fatalf("read side should lag by 2, got %d", r.Lag())
	}
	if _, ok := r.Status["o1"]; ok {
		t.Fatal("read model should be stale before CatchUp")
	}

	r.CatchUp()

	// 追いついた。遅れ 0、状態が見える。
	if r.Lag() != 0 {
		t.Fatalf("after CatchUp lag should be 0, got %d", r.Lag())
	}
	if r.Status["o1"] != "paid" || r.Revenue != 1000 {
		t.Fatalf("read model not caught up: status=%v revenue=%d", r.Status, r.Revenue)
	}
}

// TestIncrementalCatchUp は CatchUp が未処理ぶんだけを畳む(二重適用しない)ことを確かめる。
func TestIncrementalCatchUp(t *testing.T) {
	var s Store
	w := NewWriteSide(&s)
	r := NewReadSide(&s)

	w.Place("o1", "alice", 100)
	w.Pay("o1")
	r.CatchUp()
	if r.Revenue != 100 {
		t.Fatalf("revenue after first catchup: %d", r.Revenue)
	}

	w.Place("o2", "bob", 200)
	w.Pay("o2")
	r.CatchUp() // 追加ぶんだけ処理。o1 を二重計上しない。
	if r.Revenue != 300 {
		t.Fatalf("revenue should be 300 (no double count), got %d", r.Revenue)
	}
}
