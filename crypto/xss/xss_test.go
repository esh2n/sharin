package xss

import "testing"

// この章の中心その1。同じ変換でも、出す場所によって効いたり効かなかったりする。
func TestOneEscapeIsNotEnoughForEveryPlace(t *testing.T) {
	t.Logf("HTML エスケープだけを、5つの場所すべてに当てる")
	t.Logf("%-24s %8s %8s %s", "出す場所", "止めた", "値も無事", "")
	ok := 0
	for _, p := range Places() {
		s, total, intact := Score(p, HTMLEscape)
		mark := ""
		switch {
		case s < total:
			mark = "← 止まらない"
		case intact < total:
			mark = "← 止まるが値が壊れる"
		default:
			ok++
		}
		t.Logf("%-24s %4d / %-3d %4d / %-3d %s", p, s, total, intact, total, mark)
	}
	t.Logf("攻撃を止めて値も無事だったのは、5 か所のうち %d か所", ok)

	// 本文では足りる。ここだけを見て「エスケープすればよい」と思い込みやすい。
	s, total, intact := Score(HTMLText, HTMLEscape)
	if s != total || intact != total || total == 0 {
		t.Fatalf("本文で成立しない: %d/%d 無事 %d", s, total, intact)
	}
	// だが 5 か所すべてでは成立しない。
	if ok >= len(Places()) {
		t.Fatal("どこでも足りてしまっている。題材が足りない")
	}
}

// この章の中心その2。引用符を書かないと、HTML エスケープでも防げない。
func TestUnquotedAttributeDefeatsHtmlEscaping(t *testing.T) {
	a := Attack{"空白で属性を足す", `x onerror=alert(1)`, []Place{BareAttr, QuotedAttr}}

	bare := Check(BareAttr, HTMLEscape, a)
	quoted := Check(QuotedAttr, HTMLEscape, a)
	t.Logf("同じ文字列・同じ変換で")
	t.Logf("  引用符なし %s → %v", Render(BareAttr, Apply(HTMLEscape, a.Payload)), bare)
	t.Logf("  引用符あり %s → %v", Render(QuotedAttr, Apply(HTMLEscape, a.Payload)), quoted)

	if bare != Executed {
		t.Fatal("引用符なしで止まってしまった")
	}
	if quoted != Neutralized {
		t.Fatal("引用符ありで止まらない")
	}
	// 理由は単純で、HTML エスケープは空白を変換しないからだ。
	if got := Apply(HTMLEscape, a.Payload); got != a.Payload {
		t.Fatalf("空白や = が変換されている: %q", got)
	}
	t.Log("HTML エスケープは空白を変換しない。引用符が無ければ、空白1つで次の属性になる")
}

// この章の中心その3。スクリプトの中では、文字列を閉じなくても外に出られる。
func TestInsideScriptYouCanLeaveWithoutClosingTheString(t *testing.T) {
	closeStr := Attack{"文字列を閉じる", `";alert(1);//`, []Place{JSString}}
	closeTag := Attack{"スクリプトを閉じる", `</script><script>alert(1)</script>`, []Place{JSString}}

	// 引用符だけ潰す変換では、タグを閉じる側が残る。
	onlyQuote := func(v string) string {
		out := ""
		for _, c := range v {
			if c == '"' {
				out += `\"`
				continue
			}
			out += string(c)
		}
		return out
	}
	t.Logf("引用符だけ潰した結果: %s", onlyQuote(closeTag.Payload))
	if !contains(onlyQuote(closeTag.Payload), "</script>") {
		t.Fatal("タグを閉じる文字列が残っていない")
	}

	// 用意した JS 文字列エスケープは、引用符も < > も落とす。
	for _, a := range []Attack{closeStr, closeTag} {
		if got := Check(JSString, JSEscape, a); got != Neutralized {
			t.Fatalf("%s が %v", a.Name, got)
		}
	}
	// HTML エスケープを当てると、止まりはする。実体参照はスクリプトの中で
	// 展開されないので、文字列を閉じる引用符にはならないからだ。
	// だが同じ理由で、**値の中に &quot; がそのまま見える**。
	for _, a := range []Attack{closeStr, closeTag} {
		if got := Check(JSString, HTMLEscape, a); got != Mangled {
			t.Fatalf("%s が HTML エスケープで %v", a.Name, got)
		}
	}
	t.Logf("HTML エスケープの結果: %s", Apply(HTMLEscape, closeStr.Payload))
	t.Log("止まってはいる。だが実体参照が文字として残るので、出したい値のほうが壊れる")
}

