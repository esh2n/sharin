package clusterautoscaler

import (
	"testing"

	"github.com/esh2n/sharin/orchestration/scheduler"
)

// std は 1 台 2000m/2048Mi のノードを 1〜5 台、起動に 3 周期かかる設定。
func std() Config {
	return Config{
		NodeCap:       scheduler.Resources{CPU: 2000, Mem: 2048},
		MinNodes:      1,
		MaxNodes:      5,
		BootTicks:     3,
		ScaleDownUtil: 40,
		Strategy:      scheduler.BinPack,
	}
}

func small() scheduler.Resources { return scheduler.Resources{CPU: 500, Mem: 512} }

func run(a *Autoscaler, ticks int) *Autoscaler {
	for i := 0; i < ticks; i++ {
		a.Tick()
	}
	return a
}

// ノードに空きがあるうちは、増やさずに置ける。
func TestPlacesWithoutScalingUp(t *testing.T) {
	a := New(std())
	for i := 0; i < 4; i++ {
		a.Submit(small())
	}
	run(a, 5)
	if len(a.Nodes()) != 1 {
		t.Fatalf("1 台で足りるはずが %d 台", len(a.Nodes()))
	}
	if len(a.Pending()) != 0 {
		t.Fatalf("全部置けるはずが Pending %v", a.Pending())
	}
}

// 空きがなくなって Pending が出ると、ノードを足す。
// ただし起動には時間がかかり、その間 Pod は待つ。
func TestScalesUpForPendingPods(t *testing.T) {
	a := New(std())
	for i := 0; i < 6; i++ {
		a.Submit(small()) // 1 台には 4 つしか載らない
	}
	if len(a.Pending()) != 2 {
		t.Fatalf("2 つ溢れるはずが %v", a.Pending())
	}

	a.Tick() // 1 周期目でノードを足す判断をする
	if a.Booting() != 1 {
		t.Fatalf("ノードが起動中になるはずが %d", a.Booting())
	}
	if len(a.Pending()) != 2 {
		t.Fatal("起動中のノードにはまだ置けないはず")
	}

	run(a, 5)
	if len(a.Pending()) != 0 {
		t.Fatalf("起動後に置けるはずが Pending %v\n%v", a.Pending(), a.Log)
	}
	if len(a.Nodes()) != 2 {
		t.Fatalf("2 台になるはずが %d 台", len(a.Nodes()))
	}
}

// 1 台に収まらない要求は、何台足しても置けない。
// だから増やす前に確かめ、無駄なノードを作らない。
func TestDoesNotScaleUpForImpossiblePod(t *testing.T) {
	a := New(std())
	a.Submit(scheduler.Resources{CPU: 9000, Mem: 9000}) // 1 台の容量を超える
	run(a, 10)
	if len(a.Nodes())+a.Booting() != 1 {
		t.Fatalf("増やしても無駄なので増やさないはずが %d 台", len(a.Nodes())+a.Booting())
	}
	if len(a.Pending()) != 1 {
		t.Fatal("置けないまま Pending に残るはず")
	}
}

// 上限に達したらそれ以上増やさない。Pending は残る。
func TestRespectsMaxNodes(t *testing.T) {
	cfg := std()
	cfg.MaxNodes = 2
	a := New(cfg)
	for i := 0; i < 12; i++ {
		a.Submit(small())
	}
	run(a, 20)
	if len(a.Nodes()) != 2 {
		t.Fatalf("上限 2 台のはずが %d 台", len(a.Nodes()))
	}
	if len(a.Pending()) == 0 {
		t.Fatal("上限を超える分は Pending に残るはず")
	}
}

// 使用率の低いノードは、載っている Pod が他へ移せるなら消す。
func TestScalesDownWhenPodsFitElsewhere(t *testing.T) {
	cfg := std()
	cfg.BootTicks = 0
	a := New(cfg)
	for i := 0; i < 6; i++ {
		a.Submit(small())
	}
	run(a, 6)
	if len(a.Nodes()) != 2 {
		t.Fatalf("いったん 2 台になるはずが %d 台", len(a.Nodes()))
	}

	// 半分を消して、1 台に収まる状態にする。
	a.Remove("pod-5")
	a.Remove("pod-6")
	run(a, 6)
	if len(a.Nodes()) != 1 {
		t.Fatalf("1 台に集約できるはずが %d 台\n%v", len(a.Nodes()), a.Log)
	}
	if len(a.Placement()) != 4 {
		t.Fatalf("4 つの Pod が残るはずが %d", len(a.Placement()))
	}
}

// 使用率が低くても、載っている Pod の行き先がなければ消さない。
func TestKeepsNodeWhenPodsCannotMove(t *testing.T) {
	cfg := std()
	cfg.BootTicks = 0
	cfg.ScaleDownUtil = 90 // ほぼ全ノードが縮小候補になる厳しい設定
	a := New(cfg)
	// 2 台がそれぞれ半分以上埋まっていて、1 台には集約できない状態を作る。
	for i := 0; i < 6; i++ {
		a.Submit(small())
	}
	run(a, 6)
	before := len(a.Nodes())
	run(a, 10)
	if len(a.Nodes()) != before {
		t.Fatalf("集約できないので減らないはずが %d → %d\n%v", before, len(a.Nodes()), a.Log)
	}
}

// 下限より下には減らさない。
func TestRespectsMinNodes(t *testing.T) {
	cfg := std()
	cfg.MinNodes = 2
	cfg.BootTicks = 0
	a := New(cfg)
	run(a, 10)
	if len(a.Nodes()) != 2 {
		t.Fatalf("下限 2 台を保つはずが %d 台", len(a.Nodes()))
	}
}

// 置けていない Pod があるうちは減らさない。増やす側と減らす側が
// 同時に動くと、足しては消しての繰り返しになる。
func TestDoesNotScaleDownWhilePending(t *testing.T) {
	cfg := std()
	cfg.MaxNodes = 1 // 増やせないので Pending が残り続ける
	a := New(cfg)
	for i := 0; i < 6; i++ {
		a.Submit(small())
	}
	run(a, 10)
	if len(a.Nodes()) != 1 {
		t.Fatalf("Pending がある間は減らさないはずが %d 台", len(a.Nodes()))
	}
	if len(a.Pending()) == 0 {
		t.Fatal("Pending が残っているはず")
	}
}

// 縮小の判定は決定的。同じ入力なら何度走らせても同じ結果になる。
func TestDeterministic(t *testing.T) {
	build := func() *Autoscaler {
		cfg := std()
		cfg.BootTicks = 0
		a := New(cfg)
		for i := 0; i < 7; i++ {
			a.Submit(small())
		}
		run(a, 8)
		a.Remove("pod-6")
		a.Remove("pod-7")
		run(a, 8)
		return a
	}
	first := build()
	for i := 0; i < 10; i++ {
		if got := build(); len(got.Nodes()) != len(first.Nodes()) {
			t.Fatalf("結果がぶれた: %d vs %d", len(got.Nodes()), len(first.Nodes()))
		}
	}
}

func TestUtilAndItoa(t *testing.T) {
	n := scheduler.NewNode("n", scheduler.Resources{CPU: 1000, Mem: 1000})
	if util(n) != 0 {
		t.Fatalf("空なら 0%% のはずが %d", util(n))
	}
	if pct(1, 0) != 0 {
		t.Fatal("容量 0 は 0%% 扱いのはず")
	}
	if itoa(0) != "0" || itoa(305) != "305" {
		t.Fatal("itoa が違う")
	}
}
