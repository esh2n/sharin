package raft

import (
	"errors"
	"fmt"
	"testing"
)

func TestProposeOnNonLeader(t *testing.T) {
	r := NewRaft(Config{ID: 1, Peers: []uint64{1, 2, 3}, ElectionTick: 10, HeartbeatTick: 1})
	if err := r.Propose([]byte("x")); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("追従者への提案は ErrNotLeader のはず, got %v", err)
	}
	if err := r.ProposeConfChange(ConfChange{Type: ConfAddNode, NodeID: 9}); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("追従者への構成変更は ErrNotLeader のはず, got %v", err)
	}
}

func TestConfChangeOneAtATime(t *testing.T) {
	c := NewCluster(3, 10, 1, 11)
	l := electLeader(t, c)
	if err := l.ProposeConfChange(ConfChange{Type: ConfAddNode, NodeID: 4}); err != nil {
		t.Fatalf("1件目は通るはず: %v", err)
	}
	// 直前の構成変更がまだ適用されていない間は2件目を拒否する
	if err := l.ProposeConfChange(ConfChange{Type: ConfAddNode, NodeID: 5}); err == nil {
		t.Fatal("進行中の構成変更があるとき2件目は拒否のはず")
	}
}

// TestFollowerCatchUpByAppend は少し遅れた追従者が(スナップショットではなく)
// AppendEntries の巻き戻し再送で追いつくことを確認する。
func TestFollowerCatchUpByAppend(t *testing.T) {
	c := NewCluster(3, 10, 1, 21)
	l := electLeader(t, c)
	var f uint64
	for _, id := range c.ids() {
		if id != l.ID() {
			f = id
			break
		}
	}
	c.Isolate(f)
	for i := range 5 {
		l.Propose([]byte(fmt.Sprintf("k%d", i)))
		c.TickN(2)
	}
	c.Heal()
	c.TickN(30)
	if c.nodes[f].LastIndex() != l.LastIndex() {
		t.Fatalf("追従者が末尾まで追いつくはず: f=%d leader=%d", c.nodes[f].LastIndex(), l.LastIndex())
	}
	if c.nodes[f].log.snapshot.LastIndex != 0 {
		t.Fatal("この規模ならスナップショットなしで追いつくはず")
	}
}

// TestPreVoteNoDisruption は孤立ノードが復帰しても任期を吊り上げず、
// 既存リーダーを引きずり下ろさないこと(PreVote の効能)を確認する。
func TestPreVoteNoDisruption(t *testing.T) {
	c := NewCluster(3, 10, 1, 31)
	l := electLeader(t, c)
	term0 := l.Term()
	var f uint64
	for _, id := range c.ids() {
		if id != l.ID() {
			f = id
			break
		}
	}
	// 孤立させて長く放置。孤立ノードは仮投票を投げ続けるが応答が無く任期は上がらない
	c.Isolate(f)
	c.TickN(60)
	if got := c.nodes[f].Term(); got != term0 {
		t.Fatalf("孤立ノードの任期は上がらないはず(仮投票が失敗する): got %d want %d", got, term0)
	}
	// 復帰。既存リーダーはそのまま、任期も維持される
	c.Heal()
	c.TickN(20)
	if c.Leader() == nil || c.Leader().ID() != l.ID() {
		t.Fatal("復帰後も元のリーダーが維持されるはず")
	}
	if l.Term() != term0 {
		t.Fatalf("リーダーの任期は乱されないはず: got %d want %d", l.Term(), term0)
	}
}

func TestStringers(t *testing.T) {
	states := []State{Follower, PreCandidate, Candidate, Leader, State(99)}
	for _, s := range states {
		if s.String() == "" {
			t.Fatalf("State.String が空: %d", s)
		}
	}
	msgs := []MsgType{MsgHup, MsgProp, MsgPreVote, MsgPreVoteResp, MsgVote, MsgVoteResp, MsgApp, MsgAppResp, MsgSnap, MsgSnapResp, MsgType(99)}
	for _, m := range msgs {
		if m.String() == "" {
			t.Fatalf("MsgType.String が空: %d", m)
		}
	}
}

func TestClusterNodeAccessor(t *testing.T) {
	c := NewCluster(3, 10, 1, 5)
	if c.Node(2) == nil || c.Node(2).ID() != 2 {
		t.Fatal("Node(2) が取れるはず")
	}
}
