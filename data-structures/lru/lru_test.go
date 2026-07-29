package lru

import "testing"

func TestPutGet(t *testing.T) {
	c, err := New[string, int](2)
	if err != nil {
		t.Fatal(err)
	}
	c.Put("a", 1)
	c.Put("b", 2)

	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Errorf("Get(a) = (%d, %v), want (1, true)", v, ok)
	}
	if _, ok := c.Get("x"); ok {
		t.Error("存在しないキーはヒットしないべき")
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c, _ := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	// c を入れると、一番長く使われていない a が追い出される。
	key, val, evicted := c.Put("c", 3)
	if !evicted || key != "a" || val != 1 {
		t.Errorf("Put(c) の追い出し = (%v, %d, %v), want (a, 1, true)", key, val, evicted)
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a は追い出されているべき")
	}
}

func TestGetRefreshesRecency(t *testing.T) {
	c, _ := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a") // a を触ったので、次に古いのは b

	key, _, evicted := c.Put("c", 3)
	if !evicted || key != "b" {
		t.Errorf("追い出されるのは b のはず: got %v (evicted=%v)", key, evicted)
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("直前に触った a は残っているべき")
	}
}

func TestUpdateExistingKey(t *testing.T) {
	c, _ := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	// 既存キーの上書きは追い出しを起こさず、recency も更新する。
	if _, _, evicted := c.Put("a", 10); evicted {
		t.Error("既存キーの更新で追い出しは起きないべき")
	}
	if v, _ := c.Get("a"); v != 10 {
		t.Errorf("a = %d, want 10", v)
	}
	key, _, _ := c.Put("c", 3)
	if key != "b" {
		t.Errorf("追い出されるのは b のはず: got %v", key)
	}
}

func TestCapacityOne(t *testing.T) {
	c, _ := New[string, int](1)
	c.Put("a", 1)
	key, _, evicted := c.Put("b", 2)
	if !evicted || key != "a" {
		t.Errorf("容量1では毎回入れ替わる: got (%v, %v)", key, evicted)
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Error("b は残っているべき")
	}
}

func TestValidation(t *testing.T) {
	if _, err := New[string, int](0); err == nil {
		t.Error("容量0はエラーになるべき")
	}
}

// この章の中心。容量を1つ超える周回で、当たり方が崖のように落ちる。
func TestScanFallsOffACliff(t *testing.T) {
	// 容量ちょうどの範囲を何周も回る。2周目からは全部当たる。
	rate := func(keys, rounds int) float64 {
		c, err := New[int, int](100)
		if err != nil {
			t.Fatal(err)
		}
		for r := 0; r < rounds; r++ {
			for k := 0; k < keys; k++ {
				if _, ok := c.Get(k); !ok {
					c.Put(k, k)
				}
			}
		}
		return c.HitRate()
	}

	fit := rate(100, 10)  // 容量ちょうど
	over := rate(101, 10) // たった1つ多い

	if fit < 0.85 {
		t.Fatalf("容量に収まっているのに当たらない: %.3f", fit)
	}
	if over != 0 {
		t.Fatalf("1つ超えたら1度も当たらないはず: %.3f", over)
	}
}

// 局所性があれば、容量より広い範囲でも当たる。落ちるのは一巡なめるときだけ。
func TestLocalityStillHits(t *testing.T) {
	c, err := New[int, int](100)
	if err != nil {
		t.Fatal(err)
	}
	// 200 個の中から、狭い範囲を繰り返し引く。
	seq := []int{}
	for r := 0; r < 100; r++ {
		for k := 0; k < 10; k++ {
			seq = append(seq, k)
		}
		seq = append(seq, 100+r%100) // ときどき遠くを触る
	}
	for _, k := range seq {
		if _, ok := c.Get(k); !ok {
			c.Put(k, k)
		}
	}
	if c.HitRate() < 0.8 {
		t.Fatalf("局所性があるのに当たらない: %.3f", c.HitRate())
	}
}

// 数えまわり。
func TestStats(t *testing.T) {
	c, _ := New[string, int](2)
	if c.HitRate() != 0 {
		t.Fatal("引く前は 0")
	}
	c.Put("a", 1)
	c.Get("a")
	c.Get("b")
	if c.Hits() != 1 || c.Misses() != 1 {
		t.Fatalf("%d %d", c.Hits(), c.Misses())
	}
	if c.HitRate() != 0.5 {
		t.Fatalf("%.3f", c.HitRate())
	}
	c.ResetStats()
	if c.Hits() != 0 || c.Misses() != 0 || c.HitRate() != 0 {
		t.Fatal("数え直せていない")
	}
}

// Items は中身をまるごと返す。順序は見ないし、recency も変えない。
func TestItemsDoesNotTouchRecency(t *testing.T) {
	c, _ := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)

	got := c.Items()
	if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("中身が違う: %v", got)
	}
	// a を見ても順は変わらないので、次に入れると a が出ていく。
	k, _, evicted := c.Put("c", 3)
	if !evicted || k != "a" {
		t.Fatalf("recency が変わっている: %q %v", k, evicted)
	}
}

// unlink は先頭を外す場合も書いてあるが、公開された操作からはそこへ来ない
// (moveToFront は先頭なら何もしないし、追い出すのは末尾だけ)。
// 備えとして残しているので、直接呼んで確かめておく。
func TestUnlinkHead(t *testing.T) {
	c, _ := New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	head := c.head
	if head.key != "b" {
		t.Fatalf("先頭が違う: %q", head.key)
	}
	c.unlink(head)
	if c.head == nil || c.head.key != "a" {
		t.Fatal("先頭を外したあとが繋がっていない")
	}
	if c.head.prev != nil {
		t.Fatal("新しい先頭に prev が残っている")
	}
}
