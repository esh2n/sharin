package mvcc

import (
	"errors"
	"testing"
)

func TestSnapshotRead(t *testing.T) {
	s := NewStore(map[string]string{"x": "v0"})

	t1 := s.Begin(Snapshot) // x=v0 の世界を見る
	t2 := s.Begin(Snapshot)
	_ = t2.Put("x", "v1")
	if err := t2.Commit(); err != nil {
		t.Fatalf("t2 commit: %v", err)
	}

	// t1 は t2 のコミット後でも、開始時点の v0 を見続ける(repeatable read)。
	if v, _ := t1.Get("x"); v != "v0" {
		t.Fatalf("t1 は v0 を見るはず got %q", v)
	}
	// 新しく始めた t3 は v1 を見る。
	t3 := s.Begin(Snapshot)
	if v, _ := t3.Get("x"); v != "v1" {
		t.Fatalf("t3 は v1 を見るはず got %q", v)
	}
	// 版は上書きされず 2 つ積まれている。
	if len(s.Versions("x")) != 2 {
		t.Fatalf("版数 want 2 got %d", len(s.Versions("x")))
	}
}

func TestReadYourWritesAndBuffer(t *testing.T) {
	s := NewStore(map[string]string{"x": "v0"})
	t1 := s.Begin(Snapshot)
	_ = t1.Put("x", "mine")
	if v, _ := t1.Get("x"); v != "mine" {
		t.Fatalf("自分の書き込みが見えない got %q", v)
	}
	// コミット前は他から見えない。
	t2 := s.Begin(Snapshot)
	if v, _ := t2.Get("x"); v != "v0" {
		t.Fatalf("未コミットが漏れた got %q", v)
	}
	// Abort したら何も残らない。
	t1.Abort()
	t3 := s.Begin(Snapshot)
	if v, _ := t3.Get("x"); v != "v0" {
		t.Fatalf("abort 後も v0 のはず got %q", v)
	}
}

func TestMissingKey(t *testing.T) {
	s := NewStore(nil)
	t1 := s.Begin(Snapshot)
	if _, ok := t1.Get("nope"); ok {
		t.Fatal("無いキーが見えた")
	}
}

func TestFirstCommitterWins(t *testing.T) {
	// lost update の防止: 同じキーに並行で書いた 2 本のうち後コミットは敗北する。
	s := NewStore(map[string]string{"balance": "100"})
	t1 := s.Begin(Snapshot)
	t2 := s.Begin(Snapshot)
	_ = t1.Put("balance", "150") // t1: +50 のつもり
	_ = t2.Put("balance", "70")  // t2: -30 のつもり

	if err := t1.Commit(); err != nil {
		t.Fatalf("t1 commit: %v", err)
	}
	if err := t2.Commit(); !errors.Is(err, ErrWriteConflict) {
		t.Fatalf("t2 は first-committer-wins で敗北するはず got %v", err)
	}
	// 生き残ったのは t1 の版だけ。
	t3 := s.Begin(Snapshot)
	if v, _ := t3.Get("balance"); v != "150" {
		t.Fatalf("balance want 150 got %q", v)
	}
}

func TestWriteSkewUnderSI(t *testing.T) {
	// write skew: SI では「別々のキーに書く」ので両方通ってしまう。
	s := NewStore(map[string]string{"oncall:alice": "yes", "oncall:bob": "yes"})

	t1 := s.Begin(Snapshot)
	t2 := s.Begin(Snapshot)
	// 両方とも「2 人いる」ことを確認してから、自分だけ抜ける。
	readBoth := func(tx *Txn) int {
		n := 0
		for _, k := range []string{"oncall:alice", "oncall:bob"} {
			if v, _ := tx.Get(k); v == "yes" {
				n++
			}
		}
		return n
	}
	if readBoth(t1) != 2 || readBoth(t2) != 2 {
		t.Fatal("前提: 2 人当直")
	}
	_ = t1.Put("oncall:alice", "no")
	_ = t2.Put("oncall:bob", "no")

	if err := t1.Commit(); err != nil {
		t.Fatalf("t1 commit: %v", err)
	}
	if err := t2.Commit(); err != nil {
		t.Fatalf("SI では write skew が通ってしまうはず got %v", err)
	}
	// 結果: 当直ゼロ(直列実行では決して起きない状態)。
	t3 := s.Begin(Snapshot)
	if readBoth(t3) != 0 {
		t.Fatal("write skew の異常状態を確認したかった")
	}
}

