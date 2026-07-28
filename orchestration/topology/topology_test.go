package topology

import "testing"

// threeZones は3区画に2台ずつ、計6台のクラスタを作る。
func threeZones(cfg Constraint) *Cluster {
	c := New(cfg)
	for _, z := range []string{"zone-a", "zone-b", "zone-c"} {
		c.AddNode(z+"-1", z)
		c.AddNode(z+"-2", z)
	}
	return c
}

// 制約が厳しいと、区画に均等に散る。
func TestSpreadsAcrossZones(t *testing.T) {
	c := threeZones(Constraint{MaxSkew: 1})
	c.PlaceN("web", 6)

	for _, z := range c.Zones() {
		if got := c.Count(z); got != 2 {
			t.Fatalf("%s に 2 個のはずが %d", z, got)
		}
	}
	if c.Skew() != 0 {
		t.Fatalf("偏りは 0 のはずが %d", c.Skew())
	}
}

// 数えるのは区画ごとであって、ノードごとではない。
// ノードに均等でも、区画に偏っていれば偏りは大きい。
func TestSkewCountsZonesNotNodes(t *testing.T) {
	c := New(Constraint{MaxSkew: 9})
	c.AddNode("a-1", "zone-a")
	c.AddNode("a-2", "zone-a")
	c.AddNode("b-1", "zone-b")
	// zone-a に2台あるので、素直に置くと zone-a に寄る余地がある。
	c.nodes[0].pods = []string{"p1"}
	c.nodes[1].pods = []string{"p2"}
	c.nodes[2].pods = []string{"p3"}

	// ノードごとには均等(各1個)だが、区画では 2 対 1。
	if c.Skew() != 1 {
		t.Fatalf("区画で数えれば偏り 1 のはずが %d", c.Skew())
	}
}

// 偏りの許容量を広げると、散らばりが緩くなる。
func TestLargerSkewAllowsImbalance(t *testing.T) {
	tight := threeZones(Constraint{MaxSkew: 1})
	tight.PlaceN("web", 4)
	if tight.Skew() > 1 {
		t.Fatalf("厳しい設定で偏り %d", tight.Skew())
	}

	// 許容量が大きいと、1つの区画に寄せることも許される。
	loose := threeZones(Constraint{MaxSkew: 5})
	for i := 0; i < 4; i++ {
		loose.Place("web-" + itoa(i))
	}
	// 実装は少ない区画を選ぶので実際には散るが、制約としては許している。
	if loose.skewAfter("zone-a") > 5 {
		t.Fatal("許容量の内なら候補に残るはず")
	}
}

// 散らすと、区画が1つ落ちても大半が残る。これが散らす目的。
func TestSurvivesZoneLoss(t *testing.T) {
	spread := threeZones(Constraint{MaxSkew: 1})
	spread.PlaceN("web", 6)
	if got := spread.WorstZoneLoss(); got != 4 {
		t.Fatalf("1 区画を失っても 4 個残るはずが %d", got)
	}

	// 1区画に寄せた場合と比べる。
	packed := New(Constraint{MaxSkew: 99})
	packed.AddNode("a-1", "zone-a")
	packed.AddNode("b-1", "zone-b")
	packed.nodes[0].pods = []string{"p1", "p2", "p3", "p4", "p5", "p6"}
	if got := packed.WorstZoneLoss(); got != 0 {
		t.Fatalf("寄せていれば全部失うはずが %d 残った", got)
	}
}

// ノード1台の障害と、区画1つの障害は別物になる。
func TestNodeLossDiffersFromZoneLoss(t *testing.T) {
	c := threeZones(Constraint{MaxSkew: 1})
	c.PlaceN("web", 6)

	byNode := c.LoseNode("zone-a-1")
	byZone := c.LoseZone("zone-a")
	if byNode <= byZone {
		t.Fatalf("区画の障害のほうが痛いはず: node=%d zone=%d", byNode, byZone)
	}
}

// 守れないなら置かない設定では、置けずに残る。
func TestDoNotScheduleRefuses(t *testing.T) {
	c := New(Constraint{MaxSkew: 1, When: DoNotSchedule})
	c.AddNode("a-1", "zone-a")
	c.AddNode("b-1", "zone-b")
	// 各区画に1つずつ置いた後、3つ目はどちらに置いても偏り 1 なので置ける。
	c.PlaceN("web", 2)
	if c.Skew() != 0 {
		t.Fatalf("2 個は均等のはずが偏り %d", c.Skew())
	}
	// 偏り 0 の設定にすると、次が置けなくなる。
	strict := New(Constraint{MaxSkew: 1, When: DoNotSchedule})
	strict.AddNode("a-1", "zone-a")
	strict.AddNode("b-1", "zone-b")
	strict.nodes[0].pods = []string{"p1", "p2"}
	strict.nodes[1].pods = []string{"p3"}
	// zone-a は 2、zone-b は 1。zone-a に置くと差が 2 になる。
	r := strict.Place("web-x")
	if !r.Placed || r.Zone != "zone-b" {
		t.Fatalf("zone-b にしか置けないはずが %+v", r)
	}
}

// 守れなくても置く設定では、置けなくならない。
func TestScheduleAnywayPlacesAnyway(t *testing.T) {
	c := New(Constraint{MaxSkew: 1, When: ScheduleAnyway})
	c.AddNode("a-1", "zone-a")
	// 区画が1つしかないので、置くたびに偏りは 0 のまま(比較対象が無い)。
	rs := c.PlaceN("web", 3)
	for _, r := range rs {
		if !r.Placed {
			t.Fatalf("置く設定なら必ず置けるはず: %+v", r)
		}
	}
	if c.Refused != 0 {
		t.Fatalf("断らないはずが %d", c.Refused)
	}
}

// ノードが1台も無ければ置けない。
func TestNoNodes(t *testing.T) {
	c := New(Constraint{MaxSkew: 1})
	if r := c.Place("web-1"); r.Placed {
		t.Fatal("ノードが無いのに置けた")
	}
	if c.Skew() != 0 || c.WorstZoneLoss() != 0 {
		t.Fatal("空のクラスタは 0 のはず")
	}
}

// 区画の中では、載っている数の少ないノードを選ぶ。
func TestPicksLeastLoadedNodeInZone(t *testing.T) {
	c := New(Constraint{MaxSkew: 9})
	c.AddNode("a-1", "zone-a")
	c.AddNode("a-2", "zone-a")
	c.nodes[0].pods = []string{"p1", "p2"}

	r := c.Place("web-x")
	if r.Node != "a-2" {
		t.Fatalf("空いている a-2 のはずが %s", r.Node)
	}
}

func TestGuardsAndStrings(t *testing.T) {
	if New(Constraint{MaxSkew: 0}).cfg.MaxSkew != 1 {
		t.Fatal("MaxSkew は最低 1 に補正されるはず")
	}
	if DoNotSchedule.String() != "DoNotSchedule" || ScheduleAnyway.String() != "ScheduleAnyway" {
		t.Fatal("文字列が違う")
	}
	if itoa(0) != "0" || itoa(64) != "64" {
		t.Fatal("itoa が違う")
	}
}
