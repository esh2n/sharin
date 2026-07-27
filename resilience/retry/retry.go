// Package retry はリトライと指数バックオフ(ジッター付き)を最小構成で実装する。
//
// 一時的な失敗(ネットワークのゆらぎ、瞬間的な過負荷)は、少し待って再送すれば
// 通ることが多い。だが即座に再送すると、過負荷の相手にさらに負荷をかける。
// そこで待ち時間を試行ごとに指数的に伸ばす(指数バックオフ)。さらに、多数の
// クライアントが同時に失敗すると全員が同じ間隔で再送して衝突する(リトライ嵐)
// ため、待ち時間に乱数の揺らぎ(ジッター)を足して再送時刻を散らす。
// リトライしてよいのは一時的失敗だけで、400 番台のような恒久的失敗は再送しない。
package retry

import "errors"

// #region jitter

// JitterKind はバックオフに足す揺らぎの種類。
type JitterKind int

const (
	JitterNone  JitterKind = iota // 揺らぎなし(決定的、衝突しやすい)
	JitterFull                    // [0, raw] の一様乱数(最も散る)
	JitterEqual                   // raw/2 + [0, raw/2](下限を確保しつつ散らす)
)

// Rand は決定的な擬似乱数源(テスト再現性のため実 rand を使わない)。
type Rand struct{ state uint64 }

// NewRand は seed から擬似乱数源を作る。
func NewRand(seed uint64) *Rand { return &Rand{state: seed*2862933555777941757 + 1} }

// intn は [0, n) の擬似乱数を返す。
func (r *Rand) intn(n int) int {
	if n <= 0 {
		return 0
	}
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return int((r.state >> 33) % uint64(n))
}

// #endregion jitter

// #region policy

// Policy はリトライの方針。
type Policy struct {
	MaxAttempts int // 最大試行回数(0 は 1 に補正)
	BaseDelay   int // 初回の基本遅延(論理時間)
	MaxDelay    int // 遅延の上限
	Multiplier  int // 試行ごとの倍率(0/1 未満は 2 に補正)
	Jitter      JitterKind
}

// rawDelay は attempt 回目の素の遅延 base·mult^attempt(max で頭打ち)。
func (p Policy) rawDelay(attempt int) int {
	base := p.BaseDelay
	mult := p.Multiplier
	if mult < 2 {
		mult = 2
	}
	d := base
	for i := 0; i < attempt; i++ {
		d *= mult
		if p.MaxDelay > 0 && d >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		return p.MaxDelay
	}
	return d
}

// Backoff は attempt 回目の待ち時間を、ジッター種別に応じて返す。
// r はジッター用の乱数源(JitterNone なら nil でよい)。
func (p Policy) Backoff(attempt int, r *Rand) int {
	raw := p.rawDelay(attempt)
	switch p.Jitter {
	case JitterFull:
		if r == nil || raw <= 0 {
			return raw
		}
		return r.intn(raw + 1) // [0, raw]
	case JitterEqual:
		if r == nil || raw <= 0 {
			return raw
		}
		half := raw / 2
		return half + r.intn(raw-half+1) // [half, raw]
	default:
		return raw
	}
}

// #endregion policy

// #region do

// permanentError は「再試行しても無駄」なエラーをラップする。
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent はエラーを恒久的(再試行しない)としてマークする。
// 400 Bad Request のような、待っても直らない失敗に使う。
func Permanent(err error) error { return &permanentError{err: err} }

// Result はリトライの結果。
type Result struct {
	Err        error // 最終エラー(成功なら nil)
	Attempts   int   // 実際に試した回数
	TotalDelay int   // 試行の間に待った論理時間の合計
}

// Do は fn を Policy に従ってリトライする。成功するか、最大試行に達するか、
// 恒久的エラーが返るまで。試行の「間」だけ待ち、最後の失敗後は待たない。
func (p Policy) Do(fn func() error, r *Rand) Result {
	max := p.MaxAttempts
	if max < 1 {
		max = 1
	}
	var res Result
	for attempt := 0; attempt < max; attempt++ {
		res.Attempts++
		err := fn()
		if err == nil {
			res.Err = nil
			return res
		}
		var perm *permanentError
		if errors.As(err, &perm) {
			res.Err = err // 恒久的失敗は即諦める
			return res
		}
		res.Err = err
		if attempt < max-1 {
			// 次の試行の前に待つ(最後の試行の後は待たない)。
			res.TotalDelay += p.Backoff(attempt, r)
		}
	}
	return res
}

// #endregion do
