package replication

import "testing"

func mustCluster(t *testing.T, replicas int, d Durability) *Cluster {
	t.Helper()
	c, err := NewCluster(replicas, d)
	if err != nil {
		t.Fatalf("NewCluster: %v", err)
	}
	return c
}

func TestNewClusterRejectsZeroReplicas(t *testing.T) {
	if _, err := NewCluster(0, Async); err == nil {
		t.Fatal("expected error for 0 replicas")
	}
}

func TestAsyncCommitsWithoutWaiting(t *testing.T) {
	c := mustCluster(t, 2, Async)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	// レプリカ1が切れていても async は即確定する。
	committed, off := c.Write("x", 10)
	if !committed || off != 1 {
		t.Fatalf("async write should commit: committed=%v off=%d", committed, off)
	}
	if c.Committed() != 1 {
		t.Fatalf("committed = %d, want 1", c.Committed())
	}
}

func TestSyncBlocksUntilAllAck(t *testing.T) {
	c := mustCluster(t, 2, Sync)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	// 全ノード必須なので、切れたレプリカがあると確定しない。
	if committed, _ := c.Write("x", 10); committed {
		t.Fatal("sync write must not commit while a replica is disconnected")
	}
	if c.Committed() != 0 {
		t.Fatalf("committed = %d, want 0", c.Committed())
	}
	// 再接続で追いつき、確定が進む。
	if err := c.Connect(1); err != nil {
		t.Fatal(err)
	}
	if c.Committed() != 1 {
		t.Fatalf("after connect committed = %d, want 1", c.Committed())
	}
}

func TestQuorumToleratesMinority(t *testing.T) {
	// 4レプリカ + リーダー = 5ノード。quorum は 3。
	c := mustCluster(t, 4, Quorum)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	if err := c.Disconnect(2); err != nil {
		t.Fatal(err)
	}
	// リーダー + レプリカ3,4 = 3 ノードが持つ → 確定する。
	if committed, _ := c.Write("x", 10); !committed {
		t.Fatal("quorum write should commit with 3 of 5 nodes")
	}
	// さらに1台切ると リーダー + レプリカ4 = 2 < 3 → 確定しない。
	if err := c.Disconnect(3); err != nil {
		t.Fatal(err)
	}
	if committed, _ := c.Write("y", 20); committed {
		t.Fatal("quorum write must not commit with only 2 of 5 nodes")
	}
}

func TestReplicationLag(t *testing.T) {
	c := mustCluster(t, 2, Async)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	c.Write("a", 1)
	c.Write("b", 2)
	if lag, _ := c.Lag(1); lag != 2 {
		t.Fatalf("lag(1) = %d, want 2", lag)
	}
	if lag, _ := c.Lag(2); lag != 0 {
		t.Fatalf("lag(2) = %d, want 0", lag)
	}
	if err := c.Connect(1); err != nil {
		t.Fatal(err)
	}
	if lag, _ := c.Lag(1); lag != 0 {
		t.Fatalf("after connect lag(1) = %d, want 0", lag)
	}
}

func TestStaleReadFromLaggingReplica(t *testing.T) {
	c := mustCluster(t, 2, Async)
	c.Write("a", 1) // 両レプリカに届く
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	c.Write("a", 2) // レプリカ1には届かない

	if v, _ := c.LeaderValue("a"); v != 2 {
		t.Fatalf("leader a = %d, want 2", v)
	}
	if v, _, _ := c.ReplicaValue(1, "a"); v != 1 {
		t.Fatalf("stale replica1 a = %d, want 1", v)
	}
	if v, _, _ := c.ReplicaValue(2, "a"); v != 2 {
		t.Fatalf("fresh replica2 a = %d, want 2", v)
	}
}

func TestAsyncLosesDataOnFailover(t *testing.T) {
	c := mustCluster(t, 2, Async)
	c.Write("x", 10) // 全員が持つ
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	// async なので未複製でも確定してしまう。
	if committed, _ := c.Write("x", 20); !committed {
		t.Fatal("async second write should commit")
	}
	// 遅れたレプリカ1を昇格 → 確定済みの x=20 が消える。
	lost, err := c.Promote(1)
	if err != nil {
		t.Fatal(err)
	}
	if lost != 1 {
		t.Fatalf("lostCommitted = %d, want 1", lost)
	}
	if v, _ := c.LeaderValue("x"); v != 10 {
		t.Fatalf("after failover leader x = %d, want 10 (20 lost)", v)
	}
	if c.Committed() != 1 {
		t.Fatalf("after failover committed = %d, want 1", c.Committed())
	}
}

