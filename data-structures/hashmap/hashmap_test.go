package hashmap

import "testing"

func TestPutGet(t *testing.T) {
	m := New[string, int](HashString)
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	for k, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		if got, ok := m.Get(k); !ok || got != want {
			t.Errorf("Get(%q) = (%d, %v), want (%d, true)", k, got, ok, want)
		}
	}
	if _, ok := m.Get("z"); ok {
		t.Error("存在しないキーはヒットしないべき")
	}
	if m.Len() != 3 {
		t.Errorf("Len = %d, want 3", m.Len())
	}
}

func TestUpdateExistingKey(t *testing.T) {
	m := New[string, int](HashString)
	m.Put("a", 1)
	m.Put("a", 99)
	if got, _ := m.Get("a"); got != 99 {
		t.Errorf("上書き後 = %d, want 99", got)
	}
	if m.Len() != 1 {
		t.Errorf("上書きで件数は増えないべき: Len = %d", m.Len())
	}
}

func TestDelete(t *testing.T) {
	m := New[string, int](HashString)
	m.Put("a", 1)
	m.Put("b", 2)
	if !m.Delete("a") {
		t.Error("Delete は存在すれば true を返すべき")
	}
	if _, ok := m.Get("a"); ok {
		t.Error("削除後はヒットしないべき")
	}
	if m.Delete("a") {
		t.Error("2回目の Delete は false のはず")
	}
	if got, _ := m.Get("b"); got != 2 {
		t.Error("他のキーは残っているべき")
	}
}

// ハッシュが常に同じ値を返す最悪ケース。全部同じバケットに落ちるが、
// チェイン法で正しく引けること。
func TestCollisionsHandled(t *testing.T) {
	m := New[int, int](func(int) uint64 { return 42 })
	for i := 0; i < 20; i++ {
		m.Put(i, i)
	}
	for i := 0; i < 20; i++ {
		if got, ok := m.Get(i); !ok || got != i {
			t.Errorf("Get(%d) = (%d, %v)", i, got, ok)
		}
	}
	if m.Len() != 20 {
		t.Errorf("Len = %d, want 20", m.Len())
	}
}

// 負荷率が閾値を超えるとバケット数が倍増し、それでも全要素が引けること。
func TestResizeKeepsEverything(t *testing.T) {
	m := New[int, int](HashInt)
	before := m.buckets()
	for i := 0; i < 1000; i++ {
		m.Put(i, i*2)
	}
	if m.buckets() <= before {
		t.Error("要素が増えたらバケットは拡張されるべき")
	}
	for i := 0; i < 1000; i++ {
		if got, ok := m.Get(i); !ok || got != i*2 {
			t.Fatalf("リサイズ後 Get(%d) = (%d, %v)", i, got, ok)
		}
	}
}

func TestLoadFactorStaysBounded(t *testing.T) {
	m := New[int, int](HashInt)
	for i := 0; i < 10000; i++ {
		m.Put(i, i)
	}
	lf := float64(m.Len()) / float64(m.buckets())
	if lf > maxLoadFactor+0.05 {
		t.Errorf("負荷率 = %.2f, リサイズが効いていない", lf)
	}
}

func TestKeys(t *testing.T) {
	m := New[string, int](HashString)
	m.Put("a", 1)
	m.Put("b", 2)
	if len(m.Keys()) != 2 {
		t.Errorf("Keys の数 = %d, want 2", len(m.Keys()))
	}
}

func BenchmarkPut(b *testing.B) {
	m := New[int, int](HashInt)
	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
}
