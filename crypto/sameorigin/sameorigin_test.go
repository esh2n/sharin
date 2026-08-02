package sameorigin

import "testing"

func mustParse(t *testing.T, raw string) Origin {
	t.Helper()
	o, err := Parse(raw)
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return o
}

// この章の中心その1。生成元は3つ組で、1つでも違えば別になる。
func TestOriginIsThreeThingsAndPathIsNotOneOfThem(t *testing.T) {
	base := mustParse(t, "https://example.com/login")

	cases := []struct {
		url  string
		same bool
		why  string
	}{
		{"https://example.com/admin/secret", true, "パスは生成元に入らない"},
		{"https://example.com:443/", true, "既定ポートは書いても書かなくても同じ"},
		{"https://EXAMPLE.COM/", true, "ホストの大文字小文字は畳まれる"},
		{"https://example.com/?q=1#top", true, "クエリも断片も入らない"},
		{"http://example.com/", false, "スキームが違う"},
		{"https://example.com:8443/", false, "ポートが違う"},
		{"https://api.example.com/", false, "サブドメインは別のホスト"},
		{"https://example.com.evil.test/", false, "前方一致は別物"},
	}
	t.Logf("基準 %s", base)
	for _, c := range cases {
		got := Same(base, mustParse(t, c.url))
		t.Logf("  %-34s %-6v %s", c.url, got, c.why)
		if got != c.same {
			t.Fatalf("%s: %v を期待", c.url, c.same)
		}
	}
}

// この章の中心その2。既定で止まるのは読み取りだけで、送信は通る。
func TestSendingIsAllowedReadingIsNot(t *testing.T) {
	t.Logf("%-20s %8s %8s %10s", "やり方", "送れる", "読める", "クッキー")
	send, read := 0, 0
	for _, k := range Kinds() {
		a := CrossOrigin(k)
		t.Logf("%-20s %8v %8v %10v", k, a.Send, a.Read, a.Cookie)
		if a.Send {
			send++
		}
		if a.Read {
			read++
		}
	}
	t.Logf("送れる %d / %d、読める %d / %d", send, len(Kinds()), read, len(Kinds()))

	// 全部のやり方で送れて、どれも読めない。
	if send != len(Kinds()) {
		t.Fatalf("送れないやり方がある: %d", send)
	}
	if read != 0 {
		t.Fatalf("既定で読めるやり方がある: %d", read)
	}
	// フォームはクッキー付きで飛ぶ。CSRF はここに乗る。
	if !CrossOrigin("form").Cookie {
		t.Fatal("フォームにクッキーが付かない")
	}
	// fetch は明示しないとクッキーを付けない。ここがフォームと違う。
	if CrossOrigin("fetch").Cookie || !CrossOrigin("fetch-credentials").Cookie {
		t.Fatal("fetch のクッキーの扱いが違う")
	}
	// 知らないやり方には何も答えない。
	if (CrossOrigin("websocket") != Access{}) {
		t.Fatal("知らないやり方に答えている")
	}
}

// この章の中心その3。先に問い合わせが要るかどうかは、フォームで送れる範囲で決まる。
func TestPreflightLineFollowsWhatAFormCouldAlreadySend(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		pre  bool
	}{
		{"GET", Request{Method: "GET"}, false},
		{"POST フォーム形式", Request{Method: "POST", ContentType: "application/x-www-form-urlencoded"}, false},
		{"POST multipart", Request{Method: "POST", ContentType: "multipart/form-data; boundary=x"}, false},
		{"POST text/plain", Request{Method: "POST", ContentType: "text/plain"}, false},
		{"POST JSON", Request{Method: "POST", ContentType: "application/json"}, true},
		{"PUT", Request{Method: "PUT"}, true},
		{"DELETE", Request{Method: "DELETE"}, true},
		{"GET + 独自ヘッダ", Request{Method: "GET", Headers: []string{"X-Token"}}, true},
		{"GET + Accept", Request{Method: "GET", Headers: []string{"Accept"}}, false},
	}
	t.Logf("%-20s %s", "要求", "先に問い合わせが要る")
	for _, c := range cases {
		got := NeedsPreflight(c.req)
		t.Logf("%-20s %v", c.name, got)
		if got != c.pre {
			t.Fatalf("%s: %v を期待", c.name, c.pre)
		}
	}

	// 境目の読み方。フォームで送れたものは、いまさら止められないので通る。
	// JSON はフォームで送れなかったので、問い合わせが要る。
	form := Request{Method: "POST", ContentType: "application/x-www-form-urlencoded"}
	json := Request{Method: "POST", ContentType: "application/json"}
	if NeedsPreflight(form) || !NeedsPreflight(json) {
		t.Fatal("境目がフォームの範囲と揃っていない")
	}
}

