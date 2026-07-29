package throttle

import "testing"

// 100 tick ごとに 40 tick ぶんだけ CPU を使ってよい、という設定。
var cpuMax = Limit{Quota: 40, Period: 100}

func burst(n, need int) []Task {
	out := make([]Task, n)
	for i := range out {
		out[i] = Task{Name: string(rune('a' + i)), Need: need}
	}
	return out
}

// 枠に収まっていれば止まらない。まずここを固定する。
func TestWithinQuotaIsNotThrottled(t *testing.T) {
	res, periods := Run(cpuMax, 1, []Task{{Name: "a", Need: 30}})
	if res[0].Latency != 30 || res[0].Throttled != 0 {
		t.Fatalf("%+v", res[0])
	}
	if len(periods) != 1 || periods[0].Used != 30 {
		t.Fatalf("%+v", periods)
	}
}

// この章の中心その1。平均で見れば余っているのに止まる。
func TestAverageIsEnoughButItStalls(t *testing.T) {
	// 60 tick ぶんの仕事。2期間(200 tick)で見れば枠は 80 あるので足りている。
	res, periods := Run(cpuMax, 1, []Task{{Name: "a", Need: 60}})
	r := res[0]

	t.Logf("CPU が要るのは %d tick、終わったのは %d tick、止められたのは %d tick",
		60, r.Latency, r.Throttled)
	for _, p := range periods {
		t.Logf("  期間 %3d〜  使った %2d / 止まった %2d", p.Start, p.Used, p.Stalled)
	}

	if r.Throttled == 0 {
		t.Fatal("止まっていない")
	}
	// 60 tick の仕事が、倍の時間をかけて終わる。
	if r.Latency < 120 {
		t.Errorf("所要 %d tick", r.Latency)
	}
	// 平均で見れば足りている、を数字で言えるようにしておく。
	if 60*100 >= cpuMax.Quota*200 {
		t.Fatal("前提が崩れている: 平均でも足りていない")
	}
}

// この章の中心その2。並列度を上げると、期間の頭で使い切って残りを丸ごと待つ。
func TestParallelBurnsTheQuotaEarly(t *testing.T) {
	// 合計 80 tick の仕事を、8本に分けて同時に出す。
	type row struct {
		ncpu   int
		maxLat int
		stall  float64
	}
	var rows []row
	for _, ncpu := range []int{1, 2, 8} {
		res, periods := Run(cpuMax, ncpu, burst(8, 10))
		rows = append(rows, row{ncpu, MaxLatency(res), StallRatio(cpuMax, periods)})
		t.Logf("同時に %d 本   最後の1本 %3d tick   枠を使い切ったのは %2d tick 目   期間の %.0f%% を止まって過ごす",
			ncpu, MaxLatency(res), periods[0].Exhausted, StallRatio(cpuMax, periods)*100)
	}

	// 並列度を上げるほど、期間のうち止まっている割合が増える。
	if rows[2].stall <= rows[0].stall {
		t.Errorf("並列度で止まる割合が増えていない: %.2f, %.2f", rows[0].stall, rows[2].stall)
	}
	// 8本同時なら、期間の9割以上を止まって過ごす。
	if rows[2].stall < 0.9 {
		t.Errorf("8本同時で止まる割合が %.2f", rows[2].stall)
	}
}

// この章の中心その3。使い残しを繰り越せると、暇のあとの burst を吸える。
func TestCarryOverAbsorbsTheBurst(t *testing.T) {
	// 3期間まるまる暇にして、そのあと 100 tick ぶんの仕事が来る。
	arrive := 3 * cpuMax.Period
	work := []Task{{Name: "a", Arrive: arrive, Need: 100}}

	fixed, _ := Run(cpuMax, 1, work)
	withBurst, _ := Run(Limit{Quota: 40, Period: 100, Burst: 120}, 1, work)

	t.Logf("繰り越しなし  所要 %3d tick / 止められ %3d tick", fixed[0].Latency, fixed[0].Throttled)
	t.Logf("繰り越しあり  所要 %3d tick / 止められ %3d tick", withBurst[0].Latency, withBurst[0].Throttled)

	if fixed[0].Throttled == 0 {
		t.Fatal("繰り越しなしで止まっていない")
	}
	if withBurst[0].Throttled != 0 {
		t.Errorf("繰り越しありで止まった: %d tick", withBurst[0].Throttled)
	}
	if withBurst[0].Latency != 100 {
		t.Errorf("繰り越しありの所要 %d tick", withBurst[0].Latency)
	}
	// 貯めた枠は Burst までしか持ち込めない。3期間ぶん貯まるわけではない。
	small, _ := Run(Limit{Quota: 40, Period: 100, Burst: 20}, 1, work)
	if small[0].Throttled == 0 {
		t.Error("Burst が小さくても止まらないのはおかしい")
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 仕事が無ければ期間も進まない。
	res, periods := Run(cpuMax, 1, nil)
	if len(res) != 0 || len(periods) != 0 {
		t.Fatalf("%v %v", res, periods)
	}
	// CPU の本数が足りないのは、制限で止められたのとは別物。数えない。
	r, _ := Run(Limit{Quota: 1000, Period: 1000}, 1, burst(4, 10))
	if TotalThrottled(r) != 0 {
		t.Errorf("枠が余っているのに止められた: %d", TotalThrottled(r))
	}
	if MaxLatency(r) != 40 {
		t.Errorf("1本ずつ順に走るので 40 tick のはず: %d", MaxLatency(r))
	}
	// 期間が1つしかないときの割合は 0 にしておく。
	if got := StallRatio(cpuMax, periods); got != 0 {
		t.Errorf("%f", got)
	}
}
