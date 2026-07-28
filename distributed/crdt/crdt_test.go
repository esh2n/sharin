package crdt

import (
	"reflect"
	"testing"
)

func eq(a, b []string) bool { return reflect.DeepEqual(a, b) }

// この章の中心。まとめ方が3つの性質を持てば、順序も回数も関係なくなる。
func TestMergeIsCommutativeAssociativeIdempotent(t *testing.T) {
	a := GCounter{"a": 3}
	b := GCounter{"b": 5}
	c := GCounter{"a": 1, "c": 2}

	// 可換: どちらからまとめても同じ
	if !reflect.DeepEqual(a.Merge(b), b.Merge(a)) {
		t.Fatal("順序で結果が変わる")
	}
	// 結合的: どこから括っても同じ
	left := a.Merge(b).Merge(c)
	right := a.Merge(b.Merge(c))
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("括り方で結果が変わる: %v vs %v", left, right)
	}
	// 冪等: 何度まとめても同じ
	once := a.Merge(b)
	twice := once.Merge(b).Merge(b)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("二度まとめると変わる: %v vs %v", once, twice)
	}
	// 自分自身とまとめても変わらない
	if !reflect.DeepEqual(a.Merge(a), a) {
		t.Fatal("自分とまとめて変わった")
	}
}

// どの順で届いても、全員が同じ値に落ち着く。
func TestAllOrdersConverge(t *testing.T) {
	a := GCounter{}
	b := GCounter{}
	c := GCounter{}
	a.Inc("a", 3)
	b.Inc("b", 5)
	c.Inc("c", 2)

	orders := [][]GCounter{
		{a, b, c}, {a, c, b}, {b, a, c}, {b, c, a}, {c, a, b}, {c, b, a},
	}
	want := 10
	for _, o := range orders {
		m := GCounter{}
		for _, x := range o {
			m = m.Merge(x)
		}
		// 二度届いたことにしても変わらない
		m = m.Merge(o[0]).Merge(o[len(o)-1])
		if m.Value() != want {
			t.Fatalf("順序 %v で %d(want %d)", o, m.Value(), want)
		}
	}
}

// 他人の要素には触らない。だから衝突しない。
func TestEachNodeOwnsItsSlot(t *testing.T) {
	g := GCounter{}
	g.Inc("a", 1)
	g.Inc("a", 2)
	g.Inc("b", 10)
	if g["a"] != 3 || g["b"] != 10 || g.Value() != 13 {
		t.Fatalf("要素の持ち主が混ざっている: %v", g)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("負を足しても止まらない")
		}
	}()
	g.Inc("a", -1)
}

// 引けないので、引く用の数え上げを足す。
func TestPNCounterSubtractsByAdding(t *testing.T) {
	x := NewPN()
	y := NewPN()
	x.Inc("a", 10)
	x.Dec("a", 3)
	y.Inc("b", 5)
	y.Dec("b", 1)

	if x.Value() != 7 || y.Value() != 4 {
		t.Fatalf("値が違う: %d %d", x.Value(), y.Value())
	}
	m1 := x.Merge(y)
	m2 := y.Merge(x)
	if m1.Value() != 11 || m2.Value() != 11 {
		t.Fatalf("まとめた値が違う: %d %d", m1.Value(), m2.Value())
	}
	// 冪等
	if m1.Merge(y).Value() != 11 {
		t.Fatal("二度まとめて変わった")
	}
	// 減らす側も、増える一方の数え上げになっている
	if m1.N.Value() != 4 {
		t.Fatalf("減らした量が合わない: %d", m1.N.Value())
	}
}

// 最後を残す形は成立するが、同時のときに片方が黙って消える。
func TestLWWLosesOneSilently(t *testing.T) {
	a := LWW{}.Set("東京", 5, "a")
	b := LWW{}.Set("大阪", 5, "b") // 同じ時刻。つまり同時に書かれた

	m := a.Merge(b)
	if m.Value != "大阪" {
		t.Fatalf("同点はノード名で決まるはず: %q", m.Value)
	}
	// 逆向きでも同じ結果になる(まとめ方としては正しい)
	if a.Merge(b) != b.Merge(a) {
		t.Fatal("順序で結果が変わる")
	}
	// だが片方は消えている
	lost, ok := a.Lost(b)
	if !ok || lost.Value != "東京" {
		t.Fatalf("消えたほうが取れない: %+v %v", lost, ok)
	}
	// 時刻が違えば、素直に後のほうが残る
	later := LWW{}.Set("名古屋", 9, "a")
	if m.Merge(later).Value != "名古屋" {
		t.Fatal("後の書き込みが残らない")
	}
	// 向きを変えても、消えるのは同じほうになる
	if lost, ok := b.Lost(a); !ok || lost.Value != "東京" {
		t.Fatalf("向きで消えるほうが変わった: %+v", lost)
	}
	// 同じものどうしなら、消えるものは無い
	if _, ok := a.Lost(a); ok {
		t.Fatal("同じ値で何かが消えた")
	}
}

