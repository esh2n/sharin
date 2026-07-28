// Package timeseries は時系列メトリクスの整列(alignment)と集約(reduction)を
// 最小構成で実装する。
//
// [メトリクスとヒストグラム](metrics)の章では、1台のプロセスが値をどう持つかを見た。
// だが実際に画面に出るまでには、もう2段ある。同じメトリクスは Pod やインスタンスの
// 数だけ別々の系列として届き、それぞれが不揃いな時刻に点を打っている。1本の線を
// 描くには、まず時間方向に揃え、次に系列方向にまとめなければならない。
//
// この2段はどちらも「たくさんの数を1つにする」操作なので、混同されやすい。
// だが順番が決まっていて、順番が結果を変える。ここを取り違えると、グラフは
// 何の嘘もついていないのに、読み手は間違った結論に着く。
//
// この章が再現するのは、次の誤読になる。
//
//   - 累計の値をそのまま描いて、増える一方の線を見て安心する
//   - 各系列の p99 を平均して、全体の p99 だと思う
//   - 平均で見て、1台だけ刺さっているのを見落とす
//   - 窓を広げて、短いスパイクを消してしまう
//
// どれも「数え方が間違っている」のではなく、「まとめ方の順序が違う」ことから来る。
package timeseries

import (
	"sort"

	"github.com/esh2n/sharin/observability/metrics"
)

// #region model

// Kind は値が何を表すかの区別。これが整列の意味を決める。
type Kind int

const (
	// Gauge はその瞬間の値。CPU 使用率、キューの長さ。そのまま読める。
	Gauge Kind = iota
	// Delta は前回の点からの増分。区間の合計に意味がある。
	Delta
	// Cumulative は計測開始からの累計。差を取らなければ読めない。
	Cumulative
)

func (k Kind) String() string {
	switch k {
	case Gauge:
		return "GAUGE"
	case Delta:
		return "DELTA"
	case Cumulative:
		return "CUMULATIVE"
	}
	return "UNKNOWN"
}

// Point は1つの観測。分布値のときは D にヒストグラムが入り、V は使わない。
type Point struct {
	T int
	V float64
	D *metrics.Histogram
}

// Series は1本の時系列。Labels がこの系列を他と区別する。
//
// 同じメトリクスでも、Pod やゾーンの数だけ系列が存在する。ラベルの組が
// 違えば別の系列で、集約とは「どのラベルを残すか」を決めることになる。
type Series struct {
	Labels map[string]string
	Kind   Kind
	Points []Point
}

// Distribution はこの系列が分布値を持つかを返す。
func (s Series) Distribution() bool {
	return len(s.Points) > 0 && s.Points[0].D != nil
}

// Label はラベルの値を返す(無ければ空文字)。
func (s Series) Label(k string) string { return s.Labels[k] }

// #endregion model

// #region aligner

// Aligner は1本の系列の中で、不揃いな点を等間隔の窓に揃える方法。
type Aligner int

const (
	AlignNone  Aligner = iota
	AlignMean          // 窓の中の平均
	AlignMax           // 窓の中の最大
	AlignMin           // 窓の中の最小
	AlignSum           // 窓の中の合計
	AlignDelta         // 窓ぶんの増分。累計を読める形にする
	AlignRate          // 増分を時間で割る。秒あたりに直す
	AlignP50           // 窓の中の中央値
	AlignP99           // 窓の中の 99 パーセンタイル
)

func (a Aligner) String() string {
	return [...]string{"ALIGN_NONE", "ALIGN_MEAN", "ALIGN_MAX", "ALIGN_MIN",
		"ALIGN_SUM", "ALIGN_DELTA", "ALIGN_RATE", "ALIGN_PERCENTILE_50",
		"ALIGN_PERCENTILE_99"}[a]
}