func TestQuorumSurvivesFailover(t *testing.T) {
	c := mustCluster(t, 2, Quorum)
	c.Write("x", 10)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	// quorum(3ノード中2)なので リーダー+レプリカ2 で確定。レプリカ2 は最新を持つ。
	if committed, _ := c.Write("x", 20); !committed {
		t.Fatal("quorum second write should commit")
	}
	// 最新を持つレプリカ2を昇格 → 損失ゼロ。
	lost, err := c.Promote(2)
	if err != nil {
		t.Fatal(err)
	}
	if lost != 0 {
		t.Fatalf("lostCommitted = %d, want 0", lost)
	}
	if v, _ := c.LeaderValue("x"); v != 20 {
		t.Fatalf("after failover leader x = %d, want 20", v)
	}
}

func TestPromoteReconfiguresReplicas(t *testing.T) {
	c := mustCluster(t, 3, Quorum)
	c.Write("x", 10)
	before := c.Replicas()
	if len(before) != 3 {
		t.Fatalf("replicas = %v, want 3", before)
	}
	if _, err := c.Promote(2); err != nil {
		t.Fatal(err)
	}
	after := c.Replicas()
	if len(after) != 2 {
		t.Fatalf("after promote replicas = %v, want 2", after)
	}
	for _, id := range after {
		if id == 2 {
			t.Fatalf("promoted replica 2 should not remain a replica: %v", after)
		}
	}
}

func TestUnknownReplicaErrors(t *testing.T) {
	c := mustCluster(t, 1, Async)
	if err := c.Disconnect(99); err == nil {
		t.Error("Disconnect(99) should error")
	}
	if err := c.Connect(99); err == nil {
		t.Error("Connect(99) should error")
	}
	if _, err := c.Lag(99); err == nil {
		t.Error("Lag(99) should error")
	}
	if _, _, err := c.ReplicaValue(99, "x"); err == nil {
		t.Error("ReplicaValue(99) should error")
	}
	if _, err := c.Promote(99); err == nil {
		t.Error("Promote(99) should error")
	}
}

func TestDurabilityString(t *testing.T) {
	cases := map[Durability]string{Async: "async", Quorum: "quorum", Sync: "sync"}
	for d, want := range cases {
		if got := d.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(d), got, want)
		}
	}
	if got := Durability(99).String(); got != "Durability(99)" {
		t.Errorf("unknown String() = %q", got)
	}
}

func TestPromoteReplicaAheadOfCommit(t *testing.T) {
	// sync(全ノード必須)で1台切ると書き込みは確定しないが、届いたレプリカのログには載る。
	c := mustCluster(t, 2, Sync)
	if err := c.Disconnect(1); err != nil {
		t.Fatal(err)
	}
	if committed, _ := c.Write("x", 7); committed {
		t.Fatal("sync write must not commit with a replica down")
	}
	if c.LeaderOffset() != 1 {
		t.Fatalf("leader offset = %d, want 1", c.LeaderOffset())
	}
	// 未確定だが最新を持つレプリカ2を昇格 → 損失ゼロ、その書き込みが正になる。
	lost, err := c.Promote(2)
	if err != nil {
		t.Fatal(err)
	}
	if lost != 0 {
		t.Fatalf("lostCommitted = %d, want 0", lost)
	}
	if v, _ := c.LeaderValue("x"); v != 7 {
		t.Fatalf("after failover leader x = %d, want 7", v)
	}
}

func TestMissingKeyReportsAbsent(t *testing.T) {
	c := mustCluster(t, 1, Async)
	if _, ok := c.LeaderValue("nope"); ok {
		t.Error("LeaderValue for missing key should report absent")
	}
	if _, ok, _ := c.ReplicaValue(1, "nope"); ok {
		t.Error("ReplicaValue for missing key should report absent")
	}
}
