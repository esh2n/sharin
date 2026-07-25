package idgen

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// #region snowflake
// Snowflake は「41bit ミリ秒 + 10bit ノードID + 12bit 連番」を int64 に詰める発番器。
// 乱数を使わず、衝突回避を「ノードIDの事前配布」と「同一ミリ秒内の連番」で行う。
// 64bit に収まるのが UUID 系(128bit)との最大の違い。
type Snowflake struct {
	mu     sync.Mutex
	nodeID int64
	epoch  time.Time
	lastMs int64
	seq    int64
	now    func() time.Time
}

// NewSnowflake は nodeID ∈ [0, 1023] の発番器を返す。now が nil なら time.Now を使う。
func NewSnowflake(nodeID int, epoch time.Time, now func() time.Time) (*Snowflake, error) {
	if nodeID < 0 || nodeID > 1023 {
		return nil, errors.New("idgen: nodeID must fit in 10 bits (0-1023)")
	}
	if now == nil {
		now = time.Now
	}
	return &Snowflake{nodeID: int64(nodeID), epoch: epoch, lastMs: -1, now: now}, nil
}

// Next は次のIDを返す。時計が巻き戻ったら、黙って重複の危険を冒すのではなくエラーにする。
func (s *Snowflake) Next() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ms := s.now().Sub(s.epoch).Milliseconds()
	if ms < 0 || ms >= 1<<41 {
		return 0, errors.New("idgen: timestamp out of 41-bit range")
	}
	if ms < s.lastMs {
		return 0, fmt.Errorf("idgen: clock moved backwards by %dms", s.lastMs-ms)
	}

	if ms == s.lastMs {
		s.seq++
		if s.seq >= 1<<12 {
			// 同一ミリ秒で4096個使い切った。実物は次のミリ秒まで busy wait する。
			return 0, errors.New("idgen: sequence exhausted in this millisecond")
		}
	} else {
		s.lastMs = ms
		s.seq = 0
	}

	return ms<<22 | s.nodeID<<12 | s.seq, nil
}

// #endregion snowflake
