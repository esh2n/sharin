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

// 素朴な割り算(hash % n)と円環を並べて測る。台を1つ足したときに何件動くか。
func TestRingMovesFarFewerKeysThanModulo(t *testing.T) {
	const keys = 1000
	before := New(ids(5), ConsistentHash, nil)
	after := New(ids(6), ConsistentHash, nil)

	ring, mod := 0, 0
	for i := 0; i < keys; i++ {
		k := "key-" + itoa(i)
		if before.backends[before.Pick(k)].ID != after.backends[after.Pick(k)].ID {
			ring++
		}
		if int(hash32(k))%5 != int(hash32(k))%6 {
			mod++
		}
	}
	t.Logf("5台 → 6台   円環 %d/%d 動いた   割り算 %d/%d 動いた", ring, keys, mod, keys)

	// 割り算は大半が動く。円環は 1/6 前後で収まる。
	if mod < keys*3/4 {
		t.Errorf("割り算で動いたのが %d/%d しかない", mod, keys)
	}
	if ring > keys/3 {
		t.Errorf("円環で動きすぎ: %d/%d", ring, keys)
	}
	if mod < ring*3 {
		t.Errorf("差が出ていない: 円環 %d / 割り算 %d", ring, mod)
	}
}

// ラウンドロビンは「重さが同じなら完璧」で「ばらつくと崩れる」。両方を固定する。
func TestRoundRobinBreaksOnUnevenWeights(t *testing.T) {
	const n, reqs = 20, 2000

	// 重さが全部同じなら、順番に回すだけで完全に均等になる。
	even := New(ids(n), RoundRobin, nil)
	for i := 0; i < reqs; i++ {
		even.Acquire(even.Pick(""))
	}
	if maxActive(even) != reqs/n {
		t.Fatalf("等しい重さで偏った: %d", maxActive(even))
	}

	// 10 回に1回だけ、10 倍重いリクエストが来る場合。
	weight := func(i int) int {
		if i%10 == 0 {
			return 10
		}
		return 1
	}
	load := func(b *Balancer) (max, min int) {
		for i := 0; i < reqs; i++ {
			p := b.Pick("")
			for w := 0; w < weight(i); w++ {
				b.Acquire(p)
			}
		}
		max, min = 0, 1<<30
		for _, be := range b.backends {
			if be.active > max {
				max = be.active
			}
			if be.active < min {
				min = be.active
			}
		}
		return
	}
	rrMax, rrMin := load(New(ids(n), RoundRobin, nil))
	lcMax, lcMin := load(New(ids(n), LeastConn, nil))
	p2Max, p2Min := load(New(ids(n), P2C, NewRand(42)))

	t.Logf("重さがばらつくとき   ラウンドロビン 最大 %4d / 最小 %4d", rrMax, rrMin)
	t.Logf("                     最少接続       最大 %4d / 最小 %4d", lcMax, lcMin)
	t.Logf("                     P2C            最大 %4d / 最小 %4d", p2Max, p2Min)

	// 重いものが同じ台に集まるので、ラウンドロビンは大きく崩れる。
	if rrMax <= rrMin*2 {
		t.Errorf("ラウンドロビンが崩れていない: %d, %d", rrMax, rrMin)
	}
	// 混み具合を見る2つは崩れない。
	if lcMax > lcMin*2 || p2Max > p2Min*2 {
		t.Errorf("見ているのに崩れた: lc %d/%d, p2c %d/%d", lcMax, lcMin, p2Max, p2Min)
	}
}
