package gatewayapi

import "testing"

// 運用側の入口。shop.example を待ち、team-a の Route だけを受け入れる。
func gw() Gateway {
	return Gateway{
		Name: "public", Namespace: "infra",
		Listeners: []Listener{{
			Name: "https", Port: 443, Hostname: "shop.example",
			AllowedFrom: []string{"team-a"},
		}},
	}
}

func prefixRule(path, service string) Rule {
	return Rule{
		Matches:  []Match{{PathType: "PathPrefix", Path: path}},
		Backends: []Backend{{Service: service, Port: 80}},
	}
}

func route(ns, name string, rules ...Rule) HTTPRoute {
	return HTTPRoute{
		Name: name, Namespace: ns,
		ParentRefs: []string{"public"},
		Hostnames:  []string{"shop.example"},
		Rules:      rules,
	}
}

func find(as []Attachment, routeKey string) Attachment {
	for _, a := range as {
		if a.Route == routeKey {
			return a
		}
	}
	return Attachment{}
}

// 両方が同意していれば繋がる。
func TestAttachNeedsBothSides(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", prefixRule("/", "web")))

	a := find(c.Attachments(), "team-a/web")
	if !a.Attached {
		t.Fatalf("両方が同意しているのに繋がらない: %s", a.Why)
	}
	if a.Listener != "https" {
		t.Fatalf("繋がった Listener が違う: %q", a.Listener)
	}
}

// Route が親を指名していなければ繋がらない。相乗りを片側から始められない。
func TestRouteMustNameTheGateway(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	r := route("team-a", "web", prefixRule("/", "web"))
	r.ParentRefs = []string{"other"}
	c.AddRoute(r)

	if a := find(c.Attachments(), "team-a/web"); a.Attached {
		t.Fatal("指名していない Gateway に繋がった")
	}
}

// Gateway が受け入れていなければ繋がらない。指名だけでは足りない。
func TestGatewayMustAcceptTheNamespace(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-b", "web", prefixRule("/", "web"))) // 許されていない名前空間

	a := find(c.Attachments(), "team-b/web")
	if a.Attached {
		t.Fatal("受け入れていない名前空間の Route が繋がった")
	}
	if a.Why == "" {
		t.Fatal("繋がらない理由が残っていない")
	}
}

// 既定は同じ名前空間だけ。開くには明示が要る。
func TestDefaultIsSameNamespaceOnly(t *testing.T) {
	g := gw()
	g.Listeners[0].AllowedFrom = nil // 既定に戻す
	c := New()
	c.AddGateway(g)
	c.AddRoute(route("infra", "same", prefixRule("/", "web")))
	c.AddRoute(route("team-a", "other", prefixRule("/", "web")))

	if !find(c.Attachments(), "infra/same").Attached {
		t.Fatal("同じ名前空間の Route が繋がらない")
	}
	if find(c.Attachments(), "team-a/other").Attached {
		t.Fatal("明示していないのに他の名前空間から繋がった")
	}
}

// すべてを受け入れる設定にすれば、どの名前空間からでも繋がる。
func TestAllowAll(t *testing.T) {
	g := gw()
	g.Listeners[0].AllowedFrom = nil
	g.Listeners[0].AllowAll = true
	c := New()
	c.AddGateway(g)
	c.AddRoute(route("team-z", "web", prefixRule("/", "web")))

	if !find(c.Attachments(), "team-z/web").Attached {
		t.Fatal("すべて受け入れる設定で繋がらない")
	}
}

// ホスト名が重ならなければ繋がらない。
func TestHostnamesMustIntersect(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	r := route("team-a", "web", prefixRule("/", "web"))
	r.Hostnames = []string{"admin.internal"}
	c.AddRoute(r)

	if find(c.Attachments(), "team-a/web").Attached {
		t.Fatal("ホスト名が重ならないのに繋がった")
	}
}

