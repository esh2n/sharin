package service

import "testing"

const vip = "10.0.0.1"

func web() map[string]string { return map[string]string{"app": "web"} }

// build は Service 1つ・ノード2台・web な Pod を n 個の状態を作り、
// ルールが行き渡るまで進める。
func build(prop, n int) (*Cluster, []*Node) {
	c := New(Config{Propagation: prop})
	na, nb := c.AddNode("node-a"), c.AddNode("node-b")
	c.AddService("web", vip, web())
	for i := 1; i <= n; i++ {
		c.AddPod("web-"+itoa(i), "10.1.0."+itoa(i), web(), true)
	}
	for i := 0; i < prop+1; i++ {
		c.Tick()
	}
	return c, []*Node{na, nb}
}

// セレクタのラベルが合い、ready な Pod だけが宛先になる。
func TestSelectorPicksReadyMatchingPods(t *testing.T) {
	c := New(Config{})
	c.AddService("web", vip, web())
	c.AddPod("web-1", "10.1.0.1", web(), true)
	c.AddPod("web-2", "10.1.0.2", web(), false)                                    // ready でない
	c.AddPod("db-1", "10.1.0.3", map[string]string{"app": "db"}, true)             // ラベルが違う
	c.AddPod("web-3", "10.1.0.4", map[string]string{"app": "web", "t": "x"}, true) // 余分なラベルは可

	got := c.Endpoints("web")
	if len(got) != 2 || got[0] != "10.1.0.1" || got[1] != "10.1.0.4" {
		t.Fatalf("web-1 と web-3 だけのはずが %v", got)
	}
	if c.Endpoints("nosuch") != nil {
		t.Fatal("存在しない Service は空のはず")
	}
}

// 仮想 IP に対応する実体はどこにもない。あるのは各ノードのルールだけ。
func TestVirtualIPHasNoBackingPod(t *testing.T) {
	c, nodes := build(0, 2)
	if c.podByIP(vip) != nil {
		t.Fatal("ClusterIP を持つ Pod は存在しないはず")
	}
	for _, n := range nodes {
		if len(n.Rules(vip)) != 2 {
			t.Fatalf("%s にルールが2本あるはずが %v", n.Name, n.Rules(vip))
		}
	}
}

// 振り分けはノードごとに独立して起こる。中央に集約点がない。
func TestEachNodeRoutesIndependently(t *testing.T) {
	c, nodes := build(0, 2)
	a, b := nodes[0], nodes[1]

	// 同じ回数だけ送れば、どちらのノードも同じ順で選ぶ。
	ipA1, _ := a.Route(vip)
	ipB1, _ := b.Route(vip)
	if ipA1 != ipB1 {
		t.Fatalf("初回は同じ相手を選ぶはず: %s vs %s", ipA1, ipB1)
	}
	// node-a だけ進めると、以降の選び方はノードごとにずれる。
	a.Route(vip)
	ipA3, _ := a.Route(vip)
	ipB2, _ := b.Route(vip)
	if ipA3 == ipB2 {
		t.Fatal("ノードごとに順番を持つので、ずれるはず")
	}
	if c.Sent != 0 {
		t.Fatal("Route を直接呼んだだけでは数えない")
	}
}

// ルールが行き渡っていれば、パケットは生きた宛先に届く。
func TestSendReachesLivePod(t *testing.T) {
	c, nodes := build(1, 3)
	for i := 0; i < 6; i++ {
		if !c.Send(nodes[i%2], vip) {
			t.Fatalf("届くはずが失敗した\n%v", c.Log)
		}
	}
	if c.Sent != 6 || c.Blackholed != 0 || c.Dropped != 0 {
		t.Fatalf("全部届くはず: sent=%d black=%d drop=%d", c.Sent, c.Blackholed, c.Dropped)
	}
}

