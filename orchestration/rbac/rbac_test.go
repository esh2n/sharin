package rbac

import "testing"

func viewer() *Role {
	return &Role{Name: "viewer", Rules: []PolicyRule{
		{Resources: []string{"pods", "services"}, Verbs: []Verb{Get, List}},
	}}
}

func deployer() *Role {
	return &Role{Name: "deployer", Rules: []PolicyRule{
		{Resources: []string{"deployments"}, Verbs: []Verb{Get, List, Create, Update}},
	}}
}

// 何も書かなければ何も通らない。通信とは既定の向きが逆。
func TestDeniedByDefault(t *testing.T) {
	a := New()
	if a.Can("alice", "pods", Get).Allowed {
		t.Fatal("何も与えていないので通らないはず")
	}
	a.AddRole(viewer())
	if a.Can("alice", "pods", Get).Allowed {
		t.Fatal("役割を定義しただけでは効かないはず")
	}
	a.Bind("viewer", "alice")
	if !a.Can("alice", "pods", Get).Allowed {
		t.Fatal("与えて初めて通るはず")
	}
}

// 書いた資源と操作だけが通る。書いていない組み合わせは通らない。
func TestOnlyWhatIsWritten(t *testing.T) {
	a := New()
	a.AddRole(viewer())
	a.Bind("viewer", "alice")

	if !a.Can("alice", "services", List).Allowed {
		t.Fatal("書いてある組み合わせは通るはず")
	}
	if a.Can("alice", "pods", Delete).Allowed {
		t.Fatal("書いていない操作は通らないはず")
	}
	if a.Can("alice", "secrets", Get).Allowed {
		t.Fatal("書いていない資源は通らないはず")
	}
}

// 役割は複数与えられる。許可は足し算で、どれか1つが許せば通る。
func TestRolesAreAdditive(t *testing.T) {
	a := New()
	a.AddRole(viewer())
	a.AddRole(deployer())
	a.Bind("viewer", "alice")
	a.Bind("deployer", "alice")

	if !a.Can("alice", "pods", Get).Allowed {
		t.Fatal("viewer の分が効くはず")
	}
	if !a.Can("alice", "deployments", Create).Allowed {
		t.Fatal("deployer の分が効くはず")
	}
	if got := len(a.RolesOf("alice")); got != 2 {
		t.Fatalf("2 つ持つはずが %d", got)
	}
}

// 役割の中身を直せば、それを持つ全員に一度に効く。
// 人ごとに書いていたら、全員ぶんを直すことになる。
func TestEditingRoleAffectsEveryone(t *testing.T) {
	a := New()
	r := a.AddRole(viewer())
	a.Bind("viewer", "alice", "bob")

	for _, who := range []string{"alice", "bob"} {
		if a.Can(who, "pods", Delete).Allowed {
			t.Fatalf("%s はまだ消せないはず", who)
		}
	}
	r.Rules = append(r.Rules, PolicyRule{Resources: []string{"pods"}, Verbs: []Verb{Delete}})
	for _, who := range []string{"alice", "bob"} {
		if !a.Can(who, "pods", Delete).Allowed {
			t.Fatalf("%s にも一度に効くはず", who)
		}
	}
}

// ワイルドカードは強い。1つ書くだけで全部が通る。
func TestWildcardGrantsEverything(t *testing.T) {
	a := New()
	a.AddRole(&Role{Name: "admin", Rules: []PolicyRule{
		{Resources: []string{"*"}, Verbs: []Verb{"*"}},
	}})
	a.Bind("admin", "root")

	for _, res := range []string{"pods", "secrets", "nodes"} {
		for _, v := range []Verb{Get, Delete, Create} {
			if !a.Can("root", res, v).Allowed {
				t.Fatalf("%s への %s が通らない", res, v)
			}
		}
	}
}

// 拒否は書けない。広い許可を1つ与えると、狭い役割で打ち消せない。
func TestCannotDenyByAddingRole(t *testing.T) {
	a := New()
	a.AddRole(&Role{Name: "admin", Rules: []PolicyRule{
		{Resources: []string{"*"}, Verbs: []Verb{"*"}},
	}})
	a.AddRole(viewer())
	a.Bind("admin", "root")
	a.Bind("viewer", "root")

	if !a.Can("root", "secrets", Delete).Allowed {
		t.Fatal("広い許可が残っているかぎり通る。狭い役割では打ち消せない")
	}
}

// 存在しない役割を与えても何も起きない。
func TestBindingUnknownRoleIsNoop(t *testing.T) {
	a := New()
	a.Bind("nosuch", "alice")
	if a.Can("alice", "pods", Get).Allowed {
		t.Fatal("定義の無い役割では通らないはず")
	}
}

// 判定の記録が残る。誰が何で弾かれたかを追える。
func TestCountsAndLog(t *testing.T) {
	a := New()
	a.AddRole(viewer())
	a.Bind("viewer", "alice")

	a.Do("alice", "pods", Get)
	a.Do("alice", "secrets", Delete)
	if a.Allowed != 1 || a.Denied != 1 {
		t.Fatalf("allowed=%d denied=%d", a.Allowed, a.Denied)
	}
	if len(a.Log) == 0 {
		t.Fatal("拒否の記録が残るはず")
	}
}

// 判定表で、誰に何を与えているかを一望できる。
func TestMatrix(t *testing.T) {
	a := New()
	a.AddRole(viewer())
	a.AddRole(deployer())
	a.Bind("viewer", "alice")
	a.Bind("deployer", "bob")

	cells := a.Matrix([]string{"alice", "bob"}, []string{"pods", "deployments"}, Get)
	if len(cells) != 4 {
		t.Fatalf("2 × 2 = 4 マスのはずが %d", len(cells))
	}
	byKey := map[string]bool{}
	for _, c := range cells {
		byKey[c.Subject+"/"+c.Resource] = c.Allowed
	}
	if !byKey["alice/pods"] || byKey["alice/deployments"] {
		t.Fatal("alice は pods だけ見えるはず")
	}
	if byKey["bob/pods"] || !byKey["bob/deployments"] {
		t.Fatal("bob は deployments だけ見えるはず")
	}
}

func TestHelpers(t *testing.T) {
	if !contains([]string{"a"}, "a") || contains([]string{"a"}, "b") || !contains([]string{"*"}, "z") {
		t.Fatal("contains が違う")
	}
	if !containsVerb([]Verb{"*"}, Delete) || containsVerb([]Verb{Get}, Delete) {
		t.Fatal("containsVerb が違う")
	}
	if join([]string{"a", "b"}) != "a, b" {
		t.Fatal("join が違う")
	}
	if len(New().Roles()) != 0 {
		t.Fatal("最初は役割が無いはず")
	}
}
