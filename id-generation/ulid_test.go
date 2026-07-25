package idgen

import (
	"strings"
	"testing"
	"time"
)

func TestNewULID(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)

	id, err := NewULID(now, fixedRand(0x00))
	if err != nil {
		t.Fatal(err)
	}

	if len(id) != 26 {
		t.Fatalf("ULID は26文字のはず: %q (%d文字)", id, len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune(crockford, c) {
			t.Errorf("Crockford base32 以外の文字が混じっている: %c", c)
		}
	}

	// 乱数ゼロなら末尾16文字(80bit乱数部)はすべて '0'。
	if id[10:] != "0000000000000000" {
		t.Errorf("乱数部 = %s, want 全部0", id[10:])
	}

	// 同じミリ秒なら時刻部(先頭10文字)は同じ。
	id2, _ := NewULID(now, fixedRand(0xff))
	if id[:10] != id2[:10] {
		t.Errorf("同じ時刻なのに時刻部が違う: %s vs %s", id[:10], id2[:10])
	}
}

func TestULIDSortable(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	var prev string
	for i := 0; i < 5; i++ {
		id, err := NewULID(base.Add(time.Duration(i)*time.Millisecond), fixedRand(0xff))
		if err != nil {
			t.Fatal(err)
		}
		if prev != "" && !(prev < id) {
			t.Fatalf("時刻が進めば辞書順でも昇順になるべき: %s !< %s", prev, id)
		}
		prev = id
	}
}