// Align は period ごとの窓に点をまとめる。出力の時刻は窓の終わりになる。
//
// 分布値を持つ系列に AlignDelta を使うと、窓の中のヒストグラムを足し合わせた
// 分布が出る。値が分布のまま残るのが大事なところで、ここで分位点にしてしまうと
// 後の集約で正しく足せなくなる。
func Align(s Series, a Aligner, period int) Series {
	out := Series{Labels: copyLabels(s.Labels), Kind: s.Kind}
	if a == AlignNone || period <= 0 {
		out.Points = append(out.Points, s.Points...)
		return out
	}

	if s.Distribution() {
		// 分布値の系列。窓の中のヒストグラムを足し合わせる。
		// 分位点や平均を指定したときだけ数値に潰れ、それ以外は分布のまま残る。
		for _, w := range windows(s, period) {
			h := mergeHists(w.points)
			switch a {
			case AlignP50:
				out.Points = append(out.Points, Point{T: w.end, V: h.Quantile(0.5)})
			case AlignP99:
				out.Points = append(out.Points, Point{T: w.end, V: h.Quantile(0.99)})
			case AlignMean:
				out.Points = append(out.Points, Point{T: w.end, V: h.Mean()})
			default:
				out.Points = append(out.Points, Point{T: w.end, D: h})
			}
		}
		if collapses(a) {
			out.Kind = Gauge
		}
		return out
	}

	if a == AlignDelta || a == AlignRate {
		return alignChange(s, a, period)
	}

	for _, w := range windows(s, period) {
		out.Points = append(out.Points, Point{T: w.end, V: reduceValues(values(w.points), a)})
	}
	if collapses(a) {
		out.Kind = Gauge // 分位点や平均は、もう増分でも累計でもない
	}
	return out
}

// collapses は、その整列が「1つの数」に潰す種類かを返す。
func collapses(a Aligner) bool { return a == AlignP50 || a == AlignP99 || a == AlignMean }

// alignChange は累計や増分を、窓ぶんの変化量に直す。
//
// 累計は差を取る。ここで前の窓より小さくなっていたら、プロセスが再起動して
// 0 から数え直したということなので、現在値そのものを増分とみなす。この検出を
// 忘れると、再起動のたびに大きな負の値が出る。
func alignChange(s Series, a Aligner, period int) Series {
	out := Series{Labels: copyLabels(s.Labels), Kind: Gauge}
	prev := 0.0
	first := true
	for _, w := range windows(s, period) {
		var v float64
		if s.Kind == Cumulative {
			last := w.points[len(w.points)-1].V
			switch {
			case first:
				v = 0 // 起点が無いので、最初の窓では変化量を出せない
			case last < prev:
				v = last // 数え直しが起きた
			default:
				v = last - prev
			}
			prev, first = last, false
		} else {
			for _, p := range w.points {
				v += p.V
			}
		}
		if a == AlignRate {
			v /= float64(period)
		}
		out.Points = append(out.Points, Point{T: w.end, V: v})
	}
	return out
}

type window struct {
	end    int
	points []Point
}

// windows は点を period ごとの窓に振り分ける。空の窓は作らない。
func windows(s Series, period int) []window {
	byIdx := map[int][]Point{}
	var idxs []int
	for _, p := range s.Points {
		i := p.T / period
		if _, ok := byIdx[i]; !ok {
			idxs = append(idxs, i)
		}
		byIdx[i] = append(byIdx[i], p)
	}
	sort.Ints(idxs)
	out := make([]window, 0, len(idxs))
	for _, i := range idxs {
		out = append(out, window{end: (i + 1) * period, points: byIdx[i]})
	}
	return out
}

func values(ps []Point) []float64 {
	out := make([]float64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.V)
	}
	return out
}

// reduceValues は数値の並びを1つにまとめる。整列にも集約にも同じ計算を使う。
func reduceValues(vs []float64, a Aligner) float64 {
	if len(vs) == 0 {
		return 0
	}
	switch a {
	case AlignMax:
		m := vs[0]
		for _, v := range vs {
			if v > m {
				m = v
			}
		}
		return m
	case AlignMin:
		m := vs[0]
		for _, v := range vs {
			if v < m {
				m = v
			}
		}
		return m
	case AlignSum:
		s := 0.0
		for _, v := range vs {
			s += v
		}
		return s
	case AlignP50:
		return nearestRank(vs, 0.5)
	case AlignP99:
		return nearestRank(vs, 0.99)
	default: // AlignMean
		s := 0.0
		for _, v := range vs {
			s += v
		}
		return s / float64(len(vs))
	}
}

