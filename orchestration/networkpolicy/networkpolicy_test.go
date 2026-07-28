package networkpolicy

import "testing"

func lbl(app string) map[string]string { return map[string]string{"app": app} }

// three は web / api / db の3層を置いたクラスタを作る。
func three() *Cluster {
	c := New()
	c.AddPod("web", lbl("web"))
	c.AddPod("api", lbl("api"))
	c.AddPod("db", lbl("db"))
	return c
}

// 方針が1つも無ければ、全部が全部に繋がる。これが既定。
func TestOpenByDefault(t *testing.T) {
	c := three()
	for _, e := range c.Matrix(0) {
		if !e.Allowed {
			t.Fatalf("既定では全部通るはずが %s → %s が遮断", e.From, e.To)
		}
	}
}

// db に方針を1つ向けた瞬間、db への通信は既定で拒否に変わる。
// 許可を足したつもりが、同時に既定を落としている。
func TestPolicyFlipsDefaultToDeny(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "db-allow-api", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("api")}}})

	if v := c.Connect("api", "db", 5432); !v.Allowed {
		t.Fatalf("api は許可されているはずが遮断: %s", v.Reason)
	}
	if v := c.Connect("web", "db", 5432); v.Allowed {
		t.Fatal("web は許可されていないので遮断されるはず")
	}
	// db 以外は方針に選ばれていないので、既定のまま通る。
	if v := c.Connect("web", "api", 8080); !v.Allowed {
		t.Fatalf("api には方針が無いので通るはずが遮断: %s", v.Reason)
	}
}

// 方針は受け側に付く。送り側に付けても、その Pod が出す通信は縛られない。
func TestPolicyAppliesToDestination(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "web-allow-none", Selector: lbl("web")})

	// web は守られたので、誰も web に繋げない。
	if v := c.Connect("api", "web", 80); v.Allowed {
		t.Fatal("web への通信は全部遮断されるはず")
	}
	// だが web から出ていく通信は縛られない。
	if v := c.Connect("web", "db", 5432); !v.Allowed {
		t.Fatalf("送り側は縛られないはずが遮断: %s", v.Reason)
	}
}

// 許可は足し算。方針を複数向けても、どれか1つが合えば通る。
func TestRulesAreAdditive(t *testing.T) {
	c := three()
	c.AddPod("batch", lbl("batch"))
	c.AddPolicy(&Policy{Name: "db-allow-api", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("api")}}})
	c.AddPolicy(&Policy{Name: "db-allow-batch", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("batch")}}})

	for _, src := range []string{"api", "batch"} {
		if v := c.Connect(src, "db", 5432); !v.Allowed {
			t.Fatalf("%s は許可されるはずが遮断: %s", src, v.Reason)
		}
	}
	if v := c.Connect("web", "db", 5432); v.Allowed {
		t.Fatal("どの方針にも合わない web は遮断されるはず")
	}
}

// 規則を足しても、既に通っていた通信は止まらない。
// 拒否を後から足して塞ぐ、という書き方はできない。
func TestCannotDenyByAddingPolicy(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "db-allow-all", Selector: lbl("db"),
		Rules: []Rule{{From: map[string]string{}}}}) // 空の条件は全員に一致
	c.AddPolicy(&Policy{Name: "db-allow-api", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("api")}}})

	if v := c.Connect("web", "db", 5432); !v.Allowed {
		t.Fatalf("先の全許可が効いたままのはずが遮断: %s", v.Reason)
	}
}

// ポートを指定すると、そのポートだけが通る。
func TestPortNarrowsTheRule(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "db-allow-api-5432", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("api"), Port: 5432}}})

	if v := c.Connect("api", "db", 5432); !v.Allowed {
		t.Fatalf("指定のポートは通るはずが遮断: %s", v.Reason)
	}
	if v := c.Connect("api", "db", 22); v.Allowed {
		t.Fatal("別のポートは遮断されるはず")
	}
}

// 規則を1つも持たない方針は、その Pod への通信を全部止める。
// 何も許可しない = 完全に閉じる、という書き方になる。
func TestEmptyPolicyDeniesAll(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "db-deny-all", Selector: lbl("db")})
	for _, src := range []string{"web", "api"} {
		if v := c.Connect(src, "db", 5432); v.Allowed {
			t.Fatalf("%s も遮断されるはず", src)
		}
	}
	if c.Denied != 2 {
		t.Fatalf("2 件遮断されるはずが %d", c.Denied)
	}
}

// 3層を意図どおりに閉じる。web → db が塞がり、web → api → db は通る。
func TestThreeTierIsolation(t *testing.T) {
	c := three()
	c.AddPolicy(&Policy{Name: "api-allow-web", Selector: lbl("api"),
		Rules: []Rule{{From: lbl("web"), Port: 8080}}})
	c.AddPolicy(&Policy{Name: "db-allow-api", Selector: lbl("db"),
		Rules: []Rule{{From: lbl("api"), Port: 5432}}})

	cases := []struct {
		src, dst string
		port     int
		want     bool
	}{
		{"web", "api", 8080, true},
		{"api", "db", 5432, true},
		{"web", "db", 5432, false}, // 飛び越えは塞がる
		{"db", "api", 8080, false}, // 逆向きも塞がる
	}
	for _, tc := range cases {
		if got := c.Connect(tc.src, tc.dst, tc.port).Allowed; got != tc.want {
			t.Fatalf("%s → %s:%d が %v(期待 %v)", tc.src, tc.dst, tc.port, got, tc.want)
		}
	}
}

// 存在しない端点への接続は通らない。
func TestUnknownPodIsDenied(t *testing.T) {
	c := three()
	if v := c.Connect("nosuch", "db", 5432); v.Allowed {
		t.Fatal("存在しない端点は通らないはず")
	}
}

// 判定表は全組み合わせを返す(自分自身は除く)。
func TestMatrixCoversAllPairs(t *testing.T) {
	c := three()
	if got := len(c.Matrix(0)); got != 6 {
		t.Fatalf("3 Pod なら 6 組のはずが %d", got)
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(5432) != "5432" {
		t.Fatal("itoa が違う")
	}
}
