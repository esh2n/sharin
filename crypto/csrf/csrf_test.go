package csrf

import "testing"

const session = "sess-42"

// この章の中心その1。合言葉は「送れるが読めない」を利用している。
func TestTokenWorksBecauseTheAttackerCannotReadIt(t *testing.T) {
	tok := Issue(session)
	t.Logf("セッション %q の合言葉 %q", session, tok)

	if !Valid(session, tok) {
		t.Fatal("自分の合言葉が通らない")
	}
	// 別のセッションの合言葉では通らない。
	if Valid(session, Issue("sess-99")) {
		t.Fatal("他人の合言葉が通った")
	}
	// 合言葉を求めるのは状態を変える要求だけ。GET にも求めると、
	// 他サイトからのリンク流入が全部弾かれてしまう。
	d := Defense{Token: true}
	get := Attempt{Nav: Nav{CrossSite: true, TopLevel: true, Method: "GET"}}
	if !Check(d, session, get).Accepted {
		t.Fatal("合言葉を持たない GET が弾かれた")
	}
	// 攻撃者は値を知らないので、空か当てずっぽうになる。
	for _, guess := range []string{"", "0", "aaaa", tok + "x"} {
		if Valid(session, guess) {
			t.Fatalf("%q が通った", guess)
		}
	}
	// 決定的なので、何度出しても同じ値になる。
	if Issue(session) != tok {
		t.Fatal("呼ぶたびに変わっている")
	}
}

// この章の中心その2。SameSite の線は「書き換えないはずの移動」で引かれている。
func TestSameSiteLineIsDrawnAtSafeNavigation(t *testing.T) {
	navs := []struct {
		name string
		n    Nav
	}{
		{"同一サイトの POST", Nav{Method: "POST"}},
		{"他サイトからリンク(GET)", Nav{CrossSite: true, TopLevel: true, Method: "GET"}},
		{"他サイトから隠しフォーム(POST)", Nav{CrossSite: true, TopLevel: true, Method: "POST"}},
		{"他サイトの img(GET)", Nav{CrossSite: true, Method: "GET"}},
		{"他サイトからの fetch(GET)", Nav{CrossSite: true, Method: "GET"}},
	}
	t.Logf("%-34s %8s %8s %8s", "要求", "None", "Lax", "Strict")
	for _, c := range navs {
		t.Logf("%-34s %8v %8v %8v", c.name,
			Sends(None, c.n), Sends(Lax, c.n), Sends(Strict, c.n))
	}

	link := Nav{CrossSite: true, TopLevel: true, Method: "GET"}
	form := Nav{CrossSite: true, TopLevel: true, Method: "POST"}
	img := Nav{CrossSite: true, Method: "GET"}

	// Lax はリンク流入だけ通す。同じ他サイト発でも POST は通さない。
	if !Sends(Lax, link) || Sends(Lax, form) {
		t.Fatal("Lax の線が違う")
	}
	// 埋め込みの GET も通さない。アドレス欄が変わらないからだ。
	if Sends(Lax, img) {
		t.Fatal("Lax が img に付けている")
	}
	// Strict は他サイト発を全部切る。リンク流入も切れる。
	if Sends(Strict, link) || Sends(Strict, form) {
		t.Fatal("Strict が通している")
	}
	// 同一サイトならどの印でも付く。
	for _, s := range []SameSite{None, Lax, Strict} {
		if !Sends(s, Nav{Method: "POST"}) {
			t.Fatalf("%v が同一サイトで付かない", s)
		}
	}
	// None は全部通す。昔の既定がこれだった。
	if !Sends(None, form) || !Sends(None, img) {
		t.Fatal("None が止めている")
	}
}

// この章の中心その3。GET で状態を変える設計だと、Lax では守れない。
func TestLaxDoesNotCoverStateChangingGet(t *testing.T) {
	d := Defense{SameSite: Lax}
	// 攻撃者がリンクを踏ませる形。Lax でもクッキーは付く。
	link := Attempt{Name: "攻撃: リンクで GET",
		Nav: Nav{CrossSite: true, TopLevel: true, Method: "GET"}, Origin: "https://evil.test"}
	r := Check(d, session, link)
	t.Logf("Lax + リンクでの GET → クッキー %v / 受理 %v (%s)", r.CookieSent, r.Accepted, r.Reason)
	if !r.CookieSent || !r.Accepted {
		t.Fatal("Lax がリンク流入の GET を止めてしまっている")
	}

	// 同じ要求を POST にすると止まる。**メソッドを変えるだけで結果が変わる**。
	form := link
	form.Nav.Method = "POST"
	if Check(d, session, form).Accepted {
		t.Fatal("Lax が POST を通した")
	}
	t.Log("同じ相手・同じ経路でも、GET は通り POST は止まる")
}

