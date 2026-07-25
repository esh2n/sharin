// Package tracesampling は分散トレーシングのサンプリング戦略の最小実装。
//
// 全リクエストのトレースを保存することは(コスト的に)できない。
// では何を残すか。「開始時に決める」head-based と「完結を見てから決める」tail-based を
// 同じワークロードに適用して、保存量とエラー捕捉率のトレードオフを比較する。
package tracesampling

import (
	"errors"
	"time"
)

// Trace は完結した1トレースの要約。
// 実物は複数サービスにまたがる span の木だが、サンプリング判定に必要な情報だけに絞る。
type Trace struct {
	ID       int
	Duration time.Duration
	Err      bool
}

// Sampler はトレースを保存するか判定する。
type Sampler interface {
	Keep(t Trace) bool
}

// #region head
// HeadSampler は「トレース開始時」に記録するかを確率で決める。
// 開始時点では、そのリクエストがエラーになるか・遅くなるかはまだ存在しない情報なので、
// Keep はトレースの中身を一切見ない(見られない)。
// 実物では判定結果(sampled フラグ)がトレースコンテキストで下流サービスへ伝播され、
// 全サービスが同じ判断に従うことで「トレースの一部だけ欠ける」事態を防ぐ。
type HeadSampler struct {
	rate float64
	rng  func() float64
}

// NewHeadSampler は rate ∈ [0,1] の HeadSampler を返す。rng が nil なら実装依存の乱数を使う。
func NewHeadSampler(rate float64, rng func() float64) (*HeadSampler, error) {
	if rate < 0 || rate > 1 {
		return nil, errors.New("tracesampling: rate must be in [0, 1]")
	}
	if rng == nil {
		return nil, errors.New("tracesampling: rng must not be nil")
	}
	return &HeadSampler{rate: rate, rng: rng}, nil
}

// Keep はトレースの中身に関係なく、確率 rate で true を返す。
func (s *HeadSampler) Keep(_ Trace) bool {
	return s.rng() < s.rate
}

// #endregion head

// #region tail
// TailSampler は「トレース完結後」に中身を見てから決める。
// エラーと遅いトレースは必ず残し、普通のトレースはベースレートでだけ残す。
// この「中身を見る」ためには、判定できるようになるまで全 span をどこかに
// バッファしておく必要がある — それが tail-based の代償。
type TailSampler struct {
	slowThreshold time.Duration
	baseRate      float64
	rng           func() float64
}

// NewTailSampler は遅延閾値とベースレートを持つ TailSampler を返す。
func NewTailSampler(slowThreshold time.Duration, baseRate float64, rng func() float64) (*TailSampler, error) {
	if slowThreshold <= 0 {
		return nil, errors.New("tracesampling: slowThreshold must be positive")
	}
	if baseRate < 0 || baseRate > 1 {
		return nil, errors.New("tracesampling: baseRate must be in [0, 1]")
	}
	if rng == nil {
		return nil, errors.New("tracesampling: rng must not be nil")
	}
	return &TailSampler{slowThreshold: slowThreshold, baseRate: baseRate, rng: rng}, nil
}

// Keep はエラーまたは遅いトレースを必ず残し、それ以外は確率 baseRate で残す。
func (s *TailSampler) Keep(t Trace) bool {
	if t.Err || t.Duration >= s.slowThreshold {
		return true
	}
	return s.rng() < s.baseRate
}

// #endregion tail

// #region evaluate
// Summary はサンプリング戦略を1つのワークロードに適用した結果の集計。
type Summary struct {
	Total      int // 全トレース数
	Kept       int // 保存したトレース数
	Errors     int // 全エラートレース数
	ErrorsKept int // 保存できたエラートレース数
}

// Evaluate は全トレースに sampler を適用して集計する。
func Evaluate(traces []Trace, s Sampler) Summary {
	sum := Summary{Total: len(traces)}
	for _, t := range traces {
		if t.Err {
			sum.Errors++
		}
		if s.Keep(t) {
			sum.Kept++
			if t.Err {
				sum.ErrorsKept++
			}
		}
	}
	return sum
}

// KeepRatio は保存率(=ストレージコストの代理指標)を返す。
func (s Summary) KeepRatio() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Kept) / float64(s.Total)
}

// ErrorCaptureRate はエラートレースの捕捉率を返す。
func (s Summary) ErrorCaptureRate() float64 {
	if s.Errors == 0 {
		return 0
	}
	return float64(s.ErrorsKept) / float64(s.Errors)
}

// #endregion evaluate