// この章の中心その4。リンク先は、文字を変換するのではなく、行き先を選ぶ。
func TestLinksNeedAnAllowListNotAnEscape(t *testing.T) {
	a := Attack{"実行するスキーム", `javascript:alert(1)`, []Place{URLValue}}

	t.Logf("%-16s %-10s %s", "変換", "結果", "出力")
	for _, e := range []Escape{NoEscape, HTMLEscape, URLEscape, SchemeCheck} {
		t.Logf("%-16s %-10v %s", e, Check(URLValue, e, a), Apply(e, a.Payload))
	}

	// HTML エスケープは、この文字列を1文字も変えない。
	if Apply(HTMLEscape, a.Payload) != a.Payload {
		t.Fatal("HTML エスケープが何か変えている")
	}
	if Check(URLValue, HTMLEscape, a) != Executed {
		t.Fatal("HTML エスケープで止まってしまった")
	}
	// スキームを許可制にすると止まる。
	if Check(URLValue, SchemeCheck, a) != Neutralized {
		t.Fatal("スキーム検査で止まらない")
	}
	// 相対 URL や普通のリンクは通す。
	for _, ok := range []string{"/about", "https://example.com/x", "mailto:a@example.com", "?q=1"} {
		if !safeURL(ok) {
			t.Fatalf("普通のリンクを弾いた: %q", ok)
		}
	}
	for _, ng := range []string{"javascript:alert(1)", "JavaScript:alert(1)", " javascript:alert(1)", "data:text/html,<script>"} {
		if safeURL(ng) {
			t.Fatalf("実行できるスキームを通した: %q", ng)
		}
	}
}

// 場所ごとに正しい変換を選ぶと止まる。ただし 1 か所だけ、変換では直せない。
func TestOnePlaceCannotBeFixedByEscapingAtAll(t *testing.T) {
	t.Logf("%-24s %-20s %s", "出す場所", "使うべき変換", "止めた(値も無事)")
	for _, p := range Places() {
		e := Right(p)
		s, total, intact := Score(p, e)
		mark := ""
		if !FixableByEscaping(p) {
			mark = "← 変換では直せない"
		}
		t.Logf("%-24s %-20s %d / %d(%d) %s", p, e, s, total, intact, mark)
		if !FixableByEscaping(p) {
			continue
		}
		if s != total || intact != total {
			t.Fatalf("%v で成立しない: 止めた %d/%d 無事 %d", p, s, total, intact)
		}
	}

	// 引用符なしの属性値は、どの変換を当てても「止めて、かつ値が無事」にならない。
	for _, e := range Escapes() {
		s, total, intact := Score(BareAttr, e)
		t.Logf("  引用符なしに %-16v → 止めた %d/%d 無事 %d", e, s, total, intact)
		if s == total && intact == total {
			t.Fatalf("%v で守れてしまった", e)
		}
	}
	t.Log("空白を潰せば止まるが、普通の値も壊れる。ここは引用符を書くところになる")

	// 逆に、1つの変換を全部の場所に当てて「止めて、かつ値が無事」になるものは無い。
	// URL エンコードは全部止めるが、代わりに全部の値を壊す。
	t.Logf("%-16s %s", "変換", "止めて値も無事だった場所")
	for _, e := range Escapes() {
		good := 0
		for _, p := range Places() {
			s, total, intact := Score(p, e)
			if s == total && intact == total {
				good++
			}
		}
		t.Logf("%-16v %d / %d", e, good, len(Places()))
		if good == len(Places()) {
			t.Fatalf("%v が 5 か所すべてで成立した。それはおかしい", e)
		}
	}
	t.Log("どの変換も、単独で 5 か所すべてを守りきれない")
}

// 場所を間違えると、無害にはなっても値が壊れる。
func TestWrongEscapeCanAlsoBreakTheValue(t *testing.T) {
	// URL エンコードを本文に当てると、攻撃は止まるが日本語も記号も潰れる。
	a := Attack{"タグを開く", `<script>alert(1)</script>`, []Place{HTMLText}}
	if got := Check(HTMLText, URLEscape, a); got != Mangled {
		t.Fatalf("壊れの判定: %v", got)
	}
	t.Logf("URL エンコードを本文に当てた結果: %s", Apply(URLEscape, "こんにちは & さようなら"))
	t.Log("止まってはいるが、読めるものが出ていない。無害と正しいは別になる")
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 表示名。
	for _, p := range Places() {
		if p.String() == "" {
			t.Fatal("場所の名前が空")
		}
	}
	for _, e := range Escapes() {
		if e.String() == "" {
			t.Fatal("変換の名前が空")
		}
	}
	if Executed.String() != "動く" || Neutralized.String() != "無害" || Mangled.String() != "壊れる" {
		t.Fatal("結果の名前が違う")
	}
	// 狙っていない場所では刺さらない。
	a := Attack{"タグを開く", `<script>alert(1)</script>`, []Place{HTMLText}}
	if Check(URLValue, NoEscape, a) != Neutralized {
		t.Fatal("狙っていない場所で動いた")
	}
	// 何もしなければ、狙われた場所では動く。
	if Check(HTMLText, NoEscape, a) != Executed {
		t.Fatal("素通しで止まった")
	}
	// 空文字はどこでも無害。
	empty := Attack{"空", "", Places()}
	for _, p := range Places() {
		if Check(p, NoEscape, empty) == Executed {
			t.Fatalf("空文字が %v で動いた", p)
		}
	}
	// 埋め込みの形。5 か所ぶん、値がそのまま入ることを見ておく。
	for _, p := range Places() {
		if !contains(Render(p, "MARK"), "MARK") {
			t.Fatalf("%v に値が入っていない: %s", p, Render(p, "MARK"))
		}
	}
	if Render(HTMLText, "x") != "<div>x</div>" {
		t.Fatalf("本文の形: %q", Render(HTMLText, "x"))
	}
	// スキームの前に区切りが来る形。これはスキームではないので通す。
	if !safeURL("/a:b") || !safeURL("#x:y") {
		t.Fatal("区切りが先に来る相対 URL を弾いた")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