// 消したものは二度と足せない。まとめ方は正しいが、袋小路になる。
func TestTwoPSetCannotReAdd(t *testing.T) {
	s := NewTwoP()
	s.Add("x")
	s.Add("y")
	s.Remove("x")

	if !eq(s.Values(), []string{"y"}) {
		t.Fatalf("消えていない: %v", s.Values())
	}
	s.Add("x") // 足し直す
	if s.Has("x") {
		t.Fatal("一度消したものが足し直せてしまった")
	}

	// まとめ方そのものは3つの性質を満たす
	o := NewTwoP()
	o.Add("z")
	if !eq(s.Merge(o).Values(), o.Merge(s).Values()) {
		t.Fatal("順序で結果が変わる")
	}
	if !eq(s.Merge(o).Values(), s.Merge(o).Merge(o).Values()) {
		t.Fatal("二度まとめて変わった")
	}
}

// 印を使うと、消してから足し直せる。
func TestORSetCanReAdd(t *testing.T) {
	s := NewOR()
	s.Add("a", "x")
	if !s.Has("x") {
		t.Fatal("足したのに無い")
	}
	s.Remove("x")
	if s.Has("x") {
		t.Fatal("消したのにある")
	}
	s.Add("a", "x") // 新しい印がつく
	if !s.Has("x") {
		t.Fatal("足し直せない")
	}
	if s.Tags("x") != 1 {
		t.Fatalf("古い印が残っている: %d", s.Tags("x"))
	}
}

// この章のもう1つの山。並行して足したものは、消しても残る。
//
// 消す側は「そのとき見えていた印」しか消せない。見ていない印は消えない。
func TestConcurrentAddSurvivesRemove(t *testing.T) {
	// 2つのノードが、同じ値を独立に足す。
	a := NewOR()
	b := NewOR()
	a.Add("a", "x")
	b.Add("b", "x")

	// a は自分の見えているぶんだけ消す。b の追加は見ていない。
	a.Remove("x")
	if a.Has("x") {
		t.Fatal("自分のぶんが消えていない")
	}

	m := a.Merge(b)
	if !m.Has("x") {
		t.Fatal("並行して足したぶんまで消えた")
	}
	if m.Tags("x") != 1 {
		t.Fatalf("残るのは b の印1つだけのはず: %d", m.Tags("x"))
	}
	// 逆順でも同じ
	if !eq(m.Values(), b.Merge(a).Values()) {
		t.Fatal("順序で結果が変わる")
	}
}

// 両方が同じ印を見てから消せば、ちゃんと消える。
func TestRemoveAfterSeeingBothIsFinal(t *testing.T) {
	a := NewOR()
	b := NewOR()
	a.Add("a", "x")
	b.Add("b", "x")

	// まず両方の印を合わせてから消す。
	both := a.Merge(b)
	if both.Tags("x") != 2 {
		t.Fatalf("印が2つそろっていない: %d", both.Tags("x"))
	}
	both.Remove("x")
	if both.Has("x") {
		t.Fatal("両方見てから消したのに残った")
	}
	// 古い写しとまとめても、その古い写しの印は生き返る。
	// だから「見てから消す」ことに意味がある。
	revived := both.Merge(a)
	if !revived.Has("x") {
		t.Fatal("古い写しの印が生き返らない")
	}
}

// OR-Set のまとめ方も3つの性質を満たす。
func TestORSetMergeProperties(t *testing.T) {
	a, b, c := NewOR(), NewOR(), NewOR()
	a.Add("a", "x")
	b.Add("b", "y")
	c.Add("c", "x")
	c.Add("c", "z")

	if !eq(a.Merge(b).Values(), b.Merge(a).Values()) {
		t.Fatal("順序で結果が変わる")
	}
	left := a.Merge(b).Merge(c).Values()
	right := a.Merge(b.Merge(c)).Values()
	if !eq(left, right) {
		t.Fatalf("括り方で結果が変わる: %v vs %v", left, right)
	}
	once := a.Merge(b)
	if !eq(once.Values(), once.Merge(b).Merge(b).Values()) {
		t.Fatal("二度まとめて変わった")
	}
	if !eq(left, []string{"x", "y", "z"}) {
		t.Fatalf("中身が違う: %v", left)
	}
	// 印が2つついた値は、まとめた後も1つの要素として見える。
	if a.Merge(c).Tags("x") != 2 {
		t.Fatal("印が合流していない")
	}
	// 印の番号も引き継がれる。
	m := a.Merge(c)
	m.Add("c", "w")
	if !m.Has("w") {
		t.Fatal("まとめた後に足せない")
	}
}

// 空の集合まわり。
func TestEmpty(t *testing.T) {
	s := NewOR()
	if s.Has("x") || s.Values() != nil || s.Tags("x") != 0 {
		t.Fatal("空でない")
	}
	s.Remove("x") // 無いものを消しても壊れない
	tp := NewTwoP()
	if tp.Has("x") || tp.Values() != nil {
		t.Fatal("空でない")
	}
	g := GCounter{}
	if g.Value() != 0 {
		t.Fatal("空の合計が 0 でない")
	}
	if NewPN().Value() != 0 {
		t.Fatal("空の差が 0 でない")
	}
	var r LWW
	if r.Merge(LWW{}) != (LWW{}) {
		t.Fatal("空どうしで何か出た")
	}
}
