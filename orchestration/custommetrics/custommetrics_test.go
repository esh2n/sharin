package custommetrics

import "testing"

var cpuMetric = Metric{Name: "cpu", Kind: Utilization, Target: 50, Saturates: true}
var queueMetric = Metric{Name: "queue", Kind: AverageValue, Target: 10}

// 平常が続いてから 10 倍に跳ねる台本。跳ねた後もその水準が続く。
func spikeArrivals() []int {
	a := make([]int, 40)
	for i := range a {
		if i < 5 {
			a[i] = 20
		} else {
			a[i] = 200
		}
	}
	return a
}

func cpuSim(start int) *Sim {
	return NewSim(SimConfig{Capacity: 10, Arrivals: spikeArrivals()},
		NewScaler(Config{Metrics: []Metric{cpuMetric}, Min: 1, Max: 100}), start)
}

func queueSim(start int) *Sim {
	return NewSim(SimConfig{Capacity: 10, Arrivals: spikeArrivals()},
		NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 1, Max: 100}), start)
}

// 式そのものの確認。何と比べるかで意味が変わる。
func TestDesiredByKind(t *testing.T) {
	// 4 個で使用率 80%、目標 50% → ceil(4 * 80 / 50) = 7
	if got := Desired(4, cpuMetric, 80); got != 7 {
		t.Fatalf("Utilization = %d, want 7", got)
	}
	// 全体で 95 件、1 個あたり 10 件が目標 → ceil(95 / 10) = 10。今の個数は関係ない
	if got := Desired(3, queueMetric, 95); got != 10 {
		t.Fatalf("AverageValue = %d, want 10", got)
	}
	if got := Desired(50, queueMetric, 95); got != 10 {
		t.Fatal("AverageValue が現在のレプリカ数に依存している")
	}
	// Value は全体の値をそのまま目標と比べる
	if got := Desired(3, Metric{Name: "v", Kind: Value, Target: 100}, 250); got != 3 {
		t.Fatalf("Value = %d, want 3", got)
	}
	// 目標 0 は判断材料にならない
	if got := Desired(4, Metric{Name: "x", Kind: Value, Target: 0}, 100); got != 4 {
		t.Fatalf("目標 0 で値が変わった: %d", got)
	}
}

// この章の中心。上限のある指標は、上限に達した後どれだけ足りないかを言えない。
func TestSaturatedMetricCannotExpressHowShort(t *testing.T) {
	// 10 倍足りなくても、100 倍足りなくても、CPU は 100% にしかならない。
	tenfold := Desired(2, cpuMetric, 100)
	hundredfold := Desired(2, cpuMetric, 100)
	if tenfold != hundredfold {
		t.Fatal("上限のある指標が不足量を区別できてしまっている")
	}
	if tenfold != 4 {
		t.Fatalf("目標 50%% なら倍にしかならないはず: %d", tenfold)
	}

	// 待ち行列は上限が無いので、必要な数がその場で出る。
	if got := Desired(2, queueMetric, 2000); got != 200 {
		t.Fatalf("待ち行列から必要数が出ていない: %d", got)
	}
}

// 同じ負荷でも、何で測るかで追いつくまでの時間が変わる。
func TestQueueCatchesUpFasterThanCPU(t *testing.T) {
	c, q := cpuSim(2), queueSim(2)
	c.Run(40)
	q.Run(40)

	cAt, qAt := c.StabilizedAt(), q.StabilizedAt()
	if cAt < 0 || qAt < 0 {
		t.Fatalf("どちらかが追いつかない: cpu=%d queue=%d", cAt, qAt)
	}
	if qAt >= cAt {
		t.Fatalf("待ち行列のほうが遅い: cpu=%d queue=%d", cAt, qAt)
	}
	if q.PeakBacklog() >= c.PeakBacklog() {
		t.Fatalf("待ち行列のほうが溜まっている: cpu=%d queue=%d", c.PeakBacklog(), q.PeakBacklog())
	}
}

