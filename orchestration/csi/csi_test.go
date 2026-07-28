package csi

import "testing"

func fast() Class  { return Class{Name: "fast", Binding: Immediate, Reclaim: Delete} }
func kept() Class  { return Class{Name: "kept", Binding: Immediate, Reclaim: Retain} }
func zoned() Class { return Class{Name: "zoned", Binding: WaitForFirstConsumer, Reclaim: Delete} }

func claim(name, class string, mode AccessMode) Claim {
	return Claim{Name: name, Class: class, Size: 20, Mode: mode}
}

// 使う側は場所を知らない。要求を書けば、実体は別の担当が用意する。
func TestRequestCreatesTheVolume(t *testing.T) {
	d := New(fast())
	if ok, why := d.Request(claim("data", "fast", ReadWriteOnce)); !ok {
		t.Fatalf("要求が通らない: %s", why)
	}
	v := d.VolumeFor("data")
	if v == nil {
		t.Fatal("実体ができていない")
	}
	if v.Size != 20 || v.Mode != ReadWriteOnce {
		t.Fatalf("要求と違う実体ができた: %+v", v)
	}
	if ok, why := d.Request(claim("x", "nope", ReadWriteOnce)); ok || why == "" {
		t.Fatal("知らない class が通った")
	}
}

// この章の中心。ReadWriteOnce は「1つのノードから」であって「1つの Pod から」ではない。
func TestReadWriteOnceIsPerNodeNotPerPod(t *testing.T) {
	d := New(fast())
	d.Request(claim("data", "fast", ReadWriteOnce))

	if ok, why := d.Attach("data", "node-a", ""); !ok {
		t.Fatalf("繋がらない: %s", why)
	}
	// 同じノードの上なら、何個の Pod からでも見える。
	for _, p := range []string{"web-1", "web-2", "web-3"} {
		if ok, why := d.Mount("data", "node-a", p); !ok {
			t.Fatalf("%s から見えない: %s", p, why)
		}
	}
	if len(d.VolumeFor("data").MountedBy) != 3 {
		t.Fatalf("同じノードの Pod が共有できていない: %v", d.VolumeFor("data").MountedBy)
	}

	// だが別のノードには繋げない。
	if ok, why := d.Attach("data", "node-b", ""); ok || why == "" {
		t.Fatal("別のノードに繋がってしまった")
	}
	// 繋がっていないノードの Pod からは見えない。
	if ok, why := d.Mount("data", "node-b", "web-4"); ok || why == "" {
		t.Fatal("繋がっていないノードから見えてしまった")
	}
}

// 複数ノードから使える設定なら、繋ぐところで止まらない。
func TestReadWriteManyAttachesEverywhere(t *testing.T) {
	d := New(fast())
	d.Request(claim("shared", "fast", ReadWriteMany))

	if ok, _ := d.Attach("shared", "node-a", ""); !ok {
		t.Fatal("最初のノードに繋がらない")
	}
	if ok, why := d.Attach("shared", "node-b", ""); !ok {
		t.Fatalf("2 台目に繋がらない: %s", why)
	}
	if ReadOnlyMany.String() != "ReadOnlyMany" {
		t.Fatal("名前が違う")
	}
}

// 3段は順序がある。作る前に繋げないし、繋ぐ前に見えない。
func TestStagesAreOrdered(t *testing.T) {
	d := New(fast())
	if ok, why := d.Attach("nope", "node-a", ""); ok || why == "" {
		t.Fatal("要求が無いのに繋がった")
	}
	d.Request(claim("data", "fast", ReadWriteOnce))
	if ok, why := d.Mount("data", "node-a", "web"); ok || why == "" {
		t.Fatal("繋ぐ前に見えた")
	}
	d.Attach("data", "node-a", "")
	if ok, _ := d.Mount("data", "node-a", "web"); !ok {
		t.Fatal("繋いだのに見えない")
	}
	// 二度目は何も起きない。
	d.Attach("data", "node-a", "")
	d.Mount("data", "node-a", "web")
	if len(d.VolumeFor("data").MountedBy) != 1 {
		t.Fatalf("二度目で増えた: %v", d.VolumeFor("data").MountedBy)
	}
}

// 見えている Pod がある限り、ノードから外せない。
// 外れないことが、引き継ぎの遅さの正体になる。
func TestDetachBlockedWhileMounted(t *testing.T) {
	d := New(fast())
	d.Request(claim("data", "fast", ReadWriteOnce))
	d.Attach("data", "node-a", "")
	d.Mount("data", "node-a", "web")

	if ok, why := d.Detach("data"); ok || why == "" {
		t.Fatal("見えているのに外せた")
	}
	d.Unmount("data", "web")
	if ok, why := d.Detach("data"); !ok {
		t.Fatalf("見えなくなったのに外せない: %s", why)
	}
	// 外れたので、別のノードに繋げるようになる。
	if ok, why := d.Attach("data", "node-b", ""); !ok {
		t.Fatalf("外したのに繋がらない: %s", why)
	}
}

