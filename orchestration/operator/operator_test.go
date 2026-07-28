package operator

import "testing"

func res(members int, restoreFrom string) *Resource {
	return &Resource{Spec: Spec{Name: "db", Members: members, RestoreFrom: restoreFrom}}
}

// 宣言した数だけメンバーを作り、リーダーを選んで揃う。
func TestConvergesToSpec(t *testing.T) {
	r, w, o := res(3, ""), NewWorld(), New()
	o.Run(r, w, 20)

	if r.Status.Phase != Ready {
		t.Fatalf("Ready になるはずが %s\n%v", r.Status.Phase, o.Log)
	}
	if r.Status.Members != 3 {
		t.Fatalf("3 メンバーのはずが %d", r.Status.Members)
	}
	if r.Status.Leader != "db-1" {
		t.Fatalf("名前順で db-1 が選ばれるはずが %q", r.Status.Leader)
	}
}

// 揃った後の Reconcile は何もしない。何度呼んでも安全。
func TestIdempotentOnceReady(t *testing.T) {
	r, w, o := res(2, ""), NewWorld(), New()
	o.Run(r, w, 20)
	before := len(w.Members())

	for i := 0; i < 5; i++ {
		if act := o.Reconcile(r, w); act.Kind != "noop" {
			t.Fatalf("揃った後は何もしないはずが %+v", act)
		}
	}
	if len(w.Members()) != before {
		t.Fatalf("メンバー数が変わった: %d → %d", before, len(w.Members()))
	}
}

// 1回の呼び出しで1手しか打たない。途中で止めても状態から再開できる。
func TestOneStepPerCall(t *testing.T) {
	r, w, o := res(3, ""), NewWorld(), New()
	if act := o.Reconcile(r, w); act.Kind != "create" {
		t.Fatalf("1手目は create のはずが %+v", act)
	}
	if len(w.Members()) != 1 {
		t.Fatalf("1つだけ作るはずが %d", len(w.Members()))
	}
	if r.Status.Phase != Creating {
		t.Fatalf("途中の状態が書き戻るはずが %s", r.Status.Phase)
	}
}

// 復元が指定されていれば、メンバーを作る前に復元する。
// 順序のある手順が「まだ済んでいない差」として表現されている。
func TestRestoreHappensBeforeMembers(t *testing.T) {
	w := NewWorld()
	w.PutBackup("backup-2026-07-28")
	r, o := res(2, "backup-2026-07-28"), New()

	act := o.Reconcile(r, w)
	if act.Kind != "restore" {
		t.Fatalf("最初は restore のはずが %+v", act)
	}
	if len(w.Members()) != 0 {
		t.Fatal("復元前にメンバーを作ってはいけない")
	}
	if !r.Status.Restored {
		t.Fatal("復元済みが記録されるはず")
	}

	o.Run(r, w, 20)
	if r.Status.Phase != Ready || r.Status.Members != 2 {
		t.Fatalf("復元後に揃うはずが %s / %d\n%v", r.Status.Phase, r.Status.Members, o.Log)
	}
}

// 復元は一度きり。揃った後に何度呼んでも、もう復元しない。
func TestRestoreOnlyOnce(t *testing.T) {
	w := NewWorld()
	w.PutBackup("b1")
	r, o := res(1, "b1"), New()
	o.Run(r, w, 20)

	for i := 0; i < 3; i++ {
		if act := o.Reconcile(r, w); act.Kind == "restore" {
			t.Fatal("2 度目の復元をしてはいけない")
		}
	}
}

// 復元元が無ければ進めない。差を埋められないことを状態で示す。
func TestMissingBackupIsDegraded(t *testing.T) {
	r, w, o := res(2, "nosuch"), NewWorld(), New()
	o.Run(r, w, 20)
	if r.Status.Phase != Degraded {
		t.Fatalf("Degraded になるはずが %s", r.Status.Phase)
	}
	if len(w.Members()) != 0 {
		t.Fatal("復元できないうちはメンバーを作らないはず")
	}
}

// リーダーが落ちたら選び直す。障害イベントを購読していないのに直る。
func TestReelectsWhenLeaderDies(t *testing.T) {
	r, w, o := res(3, ""), NewWorld(), New()
	o.Run(r, w, 20)
	first := r.Status.Leader

	w.Kill(first)
	o.Run(r, w, 20)

	if r.Status.Leader == first {
		t.Fatalf("リーダーが選び直されるはずが %q のまま", r.Status.Leader)
	}
	if r.Status.Phase != Ready {
		t.Fatalf("選び直して Ready に戻るはずが %s\n%v", r.Status.Phase, o.Log)
	}
	if r.Status.Members != 3 {
		t.Fatalf("落ちた分も作り直されるはずが %d", r.Status.Members)
	}
}

// メンバーがまとめて落ちても、1回ずつ調整すれば全部戻る。
func TestSelfHealsAfterMultipleLosses(t *testing.T) {
	r, w, o := res(3, ""), NewWorld(), New()
	o.Run(r, w, 20)
	for _, m := range w.Members() {
		w.Kill(m.Name)
	}

	o.Run(r, w, 30)
	if r.Status.Phase != Ready || r.Status.Members != 3 {
		t.Fatalf("全滅からも戻るはずが %s / %d\n%v", r.Status.Phase, r.Status.Members, o.Log)
	}
}

// 宣言を増やせば、その差も同じループが埋める。
func TestScalingUpUsesTheSameLoop(t *testing.T) {
	r, w, o := res(2, ""), NewWorld(), New()
	o.Run(r, w, 20)

	r.Spec.Members = 4
	o.Run(r, w, 20)
	if r.Status.Members != 4 {
		t.Fatalf("4 に増えるはずが %d\n%v", r.Status.Members, o.Log)
	}
}

func TestPhaseStrings(t *testing.T) {
	for _, c := range []struct {
		p    Phase
		want string
	}{{Pending, "Pending"}, {Creating, "Creating"}, {Restoring, "Restoring"},
		{Ready, "Ready"}, {Degraded, "Degraded"}, {Phase(9), "Unknown"}} {
		if c.p.String() != c.want {
			t.Fatalf("%d の文字列が %s", c.p, c.p)
		}
	}
	if itoa(0) != "0" || itoa(42) != "42" {
		t.Fatal("itoa が違う")
	}
}