// 目標の置き方が、そのまま落ち着き先を決める。
//
// CPU 50% は「半分空けておけ」という宣言なので、行列は 0 になる代わりに
// 必要な数の倍を抱える。1 個 10 件は「10 件並んでいてよい」という宣言なので、
// 数は必要なぶんで止まる代わりに、常に行列が残る。どちらも指示どおりに動いている。
func TestTargetDecidesTheSteadyState(t *testing.T) {
	c, q := cpuSim(2), queueSim(2)
	c.Run(40)
	q.Run(40)

	if c.Backlog() != 0 {
		t.Fatalf("CPU 目標なら行列は 0 に落ちるはず: %d", c.Backlog())
	}
	if q.Backlog() == 0 {
		t.Fatal("待ち行列目標なら行列は残るはず(それが目標そのもの)")
	}
	if q.Backlog() != q.Replicas()*10 {
		t.Fatalf("落ち着き先が 1 個あたりの目標と合わない: %d 件 / %d 個", q.Backlog(), q.Replicas())
	}
	if c.Replicas() <= q.Replicas() {
		t.Fatalf("CPU 目標のほうが多く抱えるはず: cpu=%d queue=%d", c.Replicas(), q.Replicas())
	}
	// 到着 200 / 1 個 10 件なら、必要なのは 20 個。待ち行列はそこで止まる。
	if q.Replicas() != 20 {
		t.Fatalf("必要な数で止まっていない: %d", q.Replicas())
	}
	// CPU は目標 50% なので、その倍を抱えることになる。
	if c.Replicas() != 40 {
		t.Fatalf("目標 50%% なら倍を抱えるはず: %d", c.Replicas())
	}
}

// CPU は倍々にしか増えないので、必要な数へ届くまで何周期もかかる。
func TestCPUDoublesStepByStep(t *testing.T) {
	c := cpuSim(2)
	c.Run(20)

	var reps []int
	for _, h := range c.History {
		reps = append(reps, h.Replicas)
	}
	// 跳ねた直後は 2 → 4 → 8 のように、1 周期で倍までしか増えない。
	for i := 1; i < len(reps); i++ {
		if reps[i] > reps[i-1]*2 {
			t.Fatalf("t=%d で倍を超えて増えている: %d → %d", i, reps[i-1], reps[i])
		}
	}
}

// 待ち行列は1周期で必要な数へ届く。
func TestQueueReachesTargetInOneStep(t *testing.T) {
	q := queueSim(2)
	q.Run(12)

	jumped := false
	for i := 1; i < len(q.History); i++ {
		if q.History[i].Replicas > q.History[i-1].Replicas*2 {
			jumped = true
		}
	}
	if !jumped {
		t.Fatal("待ち行列でも倍までしか増えていない")
	}
}

// 指標を足すと最大を採る。どれか1つでも足りなければ足りない。
func TestMultipleMetricsTakeTheMax(t *testing.T) {
	ms := []Metric{cpuMetric, queueMetric}
	// cpu は 4 個 * 60 / 50 = ceil(4.8) = 5、queue は 200/10 = 20
	got, by := DesiredAll(4, ms, map[string]float64{"cpu": 60, "queue": 200})
	if got != 20 || by != "queue" {
		t.Fatalf("最大を採っていない: %d by %q", got, by)
	}
	// 逆に cpu が要求するほうが多い場面
	got, by = DesiredAll(4, ms, map[string]float64{"cpu": 200, "queue": 10})
	if got != 16 || by != "cpu" {
		t.Fatalf("最大を採っていない: %d by %q", got, by)
	}
	// 読み取りが無い指標は判断に加わらない
	if got, _ := DesiredAll(4, ms, map[string]float64{"cpu": 50}); got != 4 {
		t.Fatalf("読み取りの無い指標で値が変わった: %d", got)
	}
	// どの指標も読めなければ現状維持
	if got, by := DesiredAll(7, ms, map[string]float64{}); got != 7 || by != "" {
		t.Fatalf("読み取りが空で現状維持になっていない: %d by %q", got, by)
	}
}

