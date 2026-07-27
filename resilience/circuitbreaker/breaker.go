// Package circuitbreaker はサーキットブレーカーを最小構成で実装する。
//
// 落ちている依存先を呼び続けると、タイムアウト待ちでスレッドが溜まり、
// 呼び出し側まで巻き添えで倒れる(連鎖障害)。サーキットブレーカーは、
// 失敗が続いたら回路を「開いて」以後の呼び出しを即座に失敗させ(fail fast)、
// 依存先に回復の猶予を与える。一定時間後に「半開」で 1 本だけ試し、
// 回復していれば「閉じる」、まだ駄目なら開き直す。家庭のブレーカーが
// 過電流で落ちて、直したら戻す、あの動きをソフトウェアに持ち込んだもの。
package circuitbreaker

import "errors"

// ErrOpen は回路が開いているため呼び出しを弾いたときに返る。
var ErrOpen = errors.New("circuitbreaker: open")

// State は回路の状態。
type State int

const (
	StateClosed   State = iota // 通常。呼び出しを通し、失敗を数える
	StateOpen                  // 遮断中。呼び出しを即失敗させる(fail fast)
	StateHalfOpen              // 試行中。1 本だけ通して回復を確かめる
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// #region config

// Config はブレーカーの閾値。ゼロ値は安全側のデフォルトに落とす。
type Config struct {
	FailureThreshold int // closed でこの回数連続失敗したら開く
	SuccessThreshold int // half-open でこの回数連続成功したら閉じる
	OpenTimeout      int // 開いてからこの論理時間で half-open を許す
}

// #endregion config

// Breaker は 1 つの依存先を守るサーキットブレーカー。
// 時刻は論理時計(Advance で進める)で、テストと決定性のため実時計を使わない。
type Breaker struct {
	cfg       Config
	state     State
	failures  int  // 連続失敗数(closed)
	successes int  // 連続成功数(half-open)
	openedAt  int  // 開いた論理時刻
	now       int  // 論理時計
	probing   bool // half-open の試行が進行中か
}

// New は Config からブレーカーを作る。閾値のゼロ値は 1 に補正する。
func New(cfg Config) *Breaker {
	if cfg.FailureThreshold < 1 {
		cfg.FailureThreshold = 1
	}
	if cfg.SuccessThreshold < 1 {
		cfg.SuccessThreshold = 1
	}
	if cfg.OpenTimeout < 1 {
		cfg.OpenTimeout = 1
	}
	return &Breaker{cfg: cfg, state: StateClosed}
}

// State は現在の状態を返す(タイムアウト経過の遷移も反映)。
func (b *Breaker) State() State {
	b.maybeHalfOpen()
	return b.state
}

// Advance は論理時計を d だけ進める。
func (b *Breaker) Advance(d int) { b.now += d }

// #region call

// Call は fn を回路の状態に応じて実行する。
//   - closed: 実行し、成否を数える。連続失敗が閾値に達したら開く
//   - open: 実行せず ErrOpen(fail fast)。ただしタイムアウト経過なら half-open へ
//   - half-open: 1 本だけ試す。成功を重ねれば閉じ、失敗したら即開き直す
func (b *Breaker) Call(fn func() error) error {
	b.maybeHalfOpen()

	switch b.state {
	case StateOpen:
		return ErrOpen

	case StateHalfOpen:
		if b.probing {
			// 既に試行中。同時の 2 本目は通さない。
			return ErrOpen
		}
		b.probing = true
		err := fn()
		b.probing = false
		if err != nil {
			b.trip() // 回復していない。開き直す
			return err
		}
		b.successes++
		if b.successes >= b.cfg.SuccessThreshold {
			b.close()
		}
		return nil

	default: // StateClosed
		err := fn()
		if err != nil {
			b.failures++
			if b.failures >= b.cfg.FailureThreshold {
				b.trip()
			}
			return err
		}
		b.failures = 0 // 成功で連続失敗をリセット
		return nil
	}
}

// #endregion call

// #region transitions

// maybeHalfOpen は open のままタイムアウトが過ぎていれば half-open へ移す。
func (b *Breaker) maybeHalfOpen() {
	if b.state == StateOpen && b.now-b.openedAt >= b.cfg.OpenTimeout {
		b.state = StateHalfOpen
		b.successes = 0
		b.probing = false
	}
}

// trip は回路を開く。closed からも half-open からも遷移する。
func (b *Breaker) trip() {
	b.state = StateOpen
	b.openedAt = b.now
	b.failures = 0
	b.successes = 0
	b.probing = false
}

// close は回路を閉じる(回復)。
func (b *Breaker) close() {
	b.state = StateClosed
	b.failures = 0
	b.successes = 0
	b.probing = false
}

// #endregion transitions
