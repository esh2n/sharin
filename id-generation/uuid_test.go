package idgen

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// fixedRand は既知のバイト列を返す乱数源。
func fixedRand(b byte) *bytes.Reader {
	buf := make([]byte, 32)
	for i := range buf {
		buf[i] = b
	}
	return bytes.NewReader(buf)
}

func TestNewUUIDv4(t *testing.T) {
	id, err := NewUUIDv4(fixedRand(0xff))
	if err != nil {
		t.Fatal(err)
	}

	// 乱数が全部 0xff でも、version と variant のビットだけは規格通りに上書きされる。
	want := "ffffffff-ffff-4fff-bfff-ffffffffffff"
	if id != want {
		t.Errorf("NewUUIDv4 = %s, want %s", id, want)
	}

	if _, err := NewUUIDv4(bytes.NewReader(nil)); err == nil {
		t.Error("乱数が尽きたらエラーになるべき")
	}
}

func TestNewUUIDv7(t *testing.T) {
	now := time.UnixMilli(0x0123456789ab).UTC()

	id, err := NewUUIDv7(now, fixedRand(0x00))
	if err != nil {
		t.Fatal(err)
	}

	// 先頭48bitはミリ秒タイムスタンプ、version=7、variant=10。
	want := "01234567-89ab-7000-8000-000000000000"
	if id != want {
		t.Errorf("NewUUIDv7 = %s, want %s", id, want)
	}
}

func TestUUIDv7Sortable(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	var prev string
	for i := 0; i < 5; i++ {
		id, err := NewUUIDv7(base.Add(time.Duration(i)*time.Millisecond), fixedRand(0xff))
		if err != nil {
			t.Fatal(err)
		}
		if prev != "" && !(strings.Compare(prev, id) < 0) {
			t.Fatalf("時刻が進めば文字列比較でも昇順になるべき: %s !< %s", prev, id)
		}
		prev = id
	}
}
