package admission

import "testing"

// addTeamLabel は team ラベルが無ければ足す書き換え。
func addTeamLabel(failure FailurePolicy, available bool) *Webhook {
	return &Webhook{
		Name: "add-team-label", Stage: Mutating, Kinds: []string{"Pod"},
		Available: available, Failure: failure,
		Mutate: func(o *Object) string {
			if o.Labels["team"] != "" {
				return ""
			}
			o.Labels["team"] = "unknown"
			return "team=unknown を付けた"
		},
	}
}

// requireTeamLabel は team ラベルが無ければ拒否する検証。
func requireTeamLabel(failure FailurePolicy, available bool) *Webhook {
	return &Webhook{
		Name: "require-team-label", Stage: Validating, Kinds: []string{"Pod"},
		Available: available, Failure: failure,
		Check: func(o *Object) string {
			if o.Labels["team"] == "" {
				return "team ラベルが無い"
			}
			return ""
		},
	}
}

// 書き換えが先、検証が後。だから書き換えで足したラベルが検証を通る。
func TestMutateRunsBeforeValidate(t *testing.T) {
	c := New()
	// あえて検証を先に足しても、段の順序で書き換えが先に走る。
	c.Add(requireTeamLabel(Fail, true))
	c.Add(addTeamLabel(Fail, true))

	r := c.Admit(NewObject("Pod", "web-1"))
	if !r.Allowed {
		t.Fatalf("書き換えで足したラベルが検証を通るはず: %s\n%v", r.Reason, c.Log)
	}
	if r.Object.Labels["team"] != "unknown" {
		t.Fatalf("書き換えが反映されるはずが %q", r.Object.Labels["team"])
	}
	if len(r.Applied) != 1 {
		t.Fatalf("書き換えの記録が残るはずが %v", r.Applied)
	}
}

// 書き換えが無ければ、同じ検証で拒否される。
func TestValidateRejectsWithoutMutation(t *testing.T) {
	c := New()
	c.Add(requireTeamLabel(Fail, true))

	r := c.Admit(NewObject("Pod", "web-1"))
	if r.Allowed {
		t.Fatal("ラベルが無いので拒否されるはず")
	}
	if c.Rejected != 1 {
		t.Fatalf("拒否が数えられるはずが %d", c.Rejected)
	}
}

// 元のオブジェクトは書き換えられない。関門を通った複製が返る。
func TestOriginalIsNotMutated(t *testing.T) {
	c := New()
	c.Add(addTeamLabel(Fail, true))

	in := NewObject("Pod", "web-1")
	r := c.Admit(in)
	if in.Labels["team"] != "" {
		t.Fatal("元のオブジェクトが書き換えられている")
	}
	if r.Object.Labels["team"] != "unknown" {
		t.Fatal("複製のほうは書き換わっているはず")
	}
}

// 対象の種類が違えば、関門は当たらない。
func TestKindFilter(t *testing.T) {
	c := New()
	c.Add(requireTeamLabel(Fail, true))

	if r := c.Admit(NewObject("Service", "web")); !r.Allowed {
		t.Fatalf("Pod 向けの関門は Service に当たらないはず: %s", r.Reason)
	}
	if r := c.Admit(NewObject("Pod", "web-1")); r.Allowed {
		t.Fatal("Pod には当たるはず")
	}
}

// webhook が落ちているとき、Fail なら拒否し、Ignore なら素通しする。
// どちらも危険で、失うものが違う。
func TestFailurePolicySplitsTheRisk(t *testing.T) {
	strict := New()
	strict.Add(requireTeamLabel(Fail, false))
	obj := NewObject("Pod", "web-1")
	obj.Labels["team"] = "core"
	if r := strict.Admit(obj); r.Allowed {
		t.Fatal("Fail なら、合格するはずのものまで拒否される")
	}

	loose := New()
	loose.Add(requireTeamLabel(Ignore, false))
	if r := loose.Admit(NewObject("Pod", "web-2")); !r.Allowed {
		t.Fatalf("Ignore なら通るはず: %s", r.Reason)
	}
	if loose.Bypassed != 1 {
		t.Fatalf("素通しが数えられるはずが %d", loose.Bypassed)
	}
}

