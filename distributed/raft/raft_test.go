package raft

import (
	"fmt"
	"testing"
)

// electLeader はリーダーが1台に収束するまで Tick を進める。
func electLeader(t *testing.T, c *Cluster) *Raft {
	t.Helper()
	for range 200 {
		c.Tick()
		if l := c.Leader(); l != nil {
			// もう1回落ち着かせて、複数候補が残っていないか確認
			c.TickN(3)
			if l2 := c.Leader(); l2 != nil {
				return l2
			}
		}
	}
	t.Fatal("リーダーが選出されなかった")
	return nil
}

func countLeaders(c *Cluster) int {
	n := 0
	for _, id := range c.ids() {
		if c.nodes[id].State() == Leader {
			n++
		}
	}
	return n
}

func TestSingleNodeBecomesLeader(t *testing.T) {
	c := NewCluster(1, 10, 1, 1)
	l := electLeader(t, c)
	if l.ID() != 1 {
		t.Fatalf("唯一のノードがリーダーになるはず, got id=%d", l.ID())
	}
	if !c.Propose([]byte("x")) {
		t.Fatal("単独ノードで書き込みできるはず")
	}
	if got := l.Committed(); got != 2 { // no-op(1) + x(2)
		t.Fatalf("committed=2 のはず, got %d", got)
	}
}

func TestLeaderElection(t *testing.T) {
	c := NewCluster(3, 10, 1, 42)
	electLeader(t, c)
	if n := countLeaders(c); n != 1 {
		t.Fatalf("リーダーはちょうど1台のはず, got %d", n)
	}
	// 全ノードが同じ任期・同じリーダーを認識しているか
	l := c.Leader()
	for _, id := range c.ids() {
		n := c.nodes[id]
		if n.Term() != l.Term() {
			t.Errorf("node %d の任期 %d != リーダー任期 %d", id, n.Term(), l.Term())
		}
	}
}

func TestReplicationAndCommit(t *testing.T) {
	c := NewCluster(3, 10, 1, 7)
	l := electLeader(t, c)
	base := l.Committed()
	for i := range 5 {
		if !c.Propose([]byte(fmt.Sprintf("v%d", i))) {
			t.Fatalf("提案 %d 失敗", i)
		}
	}
	if got := l.Committed(); got != base+5 {
		t.Fatalf("5件 commit されるはず(base=%d), got %d", base, got)
	}
	// 全ノードのログ末尾が揃っているか
	for _, id := range c.ids() {
		if got := c.nodes[id].LastIndex(); got != l.LastIndex() {
			t.Errorf("node %d の末尾 %d != リーダー末尾 %d", id, got, l.LastIndex())
		}
	}
	// 適用列が全ノードで一致しているか(状態機械の一貫性)
	assertAppliedConsistent(t, c)
}

func TestLeaderFailureReelection(t *testing.T) {
	c := NewCluster(5, 10, 1, 99)
	old := electLeader(t, c)
	c.Propose([]byte("before"))

	// リーダーを孤立させる → 残り4台で再選出が起きるはず
	c.Isolate(old.ID())
	var neo *Raft
	for range 200 {
		c.Tick()
		if l := c.Leader(); l != nil && l.ID() != old.ID() {
			neo = l
			break
		}
	}
	if neo == nil {
		t.Fatal("旧リーダー孤立後に新リーダーが立たなかった")
	}
	if neo.Term() <= old.Term() {
		t.Fatalf("新リーダーの任期は上がるはず: old=%d new=%d", old.Term(), neo.Term())
	}
	// 新リーダー側で書き込みが進む
	if !c.Propose([]byte("after")) {
		t.Fatal("新リーダーで書き込めるはず")
	}

	// 分断を癒すと旧リーダーは追従者に戻り、全員が収束する
	c.Heal()
	c.TickN(30)
	if old.State() == Leader {
		t.Fatal("旧リーダーは復帰後 Follower に戻るはず")
	}
	assertAppliedConsistent(t, c)
}

// TestSplitBrainPrevention は少数派に落ちた旧リーダーが書き込みを確定できないことを確認する。
func TestSplitBrainPrevention(t *testing.T) {
	c := NewCluster(5, 10, 1, 123)
	l := electLeader(t, c)
	c.Propose([]byte("committed-before-split"))
	committedBefore := l.Committed()

	// {旧リーダー + 1台} の少数派 と {残り3台} の多数派に割る
	others := []uint64{}
	for _, id := range c.ids() {
		if id != l.ID() {
			others = append(others, id)
		}
	}
	minority := []uint64{l.ID(), others[0]}
	majority := others[1:]
	c.Partition(minority, majority)

	// 少数派の旧リーダーに書き込みを試みる → 過半数に届かず commit は進まないはず
	c.TickN(5)
	if l.State() == Leader {
		l.Propose([]byte("doomed-write"))
		c.TickN(10)
		if l.Committed() > committedBefore {
			t.Fatal("少数派のリーダーは書き込みを確定できてはいけない(split brain)")
		}
	}
	// 多数派側では新リーダーが立ち、書き込みが確定できる
	for range 200 {
		c.Tick()
		if nl := leaderIn(c, majority); nl != nil {
			nl.Propose([]byte("majority-write"))
			c.TickN(10)
			if nl.Committed() <= committedBefore {
				t.Fatal("多数派のリーダーは書き込みを確定できるはず")
			}
			c.Heal()
			c.TickN(30)
			assertAppliedConsistent(t, c)
			return
		}
	}
	t.Fatal("多数派で新リーダーが立たなかった")
}

func leaderIn(c *Cluster, ids []uint64) *Raft {
	for _, id := range ids {
		if c.nodes[id].State() == Leader {
			return c.nodes[id]
		}
	}
	return nil
}

// assertAppliedConsistent は「同じ位置には同じエントリ」という Raft の状態機械安全性を検証する。
func assertAppliedConsistent(t *testing.T, c *Cluster) {
	t.Helper()
	var ref []Entry
	var refID uint64
	for _, id := range c.ids() {
		got := c.applied[id]
		if ref == nil {
			ref, refID = got, id
			continue
		}
		n := min(len(ref), len(got))
		for i := range n {
			if string(ref[i].Data) != string(got[i].Data) || ref[i].Index != got[i].Index {
				t.Fatalf("適用列が食い違う: node %d[%d]=%q vs node %d[%d]=%q",
					refID, i, ref[i].Data, id, i, got[i].Data)
			}
		}
	}
}
