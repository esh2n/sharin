package kvlog

import (
	"os"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.log")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func mustPut(t *testing.T, s *Store, k, v string) {
	t.Helper()
	if err := s.Put(k, v); err != nil {
		t.Fatalf("Put(%s, %s): %v", k, v, err)
	}
}

func mustGet(t *testing.T, s *Store, k, want string) {
	t.Helper()
	got, ok, err := s.Get(k)
	if err != nil || !ok || got != want {
		t.Fatalf("Get(%s) = (%q, %v, %v), want (%q, true, nil)", k, got, ok, err, want)
	}
}

func mustMiss(t *testing.T, s *Store, k string) {
	t.Helper()
	if _, ok, err := s.Get(k); err != nil || ok {
		t.Fatalf("Get(%s) はヒットしないべき (ok=%v, err=%v)", k, ok, err)
	}
}

func TestPutGet(t *testing.T) {
	s, _ := openTemp(t)
	mustPut(t, s, "a", "1")
	mustPut(t, s, "b", "2")
	mustGet(t, s, "a", "1")
	mustGet(t, s, "b", "2")
	mustMiss(t, s, "c")
}

func TestOverwriteLastWins(t *testing.T) {
	s, _ := openTemp(t)
	mustPut(t, s, "a", "old")
	mustPut(t, s, "a", "new")
	// 上書きしても古いレコードはログに残る。ただし index が最新を指す。
	mustGet(t, s, "a", "new")
}

func TestDelete(t *testing.T) {
	s, _ := openTemp(t)
	mustPut(t, s, "a", "1")
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	mustMiss(t, s, "a")
	// 消した後にまた書ける
	mustPut(t, s, "a", "2")
	mustGet(t, s, "a", "2")
}

func TestReplayAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.log")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mustPut(t, s, "a", "1")
	mustPut(t, s, "b", "2")
	mustPut(t, s, "a", "3") // 上書き
	if err := s.Delete("b"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// 再起動 = ログの再生。最新状態(a=3, bは削除済み)が復元される。
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	mustGet(t, s2, "a", "3")
	mustMiss(t, s2, "b")
}

func TestTruncatedTailIsDiscarded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.log")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mustPut(t, s, "a", "1")
	mustPut(t, s, "b", "2")
	s.Close()

	// クラッシュを再現: 最後のレコードの途中でファイルが切れたことにする。
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, info.Size()-3); err != nil {
		t.Fatal(err)
	}

	// 再生は壊れた末尾を捨て、その手前までを復元する。
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	mustGet(t, s2, "a", "1")
	mustMiss(t, s2, "b") // 途中で切れた b は「無かったこと」になる

	// 捨てた後も普通に書き続けられる。
	mustPut(t, s2, "c", "3")
	mustGet(t, s2, "c", "3")
}

func TestValidation(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Put("", "v"); err == nil {
		t.Error("空キーはエラーになるべき")
	}
	if err := s.Delete(""); err == nil {
		t.Error("空キーの削除はエラーになるべき")
	}
}
