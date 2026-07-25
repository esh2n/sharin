package idgen

import (
	"testing"
	"time"
)

func snowClock(start time.Time) (*time.Time, func() time.Time) {
	t := start
	return &t, func() time.Time { return t }
}

func TestSnowflake(t *testing.T) {
	epoch := time.UnixMilli(1_600_000_000_000)
	cur, now := snowClock(epoch.Add(1000 * time.Millisecond))

	s, err := NewSnowflake(42, epoch, now)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ビット構造: 41bit時刻 + 10bitノード + 12bit連番", func(t *testing.T) {
		id, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		if got := id >> 22; got != 1000 {
			t.Errorf("時刻部 = %d, want 1000", got)
		}
		if got := (id >> 12) & 0x3ff; got != 42 {
			t.Errorf("ノード部 = %d, want 42", got)
		}
		if got := id & 0xfff; got != 0 {
			t.Errorf("連番部 = %d, want 0", got)
		}
	})

	t.Run("同一ミリ秒内は連番で衝突を避ける", func(t *testing.T) {
		id1, _ := s.Next()
		id2, _ := s.Next()
		if id1 == id2 {
			t.Error("同一ミリ秒でも重複しないべき")
		}
		if (id2 & 0xfff) != (id1&0xfff)+1 {
			t.Errorf("連番が+1されるべき: %d → %d", id1&0xfff, id2&0xfff)
		}
	})

	t.Run("時刻が進めばIDも数値として増える", func(t *testing.T) {
		id1, _ := s.Next()
		*cur = cur.Add(5 * time.Millisecond)
		id2, _ := s.Next()
		if !(id1 < id2) {
			t.Errorf("昇順になるべき: %d !< %d", id1, id2)
		}
		if (id2 & 0xfff) != 0 {
			t.Error("ミリ秒が変われば連番は0に戻るべき")
		}
	})

	t.Run("時計の巻き戻りを検出してエラーにする", func(t *testing.T) {
		*cur = cur.Add(-10 * time.Millisecond)
		if _, err := s.Next(); err == nil {
			t.Error("時計が巻き戻ったらエラーになるべき(黙って重複IDを出してはいけない)")
		}
	})

	if _, err := NewSnowflake(1024, epoch, now); err == nil {
		t.Error("nodeID が10bitに収まらなければエラーになるべき")
	}
	if _, err := NewSnowflake(-1, epoch, now); err == nil {
		t.Error("nodeID が負ならエラーになるべき")
	}
}

func TestSnowflakeSequenceExhaustion(t *testing.T) {
	epoch := time.UnixMilli(1_600_000_000_000)
	_, now := snowClock(epoch.Add(time.Millisecond))
	s, _ := NewSnowflake(0, epoch, now)

	for i := 0; i < 4096; i++ {
		if _, err := s.Next(); err != nil {
			t.Fatalf("%d個目で失敗: %v", i, err)
		}
	}
	if _, err := s.Next(); err == nil {
		t.Error("同一ミリ秒4097個目はエラーになるべき(実物は次のミリ秒まで待つ)")
	}
}
