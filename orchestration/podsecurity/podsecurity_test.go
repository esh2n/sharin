package podsecurity

import (
	"sort"
	"testing"
)

// naive は、権限のことを何も書かずに作った Pod。よくある形。
func naive() Pod {
	return Pod{Name: "web", Namespace: "team-a", Containers: []Container{{Name: "app"}}}
}

// tidy は baseline は通るが、restricted の書き足しをしていない Pod。
func tidy() Pod {
	return naive()
}

// hardened は restricted まで通る Pod。書き足しが要る。
func hardened() Pod {
	return Pod{Name: "hardened", Namespace: "team-a", Containers: []Container{{
		Name: "app",
		Security: SecurityContext{
			AllowPrivilegeEscalation: Bool(false),
			RunAsNonRoot:             Bool(true),
			RunAsUser:                Int64(10001),
			Capabilities:             Capabilities{Drop: []string{"ALL"}},
			SeccompProfile:           "RuntimeDefault",
		},
	}}}
}

// leaky は隔離を外している Pod。監視や収集でよくある形でもある。
func leaky() Pod {
	return Pod{Name: "agent", Namespace: "team-a", HostNetwork: true, HostPID: true, HostIPC: true,
		Containers: []Container{{
			Name:      "agent",
			Ports:     []int{9100},
			HostPaths: []string{"/var/log"},
			Security: SecurityContext{
				Privileged:     true,
				Capabilities:   Capabilities{Add: []string{"SYS_ADMIN"}},
				SeccompProfile: "Unconfined",
			},
		}}}
}

func rules(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Rule)
	}
	sort.Strings(out)
	return out
}