// 指標を足すことは、下がる方向には働かない。
func TestAddingMetricsOnlyRaisesTheFloor(t *testing.T) {
	only := []Metric{queueMetric}
	both := []Metric{queueMetric, cpuMetric}
	readings := map[string]float64{"cpu": 90, "queue": 30}

	a, _ := DesiredAll(4, only, readings)
	b, _ := DesiredAll(4, both, readings)
	if b < a {
		t.Fatalf("指標を足したら下がった: %d → %d", a, b)
	}
	if b <= a {
		t.Fatalf("この場面では cpu のほうが多くを要求するはず: %d → %d", a, b)
	}
}

// 0 個のとき、1 個あたりの使用率は計算できない。
func TestUtilizationCannotScaleFromZero(t *testing.T) {
	if got := Desired(0, cpuMetric, 100); got != 0 {
		t.Fatalf("0 個から CPU で起き上がってしまった: %d", got)
	}
	// 待ち行列は 0 個でも必要数が出る。
	if got := Desired(0, queueMetric, 50); got != 5 {
		t.Fatalf("0 個から待ち行列で起き上がれない: %d", got)
	}
}

// 0 を許さない設定では、下限を下回らない。
func TestMinKeepsOneAlive(t *testing.T) {
	s := NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 1, Max: 10})
	d := s.Decide(3, map[string]float64{"queue": 0})
	if d.Replicas != 1 {
		t.Fatalf("下限を下回った: %d", d.Replicas)
	}
	if s.Decide(0, map[string]float64{"queue": 0}).Replicas != 1 {
		t.Fatal("0 を許さない設定なのに 0 のままになった")
	}
}

// 0 まで縮めるには、値ではなく仕事の有無を見る。
func TestScaleToZeroAndBack(t *testing.T) {
	s := NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 0, Max: 10, Activation: 0, Cooldown: 2})

	// 仕事が無くなっても、すぐには落とさない。
	if got := s.Decide(2, map[string]float64{"queue": 0}).Replicas; got == 0 {
		t.Fatalf("待たずに 0 へ落とした: %d", got)
	}
	s.Decide(2, map[string]float64{"queue": 0})
	if got := s.Decide(2, map[string]float64{"queue": 0}).Replicas; got != 0 {
		t.Fatalf("待っても 0 へ落ちない: %d", got)
	}

	// 0 のままなら 0。仕事が来たら 1 個起こす。
	if got := s.Decide(0, map[string]float64{"queue": 0}).Replicas; got != 0 {
		t.Fatalf("仕事が無いのに起きた: %d", got)
	}
	d := s.Decide(0, map[string]float64{"queue": 1})
	if d.Replicas != 1 {
		t.Fatalf("仕事が来たのに起きない: %d", d.Replicas)
	}
	// 起きた後は普通の式に戻る。1 個で 100 件なら 10 個。
	if got := s.Decide(1, map[string]float64{"queue": 100}).Replicas; got != 10 {
		t.Fatalf("起きた後に式へ戻っていない: %d", got)
	}
}

// 途中で仕事が来たら、待ち時間の数え直しが起きる。
func TestCooldownResets(t *testing.T) {
	s := NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 0, Max: 10, Activation: 0, Cooldown: 2})
	s.Decide(2, map[string]float64{"queue": 0})
	s.Decide(2, map[string]float64{"queue": 5}) // 仕事が来た
	s.Decide(2, map[string]float64{"queue": 0})
	if got := s.Decide(2, map[string]float64{"queue": 0}).Replicas; got == 0 {
		t.Fatal("数え直しが効いていない")
	}
}

// 起こす閾値を上げると、少しの仕事では起きない。
func TestActivationThreshold(t *testing.T) {
	s := NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 0, Max: 10, Activation: 5, Cooldown: 0})
	if got := s.Decide(0, map[string]float64{"queue": 3}).Replicas; got != 0 {
		t.Fatalf("閾値を下回るのに起きた: %d", got)
	}
	if got := s.Decide(0, map[string]float64{"queue": 6}).Replicas; got != 1 {
		t.Fatalf("閾値を超えたのに起きない: %d", got)
	}
}

