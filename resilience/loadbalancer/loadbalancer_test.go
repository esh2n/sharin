package loadbalancer

import (
	"testing"
)

func ids(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = "b" + itoa(i)
	}
	return out
}

func TestStrategyStringAndEmpty(t *testing.T) {
	for s, want := range map[Strategy]string{
		RoundRobin: "round-robin", LeastConn: "least-conn",
		P2C: "p2c", ConsistentHash: "consistent-hash", Strategy(99): "unknown",
	} {
		if got := s.String(); got != want {
			t.Fatalf("%d: got %q want %q", int(s), got, want)
		}
	}
	// 台が無ければ -1。
	empty := New(nil, RoundRobin, nil)
	if got := empty.Pick(""); got != -1 {
		t.Fatalf("empty pick: got %d want -1", got)
	}
	if len(empty.Backends()) != 0 {
		t.Fatalf("expected no backends")
	}
	// Release は 0 未満にならない。
	b := New(ids(1), RoundRobin, nil)
	b.Release(0)
	if b.Backends()[0].Active() != 0 {
		t.Fatalf("release below zero")
	}
}

func TestRoundRobinCycles(t *testing.T) {
	b := New(ids(3), RoundRobin, nil)
	got := make([]int, 7)
	for i := range got {
		got[i] = b.Pick("")
	}
	want := []int{0, 1, 2, 0, 1, 2, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pick %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestLeastConnPicksEmptiest(t *testing.T) {
	b := New(ids(3), LeastConn, nil)
	b.Acquire(0)
	b.Acquire(0)
	b.Acquire(1)
	// b2 が最少(0)。
	if got := b.Pick(""); got != 2 {
		t.Fatalf("got %d want 2", got)
	}
	// 同数なら若い番号。全部同じにして 0 を期待。
	b2 := New(ids(3), LeastConn, nil)
	if got := b2.Pick(""); got != 0 {
		t.Fatalf("tie: got %d want 0", got)
	}
}

func TestP2CPicksLighterOfTwo(t *testing.T) {
	b := New(ids(4), P2C, NewRand(1))
	// 1 台だけ重くして、それが選ばれにくいことを確かめる。
	for i := 0; i < 50; i++ {
		b.backends[2].active = 100
		b.Pick("")
	}
	// 重い台(2)が 2 択に入っても、もう一方が軽ければそちらが選ばれる。
	heavyPicked := 0
	b2 := New(ids(4), P2C, NewRand(1))
	b2.backends[2].active = 100
	for i := 0; i < 200; i++ {
		if b2.Pick("") == 2 {
			heavyPicked++
		}
	}
	// 重い台が選ばれるのは「2 択が両方 2 番」の稀なときだけ(約 1/16)。
	if heavyPicked > 40 {
		t.Fatalf("heavy backend picked too often: %d/200", heavyPicked)
	}
}

func TestP2CSingleBackend(t *testing.T) {
	b := New(ids(1), P2C, NewRand(1))
	if got := b.Pick(""); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}

// TestP2CBeatsRandom は P2C の要。同じ負荷を投げたとき、
// P2C の最大負荷(いちばん混んだ台)がランダムよりはっきり小さいことを固定する。
func TestP2CBeatsRandom(t *testing.T) {
	const n = 20      // 台数
	const reqs = 2000 // リクエスト数

	// ランダム: 毎回 1 台を無作為に選ぶ(Acquire だけ、Release しない)。
	rnd := New(ids(n), RoundRobin, nil) // 方式は使わず active だけ観測
	rr := NewRand(42)
	for i := 0; i < reqs; i++ {
		rnd.Acquire(rr.intn(n))
	}
	randMax := maxActive(rnd)

	// P2C: 2 台選んで軽い方へ。
	p := New(ids(n), P2C, NewRand(42))
	for i := 0; i < reqs; i++ {
		p.Acquire(p.Pick(""))
	}
	p2cMax := maxActive(p)

	avg := reqs / n // 100
	// P2C の最大は平均にごく近い。ランダムは平均から大きくはみ出す。
	if p2cMax >= randMax {
		t.Fatalf("expected p2c max (%d) < random max (%d)", p2cMax, randMax)
	}
	if p2cMax > avg+avg/5 {
		t.Fatalf("p2c max %d too far above avg %d", p2cMax, avg)
	}
	t.Logf("avg=%d p2c_max=%d random_max=%d", avg, p2cMax, randMax)
}

func maxActive(b *Balancer) int {
	m := 0
	for _, be := range b.backends {
		if be.active > m {
			m = be.active
		}
	}
	return m
}

func TestConsistentHashStable(t *testing.T) {
	b := New(ids(5), ConsistentHash, nil)
	// 同じキーは常に同じ台へ。
	first := b.Pick("user-42")
	for i := 0; i < 20; i++ {
		if got := b.Pick("user-42"); got != first {
			t.Fatalf("unstable: got %d want %d", got, first)
		}
	}
}

// TestConsistentHashMinimalRemap は一貫ハッシュの核心。台を 1 つ増やしても、
// 振り先が変わるキーはごく一部(素朴な mod n なら大半が動く)であることを示す。
func TestConsistentHashMinimalRemap(t *testing.T) {
	before := New(ids(5), ConsistentHash, nil)
	after := New(ids(6), ConsistentHash, nil)

	const keys = 1000
	moved := 0
	for i := 0; i < keys; i++ {
		k := "key-" + itoa(i)
		beforeID := before.backends[before.Pick(k)].ID
		afterID := after.backends[after.Pick(k)].ID
		if beforeID != afterID {
			moved++
		}
	}
	// 6 台目に移るのは概ね 1/6 前後。半分を超えることはない。
	if moved > keys/2 {
		t.Fatalf("too many keys moved: %d/%d", moved, keys)
	}
	t.Logf("moved %d/%d keys when growing 5->6", moved, keys)
}