// ワイルドカードは下位のホスト名を含む。
func TestWildcardHostname(t *testing.T) {
	g := gw()
	g.Listeners[0].Hostname = "*.example"
	c := New()
	c.AddGateway(g)
	c.AddRoute(route("team-a", "web", prefixRule("/", "web")))

	if !find(c.Attachments(), "team-a/web").Attached {
		t.Fatal("ワイルドカードが下位のホスト名を含んでいない")
	}
	if hostMatch("*.example", "example") {
		t.Fatal("ワイルドカードが親そのものに当たっている")
	}
	if hostMatch("*.example", "other.test") {
		t.Fatal("ワイルドカードが無関係なホストに当たっている")
	}
}

// 繋がっていない Route の規則は、どれだけ一致していても使われない。
func TestUnattachedRouteIsIgnoredWhenRouting(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", prefixRule("/", "web")))
	c.AddRoute(route("team-b", "steal", prefixRule("/api", "attacker"))) // 受け入れられていない

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/api/users"})
	if got.Backend.Service != "web" {
		t.Fatalf("繋がっていない Route が使われた: %+v", got)
	}
}

// 長いパスが勝つ。
func TestLongerPathWins(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", prefixRule("/", "web"), prefixRule("/api", "api")))

	if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/api/users"}); got.Backend.Service != "api" {
		t.Fatalf("長いパスが勝っていない: %+v", got)
	}
	if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/other"}); got.Backend.Service != "web" {
		t.Fatalf("短いパスに落ちていない: %+v", got)
	}
}

// 前方一致は区切りを跨がない。/api は /apiary に当たらない。
func TestPrefixDoesNotCrossSegments(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", prefixRule("/", "web"), prefixRule("/api", "api")))

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/apiary"})
	if got.Backend.Service != "web" {
		t.Fatalf("/api が /apiary に当たった: %+v", got)
	}
	if !prefixOf("/api", "/api") || !prefixOf("/api", "/api/users") || prefixOf("/api", "/apiary") {
		t.Fatal("前方一致の区切り判定が違う")
	}
}

// Exact は長さに関係なく PathPrefix に勝つ。順序が仕様で決まっている。
func TestExactBeatsLongerPrefix(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web",
		prefixRule("/api/v2/users", "prefix"),
		Rule{
			Matches:  []Match{{PathType: "Exact", Path: "/api"}},
			Backends: []Backend{{Service: "exact", Port: 80}},
		},
	))

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/api"})
	if got.Backend.Service != "exact" {
		t.Fatalf("Exact が長い PathPrefix に負けた: %+v", got)
	}
}

// ヘッダの一致数が多いほうが勝つ。パスが同じなら次の目盛りへ進む。
func TestMoreHeaderMatchesWins(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web",
		prefixRule("/", "plain"),
		Rule{
			Matches:  []Match{{Path: "/", Headers: map[string]string{"x-canary": "yes"}}},
			Backends: []Backend{{Service: "canary", Port: 80}},
		},
	))

	plain := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"})
	if plain.Backend.Service != "plain" {
		t.Fatalf("ヘッダ無しで canary に行った: %+v", plain)
	}
	tagged := c.Route(Request{
		Gateway: "public", Host: "shop.example", Path: "/",
		Headers: map[string]string{"x-canary": "yes"},
	})
	if tagged.Backend.Service != "canary" {
		t.Fatalf("ヘッダ一致が勝っていない: %+v", tagged)
	}
}

// メソッドとクエリでも絞れる。
func TestMethodAndQueryMatch(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web",
		prefixRule("/", "plain"),
		Rule{
			Matches:  []Match{{Path: "/", Method: "POST"}},
			Backends: []Backend{{Service: "writer", Port: 80}},
		},
		Rule{
			Matches:  []Match{{Path: "/", Query: map[string]string{"debug": "1"}}},
			Backends: []Backend{{Service: "debug", Port: 80}},
		},
	))

	post := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/", Method: "POST"})
	if post.Backend.Service != "writer" {
		t.Fatalf("メソッド一致が効いていない: %+v", post)
	}
	get := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/", Method: "GET"})
	if get.Backend.Service != "plain" {
		t.Fatalf("メソッドが違うのに当たった: %+v", get)
	}
	dbg := c.Route(Request{
		Gateway: "public", Host: "shop.example", Path: "/",
		Query: map[string]string{"debug": "1"},
	})
	if dbg.Backend.Service != "debug" {
		t.Fatalf("クエリ一致が効いていない: %+v", dbg)
	}
}

