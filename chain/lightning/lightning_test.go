package lightning

import "testing"

func TestOpenAndPayOffChain(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	if a, b := ch.Balances(); a != 100 || b != 0 {
		t.Fatalf("初期残高 want 100/0 got %d/%d", a, b)
	}
	// オフチェーンで 3 回送っても番号が進むだけ(チェーンに触れない)。
	for i := 0; i < 3; i++ {
		if err := ch.Pay("alice", 10); err != nil {
			t.Fatalf("Pay: %v", err)
		}
	}
	if a, b := ch.Balances(); a != 70 || b != 30 {
		t.Fatalf("3 回送金後 want 70/30 got %d/%d", a, b)
	}
	if n := ch.Current().Number; n != 3 {
		t.Fatalf("commitment 番号 want 3 got %d", n)
	}
	// 総額(容量)は保存される。
	if a, b := ch.Balances(); a+b != ch.Capacity {
		t.Fatalf("容量保存則が破れた: %d+%d != %d", a, b, ch.Capacity)
	}
}

func TestPayValidation(t *testing.T) {
	ch := Open("alice", "bob", 50, 50)
	if err := ch.Pay("alice", 100); err != ErrInsufficient {
		t.Fatalf("残高超過 want ErrInsufficient got %v", err)
	}
	if err := ch.Pay("carol", 10); err != ErrUnknownParty {
		t.Fatalf("部外者 want ErrUnknownParty got %v", err)
	}
	_ = ch.CloseCooperative()
	if err := ch.Pay("alice", 10); err != ErrChannelClosed {
		t.Fatalf("閉鎖後 want ErrChannelClosed got %v", err)
	}
}

func TestCooperativeClose(t *testing.T) {
	ch := Open("alice", "bob", 60, 40)
	_ = ch.Pay("alice", 20) // 40/60
	if err := ch.CloseCooperative(); err != nil {
		t.Fatalf("CloseCooperative: %v", err)
	}
	if ch.State() != StateClosedCooperative {
		t.Fatalf("state want cooperative got %s", ch.State())
	}
	if a, b := ch.Final(); a != 40 || b != 60 {
		t.Fatalf("最終残高 want 40/60 got %d/%d", a, b)
	}
	if err := ch.CloseCooperative(); err != ErrChannelClosed {
		t.Fatalf("二重クローズ want ErrChannelClosed got %v", err)
	}
}

func TestUnilateralCloseLatest(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	_ = ch.Pay("alice", 30) // 70/30
	cur := ch.Current()
	if err := ch.Broadcast("alice", cur); err != nil {
		t.Fatalf("Broadcast latest: %v", err)
	}
	if ch.State() != StateClosedUnilateral {
		t.Fatalf("state want unilateral got %s", ch.State())
	}
	if a, b := ch.Final(); a != 70 || b != 30 {
		t.Fatalf("最新提出の確定 want 70/30 got %d/%d", a, b)
	}
}

func TestPenaltyOnRevokedBroadcast(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	old, _ := ch.Commitment(0) // alice が全額持っていた頃(まだ払う前)
	_ = ch.Pay("alice", 40)    // 60/40 に更新。番号 0 は revoked に
	_ = ch.Pay("alice", 20)    // 40/60

	// alice が古い commitment 0(自分が 100 持っていた頃)を提出して巻き戻しを狙う。
	if err := ch.Broadcast("alice", old); err != nil {
		t.Fatalf("Broadcast old: %v", err)
	}
	if ch.State() != StateDisputed {
		t.Fatalf("state want disputed got %s", ch.State())
	}
	// 被害者 bob が、番号 0 のリボケーション秘密でペナルティを行使 → 全額没収。
	secret := old.RevocationSecret
	if err := ch.Penalize("bob", secret); err != nil {
		t.Fatalf("Penalize: %v", err)
	}
	if ch.State() != StateClosedPenalty {
		t.Fatalf("state want penalty got %s", ch.State())
	}
	if a, b := ch.Final(); a != 0 || b != 100 {
		t.Fatalf("ペナルティ後 want 0/100(全額 bob) got %d/%d", a, b)
	}
}

func TestPenaltyRejectsWrongCaller(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	old, _ := ch.Commitment(0)
	_ = ch.Pay("alice", 40)
	_ = ch.Broadcast("alice", old)

	// 不正者本人 alice はペナルティを行使できない。
	if err := ch.Penalize("alice", old.RevocationSecret); err != ErrNotVictim {
		t.Fatalf("cheater self-penalize want ErrNotVictim got %v", err)
	}
	// 秘密が違えば拒否。
	if err := ch.Penalize("bob", "wrong-secret"); err != ErrBadSecret {
		t.Fatalf("wrong secret want ErrBadSecret got %v", err)
	}
	if ch.State() != StateDisputed {
		t.Fatalf("失敗後も係争のまま want disputed got %s", ch.State())
	}
}

