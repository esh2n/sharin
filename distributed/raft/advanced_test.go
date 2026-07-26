package raft

import (
	"fmt"
	"testing"
)

// TestSnapshotCatchUp は遅れすぎた追従者が写し(スナップショット)で追いつくことを確認する。
func TestSnapshotCatchUp(t *testing.T) {
	c := NewCluster(3, 10, 1, 55)
	l := electLeader(t, c)
	follower := uint64(0)
	for _, id := range c.ids() {
		if id != l.ID() {
			follower = id
			break
		}
	}
	// 追従者を1台孤立させる。残り2台(=過半数)で書き込みを進める
	c.Isolate(follower)
	for i := range 12 {
		l.Propose([]byte(fmt.Sprintf("e%d", i)))
		c.TickN(2)
	}
	// リーダーは applied 位置までログを畳む(圧縮)。孤立中の追従者の位置は消える
	l.Snapshot([]byte("compacted-state"))
	if l.log.firstIndex() == 1 {
		t.Fatal("スナップショットでログが圧縮されるはず")
	}

	// 分断を癒すと、リーダーは通常の AppendEntries では追いつかせられず MsgSnap を送る
	c.Heal()
	c.TickN(40)
	f := c.nodes[follower]
	if f.log.snapshot.LastIndex == 0 {
		t.Fatal("追従者はスナップショットを受け取って追いつくはず")
	}
	if f.Committed() != l.Committed() {
		t.Fatalf("追従者の committed(%d) がリーダー(%d) に追いつくはず", f.Committed(), l.Committed())
	}
}

// TestMembershipRemove はノードを1台外すと quorum が縮み、残りで合意が続くことを確認する。
func TestMembershipRemove(t *testing.T) {
	c := NewCluster(3, 10, 1, 77)
	l := electLeader(t, c)
	c.Propose([]byte("a"))

	victim := uint64(0)
	for _, id := range c.ids() {
		if id != l.ID() {
			victim = id
			break
		}
	}
	if err := l.ProposeConfChange(ConfChange{Type: ConfRemoveNode, NodeID: victim}); err != nil {
		t.Fatalf("構成変更の提案に失敗: %v", err)
	}
	c.TickN(20)

	got := l.Members()
	if len(got) != 2 {
		t.Fatalf("メンバは2台に減るはず, got %v", got)
	}
	for _, id := range got {
		if id == victim {
			t.Fatalf("外したノード %d がまだメンバに居る", victim)
		}
	}
	// 縮んだ構成でも書き込みが確定できる
	before := l.Committed()
	c.Propose([]byte("b"))
	if l.Committed() <= before {
		t.Fatal("2台構成でも commit が進むはず")
	}
}

// TestMembershipAdd は新ノードをメンバに加えると quorum が増えることを確認する。
func TestMembershipAdd(t *testing.T) {
	c := NewCluster(3, 10, 1, 88)
	l := electLeader(t, c)
	if err := l.ProposeConfChange(ConfChange{Type: ConfAddNode, NodeID: 4}); err != nil {
		t.Fatalf("追加の提案に失敗: %v", err)
	}
	c.TickN(20)
	if len(l.Members()) != 4 {
		t.Fatalf("メンバは4台に増えるはず, got %v", l.Members())
	}
	// 4台構成(quorum=3)。稼働中の3台が揃えば commit は進む
	before := l.Committed()
	c.Propose([]byte("after-add"))
	c.TickN(5)
	if l.Committed() <= before {
		t.Fatal("稼働3台(4台中の過半数)で commit が進むはず")
	}
}

// TestStaleTermRejected は古い任期のメッセージが弾かれることを確認する。
func TestStaleTermRejected(t *testing.T) {
	r := NewRaft(Config{ID: 1, Peers: []uint64{1, 2, 3}, ElectionTick: 10, HeartbeatTick: 1})
	r.becomeFollower(5, 2) // 任期5、リーダーは2 と認識させる
	// 任期3(古い)の AppendEntries を投げる → 拒否応答が返る
	r.Step(Message{Type: MsgApp, From: 3, To: 1, Term: 3, PrevLogIndex: 0})
	var found bool
	for _, m := range r.TakeMessages() {
		if m.Type == MsgAppResp && m.Reject && m.Term == 5 {
			found = true
		}
	}
	if !found {
		t.Fatal("古い任期の AppendEntries には現任期を添えて拒否を返すはず")
	}
}