// 完全一致のホスト名が、指定なしより優先される。
func TestExactHostnameWins(t *testing.T) {
	g := gw()
	g.Listeners[0].Hostname = ""
	g.Listeners[0].AllowAll = true
	c := New()
	c.AddGateway(g)

	loose := HTTPRoute{Name: "loose", Namespace: "team-a", ParentRefs: []string{"public"},
		Rules: []Rule{prefixRule("/", "loose")}}
	exact := HTTPRoute{Name: "exact", Namespace: "team-a", ParentRefs: []string{"public"},
		Hostnames: []string{"shop.example"}, Rules: []Rule{prefixRule("/", "exact")}}
	c.AddRoute(loose)
	c.AddRoute(exact)

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"})
	if got.Backend.Service != "exact" {
		t.Fatalf("ホスト名を指定したほうが勝っていない: %+v", got)
	}
}

// すべて同じ特定度なら、Route の名前順で決まる。書いた順は関係ない。
func TestTiesGoToTheOlderRouteName(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "zeta", prefixRule("/", "zeta")))
	c.AddRoute(route("team-a", "alpha", prefixRule("/", "alpha")))

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"})
	if got.Backend.Service != "alpha" {
		t.Fatalf("同点が名前順で決まっていない: %+v", got)
	}
	if got.Route != "team-a/alpha" {
		t.Fatalf("勝った Route が違う: %q", got.Route)
	}
}

// 重みは比率どおりに割り振られる。乱数を使わないので比率を数えられる。
func TestWeightedSplit(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", Rule{
		Matches: []Match{{Path: "/"}},
		Backends: []Backend{
			{Service: "stable", Port: 80, Weight: 90},
			{Service: "canary", Port: 80, Weight: 10},
		},
	}))

	count := map[string]int{}
	for i := 0; i < 200; i++ {
		got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"})
		count[got.Backend.Service]++
	}
	if count["stable"] != 180 || count["canary"] != 20 {
		t.Fatalf("比率が守られていない: %v", count)
	}
}

// 重みがすべて 0 なら、名前順で先頭を返す(割り振りようがない)。
func TestZeroWeightsFallBack(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", Rule{
		Matches:  []Match{{Path: "/"}},
		Backends: []Backend{{Service: "b", Port: 80}, {Service: "a", Port: 80}},
	}))

	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"})
	if got.Backend.Service != "a" {
		t.Fatalf("重み 0 で名前順の先頭が返らない: %+v", got)
	}
}

// 振り分け先が無い規則、当たらないリクエスト、知らない Gateway。
func TestNoMatch(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", Rule{
		Matches: []Match{{PathType: "Exact", Path: "/only"}},
	}))

	if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/other"}); got.Found {
		t.Fatalf("当たらないはずが当たった: %+v", got)
	}
	if got := c.Route(Request{Gateway: "unknown", Host: "shop.example", Path: "/only"}); got.Found {
		t.Fatalf("知らない Gateway で当たった: %+v", got)
	}
	got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/only"})
	if !got.Found || got.Backend.Service != "" {
		t.Fatalf("振り分け先の無い規則の扱いが違う: %+v", got)
	}
}

// Listener が1つも受け入れないとき、理由が残る。
func TestNoListenerLeavesReason(t *testing.T) {
	c := New()
	c.AddGateway(Gateway{Name: "public", Namespace: "infra"}) // Listener が無い
	c.AddRoute(route("team-a", "web", prefixRule("/", "web")))

	a := find(c.Attachments(), "team-a/web")
	if a.Attached || a.Why == "" {
		t.Fatalf("理由が残っていない: %+v", a)
	}
}

// 単一の振り分け先はそのまま返る(計数を使わない)。
func TestSingleBackend(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddRoute(route("team-a", "web", prefixRule("/", "web")))
	for i := 0; i < 3; i++ {
		if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"}); got.Backend.Service != "web" {
			t.Fatalf("単一の振り分け先が変わった: %+v", got)
		}
	}
}

