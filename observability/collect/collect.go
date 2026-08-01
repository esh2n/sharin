// Package collect は、メトリクスを「どう集めるか」を最小構成で実装する。
//
// [メトリクスとヒストグラム](metrics)は 1 台のプロセスが値をどう持つかで、
// [時系列の整列と集約](timeseries)は届いた後どう潰すかだった。その間に、
// 値をプロセスから収集側へ運ぶ段がある。運び方は 2 通りしかない。
//
//   - pull: 収集側が対象を定期的に叩いて読む(Prometheus 系)
//   - push: 対象が収集側へ送りつける(StatsD / OTLP 系)
//
// どちらでも同じ数字が届くように見えるが、性質が 3 か所で割れる。
//
//   - 1 周の予算: pull は対象数 × 1 回のコストが間隔を超えると取りこぼす
//   - 落ちた対象: pull は叩いて失敗するので落ちたと分かる。push は「来ない」だけで、
//     落ちたのか、そもそも送っていないのか、経路が切れたのかを区別できない
//   - 系列数: ラベルの組み合わせで掛け算に増える。運ぶ量も持つ量もここで決まる
//
// そして収集側をどこに置くかで、1 つ落ちたときに失う範囲が変わる。
//
// 実時間も乱数も使わない。コストは単位時間の整数で数える。
package collect

import "sort"

// #region model

// Target は収集対象の 1 プロセス。
type Target struct {
	ID   string
	Node string // 載っているノード
	// Series はこの対象が公開している系列の数。
	Series int
	// Up が false なら、このプロセスは落ちている。
	Up bool
}

// Config は収集の設定。時間はすべて単位時間の整数で数える。
type Config struct {
	// Interval は収集の間隔。
	Interval int
	// ScrapeCost は 1 対象を 1 回叩くのにかかる時間。
	ScrapeCost int
	// Workers は同時に叩ける本数。1 なら直列。
	Workers int
}

// #endregion model

// #region pull

// PullResult は pull で 1 周したときの結果。
type PullResult struct {
	// RoundCost は 1 周にかかった時間。
	RoundCost int
	// Scraped は読めた対象の数。
	Scraped int
	// Series は読めた系列の合計。
	Series int
	// DownDetected は「落ちている」と判定できた対象の ID。
	DownDetected []string
	// Dropped は間隔に間に合わず、この周で読めなかった対象の数。
	Dropped int
}

// Pull は収集側が対象を順に叩く。
//
// 1 周のコストは「対象数 × 1 回のコスト ÷ 同時本数」で決まる。これが間隔を超えると、
// 超えたぶんの対象はこの周で読めない。**pull には 1 周の予算がある**。
func Pull(targets []Target, cfg Config) PullResult {
	w := cfg.Workers
	if w < 1 {
		w = 1
	}
	// 間隔の中で叩ける上限。
	capacity := cfg.Interval * w / cfg.ScrapeCost

	r := PullResult{}
	for i, t := range targets {
		if i >= capacity {
			r.Dropped++
			continue
		}
		if t.Up {
			r.Scraped++
			r.Series += t.Series
		} else {
			// 叩いて返らないので、落ちていると分かる。
			r.DownDetected = append(r.DownDetected, t.ID)
		}
	}
	scrapes := len(targets)
	if scrapes > capacity {
		scrapes = capacity
	}
	r.RoundCost = scrapes * cfg.ScrapeCost / w
	sort.Strings(r.DownDetected)
	return r
}

// Fits は、その対象数が 1 周の予算に収まるかを返す。
func Fits(n int, cfg Config) bool {
	w := cfg.Workers
	if w < 1 {
		w = 1
	}
	return n*cfg.ScrapeCost <= cfg.Interval*w
}

// MaxTargets は間隔に収まる対象数の上限を返す。
func MaxTargets(cfg Config) int {
	w := cfg.Workers
	if w < 1 {
		w = 1
	}
	return cfg.Interval * w / cfg.ScrapeCost
}

// #endregion pull

// #region push

// PushResult は push で 1 周ぶん受けたときの結果。
type PushResult struct {
	// Received は届いた対象の数。
	Received int
	// Series は届いた系列の合計。
	Series int
	// Silent は「何も来なかった」対象の数。
	Silent int
	// DownDetected は落ちていると判定できた対象の ID。
	//
	// push では常に空になる。届かないことからは、落ちたのか、送る設定が無いのか、
	// 経路が切れたのかを区別できない。
	DownDetected []string
}

// Push は対象が送りつけてくる。収集側は受けるだけで、叩きに行かない。
//
// 対象がいくつ居るかを収集側は知らないので、1 周の予算という概念が無い。
// そのかわり、来なかったものについて何も言えない。
func Push(targets []Target, _ Config) PushResult {
	r := PushResult{}
	for _, t := range targets {
		if t.Up {
			r.Received++
			r.Series += t.Series
		} else {
			r.Silent++
		}
	}
	return r
}

