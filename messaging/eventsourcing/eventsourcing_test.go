package eventsourcing

import "testing"

// TestReplayDerivesState はこの章の主眼。状態を保存せず、イベント列を畳んで
// 現在残高を導けることを固定する。
func TestReplayDerivesState(t *testing.T) {
	var s Store
	Deposit(&s, 1000)
	Deposit(&s, 500)
	Withdraw(&s, 300)

	got := Replay(s.Events())
	if got.Balance != 1200 {
		t.Fatalf("balance: got %d want 1200", got.Balance)
	}
	if got.Version != 3 {
		t.Fatalf("version: got %d want 3", got.Version)
	}
}

// TestFullHistoryPreserved はイベントが不変の監査ログとして残ることを確かめる。
func TestFullHistoryPreserved(t *testing.T) {
	var s Store
	Deposit(&s, 1000)
	Withdraw(&s, 200)
	Deposit(&s, 50)

	events := s.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// 出来事の種類と順序がそのまま残る。
	wantTypes := []EventType{Deposited, Withdrawn, Deposited}
	for i, w := range wantTypes {
		if events[i].Type != w {
			t.Fatalf("event %d: got %v want %v", i, events[i].Type, w)
		}
		if events[i].Version != i+1 {
			t.Fatalf("event %d version: got %d want %d", i, events[i].Version, i+1)
		}
	}
}

// TestTimeTravel はこの章のもう一つの主眼。過去のある時点の状態に遡れることを固定する。
func TestTimeTravel(t *testing.T) {
	var s Store
	Deposit(&s, 1000) // v1: 1000
	Deposit(&s, 500)  // v2: 1500
	Withdraw(&s, 800) // v3: 700

	events := s.Events()
	cases := map[int]int{0: 0, 1: 1000, 2: 1500, 3: 700}
	for version, wantBalance := range cases {
		if got := StateAt(events, version); got.Balance != wantBalance {
			t.Fatalf("StateAt(%d): got %d want %d", version, got.Balance, wantBalance)
		}
	}
}

// TestCommandValidation は、残高を超える出金がイベントを残さず拒否されることを固定する。
func TestCommandValidation(t *testing.T) {
	var s Store
	Deposit(&s, 100)

	if _, err := Withdraw(&s, 500); err != ErrInsufficient {
		t.Fatalf("over-withdraw should be ErrInsufficient, got %v", err)
	}
	// 拒否された出金はイベントを残さない(不正な意図は事実にならない)。
	if len(s.Events()) != 1 {
		t.Fatalf("rejected command must not append an event, have %d", len(s.Events()))
	}
	// 残高は変わっていない。
	if Replay(s.Events()).Balance != 100 {
		t.Fatal("balance should be unchanged after rejected withdraw")
	}
}

func TestInvalidAmount(t *testing.T) {
	var s Store
	if _, err := Deposit(&s, 0); err != ErrInvalidAmount {
		t.Fatalf("zero deposit should be ErrInvalidAmount, got %v", err)
	}
	if _, err := Withdraw(&s, -10); err != ErrInvalidAmount {
		t.Fatalf("negative withdraw should be ErrInvalidAmount, got %v", err)
	}
	if len(s.Events()) != 0 {
		t.Fatal("invalid commands must not append events")
	}
}

// TestSnapshotMatchesFullReplay は、スナップショット + 以降のイベントが、
// 全イベントの畳み込みと同じ状態になることを固定する。
func TestSnapshotMatchesFullReplay(t *testing.T) {
	var s Store
	for i := 0; i < 5; i++ {
		Deposit(&s, 100) // v1..v5: 500
	}
	// v3 時点でスナップショットを取る。
	snap := TakeSnapshot(StateAt(s.Events(), 3))
	if snap.Balance != 300 || snap.Version != 3 {
		t.Fatalf("snapshot: got %+v want {300 3}", snap)
	}

	// スナップショット後にさらにイベントを足す。
	Withdraw(&s, 150) // v6: 350

	fromSnap := RestoreFrom(snap, s.EventsAfter(snap.Version))
	fromFull := Replay(s.Events())
	if fromSnap.Balance != fromFull.Balance || fromSnap.Version != fromFull.Version {
		t.Fatalf("snapshot restore %+v != full replay %+v", fromSnap, fromFull)
	}
	if fromSnap.Balance != 350 {
		t.Fatalf("balance: got %d want 350", fromSnap.Balance)
	}
}

func TestEventTypeString(t *testing.T) {
	if Deposited.String() != "Deposited" || Withdrawn.String() != "Withdrawn" {
		t.Fatal("unexpected event type strings")
	}
}
