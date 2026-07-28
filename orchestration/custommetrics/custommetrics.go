// Package custommetrics は、CPU 以外の指標で伸ばす自動スケールを最小構成で実装する。
//
// [水平オートスケール](autoscaler)の章では、CPU 使用率ひとつでレプリカ数を決めた。
// 式は1行で、難しいのは周りだった。だがもう1つ、式の外に大きな問題がある。
// 何で測るか、という問題だ。
//
// CPU 使用率には上限がある。100% を超えられない。ということは、100% に達した
// 後は「どれだけ足りないか」を教えてくれない。10 倍の負荷が来ても 100% だし、
// 100 倍の負荷が来ても 100% になる。式は現在値と目標の比でレプリカ数を出すので、
// 目標が 50% なら、どれだけ足りなくても倍にしかならない。追いつくまでに何周期も
// かかる。
//
// 待ち行列の長さには上限が無い。1000 件溜まっていれば 1000 と出る。1 つの Pod が
// 10 件を持つのが適正なら、必要なのは 100 個だと、その場で分かる。
//
// もう1つ、CPU では 0 にできない。レプリカが 0 のとき、1 個あたりの使用率は
// 計算できない。だから CPU で回す限り、常に1つは動かしておくことになる。
// 0 にするには、指標の値ではなく「仕事があるかどうか」を見る別の判断が要る。
// これが KEDA が 0 と 1 の間だけを担い、1 以上を HPA に渡す理由になる。
package custommetrics

import "sort"

// #region metric

// Kind は目標の与え方。同じ数でも、何と比べるかで式が変わる。
type Kind int

const (
	// Utilization は 1 個あたりの使用率(%)を目標にする。CPU がこれ。
	Utilization Kind = iota
	// AverageValue は 1 個あたりの絶対値を目標にする。「1 個が 10 件持つ」など。
	AverageValue
	// Value は全体の絶対値を目標にする。レプリカ数で割らない。
	Value
)

func (k Kind) String() string {
	return [...]string{"Utilization", "AverageValue", "Value"}[k]
}

// Metric は1つの判断材料。
type Metric struct {
	Name string
	Kind Kind
	// Target は目標値。Utilization なら %、他なら件数など。
	Target float64
	// Saturates は、この指標に上限があるかを表す(観測用)。
	// 上限のある指標は、上限に達した後どれだけ足りないかを言えない。
	Saturates bool
}

// Desired は1つの指標から必要なレプリカ数を出す。
//
// 切り上げるのは[水平オートスケール](autoscaler)の章と同じ理由で、足りないより
// 多いほうが安全だからになる。current の意味だけが Kind で変わる。
func Desired(replicas int, m Metric, current float64) int {
	if m.Target <= 0 {
		return replicas
	}
	switch m.Kind {
	case Utilization:
		if replicas <= 0 {
			// 1 個あたりの使用率は、0 個のときに計算できない。
			// これが CPU で 0 にできない理由になる。
			return replicas
		}
		return ceilDiv(float64(replicas)*current, m.Target)
	case AverageValue:
		// current は全体の合計。1 個あたりが Target になる数を出す。
		return ceilDiv(current, m.Target)
	default: // Value
		return ceilDiv(current, m.Target)
	}
}

// DesiredAll は複数の指標から必要なレプリカ数を出す。
//
// 最大を採る。どれか1つでも足りていなければ足りていないので、いちばん多くを
// 要求する指標に合わせることになる。逆に言えば、指標を足すことは減る方向には
// 働かない。1つ足すたびに、下がりにくくなる。
func DesiredAll(replicas int, ms []Metric, readings map[string]float64) (int, string) {
	best, by := 0, ""
	for _, m := range ms {
		v, ok := readings[m.Name]
		if !ok {
			continue
		}
		// 同点なら先に宣言したほうを決め手として残す(結果を決定的にする)。
		if d := Desired(replicas, m, v); by == "" || d > best {
			best, by = d, m.Name
		}
	}
	if by == "" {
		return replicas, ""
	}
	return best, by
}