// #endregion push

// #region known

// KnownTargets は、収集側が「居るはずの対象」の一覧を持っているかどうかを表す。
//
// pull は叩きに行くために一覧が要る(サービスディスカバリ)。一覧があるから、
// 来ないことを異常と判定できる。push は一覧を持たずに始められるが、
// そのぶん「来ないこと」を異常と言えない。
type KnownTargets []string

// SilentButExpected は、一覧に居るのに届かなかった対象を返す。
//
// push でも一覧を別に持てば、落ちたことを言えるようになる。つまり pull と push の
// 差は「叩くかどうか」ではなく、**一覧を持っているかどうか**にある。
func SilentButExpected(known KnownTargets, received []string) []string {
	got := map[string]bool{}
	for _, id := range received {
		got[id] = true
	}
	var out []string
	for _, id := range known {
		if !got[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// #endregion known

// #region cardinality

// Label はラベル 1 つと、それがとる値の種類の数。
type Label struct {
	Name   string
	Values int
}

// Cardinality はラベルの組み合わせから系列数を返す。
//
// 足し算ではなく掛け算になる。ラベルを 1 つ足すと、その値の種類の数だけ倍になる。
func Cardinality(labels []Label) int {
	n := 1
	for _, l := range labels {
		n *= l.Values
	}
	return n
}

// Bytes は系列数と 1 系列あたりのバイト数から、保持に要る量を返す。
func Bytes(series, perSeries int) int { return series * perSeries }

// #endregion cardinality

// #region placement

// Placement は収集側をどこに置くか。
type Placement int

const (
	// Central は 1 か所に集める。対象は全部そこへ送る、または 1 つの収集器が全部を叩く。
	Central Placement = iota
	// Sidecar は対象ごとに 1 つ置く。
	Sidecar
	// PerNode はノードごとに 1 つ置く(DaemonSet)。
	PerNode
)

func (p Placement) String() string {
	switch p {
	case Sidecar:
		return "sidecar"
	case PerNode:
		return "per-node"
	default:
		return "central"
	}
}

// Layout は配置したときの姿。
type Layout struct {
	// Collectors は収集器の数。
	Collectors int
	// MaxSeriesPerCollector は 1 つの収集器が抱える系列の最大。
	MaxSeriesPerCollector int
	// LostOnOneFailure は収集器が 1 つ落ちたときに失う系列の最大。
	LostOnOneFailure int
}

// Place は対象の並びと置き方から、収集器の数と失う範囲を出す。
func Place(targets []Target, p Placement) Layout {
	total := 0
	byNode := map[string]int{}
	for _, t := range targets {
		total += t.Series
		byNode[t.Node] += t.Series
	}

	switch p {
	case Sidecar:
		max := 0
		for _, t := range targets {
			if t.Series > max {
				max = t.Series
			}
		}
		// 1 対象に 1 つ。落ちてもその対象ぶんしか失わない。
		return Layout{Collectors: len(targets), MaxSeriesPerCollector: max, LostOnOneFailure: max}
	case PerNode:
		max := 0
		for _, n := range byNode {
			if n > max {
				max = n
			}
		}
		// ノードに 1 つ。落ちるとそのノードの対象ぶんを失う。
		return Layout{Collectors: len(byNode), MaxSeriesPerCollector: max, LostOnOneFailure: max}
	default:
		// 1 か所。落ちると全部失う。
		return Layout{Collectors: 1, MaxSeriesPerCollector: total, LostOnOneFailure: total}
	}
}

// #endregion placement

// #region helpers

// NewTargets は n 個の対象を、ノードあたり perNode 個ずつ並べて作る。
// 系列数はすべて同じ。乱数は使わない。
func NewTargets(n, perNode, series int) []Target {
	ts := make([]Target, n)
	for i := range ts {
		ts[i] = Target{
			ID:     "t" + itoa(i),
			Node:   "n" + itoa(i/perNode),
			Series: series,
			Up:     true,
		}
	}
	return ts
}

// Down は i 番目を落とした並びを返す(元の並びは変えない)。
func Down(targets []Target, idx ...int) []Target {
	out := make([]Target, len(targets))
	copy(out, targets)
	for _, i := range idx {
		if i >= 0 && i < len(out) {
			out[i].Up = false
		}
	}
	return out
}

// IDs は生きている対象の ID を返す。
func IDs(targets []Target) []string {
	var out []string
	for _, t := range targets {
		if t.Up {
			out = append(out, t.ID)
		}
	}
	return out
}

// AllIDs は生死によらず全 ID を返す。
func AllIDs(targets []Target) KnownTargets {
	out := make(KnownTargets, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.ID)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// #endregion helpers