// 上限は式より強い。
func TestMaxClamps(t *testing.T) {
	s := NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 1, Max: 8})
	if got := s.Decide(2, map[string]float64{"queue": 5000}).Replicas; got != 8 {
		t.Fatalf("上限が効いていない: %d", got)
	}
}

// 0 から始めても、仕事が来れば動き出して追いつく。
func TestSimStartsFromZero(t *testing.T) {
	s := NewSim(SimConfig{Capacity: 10, Arrivals: spikeArrivals()},
		NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 0, Max: 100, Activation: 0, Cooldown: 1}), 0)
	s.Run(40)

	if s.History[0].Replicas != 0 {
		t.Fatalf("最初から起きている: %d", s.History[0].Replicas)
	}
	if s.StabilizedAt() < 0 {
		t.Fatal("追いつかない")
	}
	if s.Replicas() == 0 {
		t.Fatal("仕事が続いているのに 0 のまま")
	}
}

// 仕事が止まれば 0 に戻る。
func TestSimReturnsToZero(t *testing.T) {
	arrivals := make([]int, 30)
	for i := 0; i < 5; i++ {
		arrivals[i] = 100
	}
	s := NewSim(SimConfig{Capacity: 10, Arrivals: arrivals},
		NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 0, Max: 100, Activation: 0, Cooldown: 2}), 0)
	s.Run(30)

	if s.Replicas() != 0 {
		t.Fatalf("仕事が止まったのに 0 に戻らない: %d", s.Replicas())
	}
	if s.Backlog() != 0 {
		t.Fatalf("捌き残しがある: %d", s.Backlog())
	}
}

// 観測用の細かい確認。
func TestAccessors(t *testing.T) {
	s := queueSim(1)
	if s.Now() != 0 || s.Replicas() != 1 || s.Backlog() != 0 {
		t.Fatal("初期状態が違う")
	}
	s.Run(3)
	if s.Now() != 3 || len(s.History) != 3 {
		t.Fatal("記録が残っていない")
	}
	if len(s.Log) == 0 {
		t.Fatal("レプリカ数が動いたのに記録が無い")
	}
	r := s.Readings()
	if _, ok := r["cpu"]; !ok {
		t.Fatal("読み取りに cpu が無い")
	}

	if Utilization.String() != "Utilization" || AverageValue.String() != "AverageValue" ||
		Value.String() != "Value" {
		t.Fatal("種類の名前が違う")
	}
	if !(Config{Min: 0}).AllowsZero() || (Config{Min: 1}).AllowsZero() {
		t.Fatal("0 を許すかの判定が違う")
	}

	// 到着の台本を使い切っても止まらない。
	empty := NewSim(SimConfig{Capacity: 10, Arrivals: nil},
		NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 1, Max: 10}), 1)
	empty.Run(3)
	if empty.StabilizedAt() != 0 {
		t.Fatal("何も来ていなければ、一度も遅れていないはず")
	}
	if empty.PeakBacklog() != 0 {
		t.Fatal("何も来ていないのに溜まっている")
	}
	if maxInt(3, 5) != 5 || maxInt(5, 3) != 5 {
		t.Fatal("maxInt が違う")
	}
	if itoa(0) != "0" || itoa(42) != "42" || itoa(-7) != "-7" {
		t.Fatal("itoa が違う")
	}
}

// 追いつかない場面では -1 を返す。
func TestNeverCatchesUp(t *testing.T) {
	arrivals := make([]int, 10)
	for i := range arrivals {
		arrivals[i] = 500
	}
	s := NewSim(SimConfig{Capacity: 10, Arrivals: arrivals},
		NewScaler(Config{Metrics: []Metric{queueMetric}, Min: 1, Max: 2}), 1) // 上限 2 では足りない
	s.Run(10)
	if s.StabilizedAt() >= 0 {
		t.Fatal("上限で足りないのに追いついたことになっている")
	}
	if s.Dropped == 0 {
		t.Fatal("遅れが記録されていない")
	}
}
