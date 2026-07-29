package blockchain

import "testing"

// 難易度を1上げると、当たりを引く確率が16分の1になる。
func TestDifficultyCostsSixteenTimesMore(t *testing.T) {
	var prev int
	for d := 1; d <= 4; d++ {
		c := New(d)
		c.ResetStats()
		for i := 0; i < 8; i++ {
			c.Add("alice -> bob 10")
		}
		per := c.Attempts() / 8
		t.Logf("難易度 %d   1ブロックあたり %7d 回", d, per)
		if d > 1 && per < prev*4 {
			t.Errorf("難易度 %d で %d 回 → %d 回。増え方が足りない", d, prev, per)
		}
		prev = per
	}
}

// この章の中心。過去を書き換えて隠す値段は、後ろに積まれたブロックの数に比例する。
func TestTamperCostGrowsWithDepth(t *testing.T) {
	const difficulty = 2

	build := func(n int) *Chain {
		c := New(difficulty)
		for i := 0; i < n; i++ {
			c.Add("alice -> bob 10")
		}
		return c
	}

	// まず、1つ直しただけでは隠せないことを確かめる。
	c := build(20)
	c.Tamper(1, "alice -> bob 1000000")
	if c.Valid() {
		t.Fatal("書き換えたのに壊れていない")
	}
	one := remine(&c.Blocks[1], difficulty)
	if c.Valid() {
		t.Fatalf("1つ直しただけで通ってしまった(%d 回)", one)
	}

	// 後ろまで全部やり直すと通る。値段は積まれた数に比例する。
	var costs []int
	for _, depth := range []int{5, 20, 80} {
		c := build(depth)
		c.Tamper(1, "alice -> bob 1000000")
		c.ResetStats()
		cost := c.Repair(1)
		costs = append(costs, cost)
		t.Logf("後ろに %2d ブロック   作り直し %2d 個 / %6d 回   1ブロックあたり %4d 回",
			depth, c.Len()-1, cost, cost/(c.Len()-1))
		if !c.Valid() {
			t.Fatalf("直しきれていない(深さ %d)", depth)
		}
	}
	// 1ブロックあたりの値段は難易度で決まるので、総額は作り直す数に比例する。
	// 16倍積まれていれば、総額も一桁上がる。
	if costs[2] < costs[0]*8 {
		t.Errorf("深さで値段が上がっていない: %v", costs)
	}
}

// 難易度と深さは掛け算になる。両方効く。
func TestCostIsDifficultyTimesDepth(t *testing.T) {
	cost := func(difficulty, depth int) int {
		c := New(difficulty)
		for i := 0; i < depth; i++ {
			c.Add("alice -> bob 10")
		}
		c.Tamper(1, "alice -> bob 1000000")
		c.ResetStats()
		return c.Repair(1)
	}
	a, b := cost(2, 8), cost(3, 8)
	t.Logf("深さ8: 難易度2 で %6d 回 / 難易度3 で %7d 回", a, b)
	if b < a*4 {
		t.Errorf("難易度を上げても値段が上がっていない: %d, %d", a, b)
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	c := New(1)
	if !c.Valid() || c.Len() != 1 {
		t.Fatal("genesis だけのチェーンが不正")
	}
	// genesis を書き換えても、直せば通る。
	c.Add("x")
	c.Tamper(0, "not genesis")
	if c.Valid() {
		t.Fatal("genesis の改竄が検出されない")
	}
	if c.Repair(0) <= 0 || !c.Valid() {
		t.Fatal("genesis から直せない")
	}
	// 数え直せる。
	c.ResetStats()
	if c.Attempts() != 0 {
		t.Errorf("数え直せていない: %d", c.Attempts())
	}
	// 難易度 0 以下は受け付けない。
	defer func() {
		if recover() == nil {
			t.Error("難易度 0 が通った")
		}
	}()
	New(0)
}
