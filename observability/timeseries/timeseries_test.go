package timeseries

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/observability/metrics"
)

var bounds = []float64{5, 10, 25, 50, 100, 250, 500, 1000}

func hist(samples ...float64) *metrics.Histogram {
	h := metrics.NewHistogram(bounds)
	for _, s := range samples {
		h.Observe(s)
	}
	return h
}

func gauge(labels map[string]string, vals ...float64) Series {
	s := Series{Labels: labels, Kind: Gauge}
	for i, v := range vals {
		s.Points = append(s.Points, Point{T: i * 10, V: v})
	}
	return s
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func vals(s Series) []float64 { return values(s.Points) }

func eqf(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !near(a[i], b[i], 1e-9) {
			return false
		}
	}
	return true
}

// 累計はそのままでは読めない。差を取ってはじめて「どれだけ増えたか」が出る。
func TestCumulativeNeedsDelta(t *testing.T) {
	s := Series{Kind: Cumulative, Points: []Point{
		{T: 0, V: 100}, {T: 30, V: 130}, {T: 60, V: 190}, {T: 90, V: 220},
	}}

	raw := Align(s, AlignNone, 0)
	if got := vals(raw)[3]; got != 220 {
		t.Fatalf("生の値は累計のまま返るはず: %v", got)
	}

	d := Align(s, AlignDelta, 30)
	// 最初の窓は起点が無いので 0。以降は 30, 60, 30 増えている。
	if !eqf(vals(d), []float64{0, 30, 60, 30}) {
		t.Fatalf("増分が違う: %v", vals(d))
	}
	if d.Kind != Gauge {
		t.Fatalf("増分にした後も累計のままになっている: %v", d.Kind)
	}
}

// 秒あたりに直すには、増分を窓の長さで割る。
func TestRateDividesByPeriod(t *testing.T) {
	s := Series{Kind: Cumulative, Points: []Point{
		{T: 0, V: 0}, {T: 60, V: 600}, {T: 120, V: 1800},
	}}
	r := Align(s, AlignRate, 60)
	if !eqf(vals(r), []float64{0, 10, 20}) {
		t.Fatalf("秒あたりの値が違う: %v", vals(r))
	}
}

// 数え直しを検出しないと、再起動のたびに大きな負の値が出る。
func TestCounterResetIsDetected(t *testing.T) {
	s := Series{Kind: Cumulative, Points: []Point{
		{T: 0, V: 1000}, {T: 30, V: 1200}, {T: 60, V: 50}, {T: 90, V: 130},
	}}
	d := Align(s, AlignDelta, 30)
	for _, v := range vals(d) {
		if v < 0 {
			t.Fatalf("負の増分が出た(数え直しを見落としている): %v", vals(d))
		}
	}
	if !eqf(vals(d), []float64{0, 200, 50, 80}) {
		t.Fatalf("数え直し後の増分が違う: %v", vals(d))
	}
}

// Delta は区間の増分なので、窓の中を足す。
func TestDeltaKindSums(t *testing.T) {
	s := Series{Kind: Delta, Points: []Point{
		{T: 0, V: 3}, {T: 10, V: 4}, {T: 30, V: 5}, {T: 40, V: 6},
	}}
	if got := vals(Align(s, AlignDelta, 30)); !eqf(got, []float64{7, 11}) {
		t.Fatalf("増分の合計が違う: %v", got)
	}
	if got := vals(Align(s, AlignRate, 30)); !eqf(got, []float64{7.0 / 30, 11.0 / 30}) {
		t.Fatalf("秒あたりが違う: %v", got)
	}
}

