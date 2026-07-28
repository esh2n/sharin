package daemonset

import "testing"

func logAgent() Spec { return Spec{Name: "log-agent"} }

func cluster(spec Spec, n int) *Set {
	s := New(spec)
	for i := 1; i <= n; i++ {
		s.AddNode("node-"+itoa(i), true, nil)
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// 対象のノードすべてに1つずつ置かれる。数はどこにも宣言していない。
func TestOnePerNode(t *testing.T) {
	s := cluster(logAgent(), 3)
	s.Reconcile()

	if len(s.Pods()) != 3 {
		t.Fatalf("3 台なら 3 個のはずが %d", len(s.Pods()))
	}
	for _, n := range s.Targets() {
		if !s.PodOn(n) {
			t.Fatalf("%s に載っていない", n)
		}
	}
	if !s.Converged() {
		t.Fatal("揃っているはず")
	}
}

// ノードが増えれば自動的に増える。数を書き換える必要がない。
func TestFollowsNodeAdditions(t *testing.T) {
	s := cluster(logAgent(), 2)
	s.Reconcile()
	if s.Desired() != 2 {
		t.Fatalf("2 のはずが %d", s.Desired())
	}

	s.AddNode("node-3", true, nil)
	if s.Desired() != 3 {
		t.Fatalf("ノードが増えれば必要数も増えるはずが %d", s.Desired())
	}
	acts := s.Reconcile()
	if len(acts) != 1 || acts[0].Kind != "create" || acts[0].Node != "node-3" {
		t.Fatalf("node-3 にだけ作るはずが %+v", acts)
	}
}

// ノードが減れば、載っていた Pod も消える。
func TestFollowsNodeRemovals(t *testing.T) {
	s := cluster(logAgent(), 3)
	s.Reconcile()
	s.RemoveNode("node-2")
	s.Reconcile()

	if len(s.Pods()) != 2 {
		t.Fatalf("2 個になるはずが %d", len(s.Pods()))
	}
	if s.PodOn("node-2") {
		t.Fatal("消えたノードに載ったままになっている")
	}
}

// ready でないノードは対象から外れ、載っていた Pod は消える。
// 戻れば、また置かれる。
func TestUnreadyNodeLeavesTargets(t *testing.T) {
	s := cluster(logAgent(), 3)
	s.Reconcile()

	s.SetReady("node-2", false)
	acts := s.Reconcile()
	if len(acts) != 1 || acts[0].Kind != "delete" {
		t.Fatalf("対象から外れた分を消すはずが %+v", acts)
	}
	if s.Desired() != 2 {
		t.Fatalf("対象は 2 のはずが %d", s.Desired())
	}

	s.SetReady("node-2", true)
	s.Reconcile()
	if !s.PodOn("node-2") {
		t.Fatal("戻れば置き直されるはず")
	}
}

// セレクタで対象を絞れる。ラベルの合うノードにだけ置かれる。
func TestSelectorNarrowsTargets(t *testing.T) {
	s := New(Spec{Name: "gpu-agent", Selector: map[string]string{"hardware": "gpu"}})
	s.AddNode("plain-1", true, nil)
	s.AddNode("gpu-1", true, map[string]string{"hardware": "gpu"})
	s.AddNode("gpu-2", true, map[string]string{"hardware": "gpu"})
	s.Reconcile()

	if s.Desired() != 2 {
		t.Fatalf("gpu の 2 台だけのはずが %d", s.Desired())
	}
	if s.PodOn("plain-1") {
		t.Fatal("対象外のノードに置かれている")
	}
}

// ラベルが変われば対象も変わる。付ければ置かれ、外せば消える。
func TestLabelChangeMovesTargets(t *testing.T) {
	s := New(Spec{Name: "agent", Selector: map[string]string{"role": "edge"}})
	n := s.AddNode("node-1", true, nil)
	s.Reconcile()
	if len(s.Pods()) != 0 {
		t.Fatal("ラベルが無いので置かれないはず")
	}

	n.labels["role"] = "edge"
	s.Reconcile()
	if !s.PodOn("node-1") {
		t.Fatal("ラベルを付ければ置かれるはず")
	}

	delete(n.labels, "role")
	s.Reconcile()
	if s.PodOn("node-1") {
		t.Fatal("ラベルを外せば消えるはず")
	}
}

// 汚れのあるノードには、許容していなければ置かれない。
// 監視や収集は、他の Pod が避けるノードにも置かれてほしい。
func TestTaintsAndTolerations(t *testing.T) {
	plain := New(Spec{Name: "agent"})
	plain.AddNode("normal", true, nil)
	plain.AddNode("tainted", true, nil, "dedicated=infra")
	plain.Reconcile()
	if plain.Desired() != 1 || plain.PodOn("tainted") {
		t.Fatalf("汚れたノードには置かれないはず: %d", plain.Desired())
	}

	tolerant := New(Spec{Name: "agent", Tolerations: []string{"*"}})
	tolerant.AddNode("normal", true, nil)
	tolerant.AddNode("tainted", true, nil, "dedicated=infra")
	tolerant.Reconcile()
	if tolerant.Desired() != 2 {
		t.Fatalf("すべて許容すれば両方に置かれるはずが %d", tolerant.Desired())
	}
}

// 揃った後の調整は何もしない(冪等)。
func TestIdempotent(t *testing.T) {
	s := cluster(logAgent(), 3)
	s.Reconcile()
	for i := 0; i < 5; i++ {
		if acts := s.Reconcile(); len(acts) != 0 {
			t.Fatalf("揃った後は何もしないはずが %+v", acts)
		}
	}
}

// まとめて増減しても、1回の調整で追いつく。
func TestConvergesAfterBulkChange(t *testing.T) {
	s := cluster(logAgent(), 2)
	s.Reconcile()

	s.AddNode("node-3", true, nil)
	s.AddNode("node-4", true, nil)
	s.RemoveNode("node-1")
	s.Reconcile()

	if !s.Converged() {
		t.Fatalf("1 回で揃うはず: %d 個 / 対象 %d\n%v", len(s.Pods()), s.Desired(), s.Log)
	}
	if len(s.Pods()) != 3 {
		t.Fatalf("3 個のはずが %d", len(s.Pods()))
	}
}

// ノードが1台も無ければ、必要数は 0 になる。
func TestNoNodes(t *testing.T) {
	s := New(logAgent())
	s.Reconcile()
	if s.Desired() != 0 || len(s.Pods()) != 0 {
		t.Fatal("ノードが無ければ 0 のはず")
	}
	if !s.Converged() {
		t.Fatal("0 個で揃っているはず")
	}
	s.RemoveNode("nosuch") // 何も起きない
}
