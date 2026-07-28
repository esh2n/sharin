package etcdops

import "testing"

// 書くたびに版が増える。上書きでも古い版は残る。
func TestEveryWriteMakesAVersion(t *testing.T) {
	s := New(0)
	r1, _ := s.Put("a", "1")
	r2, _ := s.Put("a", "2")
	r3, _ := s.Put("b", "x")

	if r1 != 1 || r2 != 2 || r3 != 3 {
		t.Fatalf("版が単調に増えていない: %d %d %d", r1, r2, r3)
	}
	// 論理的にはキーが2つ、物理的には書いた回数ぶん。
	if s.Logical() != 2 {
		t.Fatalf("キーの数が違う: %d", s.Logical())
	}
	if s.Physical() != 3 {
		t.Fatalf("書いた回数ぶんになっていない: %d", s.Physical())
	}

	// 過去の版を読める。これが watch の土台になる。
	if v, _, _ := s.GetAt("a", 1); v != "1" {
		t.Fatalf("過去の版が読めない: %q", v)
	}
	if v, ok := s.Get("a"); v != "2" || !ok {
		t.Fatalf("今の値が違う: %q", v)
	}
}

// 削除も履歴として積まれる。消しても軽くならない。
func TestDeleteIsAlsoHistory(t *testing.T) {
	s := New(0)
	s.Put("a", "1")
	before := s.Physical()
	s.Delete("a")

	if s.Physical() <= before {
		t.Fatal("削除で物理的な量が増えていない")
	}
	if s.Logical() != 0 {
		t.Fatalf("論理的には消えているはず: %d", s.Logical())
	}
	if _, ok := s.Get("a"); ok {
		t.Fatal("消したのに読めた")
	}
	// 消える前の版は、まだ読める。
	if v, ok, _ := s.GetAt("a", 1); v != "1" || !ok {
		t.Fatalf("消える前の版が読めない: %q", v)
	}
	// 無いキーを消しても版は増えない。
	r := s.Rev()
	s.Delete("nope")
	s.Delete("a") // すでに消えている
	if s.Rev() != r+1 {
		t.Fatalf("版の増え方が想定と違う: %d → %d", r, s.Rev())
	}
}

// 履歴があるから、写しは差分で追いつける。
func TestSinceFeedsTheCache(t *testing.T) {
	s := New(0)
	s.Put("a", "1")
	s.Put("b", "2")
	s.Put("a", "3")

	evs, err := s.Since(1)
	if err != nil {
		t.Fatalf("追えない: %v", err)
	}
	if len(evs) != 2 || evs[0].Rev != 2 || evs[1].Rev != 3 {
		t.Fatalf("差分が違う: %+v", evs)
	}
	if evs[1].Key != "a" || evs[1].Value != "3" {
		t.Fatalf("内容が違う: %+v", evs[1])
	}
}

// この章の中心その1。捨てると追えなくなる。
// [apiserver] の章の「履歴が古すぎる」を、こちら側から作っている。
func TestCompactBreaksOldWatchers(t *testing.T) {
	s := New(0)
	for i := 0; i < 5; i++ {
		s.Put("a", itoa(i))
	}

	if _, err := s.Since(1); err != nil {
		t.Fatalf("捨てる前は追えるはず: %v", err)
	}
	s.Compact(4)

	if _, err := s.Since(1); err != ErrCompacted {
		t.Fatalf("捨てたのに追えてしまう: %v", err)
	}
	if _, _, err := s.GetAt("a", 1); err != ErrCompacted {
		t.Fatalf("捨てた版が読めてしまう: %v", err)
	}
	// 捨てた境界より後なら追える。
	if _, err := s.Since(4); err != nil {
		t.Fatalf("境界より後が追えない: %v", err)
	}
	// 今の値はいつでも読める。
	if v, ok := s.Get("a"); !ok || v != "4" {
		t.Fatalf("今の値が読めない: %q", v)
	}
	// すでに捨てた範囲をもう一度捨てても何も起きない。
	if n := s.Compact(3); n != 0 {
		t.Fatalf("二度目で捨てた: %d", n)
	}
	if s.CompactedAt() != 4 {
		t.Fatalf("境界が戻った: %d", s.CompactedAt())
	}
}

// この章の中心その2。捨ててもファイルは小さくならない。
func TestCompactDoesNotShrinkTheFile(t *testing.T) {
	s := New(0)
	for i := 0; i < 10; i++ {
		s.Put("a", itoa(i))
	}
	before := s.Physical()

	s.Compact(9)
	if s.Physical() != before {
		t.Fatalf("捨てただけでファイルが小さくなった: %d → %d", before, s.Physical())
	}

	freed := s.Defrag()
	if freed <= 0 {
		t.Fatal("返す場所が無いことになっている")
	}
	if s.Physical() >= before {
		t.Fatalf("返しても小さくならない: %d → %d", before, s.Physical())
	}
	// 返した後も、今の値は読める。
	if v, _ := s.Get("a"); v != "9" {
		t.Fatalf("返したら値が壊れた: %q", v)
	}
}

