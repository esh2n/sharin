package ingress

import "testing"

func site() *Ingress {
	i := New()
	i.Add(Rule{Host: "shop.example", Path: "/", Backend: Backend{Service: "web", Port: 80}})
	i.Add(Rule{Host: "shop.example", Path: "/api", Backend: Backend{Service: "api", Port: 8080}})
	return i
}

// パスで別の Service へ振り分ける。1つの入口の下に複数のサービスが並ぶ。
func TestRoutesByPath(t *testing.T) {
	i := site()
	if got := i.Route("shop.example", "/").Backend.Service; got != "web" {
		t.Fatalf("/ は web のはずが %s", got)
	}
	if got := i.Route("shop.example", "/api/users").Backend.Service; got != "api" {
		t.Fatalf("/api/users は api のはずが %s", got)
	}
	if i.Routed != 2 {
		t.Fatalf("2 件振り分けたはずが %d", i.Routed)
	}
}

// 長いパスが勝つ。/api と / の両方が当たるとき、より特定的なほうを採る。
func TestLongerPathWins(t *testing.T) {
	i := site()
	r := i.Route("shop.example", "/api")
	if r.Backend.Service != "api" {
		t.Fatalf("/api は api のはずが %s(%s)", r.Backend.Service, r.Reason)
	}
}

// 書いた順は結果に影響しない。特定度で並べ直しているから。
func TestOrderOfAdditionDoesNotMatter(t *testing.T) {
	a := New()
	a.Add(Rule{Path: "/", Backend: Backend{Service: "web"}})
	a.Add(Rule{Path: "/api", Backend: Backend{Service: "api"}})

	b := New()
	b.Add(Rule{Path: "/api", Backend: Backend{Service: "api"}})
	b.Add(Rule{Path: "/", Backend: Backend{Service: "web"}})

	for _, i := range []*Ingress{a, b} {
		if got := i.Route("any", "/api/x").Backend.Service; got != "api" {
			t.Fatalf("順序によらず api のはずが %s", got)
		}
	}
}

// ホスト名でも分けられる。同じパスでも別のサービスへ行く。
func TestRoutesByHost(t *testing.T) {
	i := New()
	i.Add(Rule{Host: "shop.example", Path: "/", Backend: Backend{Service: "shop"}})
	i.Add(Rule{Host: "blog.example", Path: "/", Backend: Backend{Service: "blog"}})

	if got := i.Route("shop.example", "/").Backend.Service; got != "shop" {
		t.Fatalf("shop.example は shop のはずが %s", got)
	}
	if got := i.Route("blog.example", "/").Backend.Service; got != "blog" {
		t.Fatalf("blog.example は blog のはずが %s", got)
	}
}

// ホストを指定した規則が、指定していない規則より優先される。
func TestHostRuleBeatsWildcard(t *testing.T) {
	i := New()
	i.Add(Rule{Path: "/", Backend: Backend{Service: "catchall"}})
	i.Add(Rule{Host: "shop.example", Path: "/", Backend: Backend{Service: "shop"}})

	if got := i.Route("shop.example", "/").Backend.Service; got != "shop" {
		t.Fatalf("ホスト指定が勝つはずが %s", got)
	}
	if got := i.Route("other.example", "/").Backend.Service; got != "catchall" {
		t.Fatalf("他のホストは catchall のはずが %s", got)
	}
}

// どの規則にも当たらなければ、既定があればそこへ、無ければ見つからない。
func TestFallbackAndNotFound(t *testing.T) {
	i := New()
	i.Add(Rule{Host: "shop.example", Path: "/api", Backend: Backend{Service: "api"}})

	r := i.Route("other.example", "/")
	if r.Matched {
		t.Fatal("当たる規則が無いはず")
	}
	if i.NotFound != 1 {
		t.Fatalf("見つからない件数が %d", i.NotFound)
	}

	i.Default = &Backend{Service: "fallback", Port: 80}
	r2 := i.Route("other.example", "/")
	if !r2.Matched || r2.Backend.Service != "fallback" {
		t.Fatalf("既定へ落ちるはずが %+v", r2)
	}
}

// パスを省略すると "/" として扱う。すべてに当たる規則になる。
func TestEmptyPathMeansRoot(t *testing.T) {
	i := New()
	i.Add(Rule{Backend: Backend{Service: "web"}})
	if got := i.Rules()[0].Path; got != "/" {
		t.Fatalf("既定は / のはずが %q", got)
	}
	if !i.Route("any", "/anything").Matched {
		t.Fatal("すべてに当たるはず")
	}
}

// 前方一致なので、パスの途中で切れた文字列には当たらない。
func TestPrefixMatchBoundaries(t *testing.T) {
	i := New()
	i.Add(Rule{Path: "/api", Backend: Backend{Service: "api"}})
	if i.Route("h", "/ap").Matched {
		t.Fatal("/ap には当たらないはず")
	}
	if !i.Route("h", "/api").Matched {
		t.Fatal("/api には当たるはず")
	}
}

// 規則は特定度の高い順に並ぶ。評価順がそのまま見える。
func TestRulesAreSortedBySpecificity(t *testing.T) {
	i := New()
	i.Add(Rule{Path: "/", Backend: Backend{Service: "web"}})
	i.Add(Rule{Host: "h", Path: "/", Backend: Backend{Service: "host"}})
	i.Add(Rule{Path: "/api/v2", Backend: Backend{Service: "v2"}})
	i.Add(Rule{Path: "/api", Backend: Backend{Service: "api"}})

	want := []string{"host", "v2", "api", "web"}
	for n, r := range i.Rules() {
		if r.Service != want[n] {
			t.Fatalf("%d 番目は %s のはずが %s", n, want[n], r.Service)
		}
	}
}

func TestHelpers(t *testing.T) {
	if !hasPrefix("/api/users", "/api") || hasPrefix("/a", "/api") {
		t.Fatal("hasPrefix が違う")
	}
	if label(Rule{Path: "/", Backend: Backend{Service: "web", Port: 80}}) != "*/ → web:80" {
		t.Fatal("label が違う")
	}
	if itoa(0) != "0" || itoa(8080) != "8080" {
		t.Fatal("itoa が違う")
	}
}