// Pod を消しても、ルールが配り終わるまでは古い宛先が残る。
// その間に出したパケットは、もう受けられない相手へ書き換えられる。
func TestStaleRulesSendToDeadPod(t *testing.T) {
	c, nodes := build(3, 2)
	c.RemovePod("web-1")

	// 配布前。ルールにはまだ web-1 の IP が残っている。
	if len(nodes[0].Rules(vip)) != 2 {
		t.Fatalf("配布前はまだ2本のはずが %v", nodes[0].Rules(vip))
	}
	for i := 0; i < 4; i++ {
		c.Send(nodes[0], vip)
	}
	if c.Blackholed == 0 {
		t.Fatalf("消えた宛先へ飛ぶはずが 0 件\n%v", c.Log)
	}

	// 配り終われば、その宛先は選ばれなくなる。
	for i := 0; i < 4; i++ {
		c.Tick()
	}
	before := c.Blackholed
	for i := 0; i < 4; i++ {
		c.Send(nodes[0], vip)
	}
	if c.Blackholed != before {
		t.Fatalf("配布後は届くはずが %d 件増えた\n%v", c.Blackholed-before, c.Log)
	}
	if !c.Converged() {
		t.Fatal("全ノードのルールが一致しているはず")
	}
}

// ready を落とすだけでも宛先から外れる。Pod は生きたまま。
func TestUnreadyPodLeavesRules(t *testing.T) {
	c, nodes := build(1, 2)
	c.SetReady("web-1", false)
	for i := 0; i < 2; i++ {
		c.Tick()
	}
	if got := nodes[0].Rules(vip); len(got) != 1 || got[0] != "10.1.0.2" {
		t.Fatalf("web-2 だけが残るはずが %v", got)
	}
	c.SetReady("web-1", true)
	for i := 0; i < 2; i++ {
		c.Tick()
	}
	if len(nodes[0].Rules(vip)) != 2 {
		t.Fatal("戻せば宛先に復帰するはず")
	}
}

// 宛先が1つも無くなると、ルール自体が消える。書き換え先がないので届かない。
func TestNoEndpointsDropsPacket(t *testing.T) {
	c, nodes := build(1, 1)
	c.RemovePod("web-1")
	for i := 0; i < 2; i++ {
		c.Tick()
	}
	if len(nodes[0].Rules(vip)) != 0 {
		t.Fatalf("ルールが消えるはずが %v", nodes[0].Rules(vip))
	}
	if c.Send(nodes[0], vip) {
		t.Fatal("届くはずがない")
	}
	if c.Dropped != 1 {
		t.Fatalf("行き場を失うはずが drop=%d black=%d", c.Dropped, c.Blackholed)
	}
}

// 後から足したノードにも、同じルールが配られる。
func TestNewNodeReceivesRules(t *testing.T) {
	c, _ := build(1, 2)
	late := c.AddNode("node-c")
	if len(late.Rules(vip)) != 0 {
		t.Fatal("足した直後はまだ空のはず")
	}
	for i := 0; i < 2; i++ {
		c.Tick()
	}
	if len(late.Rules(vip)) != 2 {
		t.Fatalf("配布後は2本のはずが %v", late.Rules(vip))
	}
}

// ルールの本数は Service と宛先の積で増える。この増え方が
// iptables 方式の重さの正体になる。
func TestRuleCountGrowsWithServicesAndPods(t *testing.T) {
	c := New(Config{})
	n := c.AddNode("node-a")
	for s := 1; s <= 3; s++ {
		c.AddService("svc-"+itoa(s), "10.0.0."+itoa(s), map[string]string{"app": "svc-" + itoa(s)})
		for p := 1; p <= 4; p++ {
			c.AddPod("svc-"+itoa(s)+"-"+itoa(p), "10.1."+itoa(s)+"."+itoa(p),
				map[string]string{"app": "svc-" + itoa(s)}, true)
		}
	}
	c.Tick()
	if n.RuleCount() != 12 {
		t.Fatalf("3 Service × 4 宛先 = 12 本のはずが %d", n.RuleCount())
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(10) != "10" || itoa(255) != "255" {
		t.Fatal("itoa が違う")
	}
}