func ceilDiv(a, b float64) int {
	n := int(a / b)
	if float64(n)*b < a {
		n++
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// #endregion metric

// #region scaler

// Config は自動スケールの設定。
type Config struct {
	Metrics []Metric
	Min     int
	Max     int
	// Activation は 0 個のときに 1 個へ起こす閾値。仕事の量ではなく有無を見る。
	// 0 を許さない設定(Min >= 1)では使われない。
	Activation float64
	// Cooldown は 0 へ落とすまでに、仕事が無い状態が続くべき tick 数。
	Cooldown int
}

// AllowsZero は 0 まで縮められる設定かを返す。
func (c Config) AllowsZero() bool { return c.Min == 0 }

// Scaler は読み取りからレプリカ数を決める。
type Scaler struct {
	cfg  Config
	idle int // 仕事が無い状態が続いた tick 数
}

// NewScaler は設定からスケーラを作る。
func NewScaler(cfg Config) *Scaler { return &Scaler{cfg: cfg} }

// Decision は1回の判断の結果と、その理由。
type Decision struct {
	Replicas int
	By       string // 決め手になった指標名
	Reason   string
}

// Decide は現在のレプリカ数と読み取りから、次のレプリカ数を決める。
//
// 0 の扱いだけが別の判断になっている。0 個のときは指標の値を計算できないので、
// 仕事があるかどうかだけを見て 1 個へ起こす。1 個以上になれば、あとは普通の式で
// 決まる。この境目が、KEDA と HPA の分担になっている。
func (s *Scaler) Decide(replicas int, readings map[string]float64) Decision {
	work := s.work(readings)

	if replicas == 0 {
		if !s.cfg.AllowsZero() {
			return Decision{Replicas: s.clamp(s.cfg.Min), Reason: "0 を許さない設定なので下限まで戻す"}
		}
		if work > s.cfg.Activation {
			s.idle = 0
			return Decision{Replicas: s.clamp(1), Reason: "仕事が来たので 1 個起こす"}
		}
		return Decision{Replicas: 0, Reason: "仕事が無いので 0 のまま"}
	}

	if s.cfg.AllowsZero() && work <= s.cfg.Activation {
		s.idle++
	} else {
		s.idle = 0
	}

	want, by := DesiredAll(replicas, s.cfg.Metrics, readings)
	d := Decision{Replicas: s.clamp(want), By: by}
	if d.Replicas == 0 {
		if s.idle <= s.cfg.Cooldown {
			// 式は 0 でよいと言っているが、まだ待ち時間の内側。落とさない。
			return Decision{Replicas: 1, By: by, Reason: "仕事は無いが、落とすまでの待ち時間の内側"}
		}
		return Decision{Replicas: 0, By: by, Reason: "仕事が無い状態が続いたので 0 へ落とす"}
	}
	switch {
	case d.Replicas > replicas:
		d.Reason = by + " が目標を超えている"
	case d.Replicas < replicas:
		d.Reason = by + " に余裕がある"
	default:
		d.Reason = "変えない"
	}
	return d
}

// work は「仕事があるか」を1つの数にする。指標の値ではなく有無を見るための量で、
// 読み取りの中でいちばん大きいものを使う。
func (s *Scaler) work(readings map[string]float64) float64 {
	keys := make([]string, 0, len(readings))
	for k := range readings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := 0.0
	for _, k := range keys {
		if readings[k] > m {
			m = readings[k]
		}
	}
	return m
}

func (s *Scaler) clamp(n int) int {
	if n < s.cfg.Min {
		n = s.cfg.Min
	}
	if s.cfg.Max > 0 && n > s.cfg.Max {
		n = s.cfg.Max
	}
	return n
}

// #endregion scaler

// #region sim

// SimConfig は仕事の流れ方。
type SimConfig struct {
	// Capacity は Pod 1 個が 1 tick に捌ける件数。
	Capacity int
	// Arrivals は tick ごとの到着件数(台本)。乱数を使わないので再現する。
	Arrivals []int
}

// Sim は待ち行列のある系を動かし、指標がどう見えるかを再現する。
type Sim struct {
	cfg    SimConfig
	scaler *Scaler

	now      int
	replicas int
	backlog  int

	// CPU は直近の tick で、用意した処理能力のうち実際に使った割合(%)。
	// 上限があるので、足りていないときは常に 100 になる。
	CPU float64

	Dropped int // 到着したが、この tick では捌けなかった累計(遅れの総量)
	Log     []string
	History []Snapshot
}

// Snapshot は1 tick ぶんの記録(観測用)。
type Snapshot struct {
	T        int
	Replicas int
	Backlog  int
	CPU      float64
	Arrivals int
	Served   int
	By       string
}

// NewSim は初期レプリカ数 start から始める。
func NewSim(cfg SimConfig, sc *Scaler, start int) *Sim {
	return &Sim{cfg: cfg, scaler: sc, replicas: start}
}

// Now は現在の論理時刻を返す。
func (s *Sim) Now() int { return s.now }

// Replicas は現在のレプリカ数を返す。
func (s *Sim) Replicas() int { return s.replicas }

// Backlog は待ち行列の長さを返す。
func (s *Sim) Backlog() int { return s.backlog }

// Readings は今この瞬間に見える指標の値を返す。
func (s *Sim) Readings() map[string]float64 {
	return map[string]float64{"cpu": s.CPU, "queue": float64(s.backlog)}
}

// Tick は時刻を1つ進める。
//
// 見る、決める、届く、捌く、の順になっている。判断は前の tick までに見えた値で
// 行うので、指標の遅れがそのまま反応の遅れとして出る。
func (s *Sim) Tick() {
	d := s.scaler.Decide(s.replicas, s.Readings())
	if d.Replicas != s.replicas {
		s.logf("レプリカを " + itoa(s.replicas) + " から " + itoa(d.Replicas) + " へ(" + d.Reason + ")")
	}
	s.replicas = d.Replicas

	arrivals := 0
	if s.now < len(s.cfg.Arrivals) {
		arrivals = s.cfg.Arrivals[s.now]
	}
	s.backlog += arrivals

	capacity := s.replicas * s.cfg.Capacity
	served := s.backlog
	if served > capacity {
		served = capacity
	}
	s.backlog -= served
	s.Dropped += s.backlog

	if capacity > 0 {
		s.CPU = float64(served) / float64(capacity) * 100
	} else {
		s.CPU = 0
	}

	s.History = append(s.History, Snapshot{
		T: s.now, Replicas: s.replicas, Backlog: s.backlog, CPU: s.CPU,
		Arrivals: arrivals, Served: served, By: d.By,
	})
	s.now++
}

// Run は n tick 進める。
func (s *Sim) Run(n int) {
	for i := 0; i < n; i++ {
		s.Tick()
	}
}

// StabilizedAt は、その tick に届いた量を捌ききれるようになった最初の時刻を返す
// (最後まで追いつかなければ -1)。
//
// 待ち行列を目標にすると、行列は 0 にはならず「1 個あたり目標件数」で釣り合う。
// だから追いついたかどうかは、行列が空になったかではなく、届く量に処理能力が
// 並んだかで測る。
func (s *Sim) StabilizedAt() int {
	last := -1
	for i, h := range s.History {
		if h.Arrivals > 0 && h.Served < h.Arrivals {
			last = i
		}
	}
	if last < 0 {
		return 0 // 一度も遅れていない
	}
	if last == len(s.History)-1 {
		return -1 // 最後まで遅れたまま
	}
	return s.History[last+1].T
}

// PeakBacklog は待ち行列の最大長を返す。
func (s *Sim) PeakBacklog() int {
	m := 0
	for _, h := range s.History {
		if h.Backlog > m {
			m = h.Backlog
		}
	}
	return m
}

// #endregion sim

func (s *Sim) logf(msg string) { s.Log = append(s.Log, "t="+itoa(s.now)+" "+msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