func TestWriteSkewPreventedBySerializable(t *testing.T) {
	// 同じ筋書きを Serializable で流すと、後コミットが読み集合の検証で中止される。
	s := NewStore(map[string]string{"oncall:alice": "yes", "oncall:bob": "yes"})

	t1 := s.Begin(Serializable)
	t2 := s.Begin(Serializable)
	for _, k := range []string{"oncall:alice", "oncall:bob"} {
		_, _ = t1.Get(k)
		_, _ = t2.Get(k)
	}
	_ = t1.Put("oncall:alice", "no")
	_ = t2.Put("oncall:bob", "no")

	if err := t1.Commit(); err != nil {
		t.Fatalf("t1 commit: %v", err)
	}
	if err := t2.Commit(); !errors.Is(err, ErrRWConflict) {
		t.Fatalf("t2 は読み集合の検証で中止されるはず got %v", err)
	}
	// 当直は 1 人残る(規則が守られた)。
	t3 := s.Begin(Snapshot)
	n := 0
	for _, k := range []string{"oncall:alice", "oncall:bob"} {
		if v, _ := t3.Get(k); v == "yes" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("当直 want 1 got %d", n)
	}
}

func TestSerializableReadOnlyPasses(t *testing.T) {
	// 読んだキーが書き換えられていなければ、Serializable でも普通に通る。
	s := NewStore(map[string]string{"x": "v0", "y": "v0"})
	t1 := s.Begin(Serializable)
	_, _ = t1.Get("x")
	_ = t1.Put("y", "v1")

	// 無関係のキーへのコミットは邪魔しない。
	t2 := s.Begin(Snapshot)
	_ = t2.Put("z", "zzz")
	if err := t2.Commit(); err != nil {
		t.Fatalf("t2 commit: %v", err)
	}
	if err := t1.Commit(); err != nil {
		t.Fatalf("t1 は通るはず got %v", err)
	}
}

func TestDoneTxnRejects(t *testing.T) {
	s := NewStore(nil)
	t1 := s.Begin(Snapshot)
	t1.Abort()
	if err := t1.Put("x", "v"); !errors.Is(err, ErrTxnDone) {
		t.Fatalf("abort 後 Put want ErrTxnDone got %v", err)
	}
	if err := t1.Commit(); !errors.Is(err, ErrTxnDone) {
		t.Fatalf("abort 後 Commit want ErrTxnDone got %v", err)
	}
	t2 := s.Begin(Snapshot)
	if err := t2.Commit(); err != nil {
		t.Fatalf("空コミット: %v", err)
	}
	if err := t2.Commit(); !errors.Is(err, ErrTxnDone) {
		t.Fatalf("二重コミット want ErrTxnDone got %v", err)
	}
}

func TestCommitIsAtomic(t *testing.T) {
	// 複数キーの書き込みは同じ commitTS で積まれ、途中状態は見えない。
	s := NewStore(map[string]string{"a": "0", "b": "0"})
	t1 := s.Begin(Snapshot)
	_ = t1.Put("a", "1")
	_ = t1.Put("b", "1")
	if err := t1.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	va := s.Versions("a")
	vb := s.Versions("b")
	if va[len(va)-1].CommitTS != vb[len(vb)-1].CommitTS {
		t.Fatal("同一トランザクションの版は同じ commitTS のはず")
	}
}