// この章の中心その1。分位点は平均できない。
//
// 3台のうち1台だけが遅いとき、各台の p99 を平均すると、遅い台の値が
// 速い2台に薄められる。分布のまま足してから p99 を取れば、全体の p99 が出る。
func TestAveragingPercentilesIsWrong(t *testing.T) {
	fast1 := Series{Labels: map[string]string{"pod": "a"}, Kind: Delta,
		Points: []Point{{T: 0, D: hist(repeat(4, 99)...)}}}
	fast2 := Series{Labels: map[string]string{"pod": "b"}, Kind: Delta,
		Points: []Point{{T: 0, D: hist(repeat(4, 99)...)}}}
	slow := Series{Labels: map[string]string{"pod": "c"}, Kind: Delta,
		Points: []Point{{T: 0, D: hist(append(repeat(4, 90), repeat(900, 9)...)...)}}}
	all := []Series{fast1, fast2, slow}

	// 間違ったやり方: 先に各系列を分位点にして、それを平均する。
	var perSeries []Series
	for _, s := range all {
		perSeries = append(perSeries, Align(s, AlignP99, 60))
	}
	wrong := Reduce(perSeries, ReduceMean)[0].Points[0].V

	// 正しいやり方: 分布のまま足してから分位点を取る。
	var kept []Series
	for _, s := range all {
		kept = append(kept, Align(s, AlignDelta, 60))
	}
	right := Reduce(kept, ReduceP99)[0].Points[0].V

	if !(right > wrong) {
		t.Fatalf("分布のまま足したほうが低く出た: 正 %v 誤 %v", right, wrong)
	}
	if wrong > 400 {
		t.Fatalf("平均が遅い台に引きずられていない設定になっている: %v", wrong)
	}
	if right < 400 {
		t.Fatalf("全体の p99 が遅い側を捉えていない: %v", right)
	}
}

// 分布のまま集約すれば、点の数の重みも正しく効く。
func TestMergingDistributionsKeepsWeights(t *testing.T) {
	big := Series{Labels: map[string]string{"pod": "big"}, Kind: Delta,
		Points: []Point{{T: 60, D: hist(repeat(900, 100)...)}}}
	small := Series{Labels: map[string]string{"pod": "small"}, Kind: Delta,
		Points: []Point{{T: 60, D: hist(repeat(4, 1)...)}}}

	merged := Reduce([]Series{big, small}, ReduceP50)[0].Points[0].V
	if merged < 500 {
		t.Fatalf("件数の多い側に寄っていない: %v", merged)
	}
	// 系列の数だけで平均すると、1件しかない側が半分の重みを持ってしまう。
	naive := (900.0 + 4.0) / 2
	if near(merged, naive, 1.0) {
		t.Fatalf("系列を等しく扱ってしまっている: %v", merged)
	}
}

// この章の中心その2。平均は刺さっている1本を隠す。
func TestMeanHidesTheHotOne(t *testing.T) {
	var all []Series
	for i := 0; i < 9; i++ {
		all = append(all, gauge(map[string]string{"pod": "p" + string(rune('a'+i))}, 10, 10, 10))
	}
	all = append(all, gauge(map[string]string{"pod": "hot"}, 98, 99, 97))

	mean := Reduce(all, ReduceMean)[0]
	max := Reduce(all, ReduceMax)[0]

	if mean.Points[1].V > 30 {
		t.Fatalf("平均が刺さりを映してしまっている: %v", mean.Points[1].V)
	}
	if max.Points[1].V < 90 {
		t.Fatalf("最大が刺さりを捉えていない: %v", max.Points[1].V)
	}
	// 閾値 65% で見ていると、平均では一度も鳴らない。
	for _, p := range mean.Points {
		if p.V >= 65 {
			t.Fatal("平均で閾値を超えてしまった。設定が対照になっていない")
		}
	}
	hit := false
	for _, p := range max.Points {
		if p.V >= 65 {
			hit = true
		}
	}
	if !hit {
		t.Fatal("最大でも閾値を超えない")
	}
}

// この章の中心その3。窓を広げるとスパイクが薄まって消える。
func TestWideWindowHidesSpikes(t *testing.T) {
	s := Series{Kind: Gauge}
	for i := 0; i < 60; i++ {
		v := 10.0
		if i == 30 {
			v = 610 // 10 秒だけ跳ねる
		}
		s.Points = append(s.Points, Point{T: i * 10, V: v})
	}

	narrow := Align(s, AlignMean, 10)  // 10 秒窓
	wide := Align(s, AlignMean, 600)   // 600 秒窓
	wideMax := Align(s, AlignMax, 600) // 同じ窓でも最大なら残る

	if peak(narrow) < 600 {
		t.Fatalf("細かい窓でスパイクが消えた: %v", peak(narrow))
	}
	if peak(wide) > 100 {
		t.Fatalf("広い窓でスパイクが残ってしまった: %v", peak(wide))
	}
	if peak(wideMax) < 600 {
		t.Fatalf("最大で取ればスパイクは残るはず: %v", peak(wideMax))
	}
}

