package reconcile

import "testing"

func countPhase(cl *Cluster, ph Phase) int {
	n := 0
	for _, p := range cl.Pods() {
		if p.Phase == ph {
			n++
		}
	}
	return n
}

// TestCreatesToDesired は、空のクラスタから目標数まで Pod を作ることを固定する。
func TestCreatesToDesired(t *testing.T) {
	c := New(3)
	cl := NewCluster()
	acts := c.Reconcile(cl)
	if len(acts) != 3 {
		t.Fatalf("expected 3 creates, got %d", len(acts))
	}
	if len(cl.Pods()) != 3 {
		t.Fatalf("cluster should have 3 pods, got %d", len(cl.Pods()))
	}
}

// TestIdempotent はこの章の主眼その1。目標に達していれば、何度 Reconcile しても
// 何も起こさない(冪等)ことを固定する。
func TestIdempotent(t *testing.T) {
	c := New(3)
	cl := NewCluster()
	c.Reconcile(cl)   // 3 作る
	cl.StartPending() // 起動
	acts := c.Reconcile(cl)
	if len(acts) != 0 {
		t.Fatalf("second reconcile should be a no-op, got %v", acts)
	}
	acts = c.Reconcile(cl)
	if len(acts) != 0 {
		t.Fatal("reconcile must stay a no-op when converged")
	}
}

// TestSelfHealing はこの章の主眼その2。Pod が落ちると、次の Reconcile が
// それに気づいて作り直す(自己修復)ことを固定する。障害イベントを購読していない
// のに直せるのが level-triggered の効用。
func TestSelfHealing(t *testing.T) {
	c := New(3)
	cl := NewCluster()
	c.Reconcile(cl)
	cl.StartPending() // 3 Running

	cl.Fail("pod-2") // 1 つ落ちる
	acts := c.Reconcile(cl)

	// 落ちた pod-2 が消され、新しい pod が 1 つ作られる。
	del, cre := 0, 0
	for _, a := range acts {
		if a.Kind == "delete" {
			del++
		}
		if a.Kind == "create" {
			cre++
		}
	}
	if del != 1 || cre != 1 {
		t.Fatalf("self-heal should delete 1 and create 1, got del=%d cre=%d", del, cre)
	}
	cl.StartPending()
	if countPhase(cl, Running) != 3 {
		t.Fatalf("should be back to 3 running, got %d", countPhase(cl, Running))
	}
}

// TestLevelTriggered は level-triggered の強さを固定する。障害が複数まとめて
// 起きても(その間 Reconcile しなくても)、1 回の Reconcile が全部を復旧する。
func TestLevelTriggered(t *testing.T) {
	c := New(5)
	cl := NewCluster()
	c.Reconcile(cl)
	cl.StartPending() // 5 Running

	// イベントを一切 Reconcile せずに 3 つ落とす。
	cl.Fail("pod-1")
	cl.Fail("pod-3")
	cl.Fail("pod-5")

	// たった 1 回の Reconcile が、数え直して 3 つとも作り直す。
	c.Reconcile(cl)
	cl.StartPending()
	if countPhase(cl, Running) != 5 {
		t.Fatalf("one reconcile should restore all 5, got %d running", countPhase(cl, Running))
	}
	if len(cl.Pods()) != 5 {
		t.Fatalf("should have exactly 5 pods, got %d", len(cl.Pods()))
	}
}

// TestScaleUpDown は宣言的なスケールを固定する。目標数を変えるだけで、
// 同じ Reconcile が過不足を埋める。
func TestScaleUpDown(t *testing.T) {
	c := New(2)
	cl := NewCluster()
	c.Reconcile(cl)
	cl.StartPending() // 2

	c.SetDesired(5) // スケールアップ(宣言を変えるだけ)
	acts := c.Reconcile(cl)
	if len(acts) != 3 {
		t.Fatalf("scale up should create 3, got %d", len(acts))
	}
	cl.StartPending()
	if len(cl.Pods()) != 5 {
		t.Fatalf("should have 5 pods, got %d", len(cl.Pods()))
	}

	c.SetDesired(1) // スケールダウン
	acts = c.Reconcile(cl)
	del := 0
	for _, a := range acts {
		if a.Kind == "delete" {
			del++
		}
	}
	if del != 4 {
		t.Fatalf("scale down should delete 4, got %d", del)
	}
	if len(cl.Pods()) != 1 {
		t.Fatalf("should have 1 pod, got %d", len(cl.Pods()))
	}
}

func TestConverged(t *testing.T) {
	c := New(2)
	cl := NewCluster()
	if c.Converged(cl) {
		t.Fatal("empty cluster is not converged to 2")
	}
	c.Reconcile(cl)
	if c.Converged(cl) {
		t.Fatal("pending pods are not yet converged (not Running)")
	}
	cl.StartPending()
	if !c.Converged(cl) {
		t.Fatal("2 running pods should be converged")
	}
}

func TestZeroReplicas(t *testing.T) {
	c := New(2)
	cl := NewCluster()
	c.Reconcile(cl)
	cl.StartPending()
	c.SetDesired(0) // 全部畳む
	c.Reconcile(cl)
	if len(cl.Pods()) != 0 {
		t.Fatalf("desired 0 should remove all pods, got %d", len(cl.Pods()))
	}
}
