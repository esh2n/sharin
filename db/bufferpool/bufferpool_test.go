package bufferpool

import (
	"bytes"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T, capacity int) (*Pool, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	p, err := New(path, capacity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p, path
}

func pageOf(b byte) []byte {
	buf := make([]byte, PageSize)
	for i := range buf {
		buf[i] = b
	}
	return buf
}

func TestWriteRead(t *testing.T) {
	p, _ := openTemp(t, 4)
	if err := p.Write(0, pageOf(0xaa)); err != nil {
		t.Fatal(err)
	}
	got, err := p.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pageOf(0xaa)) {
		t.Error("書いた内容が読めるべき")
	}
}

func TestUnwrittenPageIsZero(t *testing.T) {
	p, _ := openTemp(t, 4)
	got, err := p.Read(7)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, make([]byte, PageSize)) {
		t.Error("未書き込みページはゼロで埋まっているべき")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	p, err := New(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Write(3, pageOf(0x11)); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil { // Close は FlushAll を含む
		t.Fatal(err)
	}

	p2, err := New(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	got, err := p2.Read(3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pageOf(0x11)) {
		t.Error("Close 後も内容が残るべき")
	}
}

// dirty ページは追い出される前にディスクへ書き戻される。
// これを怠ると「キャッシュから消えた = 変更が消えた」になってしまう。
func TestEvictionFlushesDirtyPage(t *testing.T) {
	p, _ := openTemp(t, 2)
	if err := p.Write(0, pageOf(0x01)); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(1, pageOf(0x02)); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(2, pageOf(0x03)); err != nil { // 容量2なので page0 が追い出される
		t.Fatal(err)
	}

	_, _, flushes := p.Stats()
	if flushes != 1 {
		t.Errorf("追い出しで dirty ページ1枚が書き戻されるべき: flushes = %d", flushes)
	}

	// 追い出された page0 を読み直す。ディスクから正しく戻ってくるはず。
	got, err := p.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pageOf(0x01)) {
		t.Error("追い出されたページの内容が失われている")
	}
}

func TestHitMissAccounting(t *testing.T) {
	p, _ := openTemp(t, 2)

	if _, err := p.Read(0); err != nil { // miss
		t.Fatal(err)
	}
	if _, err := p.Read(0); err != nil { // hit
		t.Fatal(err)
	}
	if _, err := p.Read(1); err != nil { // miss
		t.Fatal(err)
	}
	hits, misses, _ := p.Stats()
	if hits != 1 || misses != 2 {
		t.Errorf("(hits, misses) = (%d, %d), want (1, 2)", hits, misses)
	}
}

// この章の主役テスト: アクセスの局所性がヒット率を決める。
// 「同じページばかり触る」= B-Tree の右端挿入(昇順キー)の世界はほぼ全部ヒットし、
// 「毎回違うページ」= ランダムキーの世界は容量を超えた瞬間からミスだらけになる。
func TestLocalityDeterminesHitRate(t *testing.T) {
	t.Run("同じページを触り続けるとヒット率99%", func(t *testing.T) {
		p, _ := openTemp(t, 4)
		for i := 0; i < 100; i++ {
			if _, err := p.Read(0); err != nil {
				t.Fatal(err)
			}
		}
		hits, misses, _ := p.Stats()
		if hits != 99 || misses != 1 {
			t.Errorf("(hits, misses) = (%d, %d), want (99, 1)", hits, misses)
		}
	})

	t.Run("毎回違うページだとヒット率0%", func(t *testing.T) {
		p, _ := openTemp(t, 4)
		for i := 0; i < 100; i++ {
			if _, err := p.Read(i); err != nil {
				t.Fatal(err)
			}
		}
		hits, misses, _ := p.Stats()
		if hits != 0 || misses != 100 {
			t.Errorf("(hits, misses) = (%d, %d), want (0, 100)", hits, misses)
		}
	})
}

func TestValidation(t *testing.T) {
	p, _ := openTemp(t, 2)
	if _, err := p.Read(-1); err == nil {
		t.Error("負のページIDはエラーになるべき")
	}
	if err := p.Write(0, []byte{1, 2}); err == nil {
		t.Error("サイズ違いのページはエラーになるべき")
	}
	if _, err := New(filepath.Join(t.TempDir(), "x", "y.db"), 2); err == nil {
		t.Error("開けないパスはエラーになるべき")
	}
	if _, err := New(filepath.Join(t.TempDir(), "z.db"), 0); err == nil {
		t.Error("容量0はエラーになるべき")
	}
}

// FlushAll は dirty ページを書き戻し、ResetStats は数だけ 0 に戻す。
func TestFlushAllAndResetStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	p, err := New(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	data := make([]byte, PageSize)
	data[0] = 7
	if err := p.Write(0, data); err != nil {
		t.Fatal(err)
	}
	if err := p.FlushAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read(0); err != nil {
		t.Fatal(err)
	}

	if h, m, f := p.Stats(); h+m == 0 || f == 0 {
		t.Errorf("数えていない: %d, %d, %d", h, m, f)
	}
	p.ResetStats()
	if h, m, f := p.Stats(); h != 0 || m != 0 || f != 0 {
		t.Errorf("数え直せていない: %d, %d, %d", h, m, f)
	}
	// 中身は捨てていないので、そのまま読める。
	got, err := p.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 7 {
		t.Errorf("中身が消えた: %d", got[0])
	}
}