// 優先順位の比較そのもの。
func TestPriorityBeats(t *testing.T) {
	base := Priority{ExactHost: 1, ExactPath: 1, PathLen: 4, Method: 1, HeaderHits: 1, QueryHits: 1}
	lower := []Priority{
		{ExactHost: 0, ExactPath: 1, PathLen: 4, Method: 1, HeaderHits: 1, QueryHits: 1},
		{ExactHost: 1, ExactPath: 0, PathLen: 40, Method: 1, HeaderHits: 1, QueryHits: 1},
		{ExactHost: 1, ExactPath: 1, PathLen: 3, Method: 1, HeaderHits: 1, QueryHits: 1},
		{ExactHost: 1, ExactPath: 1, PathLen: 4, Method: 0, HeaderHits: 1, QueryHits: 1},
		{ExactHost: 1, ExactPath: 1, PathLen: 4, Method: 1, HeaderHits: 0, QueryHits: 1},
		{ExactHost: 1, ExactPath: 1, PathLen: 4, Method: 1, HeaderHits: 1, QueryHits: 0},
	}
	for i, l := range lower {
		if !base.beats(l) {
			t.Errorf("%d: base が勝っていない", i)
		}
		if l.beats(base) {
			t.Errorf("%d: 下位が勝っている", i)
		}
	}
	if base.beats(base) {
		t.Error("同じもの同士で勝ち負けがついた")
	}
}

func TestOrAny(t *testing.T) {
	if orAny("") != "(指定なし)" || orAny("a.example") != "a.example" {
		t.Fatal("ホスト名の表示が違う")
	}
}

// 入口が複数あるとき、繋がりは Gateway ごとに判定され、リクエストは
// 宛先の入口に繋がった Route だけを見る。
func TestMultipleGateways(t *testing.T) {
	c := New()
	c.AddGateway(gw())
	c.AddGateway(Gateway{
		Name: "internal", Namespace: "infra",
		Listeners: []Listener{{Name: "http", Port: 80, Hostname: "shop.example", AllowAll: true}},
	})
	c.AddRoute(route("team-a", "web", prefixRule("/", "public-web")))

	inner := HTTPRoute{Name: "admin", Namespace: "team-b", ParentRefs: []string{"internal"},
		Hostnames: []string{"shop.example"}, Rules: []Rule{prefixRule("/", "admin")}}
	c.AddRoute(inner)

	as := c.Attachments()
	if as[0].Gateway != "internal" {
		t.Fatalf("Gateway 名順に並んでいない: %+v", as[0])
	}
	if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"}); got.Backend.Service != "public-web" {
		t.Fatalf("public の振り分けが違う: %+v", got)
	}
	if got := c.Route(Request{Gateway: "internal", Host: "shop.example", Path: "/"}); got.Backend.Service != "admin" {
		t.Fatalf("internal の振り分けが違う: %+v", got)
	}
}

// 繋がってはいるが、そのリクエストのホスト名を持たない Route は使われない。
func TestAttachedRouteWithOtherHostnameIsSkipped(t *testing.T) {
	g := gw()
	g.Listeners[0].Hostname = "*.example"
	g.Listeners[0].AllowAll = true
	c := New()
	c.AddGateway(g)

	shop := HTTPRoute{Name: "shop", Namespace: "team-a", ParentRefs: []string{"public"},
		Hostnames: []string{"shop.example"}, Rules: []Rule{prefixRule("/", "shop")}}
	blog := HTTPRoute{Name: "blog", Namespace: "team-a", ParentRefs: []string{"public"},
		Hostnames: []string{"blog.example"}, Rules: []Rule{prefixRule("/", "blog")}}
	c.AddRoute(blog)
	c.AddRoute(shop)

	for _, a := range c.Attachments() {
		if !a.Attached {
			t.Fatalf("どちらも繋がるはず: %+v", a)
		}
	}
	if got := c.Route(Request{Gateway: "public", Host: "shop.example", Path: "/"}); got.Backend.Service != "shop" {
		t.Fatalf("ホスト名で絞れていない: %+v", got)
	}
	if got := c.Route(Request{Gateway: "public", Host: "blog.example", Path: "/"}); got.Backend.Service != "blog" {
		t.Fatalf("ホスト名で絞れていない: %+v", got)
	}
}