// この章の中心その4。どの守りも単独では、攻撃を全部止めて正規を全部通す、にならない。
func TestNoSingleDefenseBlocksEverythingAndPassesEverything(t *testing.T) {
	t.Logf("%-22s %10s %10s", "守り", "攻撃を止めた", "正規を通した")
	type row struct {
		name                            string
		blocked, attacks, passed, legit int
	}
	var rows []row
	for _, c := range Defenses() {
		b, at, p, lg := Score(c.D, session)
		rows = append(rows, row{c.Name, b, at, p, lg})
		t.Logf("%-22s %6d / %-3d %6d / %-3d", c.Name, b, at, p, lg)
	}

	// 何もしなければ、攻撃は1つも止まらない。
	if rows[0].blocked != 0 {
		t.Fatalf("何もしないのに止まった: %d", rows[0].blocked)
	}
	// Strict は攻撃をよく止めるが、正規のリンク流入まで切る。
	var strict row
	for _, r := range rows {
		if r.name == "SameSite=Strict だけ" {
			strict = r
		}
	}
	if strict.passed >= strict.legit {
		t.Fatal("Strict が正規を全部通してしまっている。題材が足りない")
	}
	t.Logf("Strict は正規を %d / %d しか通さない", strict.passed, strict.legit)

	// 全部止めて全部通す守りは、この一覧には無い。
	perfect := 0
	for _, r := range rows {
		if r.blocked == r.attacks && r.passed == r.legit {
			perfect++
			t.Logf("全部止めて全部通した: %s", r.name)
		}
	}
	if perfect != 0 {
		t.Fatalf("完全な守りがある: %d", perfect)
	}
}

// XSS が刺さると、合言葉は読めるようになるので効かなくなる。
func TestTokenIsUselessOnceScriptRunsOnYourPage(t *testing.T) {
	d := Defense{Token: true, SameSite: Lax}

	// 外からの攻撃は止まる。合言葉を知りようがない。
	outside := Attempt{Name: "攻撃: 隠しフォームで POST",
		Nav: Nav{CrossSite: true, TopLevel: true, Method: "POST"}, Origin: "https://evil.test"}
	if Check(d, session, outside).Accepted {
		t.Fatal("外からの攻撃が通った")
	}

	// 自分のページで動くスクリプトは、同一生成元なので合言葉を読める。
	inside := Attempt{Name: "攻撃: XSS で合言葉を読んで POST",
		Nav: Nav{Method: "POST"}, Origin: "https://bank.example", KnowsToken: true}
	r := Check(d, session, inside)
	t.Logf("XSS 経由 → %v (%s)", r.Accepted, r.Reason)
	if !r.Accepted {
		t.Fatal("XSS 経由が止まっている。合言葉は読めるはずなので、この守りでは止まらない")
	}
	t.Log("合言葉も SameSite も、同一生成元で動くスクリプトには効かない")
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 印の表示。
	for s, want := range map[SameSite]string{None: "None", Lax: "Lax", Strict: "Strict"} {
		if s.String() != want {
			t.Fatalf("%d の表示: %q", s, s.String())
		}
	}
	// HEAD も安全なメソッドなので Lax で通る。
	if !Sends(Lax, Nav{CrossSite: true, TopLevel: true, Method: "head"}) {
		t.Fatal("HEAD が Lax で止まった")
	}
	// Origin 検査は、同一サイトなら邪魔をしない。
	d := Defense{Origin: true, SameSite: None}
	if !Check(d, session, Attempt{Nav: Nav{Method: "POST"}}).Accepted {
		t.Fatal("Origin 検査が同一サイトを止めた")
	}
	// 攻撃かどうかの判定。
	all := Attempts()
	n := 0
	for _, a := range all {
		if IsAttack(a) {
			n++
		}
	}
	if n != 3 || len(all)-n != 2 {
		t.Fatalf("筋書きの内訳: 攻撃 %d / 正規 %d", n, len(all)-n)
	}
	// 空の合言葉は常に不正。
	if Valid(session, "") {
		t.Fatal("空が通った")
	}
}