func TestUnwatchedCheatSucceeds(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	old, _ := ch.Commitment(0)
	_ = ch.Pay("alice", 40) // 60/40
	_ = ch.Broadcast("alice", old)

	// bob が見張っておらず、期間内に何もしない。
	ch.Tick(5) // disputeWindow(=3)を超える
	ch.FinalizeDispute()
	if ch.State() != StateClosedExpiredCheat {
		t.Fatalf("state want expired-cheat got %s", ch.State())
	}
	if a, b := ch.Final(); a != 100 || b != 0 {
		t.Fatalf("監視失敗で不正が通る want 100/0 got %d/%d", a, b)
	}
}

func TestPenaltyTooLate(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	old, _ := ch.Commitment(0)
	_ = ch.Pay("alice", 40)
	_ = ch.Broadcast("alice", old)
	ch.Tick(5) // 期間切れ
	if err := ch.Penalize("bob", old.RevocationSecret); err != ErrWindowClosed {
		t.Fatalf("期間後のペナルティ want ErrWindowClosed got %v", err)
	}
}

func TestBroadcastInvalid(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	if err := ch.Broadcast("carol", ch.Current()); err != ErrUnknownParty {
		t.Fatalf("部外者提出 want ErrUnknownParty got %v", err)
	}
	// 存在しない未来の番号は提出できない。
	future := Commitment{Number: 9}
	if err := ch.Broadcast("alice", future); err != ErrInvalidCommit {
		t.Fatalf("未来番号 want ErrInvalidCommit got %v", err)
	}
}

func TestNoDisputePenalize(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	if err := ch.Penalize("bob", "x"); err != ErrNoDispute {
		t.Fatalf("係争なしペナルティ want ErrNoDispute got %v", err)
	}
}

func TestBobSidePayAndLock(t *testing.T) {
	ch := Open("alice", "bob", 40, 60)
	if err := ch.Pay("bob", 25); err != nil { // 65/35
		t.Fatalf("bob Pay: %v", err)
	}
	if a, b := ch.Balances(); a != 65 || b != 35 {
		t.Fatalf("bob 送金後 want 65/35 got %d/%d", a, b)
	}
	h, err := ch.LockHTLC("bob", 15, Hash("p"), 10) // bob 側ロック
	if err != nil {
		t.Fatalf("bob LockHTLC: %v", err)
	}
	if _, b := ch.Balances(); b != 20 {
		t.Fatalf("bob ロック中 want 20 got %d", b)
	}
	if err := ch.SettleHTLC(h, "p"); err != nil {
		t.Fatalf("SettleHTLC: %v", err)
	}
	if a, _ := ch.Balances(); a != 80 { // alice(payee)が 15 得る
		t.Fatalf("成立後 alice want 80 got %d", a)
	}
	// bob 残高不足のロックは弾く。
	if _, err := ch.LockHTLC("bob", 999, Hash("p"), 10); err != ErrInsufficient {
		t.Fatalf("bob 超過ロック want ErrInsufficient got %v", err)
	}
}

func TestStateStringAndClock(t *testing.T) {
	ch := Open("alice", "bob", 10, 10)
	if ch.State().String() != "open" {
		t.Fatalf("open string got %s", ch.State())
	}
	ch.Tick(4)
	if ch.Now() != 4 {
		t.Fatalf("clock want 4 got %d", ch.Now())
	}
	_ = ch.CloseCooperative()
	if ch.State().String() != "closed-cooperative" {
		t.Fatalf("cooperative string got %s", ch.State())
	}
	if _, ok := ch.Commitment(99); ok {
		t.Fatal("範囲外番号が取れてしまった")
	}
}

func TestHashPreimage(t *testing.T) {
	if Hash("secret") == Hash("other") {
		t.Fatal("異なる preimage が同じ hash になった")
	}
	if Hash("secret") != Hash("secret") {
		t.Fatal("同じ preimage が違う hash になった")
	}
}

func TestHTLCSettle(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	h, err := ch.LockHTLC("alice", 30, Hash("preimg"), 10)
	if err != nil {
		t.Fatalf("LockHTLC: %v", err)
	}
	// ロック中は宙に浮く: alice 70, bob 0(合計は容量より少ない)。
	if a, b := ch.Balances(); a != 70 || b != 0 {
		t.Fatalf("ロック中 want 70/0 got %d/%d", a, b)
	}
	if err := ch.SettleHTLC(h, "preimg"); err != nil {
		t.Fatalf("SettleHTLC: %v", err)
	}
	if a, b := ch.Balances(); a != 70 || b != 30 {
		t.Fatalf("成立後 want 70/30 got %d/%d", a, b)
	}
}

func TestHTLCWrongPreimage(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	h, _ := ch.LockHTLC("alice", 30, Hash("preimg"), 10)
	if err := ch.SettleHTLC(h, "wrong"); err != ErrHTLCPreimage {
		t.Fatalf("誤 preimage want ErrHTLCPreimage got %v", err)
	}
}

