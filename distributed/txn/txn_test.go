package txn

import (
	"errors"
	"testing"
)

func TestTwoPCAllYesCommits(t *testing.T) {
	a := NewParticipant("inventory", 100)
	b := NewParticipant("payment", 100)
	c := NewCoordinator(a, b)

	d, err := c.Run(30)
	if err != nil || d != DecisionCommit {
		t.Fatalf("want commit got %v err=%v", d, err)
	}
	if a.Balance() != 70 || b.Balance() != 70 {
		t.Fatalf("残高 want 70/70 got %d/%d", a.Balance(), b.Balance())
	}
	if a.State() != PCommitted || b.State() != PCommitted {
		t.Fatalf("state want committed got %s/%s", a.State(), b.State())
	}
	if a.Locked() != 0 || b.Locked() != 0 {
		t.Fatal("commit 後にロックが残っている")
	}
}

func TestTwoPCOneNoAbortsAll(t *testing.T) {
	a := NewParticipant("inventory", 100)
	b := NewParticipant("payment", 10) // 30 は払えない → No
	c := NewCoordinator(a, b)

	d, err := c.Run(30)
	if !errors.Is(err, ErrAborted) || d != DecisionAbort {
		t.Fatalf("want abort got %v err=%v", d, err)
	}
	// a は Yes と答えてロックしていたが、abort で全額戻る。
	if a.Balance() != 100 || b.Balance() != 10 {
		t.Fatalf("abort 後の残高 want 100/10 got %d/%d", a.Balance(), b.Balance())
	}
	if a.Locked() != 0 {
		t.Fatal("abort 後にロックが残っている")
	}
	// 「一部だけ引き落とされた」状態は確定しない。
	if a.State() != PAborted {
		t.Fatalf("a state want aborted got %s", a.State())
	}
}

func TestTwoPCCrashedParticipantVotesNo(t *testing.T) {
	a := NewParticipant("inventory", 100)
	b := NewParticipant("payment", 100)
	b.Crash()
	c := NewCoordinator(a, b)

	if d, _ := c.Run(30); d != DecisionAbort {
		t.Fatalf("落ちた参加者がいれば abort want abort got %v", d)
	}
	if a.Balance() != 100 {
		t.Fatalf("abort で戻るはず got %d", a.Balance())
	}
	b.Recover()
	if d, err := c.Run(30); d != DecisionCommit || err != nil {
		t.Fatalf("復旧後は commit するはず got %v err=%v", d, err)
	}
}

func TestTwoPCBlocking(t *testing.T) {
	// 調整役が決定を配る前に落ちると、Yes と答えた参加者はロックを抱えて動けない。
	a := NewParticipant("inventory", 100)
	b := NewParticipant("payment", 100)
	c := NewCoordinator(a, b)

	votes := c.RunPrepareOnly(30)
	if votes[0] != VoteYes || votes[1] != VoteYes {
		t.Fatalf("両者 Yes のはず got %v", votes)
	}
	if c.Decision() != DecisionNone {
		t.Fatalf("決定は届いていないはず got %v", c.Decision())
	}
	// 参加者は prepared のまま。残高は減って見え、ロックだけが積まれている。
	if a.State() != PPrepared || a.Locked() != 30 || a.Balance() != 70 {
		t.Fatalf("a はロックを抱えて待つはず state=%s locked=%d bal=%d", a.State(), a.Locked(), a.Balance())
	}
	blocked := c.Blocked()
	if len(blocked) != 2 {
		t.Fatalf("blocked want 2 got %v", blocked)
	}
}

func TestSagaAllSuccess(t *testing.T) {
	var flight, hotel, car bool
	s := NewSaga(
		Step{Name: "flight", Do: func() error { flight = true; return nil }, Compensate: func() { flight = false }},
		Step{Name: "hotel", Do: func() error { hotel = true; return nil }, Compensate: func() { hotel = false }},
		Step{Name: "car", Do: func() error { car = true; return nil }, Compensate: func() { car = false }},
	)
	if err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !flight || !hotel || !car {
		t.Fatal("全ステップ完了のはず")
	}
	if len(s.Log()) != 3 {
		t.Fatalf("log want 3 got %d", len(s.Log()))
	}
}

func TestSagaCompensatesInReverse(t *testing.T) {
	var order []string
	fail := errors.New("no car available")
	s := NewSaga(
		Step{Name: "flight", Do: func() error { return nil }, Compensate: func() { order = append(order, "cancel-flight") }},
		Step{Name: "hotel", Do: func() error { return nil }, Compensate: func() { order = append(order, "cancel-hotel") }},
		Step{Name: "car", Do: func() error { return fail }, Compensate: func() { t.Fatal("失敗したステップは補償しない") }},
	)
	if err := s.Run(); !errors.Is(err, fail) {
		t.Fatalf("元の失敗が返るはず got %v", err)
	}
	// 補償は完了済みの逆順: hotel → flight。
	if len(order) != 2 || order[0] != "cancel-hotel" || order[1] != "cancel-flight" {
		t.Fatalf("補償順 want [cancel-hotel cancel-flight] got %v", order)
	}
	// ログには do → do → do-failed → compensate ×2 が残る。
	log := s.Log()
	if len(log) != 5 || log[2].Action != "do-failed" || log[2].Err == "" {
		t.Fatalf("log が不正: %+v", log)
	}
}

func TestSagaFirstStepFails(t *testing.T) {
	fail := errors.New("sold out")
	called := false
	s := NewSaga(
		Step{Name: "flight", Do: func() error { return fail }, Compensate: func() { called = true }},
	)
	if err := s.Run(); !errors.Is(err, fail) {
		t.Fatalf("want fail got %v", err)
	}
	if called {
		t.Fatal("何も完了していないので補償は走らない")
	}
}

func TestSagaIntermediateStateVisible(t *testing.T) {
	// Saga の代償: 途中状態(航空券だけ確定)が外から見える瞬間がある。
	var flightConfirmed bool
	probe := false
	fail := errors.New("hotel full")
	s := NewSaga(
		Step{Name: "flight", Do: func() error { flightConfirmed = true; return nil }, Compensate: func() { flightConfirmed = false }},
		Step{Name: "probe", Do: func() error {
			probe = flightConfirmed // 外部の観測者: この時点で航空券は確定して見える
			return nil
		}, Compensate: func() {}},
		Step{Name: "hotel", Do: func() error { return fail }, Compensate: func() {}},
	)
	_ = s.Run()
	if !probe {
		t.Fatal("途中状態が観測できるはず(2PC との違い)")
	}
	if flightConfirmed {
		t.Fatal("補償で打ち消されているはず")
	}
}