// Ignore は、本来なら拒否されるものを通してしまう。
func TestIgnoreLetsBadObjectsThrough(t *testing.T) {
	c := New()
	c.Add(requireTeamLabel(Ignore, false))
	r := c.Admit(NewObject("Pod", "no-label"))
	if !r.Allowed {
		t.Fatal("Ignore なので通る")
	}
	if r.Object.Labels["team"] != "" {
		t.Fatal("検証されていないので、ラベルは無いまま")
	}
}

// Fail にしていると、webhook 自身を動かす Pod の作成すら止まる。
// 落ちた webhook を直すために Pod を作りたいのに、その作成が止まる。
func TestFailPolicyCanLockOutRecovery(t *testing.T) {
	c := New()
	// この関門は自分自身を含むすべての Pod に当たる。
	c.Add(&Webhook{Name: "policy-webhook", Stage: Validating, Kinds: []string{"Pod"},
		Available: false, Failure: Fail,
		Check: func(o *Object) string { return "" }})

	// webhook 自身を動かす Pod を作り直そうとする。
	r := c.Admit(NewObject("Pod", "policy-webhook-pod"))
	if r.Allowed {
		t.Fatal("この設定では自分自身も作れなくなるはず")
	}

	// 対象から自分を外しておけば、作り直せる。
	fixed := New()
	fixed.Add(&Webhook{Name: "policy-webhook", Stage: Validating, Kinds: []string{"Deployment"},
		Available: false, Failure: Fail,
		Check: func(o *Object) string { return "" }})
	if r := fixed.Admit(NewObject("Pod", "policy-webhook-pod")); !r.Allowed {
		t.Fatalf("対象から外れていれば作れるはず: %s", r.Reason)
	}
}

// 複数の書き換えは順に積み上がる。後の関門は前の結果を見る。
func TestMutationsCompose(t *testing.T) {
	c := New()
	c.Add(addTeamLabel(Fail, true))
	c.Add(&Webhook{Name: "add-owner", Stage: Mutating, Kinds: []string{"Pod"},
		Available: true, Failure: Fail,
		Mutate: func(o *Object) string {
			o.Annotations["owner"] = o.Labels["team"] // 前の書き換えの結果を見る
			return "owner=" + o.Labels["team"] + " を付けた"
		}})

	r := c.Admit(NewObject("Pod", "web-1"))
	if r.Object.Annotations["owner"] != "unknown" {
		t.Fatalf("前の書き換えを見られるはずが %q", r.Object.Annotations["owner"])
	}
	if len(r.Applied) != 2 {
		t.Fatalf("2 つ記録されるはずが %v", r.Applied)
	}
}

// 何度通しても結果は同じ。書き換えは足りないものを足すだけ。
func TestIdempotentMutation(t *testing.T) {
	c := New()
	c.Add(addTeamLabel(Fail, true))

	obj := NewObject("Pod", "web-1")
	obj.Labels["team"] = "core"
	r := c.Admit(obj)
	if r.Object.Labels["team"] != "core" {
		t.Fatalf("すでにあるものは触らないはずが %q", r.Object.Labels["team"])
	}
	if len(r.Applied) != 0 {
		t.Fatalf("何もしていないはずが %v", r.Applied)
	}
}

// 関門を段の順に並べて見せられる。
func TestHooksAreOrderedByStage(t *testing.T) {
	c := New()
	c.Add(requireTeamLabel(Fail, true))
	c.Add(addTeamLabel(Fail, true))
	hooks := c.Hooks()
	if hooks[0].Stage != Mutating || hooks[1].Stage != Validating {
		t.Fatal("書き換えが先に並ぶはず")
	}
}

func TestStrings(t *testing.T) {
	if Mutating.String() != "Mutating" || Validating.String() != "Validating" {
		t.Fatal("Stage の文字列が違う")
	}
	if Fail.String() != "Fail" || Ignore.String() != "Ignore" {
		t.Fatal("FailurePolicy の文字列が違う")
	}
	o := NewObject("Pod", "x")
	o.Labels["b"] = "1"
	o.Labels["a"] = "2"
	if keys := o.Keys(); keys[0] != "a" || keys[1] != "b" {
		t.Fatal("鍵は名前順のはず")
	}
}