// nearestRank は並べ替えて下から q の位置の値を返す。
func nearestRank(vs []float64, q float64) float64 {
	sorted := append([]float64(nil), vs...)
	sort.Float64s(sorted)
	i := int(q*float64(len(sorted))+0.999999) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// #endregion aligner

// #region reducer

// Reducer は同じ時刻の複数の系列を1つにまとめる方法。
type Reducer int

const (
	ReduceNone Reducer = iota
	ReduceMean
	ReduceSum
	ReduceMax
	ReduceMin
	ReduceP50
	ReduceP99
)

func (r Reducer) String() string {
	return [...]string{"REDUCE_NONE", "REDUCE_MEAN", "REDUCE_SUM", "REDUCE_MAX",
		"REDUCE_MIN", "REDUCE_PERCENTILE_50", "REDUCE_PERCENTILE_99"}[r]
}

// Reduce は groupBy に挙げたラベルだけを残して、残りが同じ系列を1本にまとめる。
//
// 分布値のままの系列に ReduceP99 を使うと、まずヒストグラムを足し合わせ、
// それから分位点を取る。これが正しい順序になる。すでに分位点になってしまった
// 数値を平均しても、全体の分位点にはならない。
func Reduce(all []Series, r Reducer, groupBy ...string) []Series {
	if r == ReduceNone {
		return append([]Series(nil), all...)
	}

	groups := map[string][]Series{}
	var order []string
	for _, s := range all {
		k := groupKey(s, groupBy)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], s)
	}
	sort.Strings(order)

	out := make([]Series, 0, len(order))
	for _, k := range order {
		out = append(out, reduceGroup(groups[k], r, groupBy))
	}
	return out
}

func reduceGroup(group []Series, r Reducer, groupBy []string) Series {
	res := Series{Labels: map[string]string{}, Kind: group[0].Kind}
	for _, k := range groupBy {
		res.Labels[k] = group[0].Labels[k]
	}

	byTime := map[int][]Point{}
	var times []int
	for _, s := range group {
		for _, p := range s.Points {
			if _, ok := byTime[p.T]; !ok {
				times = append(times, p.T)
			}
			byTime[p.T] = append(byTime[p.T], p)
		}
	}
	sort.Ints(times)

	for _, t := range times {
		ps := byTime[t]
		if ps[0].D != nil {
			h := mergeHists(ps)
			switch r {
			case ReduceP50:
				res.Points = append(res.Points, Point{T: t, V: h.Quantile(0.5)})
			case ReduceP99:
				res.Points = append(res.Points, Point{T: t, V: h.Quantile(0.99)})
			case ReduceMean:
				res.Points = append(res.Points, Point{T: t, V: h.Mean()})
			default:
				res.Points = append(res.Points, Point{T: t, D: h})
			}
			continue
		}
		res.Points = append(res.Points, Point{T: t, V: reduceValues(values(ps), asAligner(r))})
	}
	if r == ReduceP50 || r == ReduceP99 || r == ReduceMean {
		res.Kind = Gauge
	}
	return res
}

// asAligner は同じ計算を指す整列側の名前に読み替える。
// 時間方向か系列方向かが違うだけで、数の潰し方は同じになる。
func asAligner(r Reducer) Aligner {
	switch r {
	case ReduceSum:
		return AlignSum
	case ReduceMax:
		return AlignMax
	case ReduceMin:
		return AlignMin
	case ReduceP50:
		return AlignP50
	case ReduceP99:
		return AlignP99
	default:
		return AlignMean
	}
}

func groupKey(s Series, groupBy []string) string {
	k := ""
	for _, g := range groupBy {
		k += g + "=" + s.Labels[g] + ";"
	}
	return k
}

// #endregion reducer

// #region helpers

// mergeHists は同じ境界を持つヒストグラムを足し合わせた新しいものを返す。
// 元は変えない。足せることがヒストグラムの取り柄で、この足し算があるから
// 「まとめてから分位点」が成り立つ。
func mergeHists(ps []Point) *metrics.Histogram {
	var out *metrics.Histogram
	for _, p := range ps {
		if p.D == nil {
			continue
		}
		if out == nil {
			bounds, _ := p.D.Buckets()
			out = metrics.NewHistogram(append([]float64(nil), bounds...))
		}
		out.Merge(p.D)
	}
	if out == nil {
		return metrics.NewHistogram(nil)
	}
	return out
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// #endregion helpers