func TestHTLCTimeoutRefund(t *testing.T) {
	ch := Open("alice", "bob", 100, 0)
	h, _ := ch.LockHTLC("alice", 30, Hash("preimg"), 10)
	if err := ch.FailHTLC(h, 5); err != ErrHTLCNotExpired {
		t.Fatalf("期限前失効 want ErrHTLCNotExpired got %v", err)
	}
	if err := ch.FailHTLC(h, 12); err != nil {
		t.Fatalf("FailHTLC: %v", err)
	}
	if a, b := ch.Balances(); a != 100 || b != 0 {
		t.Fatalf("失効後は返る want 100/0 got %d/%d", a, b)
	}
	if err := ch.SettleHTLC(h, "preimg"); err != ErrHTLCDone {
		t.Fatalf("確定済み want ErrHTLCDone got %v", err)
	}
}

func TestMultiHopRouteSettle(t *testing.T) {
	// A—B—C。A は C と直接繋がらないが、B 経由で送る。
	ab := Open("alice", "bob", 100, 100)
	bc := Open("bob", "carol", 100, 100)
	net := NewNetwork(ab, bc)

	preimage := "carol-secret"
	pay, err := net.Route([]string{"alice", "bob", "carol"}, 20, Hash(preimage), 10, 2)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(pay.Hops()) != 2 {
		t.Fatalf("ホップ数 want 2 got %d", len(pay.Hops()))
	}
	// タイムアウトは送金側ほど長い(A-B が 10, B-C が 8)。
	if pay.Hops()[0].Expiry <= pay.Hops()[1].Expiry {
		t.Fatalf("上流の期限が下流以下: %d <= %d", pay.Hops()[0].Expiry, pay.Hops()[1].Expiry)
	}
	// ロック中は各チャネルで offerer の残高が減っている。
	if a, _ := ab.Balances(); a != 80 {
		t.Fatalf("A-B ロック中 alice want 80 got %d", a)
	}

	if err := pay.Settle(preimage); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// 成立後: A は 20 減、C は 20 増、B は素通し(受けて渡すので増減ゼロ)。
	if a, b := ab.Balances(); a != 80 || b != 120 {
		t.Fatalf("A-B 成立後 want 80/120 got %d/%d", a, b)
	}
	if b, c := bc.Balances(); b != 80 || c != 120 {
		t.Fatalf("B-C 成立後 want 80/120 got %d/%d", b, c)
	}
	// B の純資産(2 チャネル合計)は不変: 120 + 80 = 200。
	if bcBob, _ := bc.Balances(); (func() uint64 { _, x := ab.Balances(); return x })()+bcBob != 200 {
		t.Fatalf("仲介 bob の純資産が変動した")
	}
}

func TestMultiHopTimeout(t *testing.T) {
	ab := Open("alice", "bob", 100, 100)
	bc := Open("bob", "carol", 100, 100)
	net := NewNetwork(ab, bc)

	pay, err := net.Route([]string{"alice", "bob", "carol"}, 20, Hash("never-revealed"), 10, 2)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if err := pay.Timeout(20); err != nil {
		t.Fatalf("Timeout: %v", err)
	}
	// preimage が出ないので全ホップ失効。全残高が元に戻る。
	if a, b := ab.Balances(); a != 100 || b != 100 {
		t.Fatalf("A-B 失効後 want 100/100 got %d/%d", a, b)
	}
	if b, c := bc.Balances(); b != 100 || c != 100 {
		t.Fatalf("B-C 失効後 want 100/100 got %d/%d", b, c)
	}
}

func TestRouteMissingChannel(t *testing.T) {
	ab := Open("alice", "bob", 100, 100)
	net := NewNetwork(ab)
	if _, err := net.Route([]string{"alice", "bob", "carol"}, 20, Hash("x"), 10, 2); err != ErrNoChannel {
		t.Fatalf("欠けた経路 want ErrNoChannel got %v", err)
	}
}

func TestNetworkNodes(t *testing.T) {
	ab := Open("alice", "bob", 100, 100)
	bc := Open("bob", "carol", 100, 100)
	net := NewNetwork(ab, bc)
	got := net.Nodes()
	want := []string{"alice", "bob", "carol"}
	if len(got) != len(want) {
		t.Fatalf("Nodes want %v got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Nodes[%d] want %s got %s", i, want[i], got[i])
		}
	}
	if net.Channel("alice", "carol") != nil {
		t.Fatal("存在しないチャネルが返った")
	}
	if net.Channel("carol", "bob") == nil {
		t.Fatal("無向キーで引けない")
	}
}

func TestSettleWrongPreimageRoute(t *testing.T) {
	ab := Open("alice", "bob", 100, 100)
	bc := Open("bob", "carol", 100, 100)
	net := NewNetwork(ab, bc)
	pay, _ := net.Route([]string{"alice", "bob", "carol"}, 20, Hash("real"), 10, 2)
	if err := pay.Settle("fake"); err != ErrHTLCPreimage {
		t.Fatalf("誤 preimage で経路成立 want ErrHTLCPreimage got %v", err)
	}
}
