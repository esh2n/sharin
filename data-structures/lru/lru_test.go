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
