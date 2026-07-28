package autoscaler

import "testing"

// std は本章で標準的に使う設定。目標 50%、1〜10 レプリカ、
// 許容誤差 10%、縮小は 3 回連続で提案が下がったときだけ。
func std() Config {
	return Config{Target: 50, Min: 1, Max: 10, Tolerance: 10, StabilizeDown: 3}
}

// HPA の中心の式。使用率が目標の 2 倍なら、レプリカも 2 倍になる。
func TestRatioFormula(t *testing.T) {
	cases := []struct {
		replicas, util, target, want int
	}{
		{4, 100, 50, 8}, // 2 倍の負荷 → 2 倍のレプリカ
		{4, 50, 50, 4},  // 目標どおり → そのまま
		{4, 25, 50, 2},  // 半分の負荷 → 半分のレプリカ
		{3, 70, 50, 5},  // 4.2 → 切り上げて 5(足りないより多いほうが安全)
		{0, 100, 50, 0}, // レプリカ 0 からは比例で増やせない
		{4, 100, 0, 4},  // 目標 0 は割れないので据え置く
	}
	for _, c := range cases {
		got := Ratio(Sample{Replicas: c.replicas, Utilization: c.util}, c.target)
		if got != c.want {
			t.Errorf("Ratio(%d個, %d%%, 目標%d%%) = %d, want %d", c.replicas, c.util, c.target, got, c.want)
		}
	}
}

// 使用率が目標を超えたら、待たずにすぐ拡大する。負荷は待ってくれない。
func TestScaleUpIsImmediate(t *testing.T) {
	a := New(std())
	d := a.Decide(Sample{Replicas: 2, Utilization: 100})
	if d.To != 4 {
		t.Fatalf("2個・100%%・目標50%% なら 4 個のはずが %d(%s)", d.To, d.Reason)
	}
	if !d.Changed() {
		t.Fatal("拡大したのに Changed が false")
	}
}

// 許容誤差の内側の揺れは無視する。これがないと目標付近で上下し続ける。
func TestToleranceIgnoresNoise(t *testing.T) {
	a := New(std())
	for _, u := range []int{46, 50, 54} { // 目標 50 の ±10% = 45〜55
		if d := a.Decide(Sample{Replicas: 4, Utilization: u}); d.Changed() {
			t.Fatalf("%d%% は許容誤差の内側。動かないはずが %d 個へ", u, d.To)
		}
	}
	if d := a.Decide(Sample{Replicas: 4, Utilization: 60}); !d.Changed() {
		t.Fatal("60%% は許容誤差の外側。動くはず")
	}
}

// 縮小は 1 回の観測では起こらない。一瞬の谷で縮めると、
// 戻ってきた負荷に耐えられない。
func TestScaleDownWaitsForStability(t *testing.T) {
	a := New(std())
	// 1 回目・2 回目の低い観測では縮まない(ウィンドウは 3)。
	if d := a.Decide(Sample{Replicas: 8, Utilization: 20}); d.Changed() {
		t.Fatalf("1 回目で縮んだ: %d(%s)", d.To, d.Reason)
	}
	if d := a.Decide(Sample{Replicas: 8, Utilization: 20}); d.Changed() {
		t.Fatalf("2 回目で縮んだ: %d(%s)", d.To, d.Reason)
	}
	// 3 回続けて低ければ縮む。
	d := a.Decide(Sample{Replicas: 8, Utilization: 20})
	if !d.Changed() || d.To != 4 {
		t.Fatalf("3 回目は 4 個へ縮むはずが %d(%s)", d.To, d.Reason)
	}
}

// ウィンドウ内に 1 度でも高い提案があれば縮まない。最大値を採るため。
func TestSpikeInWindowBlocksScaleDown(t *testing.T) {
	a := New(std())
	a.Decide(Sample{Replicas: 8, Utilization: 20}) // 低い
	a.Decide(Sample{Replicas: 8, Utilization: 90}) // バースト
	if d := a.Decide(Sample{Replicas: 8, Utilization: 20}); d.Changed() {
		t.Fatalf("直近にバーストがあるのに縮んだ: %d(%s)", d.To, d.Reason)
	}
}

// 上下限は式より強い。式が何を出しても、この外には出ない。
func TestClampToMinMax(t *testing.T) {
	a := New(Config{Target: 50, Min: 2, Max: 5, Tolerance: 10, StabilizeDown: 1})
	up := a.Decide(Sample{Replicas: 4, Utilization: 200}) // 式は 16
	if up.Raw != 16 || up.To != 5 {
		t.Fatalf("上限 5 で止まるはず: raw=%d to=%d", up.Raw, up.To)
	}
	down := a.Decide(Sample{Replicas: 4, Utilization: 1}) // 式は 1
	if down.To != 2 {
		t.Fatalf("下限 2 で止まるはず: %d(%s)", down.To, down.Reason)
	}
}

// 壊れた設定は最小限に補正する。Min 0 や Max < Min で破綻しない。
func TestConfigIsSanitized(t *testing.T) {
	a := New(Config{Target: 50, Min: 0, Max: -3, Tolerance: 0, StabilizeDown: 0})
	if a.cfg.Min != 1 || a.cfg.Max != 1 || a.cfg.StabilizeDown != 1 {
		t.Fatalf("補正されていない: %+v", a.cfg)
	}
	if d := a.Decide(Sample{Replicas: 1, Utilization: 100}); d.To != 1 {
		t.Fatalf("Min=Max=1 なら動かないはずが %d", d.To)
	}
}

// バーストの一連の流れ。上がるときは速く、下がるときは遅い。
// この非対称が、可用性とコストの折り合いになっている。
func TestBurstThenSettle(t *testing.T) {
	a := New(std())
	rep := 2

	// バースト到来。1 回で必要数まで跳ぶ。
	d := a.Decide(Sample{Replicas: rep, Utilization: 100})
	rep = d.To
	if rep != 4 {
		t.Fatalf("バーストで 4 個へ跳ぶはずが %d", rep)
	}

	// 負荷が引いた。すぐには縮まない。
	for i := 0; i < 2; i++ {
		d = a.Decide(Sample{Replicas: rep, Utilization: 10})
		if d.Changed() {
			t.Fatalf("引いた直後に縮んだ(%d 回目): %d", i+1, d.To)
		}
	}
	// 低いまま続けば縮む。
	d = a.Decide(Sample{Replicas: rep, Utilization: 10})
	if d.To != 1 {
		t.Fatalf("落ち着いたら下限 1 まで縮むはずが %d(%s)", d.To, d.Reason)
	}
}

// 式の結果が現在と同じなら動かない(許容誤差の外側でも)。
func TestNoChangeWhenFormulaAgrees(t *testing.T) {
	a := New(Config{Target: 50, Min: 1, Max: 10, Tolerance: 0, StabilizeDown: 1})
	d := a.Decide(Sample{Replicas: 4, Utilization: 50})
	if d.Changed() {
		t.Fatalf("式が 4 を出したのに動いた: %d(%s)", d.To, d.Reason)
	}
}
