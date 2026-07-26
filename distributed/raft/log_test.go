package raft

import "testing"

func TestLogAppendAndTerm(t *testing.T) {
	l := newLog()
	l.append(Entry{Term: 1, Index: 1}, Entry{Term: 1, Index: 2}, Entry{Term: 2, Index: 3})
	if l.lastIndex() != 3 {
		t.Fatalf("lastIndex=3 のはず, got %d", l.lastIndex())
	}
	if l.lastTerm() != 2 {
		t.Fatalf("lastTerm=2 のはず, got %d", l.lastTerm())
	}
	if tm, ok := l.term(2); !ok || tm != 1 {
		t.Fatalf("term(2)=1 のはず, got %d ok=%v", tm, ok)
	}
	if _, ok := l.term(99); ok {
		t.Fatal("範囲外は ok=false のはず")
	}
}

func TestLogMaybeAppendConflict(t *testing.T) {
	l := newLog()
	l.append(Entry{Term: 1, Index: 1}, Entry{Term: 1, Index: 2}, Entry{Term: 1, Index: 3})
	// prev=(1,任期1)一致。index2以降を任期2で上書きする
	last, ok := l.maybeAppend(1, 1, 0, []Entry{{Term: 2, Index: 2}, {Term: 2, Index: 3}})
	if !ok || last != 3 {
		t.Fatalf("上書き成功で last=3 のはず, got last=%d ok=%v", last, ok)
	}
	if tm, _ := l.term(3); tm != 2 {
		t.Fatalf("index3 は任期2 に書き換わるはず, got %d", tm)
	}
	// prev が一致しなければ拒否
	if _, ok := l.maybeAppend(2, 99, 0, nil); ok {
		t.Fatal("prevTerm 不一致は拒否のはず")
	}
}

func TestLogMaybeAppendIdempotent(t *testing.T) {
	l := newLog()
	l.append(Entry{Term: 1, Index: 1}, Entry{Term: 1, Index: 2})
	// 同じ内容を再送されても末尾を壊さない(冪等)
	last, ok := l.maybeAppend(0, 0, 0, []Entry{{Term: 1, Index: 1}, {Term: 1, Index: 2}})
	if !ok || last != 2 || l.lastIndex() != 2 {
		t.Fatalf("冪等に受理して末尾維持のはず, last=%d lastIndex=%d", last, l.lastIndex())
	}
}

func TestLogCommitBoundedByAppend(t *testing.T) {
	l := newLog()
	// commit ヒントが手持ちを超えても、持っている範囲までしか commit しない
	l.maybeAppend(0, 0, 100, []Entry{{Term: 1, Index: 1}, {Term: 1, Index: 2}})
	if l.committed != 2 {
		t.Fatalf("committed は末尾(2)で頭打ちのはず, got %d", l.committed)
	}
}

func TestLogSnapshotRestore(t *testing.T) {
	l := newLog()
	l.append(Entry{Term: 1, Index: 1}, Entry{Term: 1, Index: 2})
	s := Snapshot{LastIndex: 5, LastTerm: 3, Conf: []uint64{1, 2, 3}}
	l.restore(s)
	if l.firstIndex() != 6 || l.lastIndex() != 5 {
		t.Fatalf("restore 後 first=6 last=5 のはず, got first=%d last=%d", l.firstIndex(), l.lastIndex())
	}
	if tm, ok := l.term(5); !ok || tm != 3 {
		t.Fatalf("スナップ境界の term(5)=3 のはず, got %d ok=%v", tm, ok)
	}
	if _, ok := l.term(3); ok {
		t.Fatal("圧縮済み位置は ok=false のはず")
	}
}

func TestLogCompact(t *testing.T) {
	l := newLog()
	for i := uint64(1); i <= 6; i++ {
		l.append(Entry{Term: 1, Index: i})
	}
	l.applied = 4
	l.compact(4, 1, []uint64{1, 2, 3}, []byte("state"))
	if l.firstIndex() != 5 {
		t.Fatalf("compact 後 firstIndex=5 のはず, got %d", l.firstIndex())
	}
	if l.lastIndex() != 6 {
		t.Fatalf("compact 後も末尾は6 のはず, got %d", l.lastIndex())
	}
	if string(l.snapshot.Data) != "state" {
		t.Fatal("スナップショットに状態が入るはず")
	}
}
