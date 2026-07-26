package tcp

import (
	"bytes"
	"testing"
)

// setup は client(能動オープン)と server(受け身)を繋いだ模擬網を返す。
func setup(win uint32) (*Sim, *Endpoint, *Endpoint) {
	client := NewEndpoint("client", 100, win)
	server := NewEndpoint("server", 300, win)
	server.Listen()
	client.Connect()
	return NewSim(client, server), client, server
}

func TestHandshakeEstablishes(t *testing.T) {
	sim, client, server := setup(16)
	ok := sim.RunUntil(func() bool {
		return client.State() == Established && server.State() == Established
	}, 30)
	if !ok {
		t.Fatalf("handshake did not establish: client=%s server=%s", client.State(), server.State())
	}
	// SYN / SYN-ACK / ACK が観測されること。
	var sawSyn, sawSynAck bool
	for _, e := range sim.Trace() {
		if e.Seg.Flags == SYN {
			sawSyn = true
		}
		if e.Seg.Flags == (SYN | ACK) {
			sawSynAck = true
		}
	}
	if !sawSyn || !sawSynAck {
		t.Fatalf("handshake segments missing: syn=%v synack=%v", sawSyn, sawSynAck)
	}
}

func TestReliableTransferNoLoss(t *testing.T) {
	sim, client, server := setup(16)
	data := []byte("HELLO TCP")
	client.Connect()
	client.Send(data)
	ok := sim.RunUntil(func() bool { return len(server.Received()) == len(data) }, 60)
	if !ok {
		t.Fatalf("not fully received: got %q", server.Received())
	}
	if !bytes.Equal(server.Received(), data) {
		t.Fatalf("payload mismatch: got %q want %q", server.Received(), data)
	}
	if client.Retransmits != 0 {
		t.Fatalf("no loss should mean no retransmits, got %d", client.Retransmits)
	}
}

func TestRetransmitRecoversFromLoss(t *testing.T) {
	sim, client, server := setup(16)
	data := []byte("RETRANSMIT ME")
	client.Connect()
	client.Send(data)
	// ハンドシェイク後の最初のデータセグメント付近を落とす(通し番号 3〜4)。
	sim.Drop(3, 4)
	ok := sim.RunUntil(func() bool { return len(server.Received()) == len(data) }, 120)
	if !ok {
		t.Fatalf("loss not recovered: got %q (retransmits=%d)", server.Received(), client.Retransmits)
	}
	if !bytes.Equal(server.Received(), data) {
		t.Fatalf("payload mismatch after loss: got %q want %q", server.Received(), data)
	}
	if client.Retransmits == 0 {
		t.Fatalf("expected retransmissions after dropping segments")
	}
}

func TestOutOfOrderBuffering(t *testing.T) {
	// 先頭のデータセグメントだけ落とすと、後続が先に届いて ooo に退避される。
	// 再送で隙間が埋まると、まとめてアプリへ渡る。
	sim, client, server := setup(32)
	data := []byte("ABCDEFGHIJKL")
	client.Connect()
	client.Send(data)

	// ハンドシェイク(0:SYN, 1:SYN-ACK)。最初のデータセグメント = 通し番号 2 を落とす。
	sim.Drop(2)

	sawOOO := false
	for i := 0; i < 120 && len(server.Received()) < len(data); i++ {
		sim.Step()
		if len(server.OOOKeys()) > 0 {
			sawOOO = true
		}
	}
	if !sawOOO {
		t.Fatalf("expected out-of-order buffering when first data segment dropped")
	}
	if !bytes.Equal(server.Received(), data) {
		t.Fatalf("reassembly wrong: got %q want %q", server.Received(), data)
	}
}

func TestWindowLimitsInFlight(t *testing.T) {
	// ウィンドウを小さくすると、未確認のまま飛ぶ量がそれを超えない(フロー制御)。
	win := uint32(8)
	sim, client, server := setup(win)
	client.Connect()
	client.Send([]byte("THIS IS A LONGER MESSAGE"))
	for i := 0; i < 200 && len(server.Received()) < 24; i++ {
		sim.Step()
		if client.InFlight() > win {
			t.Fatalf("in-flight %d exceeded window %d", client.InFlight(), win)
		}
	}
	if len(server.Received()) != 24 {
		t.Fatalf("transfer incomplete under small window: %d/24", len(server.Received()))
	}
}

func TestConnectionTeardown(t *testing.T) {
	sim, client, server := setup(16)
	client.Connect()
	client.Send([]byte("bye"))
	client.Close()
	// サーバは受け取り切ったら自分も閉じる、という運用にする。
	done := func() bool {
		if server.State() == CloseWait && server.wantFin == false {
			server.Close()
		}
		return client.State() == Closed && server.State() == Closed
	}
	if !sim.RunUntil(done, 120) {
		t.Fatalf("teardown incomplete: client=%s server=%s", client.State(), server.State())
	}
	if !bytes.Equal(server.Received(), []byte("bye")) {
		t.Fatalf("data before FIN lost: %q", server.Received())
	}
}

func TestStateAndFlagStrings(t *testing.T) {
	states := []State{Closed, Listen, SynSent, SynRcvd, Established, CloseWait, LastAck, FinWait, State(99)}
	want := []string{"CLOSED", "LISTEN", "SYN_SENT", "SYN_RCVD", "ESTABLISHED", "CLOSE_WAIT", "LAST_ACK", "FIN_WAIT", "?"}
	for i, s := range states {
		if s.String() != want[i] {
			t.Fatalf("State(%d)=%q want %q", s, s.String(), want[i])
		}
	}
	if (SYN | ACK).String() != "SYN ACK" {
		t.Fatalf("flag string: %q", (SYN | ACK).String())
	}
	if Flag(0).String() != "-" {
		t.Fatalf("empty flag: %q", Flag(0).String())
	}
	seg := Segment{Seq: 5, Ack: 9, Flags: ACK, Window: 16, Payload: []byte("ab")}
	if seg.String() == "" {
		t.Fatalf("segment string empty")
	}
}

func TestMiscAccessors(t *testing.T) {
	if FIN.String() != "FIN" {
		t.Fatalf("FIN string: %q", FIN.String())
	}
	// Send を Connect 前に呼ぶと dataBase が iss+1 に初期化される。
	e := NewEndpoint("x", 500, 16)
	e.Send([]byte("z"))
	if e.dataBase != 501 {
		t.Fatalf("Send should init dataBase to iss+1, got %d", e.dataBase)
	}
	// Clock はステップ数を返す。
	sim, client, server := setup(16)
	client.Connect()
	client.Send([]byte("hi"))
	sim.RunUntil(func() bool { return len(server.Received()) == 2 }, 40)
	if sim.Clock() < 1 {
		t.Fatalf("clock should advance, got %d", sim.Clock())
	}
}

func TestDuplicateDataIgnored(t *testing.T) {
	// 同じデータを二重に配送しても、受信は重複せず正しい(seq<rcvNxt を捨てる)。
	sim, client, server := setup(16)
	client.Connect()
	client.Send([]byte("XY"))
	sim.RunUntil(func() bool { return len(server.Received()) == 2 }, 40)
	// 余分にステップを回しても増えない。
	before := server.Received()
	for i := 0; i < 10; i++ {
		sim.Step()
	}
	if !bytes.Equal(server.Received(), before) || len(server.Received()) != 2 {
		t.Fatalf("duplicate handling wrong: %q", server.Received())
	}
}