// ノードが応答しないときは、待たずに外すしかない。
func TestForceDetachIsTheLastResort(t *testing.T) {
	d := New(fast())
	d.Request(claim("data", "fast", ReadWriteOnce))
	d.Attach("data", "node-a", "")
	d.Mount("data", "node-a", "web")

	if ok, _ := d.Detach("data"); ok {
		t.Fatal("普通に外せてしまった")
	}
	d.ForceDetach("data")
	if v := d.VolumeFor("data"); v.AttachedTo != "" || len(v.MountedBy) != 0 {
		t.Fatalf("外れていない: %+v", v)
	}
	if ok, _ := d.Attach("data", "node-b", ""); !ok {
		t.Fatal("外した後に繋がらない")
	}
	// 知らない要求への操作は何も起こさない。
	d.ForceDetach("nope")
	d.Unmount("nope", "web")
	if ok, _ := d.Detach("nope"); ok {
		t.Fatal("知らない要求を外せた")
	}
}

// 先に作ると区画が固定される。区画をまたぐノードには繋がらない。
func TestImmediateBindingCanStrandTheVolume(t *testing.T) {
	d := New(fast())
	c := claim("data", "fast", ReadWriteOnce)
	c.Zone = "zone-a"
	d.Request(c)

	if v := d.VolumeFor("data"); v.Zone != "zone-a" {
		t.Fatalf("区画が固定されていない: %+v", v)
	}
	if ok, why := d.Attach("data", "node-b", "zone-b"); ok || why == "" {
		t.Fatal("別の区画のノードに繋がった")
	}
	if ok, why := d.Attach("data", "node-a", "zone-a"); !ok {
		t.Fatalf("同じ区画なのに繋がらない: %s", why)
	}
}

// 待つ設定なら、置き場所が決まってから作るので、区画がずれない。
func TestWaitForFirstConsumerFollowsThePod(t *testing.T) {
	d := New(zoned())
	d.Request(claim("data", "zoned", ReadWriteOnce))

	if d.VolumeFor("data") != nil {
		t.Fatal("使う Pod が決まる前に作ってしまった")
	}
	if ok, why := d.Attach("data", "node-b", "zone-b"); !ok {
		t.Fatalf("置き場所が決まったのに作られない: %s", why)
	}
	v := d.VolumeFor("data")
	if v == nil || v.Zone != "zone-b" {
		t.Fatalf("Pod の区画に合わせて作られていない: %+v", v)
	}
	if v.AttachedTo != "node-b" {
		t.Fatalf("繋がっていない: %+v", v)
	}
}

// 実体がまだ無い状態で見ようとしても何も起きない。
func TestMountBeforeProvisionFails(t *testing.T) {
	d := New(zoned())
	d.Request(claim("data", "zoned", ReadWriteOnce))
	if ok, why := d.Mount("data", "node-a", "web"); ok || why == "" {
		t.Fatal("実体が無いのに見えた")
	}
	if ok, why := d.Detach("data"); ok || why == "" {
		t.Fatal("実体が無いのに外せた")
	}
}

// 要求を消したときに実体をどうするかは、class が決めてある。
func TestReclaimPolicy(t *testing.T) {
	d := New(fast(), kept())
	d.Request(claim("temp", "fast", ReadWriteOnce))
	d.Request(claim("keep", "kept", ReadWriteOnce))

	d.DeleteClaim("temp")
	if d.VolumeFor("temp") != nil {
		t.Fatal("消す設定なのに残った")
	}
	if len(d.Volumes()) != 1 {
		t.Fatalf("実体の数が違う: %d", len(d.Volumes()))
	}

	d.DeleteClaim("keep")
	if d.VolumeFor("keep") != nil {
		t.Fatal("要求から切り離されていない")
	}
	if len(d.Volumes()) != 1 || !d.Volumes()[0].Released {
		t.Fatalf("残す設定なのに消えた: %v", d.Volumes())
	}
	d.DeleteClaim("nope") // 知らない要求
}

// 実体は名前順で返る。
func TestVolumesAreOrdered(t *testing.T) {
	d := New(fast())
	for _, n := range []string{"a", "b", "c"} {
		d.Request(claim(n, "fast", ReadWriteOnce))
	}
	got := d.Volumes()
	for i := 1; i < len(got); i++ {
		if got[i-1].Name >= got[i].Name {
			t.Fatalf("名前順でない: %v", got)
		}
	}
	if orNone("") == "" || orNone("node-a") != "node-a" {
		t.Fatal("表示が違う")
	}
	if itoa(0) != "0" || itoa(20) != "20" {
		t.Fatal("itoa が違う")
	}
	if len(d.Log) == 0 {
		t.Fatal("記録が残っていない")
	}
}

// 繋がっていない実体を外しても、何も壊れない。
func TestDetachWhenNotAttached(t *testing.T) {
	d := New(fast())
	d.Request(claim("data", "fast", ReadWriteOnce))
	if ok, why := d.Detach("data"); !ok {
		t.Fatalf("繋がっていないのに外せない: %s", why)
	}

	// 見えていない Pod を外しても、他は残る。
	d.Attach("data", "node-a", "")
	d.Mount("data", "node-a", "web-1")
	d.Mount("data", "node-a", "web-2")
	d.Unmount("data", "web-9") // 居ない Pod
	if len(d.VolumeFor("data").MountedBy) != 2 {
		t.Fatalf("居ない Pod を外して他が消えた: %v", d.VolumeFor("data").MountedBy)
	}
	d.Unmount("data", "web-1")
	if len(d.VolumeFor("data").MountedBy) != 1 {
		t.Fatalf("外れていない: %v", d.VolumeFor("data").MountedBy)
	}
}
