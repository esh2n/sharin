// Package autoscaler は Kubernetes の水平オートスケーラ(HPA)を最小構成で実装する。
//
// 調整ループは「Pod を何個に保つか」を宣言どおり守る。スケジューラは
// 作られた Pod をどのノードに置くかを決める。だが「何個であるべきか」を
// 決めるのは誰か。負荷は時間で変わる。朝は暇で、昼にバーストが来る。
// その数を人が張り付いて変えるのは現実的でない。それを負荷から自動で
// 決めるのが水平オートスケーラだ。
//
// 計算の中心は 1 行の式しかない。
//
//	desired = ceil(current * currentMetric / targetMetric)
//
// 「今 4 個で使用率 80%、目標が 40% なら、8 個にすれば 40% になるはず」
// という比例の当てずっぽうだ。難しいのは式でなく、その周りにある。
// 測った値は揺れる。増やした Pod が効き始めるまで時間がかかる。素直に
// 従うと、増やしては減らしを延々と繰り返す(flapping)。だから実物は
// 許容誤差(tolerance)、安定化ウィンドウ、上下限を式の周りに積んでいる。
// この章はその全部を作る。
package autoscaler

// #region metrics

// Sample は 1 回の観測。Replicas はそのとき動いていたレプリカ数、
// Utilization は 1 レプリカあたりの平均使用率(百分率)。
type Sample struct {
	Replicas    int
	Utilization int
}

// #endregion metrics

// #region formula

// ceilDiv は a/b を切り上げた整数を返す(b > 0 を前提)。
// 切り上げるのは、足りないより多いほうが安全だから。3.2 個必要なら 4 個要る。
func ceilDiv(a, b int) int { return (a + b - 1) / b }

// Ratio は「今の使用率 / 目標の使用率」に基づく必要レプリカ数を返す。
// HPA の中心の式そのもので、負荷とレプリカ数が比例するという仮定に立つ。
// 使用率が目標の 2 倍なら、レプリカを 2 倍にすれば目標に戻る、と読む。
func Ratio(s Sample, target int) int {
	if target <= 0 || s.Replicas <= 0 {
		return s.Replicas
	}
	return ceilDiv(s.Replicas*s.Utilization, target)
}

// #endregion formula

// #region config

// Config はオートスケーラの設定。式の周りを固めるための値が並ぶ。
type Config struct {
	Target    int // 目標使用率(百分率)
	Min       int // 下限レプリカ数
	Max       int // 上限レプリカ数
	Tolerance int // 許容誤差(百分率)。この幅の中の揺れは無視する
	// StabilizeDown は縮小を決めるまでに待つ観測回数。
	// 直近この回数ぶんの提案の最大値を採るので、一瞬の谷では縮まない。
	StabilizeDown int
}

// #endregion config

// #region tolerance

// withinTolerance は使用率が目標の許容誤差の内側かを返す。
// 目標 50%・許容誤差 10% なら、45% から 55% までは目標どおりとみなす。
func (c Config) withinTolerance(u int) bool {
	d := u - c.Target
	if d < 0 {
		d = -d
	}
	return d*100 <= c.Target*c.Tolerance
}

// #endregion tolerance

// #region clamp

// clamp は n を [Min, Max] に収める。式が何を出しても、ここを超えることはない。
func (c Config) clamp(n int) int {
	if n < c.Min {
		return c.Min
	}
	if n > c.Max {
		return c.Max
	}
	return n
}

// #endregion clamp

// #region autoscaler

// Decision は 1 回の判断の結果と、その理由。
type Decision struct {
	From   int // 判断前のレプリカ数
	To     int // 判断後のレプリカ数
	Raw    int // 式が出した生の値(clamp も安定化も通す前)
	Reason string
}

// Changed はレプリカ数が変わったかを返す。
func (d Decision) Changed() bool { return d.From != d.To }

// Autoscaler は観測から目標レプリカ数を決める。直近の提案を覚えていて、
// 縮小のときだけその履歴を使う(急に縮めないため)。
type Autoscaler struct {
	cfg     Config
	history []int // 直近の提案(古い順)
}

// New は設定 cfg のオートスケーラを作る。値が壊れていれば最小限に補正する。
func New(cfg Config) *Autoscaler {
	if cfg.Min < 1 {
		cfg.Min = 1
	}
	if cfg.Max < cfg.Min {
		cfg.Max = cfg.Min
	}
	if cfg.StabilizeDown < 1 {
		cfg.StabilizeDown = 1
	}
	return &Autoscaler{cfg: cfg}
}

// Decide は 1 回の観測から次のレプリカ数を決める。
// 式を当て、許容誤差の内側なら動かさず、上下限に収め、縮小なら安定化を通す。
func (a *Autoscaler) Decide(s Sample) Decision {
	raw := Ratio(s, a.cfg.Target)
	a.remember(raw)

	d := Decision{From: s.Replicas, To: s.Replicas, Raw: raw}

	// 許容誤差の内側の揺れは無視する。これがないと、目標付近の小さな
	// 上下でレプリカ数が動き続ける(flapping)。
	if a.cfg.withinTolerance(s.Utilization) {
		d.Reason = "目標の許容誤差の内側。動かさない"
		return d
	}

	want := a.cfg.clamp(raw)

	switch {
	case want > s.Replicas:
		// 拡大は即座に行う。負荷は待ってくれない。
		d.To = want
		d.Reason = "使用率が目標を超えた。すぐ拡大する"
	case want < s.Replicas:
		// 縮小は直近の提案の最大値まで。一瞬の谷では縮まない。
		stable := a.cfg.clamp(a.maxRecent(s.Replicas))
		if stable >= s.Replicas {
			d.Reason = "縮小の提案が安定していない。据え置く"
			return d
		}
		d.To = stable
		d.Reason = "使用率が目標を下回り続けた。縮小する"
	default:
		d.Reason = "式の結果が現在と同じ。動かさない"
	}
	return d
}

// remember は提案を履歴に積み、安定化ウィンドウのぶんだけ残す。
func (a *Autoscaler) remember(raw int) {
	a.history = append(a.history, raw)
	if len(a.history) > a.cfg.StabilizeDown {
		a.history = a.history[len(a.history)-a.cfg.StabilizeDown:]
	}
}

// maxRecent は安定化ウィンドウ内の提案の最大値を返す。最大を採るから、
// ウィンドウ内に 1 度でも高い提案があれば縮小しない。
//
// ウィンドウがまだ埋まっていない間は、現在のレプリカ数 now も候補に含める。
// 観測が足りない段階では「今の数が必要だった」と見なす、という意味で、
// 起動直後の 1 回の低い観測で縮めてしまうのを防ぐ。
func (a *Autoscaler) maxRecent(now int) int {
	m := 0
	if len(a.history) < a.cfg.StabilizeDown {
		m = now
	}
	for _, v := range a.history {
		if v > m {
			m = v
		}
	}
	return m
}

// #endregion autoscaler