func hasRule(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// 何も書かなければ何も制限されない。既定は許可になっている。
func TestPrivilegedAllowsEverything(t *testing.T) {
	if vs := Check(leaky(), Privileged); len(vs) != 0 {
		t.Fatalf("privileged で違反が出た: %v", rules(vs))
	}
	if vs := Check(naive(), Privileged); len(vs) != 0 {
		t.Fatalf("privileged で違反が出た: %v", rules(vs))
	}
}

// baseline は、隔離を外しているものを塞ぐ。
func TestBaselineBlocksEscapes(t *testing.T) {
	vs := Check(leaky(), Baseline)
	for _, want := range []string{"hostNetwork", "hostPID", "hostIPC", "hostPath", "hostPort", "privileged", "capabilities.add", "seccompProfile"} {
		if !hasRule(vs, want) {
			t.Errorf("%s が検出されていない: %v", want, rules(vs))
		}
	}
}

// 何も書いていない素朴な Pod は、baseline なら通る。
// 危ないことを「していない」からで、安全を明示したわけではない。
func TestNaivePassesBaseline(t *testing.T) {
	if vs := Check(naive(), Baseline); len(vs) != 0 {
		t.Fatalf("素朴な Pod が baseline で落ちた: %v", rules(vs))
	}
}

// restricted は、書かなかったことを違反にする。だから素朴な Pod は落ちる。
func TestRestrictedRequiresWritingItDown(t *testing.T) {
	vs := Check(tidy(), Restricted)
	for _, want := range []string{"allowPrivilegeEscalation", "runAsNonRoot", "capabilities.drop", "seccompProfile"} {
		if !hasRule(vs, want) {
			t.Errorf("%s が検出されていない: %v", want, rules(vs))
		}
	}
	if len(Check(hardened(), Restricted)) != 0 {
		t.Fatalf("書き足した Pod が落ちた: %v", rules(Check(hardened(), Restricted)))
	}
}

// 書かなかったのと false と書いたのは、区別しなければならない。
func TestUnsetIsNotFalse(t *testing.T) {
	p := hardened()
	p.Containers[0].Security.AllowPrivilegeEscalation = nil // 書かなかった
	if !hasRule(Check(p, Restricted), "allowPrivilegeEscalation") {
		t.Fatal("書かなかったことが違反になっていない")
	}
	p.Containers[0].Security.AllowPrivilegeEscalation = Bool(true) // true と書いた
	if !hasRule(Check(p, Restricted), "allowPrivilegeEscalation") {
		t.Fatal("true と書いたことが違反になっていない")
	}
	p.Containers[0].Security.AllowPrivilegeEscalation = Bool(false)
	if hasRule(Check(p, Restricted), "allowPrivilegeEscalation") {
		t.Fatal("false と書いたのに違反になっている")
	}
}

// root を明示していれば、非 root を宣言していても落ちる。
func TestExplicitRootIsCaught(t *testing.T) {
	p := hardened()
	p.Containers[0].Security.RunAsUser = Int64(0)
	if !hasRule(Check(p, Restricted), "runAsUser") {
		t.Fatal("root の指定が検出されていない")
	}
}

// 段階は積み重ね。restricted の検査は baseline の検査を含む。
func TestRestrictedIncludesBaseline(t *testing.T) {
	base := Check(leaky(), Baseline)
	strict := Check(leaky(), Restricted)
	if len(strict) <= len(base) {
		t.Fatalf("restricted が baseline を含んでいない: %d vs %d", len(strict), len(base))
	}
	for _, b := range base {
		if !hasRule(strict, b.Rule) {
			t.Fatalf("baseline の %s が restricted で抜けた", b.Rule)
		}
	}
}

// baseline は NET_BIND_SERVICE だけを許す。
func TestBaselineAllowsOneCapability(t *testing.T) {
	p := naive()
	p.Containers[0].Security.Capabilities.Add = []string{"NET_BIND_SERVICE"}
	if len(Check(p, Baseline)) != 0 {
		t.Fatalf("許されている権限で落ちた: %v", rules(Check(p, Baseline)))
	}
	p.Containers[0].Security.Capabilities.Add = []string{"NET_BIND_SERVICE", "SYS_TIME"}
	if !hasRule(Check(p, Baseline), "capabilities.add") {
		t.Fatal("許されていない権限が通った")
	}
}

// この章の中心。判定は1つ、扱いが3つ。
//
// 拒否は baseline、警告は restricted にしておくと、今動いているものを止めずに、
// 何を直せばよいかだけを知らせられる。
func TestSameCheckThreeOutcomes(t *testing.T) {
	pol := Policy{Enforce: Baseline, Audit: Restricted, Warn: Restricted}
	d := pol.Admit(tidy())

	if !d.Admitted {
		t.Fatalf("baseline は通るはずが拒否された: %v", rules(d.Denied))
	}
	if len(d.Warned) == 0 {
		t.Fatal("restricted の違反が警告されていない")
	}
	if len(d.Audited) != len(d.Warned) {
		t.Fatal("同じ段階なら記録と警告の内容は同じはず")
	}

	// 隔離を外している Pod は、拒否の段階でも落ちる。
	if pol.Admit(leaky()).Admitted {
		t.Fatal("baseline に反する Pod が通った")
	}
	// 書き足した Pod は、どの扱いでも何も出ない。
	h := pol.Admit(hardened())
	if !h.Admitted || len(h.Warned) != 0 || len(h.Audited) != 0 {
		t.Fatalf("書き足した Pod に何か出た: %v", rules(h.Warned))
	}
}

// 3つの扱いは独立に設定できる。すべて privileged なら何も起きない。
func TestAllPrivilegedIsSilent(t *testing.T) {
	pol := Policy{}
	d := pol.Admit(leaky())
	if !d.Admitted || len(d.Denied) != 0 || len(d.Audited) != 0 || len(d.Warned) != 0 {
		t.Fatal("何も設定していないのに何か起きた")
	}
}

// 締める前に、何が落ちるかを落とさずに知る。
func TestTightenTellsWhatBreaks(t *testing.T) {
	pods := []Pod{hardened(), tidy(), leaky()}

	ok, breaks := Tighten(pods, Baseline)
	if ok {
		t.Fatal("baseline へ上げても何も落ちないことになっている")
	}
	if len(breaks) != 1 || breaks[0] != "agent" {
		t.Fatalf("baseline で落ちるのは agent だけのはず: %v", breaks)
	}

	ok, breaks = Tighten(pods, Restricted)
	if ok {
		t.Fatal("restricted へ上げても何も落ちないことになっている")
	}
	if len(breaks) != 2 {
		t.Fatalf("restricted では 2 つ落ちるはず: %v", breaks)
	}

	// 直したものだけなら、上げても落ちない。
	if ok, breaks := Tighten([]Pod{hardened()}, Restricted); !ok || len(breaks) != 0 {
		t.Fatalf("書き足した Pod だけなら上げられるはず: %v", breaks)
	}
	// privileged へは常に上げられる(何も制限しないので)。
	if ok, _ := Tighten(pods, Privileged); !ok {
		t.Fatal("privileged で何かが落ちた")
	}
}

// 違反の並びは決定的。コンテナ名、規則名の順。
func TestViolationsAreOrdered(t *testing.T) {
	p := Pod{Name: "multi", Containers: []Container{
		{Name: "zeta", Security: SecurityContext{Privileged: true}},
		{Name: "alpha", Security: SecurityContext{Privileged: true}},
	}}
	vs := Check(p, Baseline)
	if len(vs) != 2 {
		t.Fatalf("違反数が違う: %d", len(vs))
	}
	if vs[0].Container != "alpha" || vs[1].Container != "zeta" {
		t.Fatalf("並びが名前順でない: %v", []string{vs[0].Container, vs[1].Container})
	}
	// Pod 全体の設定は、コンテナ名が空なので先に来る。
	p.HostNetwork = true
	vs = Check(p, Baseline)
	if vs[0].Container != "" || vs[0].Rule != "hostNetwork" {
		t.Fatalf("Pod 全体の違反が先頭に来ていない: %+v", vs[0])
	}
}

// 違反には、どの段階から効く規則かが残る。
func TestViolationCarriesItsLevel(t *testing.T) {
	for _, v := range Check(leaky(), Restricted) {
		if v.Level != Baseline && v.Level != Restricted {
			t.Fatalf("段階が入っていない: %+v", v)
		}
		if v.Detail == "" {
			t.Fatalf("説明が入っていない: %+v", v)
		}
	}
	found := false
	for _, v := range Check(leaky(), Restricted) {
		if v.Level == Baseline {
			found = true
		}
	}
	if !found {
		t.Fatal("restricted の結果に baseline 由来の違反が無い")
	}
}

// 表示まわり。
func TestNamesAndHelpers(t *testing.T) {
	if Privileged.String() != "privileged" || Baseline.String() != "baseline" ||
		Restricted.String() != "restricted" {
		t.Fatal("段階の名前が違う")
	}
	if !deref(Bool(true)) || deref(Bool(false)) || deref(nil) {
		t.Fatal("deref が違う")
	}
	if !written(Bool(false)) || written(nil) {
		t.Fatal("written が違う")
	}
	if !has([]string{"a", "b"}, "b") || has([]string{"a"}, "z") {
		t.Fatal("has が違う")
	}
	if *Int64(7) != 7 {
		t.Fatal("Int64 が違う")
	}
	if itoa(0) != "0" || itoa(9100) != "9100" {
		t.Fatal("itoa が違う")
	}
}

// Localhost の seccomp も restricted を通る。
func TestLocalhostSeccompPasses(t *testing.T) {
	p := hardened()
	p.Containers[0].Security.SeccompProfile = "Localhost"
	if hasRule(Check(p, Restricted), "seccompProfile") {
		t.Fatal("Localhost が拒まれた")
	}
}