// 集約は「どのラベルを残すか」を決めること。残したラベルの数だけ線が出る。
func TestGroupByKeepsOnlyNamedLabels(t *testing.T) {
	all := []Series{
		gauge(map[string]string{"zone": "a", "pod": "1"}, 10),
		gauge(map[string]string{"zone": "a", "pod": "2"}, 30),
		gauge(map[string]string{"zone": "b", "pod": "3"}, 50),
	}

	byZone := Reduce(all, ReduceMean, "zone")
	if len(byZone) != 2 {
		t.Fatalf("ゾーンの数だけ線が出るはず: %d", len(byZone))
	}
	if byZone[0].Labels["zone"] != "a" || byZone[0].Points[0].V != 20 {
		t.Fatalf("ゾーン a のまとめ方が違う: %+v", byZone[0])
	}
	if _, ok := byZone[0].Labels["pod"]; ok {
		t.Fatal("残していないラベルが残っている")
	}

	whole := Reduce(all, ReduceSum)
	if len(whole) != 1 || whole[0].Points[0].V != 90 {
		t.Fatalf("すべてを1本にまとめられていない: %+v", whole)
	}
}

// 系列ごとに時刻がずれていても、同じ時刻の点どうしでまとまる。
func TestReduceAlignsByTimestamp(t *testing.T) {
	a := Series{Labels: map[string]string{"pod": "a"}, Points: []Point{{T: 10, V: 2}, {T: 20, V: 4}}}
	b := Series{Labels: map[string]string{"pod": "b"}, Points: []Point{{T: 20, V: 6}, {T: 30, V: 8}}}

	got := Reduce([]Series{a, b}, ReduceSum)[0]
	if !eqf(vals(got), []float64{2, 10, 8}) {
		t.Fatalf("時刻ごとの合算が違う: %v", vals(got))
	}
}

// 整列を挟まずに集約すると、点が打たれた時刻の偶然が結果に出る。
// これが「整列が先」である理由になる。
func TestAlignFirstRemovesSamplingLuck(t *testing.T) {
	// a は 10 秒ごと、b は 30 秒ごとに点を打つ。値はどちらもずっと 10 と 20。
	a := Series{Labels: map[string]string{"pod": "a"}}
	for i := 0; i < 6; i++ {
		a.Points = append(a.Points, Point{T: i * 10, V: 10})
	}
	b := Series{Labels: map[string]string{"pod": "b"}}
	for i := 0; i < 2; i++ {
		b.Points = append(b.Points, Point{T: i * 30, V: 20})
	}

	// 整列せずに合算すると、点が揃っている時刻だけ 30 になり、他は片方だけになる。
	raw := Reduce([]Series{a, b}, ReduceMean)[0]
	if len(raw.Points) != 6 {
		t.Fatalf("生のまま集約すると時刻がばらつくはず: %d", len(raw.Points))
	}
	if raw.Points[0].V == raw.Points[1].V {
		t.Fatal("点の有無で値が揺れていない。対照になっていない")
	}

	// 先に 60 秒窓へ揃えてから集約すれば、両方が同じ時刻に1点ずつになる。
	aligned := []Series{Align(a, AlignMean, 60), Align(b, AlignMean, 60)}
	fixed := Reduce(aligned, ReduceMean)[0]
	if len(fixed.Points) != 1 || fixed.Points[0].V != 15 {
		t.Fatalf("整列してから集約した結果が違う: %+v", fixed.Points)
	}
}

// 表示のための細かい確認。
func TestAccessorsAndNames(t *testing.T) {
	s := gauge(map[string]string{"pod": "a"}, 1, 2)
	if s.Label("pod") != "a" || s.Label("none") != "" {
		t.Fatal("ラベルが引けない")
	}
	if s.Distribution() {
		t.Fatal("数値の系列が分布と判定された")
	}
	d := Series{Points: []Point{{T: 0, D: hist(1)}}}
	if !d.Distribution() {
		t.Fatal("分布の系列が数値と判定された")
	}
	if (Series{}).Distribution() {
		t.Fatal("空の系列が分布と判定された")
	}

	if Gauge.String() != "GAUGE" || Delta.String() != "DELTA" ||
		Cumulative.String() != "CUMULATIVE" || Kind(9).String() != "UNKNOWN" {
		t.Fatal("種類の名前が違う")
	}
	if AlignP99.String() != "ALIGN_PERCENTILE_99" || AlignRate.String() != "ALIGN_RATE" {
		t.Fatal("整列の名前が違う")
	}
	if ReduceP99.String() != "REDUCE_PERCENTILE_99" || ReduceMax.String() != "REDUCE_MAX" {
		t.Fatal("集約の名前が違う")
	}
}