// この章の中心その3。容量を使い切ると書けなくなり、自分では抜けられない。
func TestOutOfSpaceNeedsThreeStepsToRecover(t *testing.T) {
	s := New(5)
	for i := 0; i < 6; i++ {
		s.Put("a", itoa(i))
	}
	if !s.Alarm() {
		t.Fatal("上限を超えたのに止まっていない")
	}

	// 書けない。読めるが書けない。
	if _, err := s.Put("b", "x"); err != ErrNoSpace {
		t.Fatalf("止まっているのに書けた: %v", err)
	}
	if _, err := s.Delete("a"); err != ErrNoSpace {
		t.Fatalf("止まっているのに消せた: %v", err)
	}
	if v, ok := s.Get("a"); !ok || v == "" {
		t.Fatal("読めなくなっている")
	}

	// ① 捨てるだけでは抜けられない。ファイルが小さくなっていないので。
	s.Compact(s.Rev())
	if ok := s.Disarm(); ok {
		t.Fatal("捨てただけで抜けられることになっている")
	}
	if _, err := s.Put("b", "x"); err == nil {
		s.Compact(s.Rev())
	}

	// ② 返して ③ 解除して、はじめて書ける。
	s2 := New(5)
	for i := 0; i < 6; i++ {
		s2.Put("a", itoa(i))
	}
	s2.Compact(s2.Rev())
	s2.Defrag()
	if !s2.Disarm() {
		t.Fatal("捨てて返しても抜けられない")
	}
	if _, err := s2.Put("b", "x"); err != nil {
		t.Fatalf("抜けたのに書けない: %v", err)
	}
}

// 上限を決めていなければ止まらない。
func TestNoQuotaNeverStops(t *testing.T) {
	s := New(0)
	for i := 0; i < 100; i++ {
		if _, err := s.Put("a", itoa(i)); err != nil {
			t.Fatalf("上限が無いのに止まった: %v", err)
		}
	}
	if s.Alarm() || s.ReadOnly() {
		t.Fatal("上限が無いのに止まっている")
	}
	if s.Disarm() != true {
		t.Fatal("止まっていないのに解除が失敗した")
	}
}

// 写しは今の値だけを運ぶ。復元した先に過去は無い。
func TestSnapshotCarriesValuesNotHistory(t *testing.T) {
	s := New(0)
	s.Put("a", "1")
	s.Put("a", "2")
	s.Put("b", "x")
	s.Delete("b")

	snap := s.Take()
	if len(snap.Data) != 1 || snap.Data["a"] != "2" {
		t.Fatalf("写しの中身が違う: %+v", snap.Data)
	}

	r := Restore(snap, 0)
	if r.Rev() != s.Rev() {
		t.Fatalf("版が引き継がれていない: %d vs %d", r.Rev(), s.Rev())
	}
	if v, ok := r.Get("a"); !ok || v != "2" {
		t.Fatalf("値が引き継がれていない: %q", v)
	}
	// 過去は運ばれないので、復元直後は誰も差分で追いつけない。
	if _, err := r.Since(1); err != ErrCompacted {
		t.Fatalf("復元先で過去が追えてしまう: %v", err)
	}
	// 復元した先からは、また普通に書ける。
	if _, err := r.Put("c", "3"); err != nil {
		t.Fatalf("復元先で書けない: %v", err)
	}
	if r.Rev() != s.Rev()+1 {
		t.Fatalf("復元先の版が続いていない: %d", r.Rev())
	}
}

// 止まっていても写しは取れる。復旧の前に手を打てる。
func TestSnapshotWorksWhileStopped(t *testing.T) {
	s := New(3)
	for i := 0; i < 5; i++ {
		s.Put("a", itoa(i))
	}
	if !s.Alarm() {
		t.Fatal("止まっていない")
	}
	snap := s.Take()
	if snap.Data["a"] == "" {
		t.Fatal("止まっていると写しが取れない")
	}
}

// 表示まわり。
func TestErrorsAndItoa(t *testing.T) {
	if ErrNoSpace.Error() == "" || ErrCompacted.Error() == "" {
		t.Fatal("説明が空")
	}
	if itoa(0) != "0" || itoa(42) != "42" || itoa(-3) != "-3" {
		t.Fatal("itoa が違う")
	}
	s := New(0)
	if s.Logical() != 0 || s.Physical() != 0 || s.CompactedAt() != 0 {
		t.Fatal("初期状態が違う")
	}
	if _, ok := s.Get("nope"); ok {
		t.Fatal("無いキーが読めた")
	}
	if v, ok, err := s.GetAt("nope", 0); ok || v != "" || err != nil {
		t.Fatal("無いキーの過去が読めた")
	}
	s.Put("a", "1")
	s.Compact(1)
	s.Defrag()
	if len(s.Log) == 0 {
		t.Fatal("記録が残っていない")
	}
}