// この章の中心その4。開け方を間違えると、書いた覚えがなくても全開になる。
func TestOpeningItWrongOpensEverything(t *testing.T) {
	const me = "https://app.example.com"
	const attacker = "https://evil.test"

	strict := Policy{Allow: []string{me}, Credentials: true}
	star := Policy{Wildcard: true}
	starCred := Policy{Wildcard: true, Credentials: true}
	reflect := Policy{Reflect: true, Credentials: true}

	t.Logf("%-16s %-24s %-8s %s", "設定", "攻撃者から", "読める", "理由")
	for _, c := range []struct {
		name string
		p    Policy
	}{{"名指し", strict}, {"*", star}, {"* + cookie", starCred}, {"Origin 反射", reflect}} {
		d := Decide(c.p, attacker, true)
		t.Logf("%-16s %-24s %-8v %s", c.name, d.AllowOrigin, d.Readable, d.Reason)
	}

	// 名指しなら、攻撃者からは読めない。
	if Decide(strict, attacker, true).Readable {
		t.Fatal("名指しなのに読めた")
	}
	// 自分のところからは読める。
	if !Decide(strict, me, true).Readable {
		t.Fatal("許可した相手が読めない")
	}
	// クッキーを許していなければ、名指しでもクッキー付きは読ませない。
	noCred := Policy{Allow: []string{me}}
	if Decide(noCred, me, true).Readable {
		t.Fatal("クッキー付きを許していないのに読めた")
	}
	if !Decide(noCred, me, false).Readable {
		t.Fatal("クッキー無しなら読めるはず")
	}

	// `*` は、クッキー付きでは読ませない。ここが安全弁になっている。
	if Decide(star, attacker, true).Readable {
		t.Fatal("* でクッキー付きが読めた")
	}
	if !Decide(star, attacker, false).Readable {
		t.Fatal("* でクッキー無しが読めない")
	}
	// クッキーも許すと宣言しても、組み合わせとして成り立たないので読ませない。
	if Decide(starCred, attacker, true).Readable {
		t.Fatal("* + クッキーが通ってしまった")
	}

	// 反射だけが、クッキー付きで攻撃者に読ませてしまう。
	d := Decide(reflect, attacker, true)
	if !d.Readable {
		t.Fatal("反射で読めないのは実装が違う")
	}
	t.Logf("反射は攻撃者に %q を返し、クッキー付きで読ませる", d.AllowOrigin)

	// 設定の見た目では、反射は一覧が空なので厳しく見える。
	t.Logf("一覧の長さ  名指し %d / 反射 %d", len(strict.Allow), len(reflect.Allow))
	if len(reflect.Allow) != 0 || !reflect.EffectivelyOpen() || strict.EffectivelyOpen() {
		t.Fatal("実質全開の判定が合っていない")
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	if _, err := Parse("https://example.com:notaport/"); err == nil {
		t.Fatal("壊れたポートが通った")
	}
	if _, err := Parse("://"); err == nil {
		t.Fatal("壊れた URL が通った")
	}
	if _, err := Parse("https://exa mple.com/"); err == nil {
		t.Fatal("空白入りのホストが通った")
	}
	// 知らないスキームには既定ポートが無い。
	o := mustParse(t, "ftp://example.com/")
	if o.Port != 0 {
		t.Fatalf("既定ポート: %d", o.Port)
	}
	// 表示は既定ポートを省く。
	if s := mustParse(t, "https://example.com:443/").String(); s != "https://example.com" {
		t.Fatalf("表示: %q", s)
	}
	if s := mustParse(t, "http://example.com:8080/").String(); s != "http://example.com:8080" {
		t.Fatalf("表示: %q", s)
	}
	// Content-Type に文字コードが付いていても判定は変わらない。
	if NeedsPreflight(Request{Method: "POST", ContentType: "TEXT/PLAIN; charset=utf-8"}) {
		t.Fatal("文字コード付きで判定が変わった")
	}
	// Content-Type が無い POST は単純要求。
	if NeedsPreflight(Request{Method: "POST"}) {
		t.Fatal("Content-Type 無しで問い合わせが要ると判定した")
	}
	// メソッドの大文字小文字は畳む。
	if NeedsPreflight(Request{Method: "get"}) {
		t.Fatal("小文字のメソッドで判定が変わった")
	}
}