// 何もしない指定は、元のまま返す。
func TestNoneIsIdentity(t *testing.T) {
	s := gauge(nil, 1, 2, 3)
	if !eqf(vals(Align(s, AlignNone, 10)), []float64{1, 2, 3}) {
		t.Fatal("AlignNone が値を変えた")
	}
	if !eqf(vals(Align(s, AlignMean, 0)), []float64{1, 2, 3}) {
		t.Fatal("窓の長さ 0 で値が変わった")
	}
	if got := Reduce([]Series{s}, ReduceNone); len(got) != 1 || !eqf(vals(got[0]), []float64{1, 2, 3}) {
		t.Fatal("ReduceNone が値を変えた")
	}
}

// 整列と集約の残りの計算。
func TestRemainingAggregations(t *testing.T) {
	s := gauge(nil, 3, 1, 9, 5)
	cases := []struct {
		a    Aligner
		want float64
	}{
		{AlignMin, 1}, {AlignMax, 9}, {AlignSum, 18}, {AlignMean, 4.5}, {AlignP50, 3}, {AlignP99, 9},
	}
	for _, c := range cases {
		if got := Align(s, c.a, 100).Points[0].V; !near(got, c.want, 1e-9) {
			t.Errorf("%v = %v, want %v", c.a, got, c.want)
		}
	}

	all := []Series{gauge(nil, 3), gauge(nil, 1), gauge(nil, 9)}
	rcases := []struct {
		r    Reducer
		want float64
	}{{ReduceMin, 1}, {ReduceMax, 9}, {ReduceSum, 13}, {ReduceP50, 3}, {ReduceP99, 9}}
	for _, c := range rcases {
		if got := Reduce(all, c.r)[0].Points[0].V; !near(got, c.want, 1e-9) {
			t.Errorf("%v = %v, want %v", c.r, got, c.want)
		}
	}
	if reduceValues(nil, AlignMean) != 0 {
		t.Error("空の並びは 0 になるはず")
	}
	if nearestRank([]float64{1, 2, 3}, 0) != 1 {
		t.Error("下限の分位点が違う")
	}
	if nearestRank([]float64{1, 2, 3}, 1.5) != 3 {
		t.Error("上限の分位点が違う")
	}
}

// 分布を保ったままの整列と集約。
func TestDistributionsSurviveAlignAndReduce(t *testing.T) {
	s := Series{Labels: map[string]string{"pod": "a"}, Kind: Delta, Points: []Point{
		{T: 0, D: hist(4, 4, 900)}, {T: 10, D: hist(4, 4)},
	}}
	kept := Align(s, AlignDelta, 60)
	if len(kept.Points) != 1 || kept.Points[0].D == nil {
		t.Fatal("整列で分布が失われた")
	}
	if kept.Points[0].D.Count() != 5 {
		t.Fatalf("窓の中の分布が足されていない: %d", kept.Points[0].D.Count())
	}
	if got := Align(s, AlignMean, 60).Points[0].V; !near(got, (4+4+900+4+4)/5.0, 1e-9) {
		t.Fatalf("分布からの平均が違う: %v", got)
	}
	if got := Align(s, AlignP50, 60).Points[0].V; got > 10 {
		t.Fatalf("分布からの中央値が違う: %v", got)
	}

	summed := Reduce([]Series{kept, kept}, ReduceSum)[0]
	if summed.Points[0].D == nil || summed.Points[0].D.Count() != 10 {
		t.Fatalf("集約で分布が足されていない: %+v", summed.Points[0])
	}
	if got := Reduce([]Series{kept}, ReduceMean)[0].Points[0].V; got == 0 {
		t.Fatal("分布からの平均が取れていない")
	}
	if mergeHists(nil).Count() != 0 {
		t.Fatal("空の合流が空にならない")
	}
	if mergeHists([]Point{{T: 0}}).Count() != 0 {
		t.Fatal("分布を持たない点が数えられた")
	}
}

func repeat(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func peak(s Series) float64 {
	m := 0.0
	for _, p := range s.Points {
		if p.V > m {
			m = p.V
		}
	}
	return m
}
