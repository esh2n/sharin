package consistenthash

import (
	"fmt"
	"hash/fnv"
	"testing"
)

func keys(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("key-%d", i)
	}
	return out
}

func distribution(r *Ring, ks []string) map[string]int {
	d := map[string]int{}
	for _, k := range ks {
		n, ok := r.Get(k)
		if ok {
			d[n]++
		}
	}
	return d
}

func TestEmptyRing(t *testing.T) {
	r := New(10, nil)
	if _, ok := r.Get("x"); ok {
		t.Fatal("empty ring should return ok=false")
	}
	if got := r.GetN("x", 3); got != nil {
		t.Fatalf("GetN on empty ring = %v, want nil", got)
	}
}

func TestGetIsConsistent(t *testing.T) {
	r := New(50, nil)
	r.Add("a", "b", "c")
	first, ok := r.Get("hello")
	if !ok {
		t.Fatal("expected a node")
	}
	for i := 0; i < 100; i++ {
		got, _ := r.Get("hello")
		if got != first {
			t.Fatalf("Get not deterministic: %s vs %s", got, first)
		}
	}
}

func TestEveryKeyMapsToARegisteredNode(t *testing.T) {
	r := New(50, nil)
	r.Add("a", "b", "c")
	valid := map[string]bool{"a": true, "b": true, "c": true}
	for _, k := range keys(500) {
		n, ok := r.Get(k)
		if !ok || !valid[n] {
			t.Fatalf("key %s -> %q (ok=%v), not a registered node", k, n, ok)
		}
	}
}

// リングの肝: ノードを1台外しても、その担当キーだけが動く。他は不変。
func TestRemoveOnlyRemapsAffectedKeys(t *testing.T) {
	r := New(100, nil)
	r.Add("a", "b", "c")
	ks := keys(3000)
	before := map[string]string{}
	for _, k := range ks {
		before[k], _ = r.Get(k)
	}

	r.Remove("b")
	moved := 0
	for _, k := range ks {
		after, _ := r.Get(k)
		if after == "b" {
			t.Fatalf("key %s still maps to removed node b", k)
		}
		if before[k] == "b" {
			moved++ // b が持っていたキーは動いて当然
			continue
		}
		if after != before[k] {
			t.Fatalf("key %s moved from %s to %s but its node was not removed", k, before[k], after)
		}
	}
	if moved == 0 {
		t.Fatal("expected some keys to move off b")
	}
	// 動いたのは b の担当ぶんだけ。全体の半分未満(素朴な mod なら大半が動く)。
	if moved >= len(ks)/2 {
		t.Fatalf("too many keys moved: %d/%d", moved, len(ks))
	}
}

// 素朴な mod N との対比: ノード増減でコンシステントハッシュの方が圧倒的に動きが少ない。
func TestFewerRemapsThanModulo(t *testing.T) {
	ks := keys(3000)
	nodes := []string{"a", "b", "c", "d"}

	// consistent: 4台 -> 3台(d を外す)
	r := New(100, nil)
	r.Add(nodes...)
	chBefore := map[string]string{}
	for _, k := range ks {
		chBefore[k], _ = r.Get(k)
	}
	r.Remove("d")
	chMoved := 0
	for _, k := range ks {
		if after, _ := r.Get(k); after != chBefore[k] {
			chMoved++
		}
	}

	// modulo: 4 -> 3 で hash%N のノードがどれだけ変わるか
	mod := func(k string, n int) string {
		h := fnv.New32a()
		_, _ = h.Write([]byte(k))
		return nodes[int(h.Sum32())%n]
	}
	modMoved := 0
	for _, k := range ks {
		if mod(k, 4) != mod(k, 3) {
			modMoved++
		}
	}

	if chMoved >= modMoved {
		t.Fatalf("consistent hashing should remap fewer keys: ch=%d mod=%d", chMoved, modMoved)
	}
	t.Logf("remaps on 4->3: consistent=%d, modulo=%d (of %d)", chMoved, modMoved, len(ks))
}

// 仮想ノードを増やすと負荷が平準化する。
func TestVirtualNodesImproveBalance(t *testing.T) {
	ks := keys(6000)
	spread := func(replicas int) float64 {
		r := New(replicas, nil)
		r.Add("a", "b", "c")
		d := distribution(r, ks)
		mx, mn := 0, len(ks)+1
		for _, n := range []string{"a", "b", "c"} {
			if d[n] > mx {
				mx = d[n]
			}
			if d[n] < mn {
				mn = d[n]
			}
		}
		return float64(mx) / float64(mn) // 1.0 に近いほど均等
	}
	few := spread(1)
	many := spread(300)
	if many >= few {
		t.Fatalf("more vnodes should balance better: replicas=1 spread=%.2f, replicas=300 spread=%.2f", few, many)
	}
	if many > 1.5 {
		t.Fatalf("300 vnodes should keep max/min under 1.5, got %.2f", many)
	}
	t.Logf("max/min spread: replicas=1 -> %.2f, replicas=300 -> %.2f", few, many)
}

func TestGetNReturnsDistinctNodes(t *testing.T) {
	r := New(50, nil)
	r.Add("a", "b", "c", "d")
	got := r.GetN("some-key", 3)
	if len(got) != 3 {
		t.Fatalf("GetN(3) = %v, want 3 nodes", got)
	}
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] {
			t.Fatalf("GetN returned duplicate node: %v", got)
		}
		seen[n] = true
	}
	// 先頭は Get と一致する。
	if first, _ := r.Get("some-key"); got[0] != first {
		t.Fatalf("GetN[0]=%s should equal Get=%s", got[0], first)
	}
}

func TestGetNCapsAtNodeCount(t *testing.T) {
	r := New(50, nil)
	r.Add("a", "b")
	got := r.GetN("k", 5)
	if len(got) != 2 {
		t.Fatalf("GetN(5) with 2 nodes = %v, want 2", got)
	}
}

func TestAddIsIdempotentAndRemoveUnknownIsNoop(t *testing.T) {
	r := New(10, nil)
	r.Add("a")
	n1 := len(r.points)
	r.Add("a") // 二重追加は無視
	if len(r.points) != n1 {
		t.Fatalf("double Add changed points: %d -> %d", n1, len(r.points))
	}
	r.Remove("zzz") // 未知の削除は no-op
	if len(r.points) != n1 {
		t.Fatal("Remove of unknown node changed the ring")
	}
}

func TestNewClampsReplicasAndGetNZero(t *testing.T) {
	r := New(0, nil) // replicas<1 は 1 に切り上げ
	r.Add("a", "b")
	if len(r.points) != 2 {
		t.Fatalf("New(0) should clamp replicas to 1: points=%d, want 2", len(r.points))
	}
	if got := r.GetN("k", 0); got != nil {
		t.Fatalf("GetN(0) = %v, want nil", got)
	}
}

func TestNodes(t *testing.T) {
	r := New(10, nil)
	r.Add("c", "a", "b")
	got := r.Nodes()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Nodes() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Nodes() = %v, want sorted %v", got, want)
		}
	}
}
