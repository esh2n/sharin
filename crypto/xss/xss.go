// Package xss は、外から来た文字列をページに出すときの変換を実装する。
//
// この話が「エスケープすればいい」で終わらないのは、**正しい変換が出す場所ごとに
// 違う**からだ。同じ文字列でも、本文に出すのか、属性値に入れるのか、
// スクリプトの中に埋めるのか、リンク先にするのかで、危ない文字が変わる。
//
// だから「エスケープした」だけでは、何も言っていないのと同じになる。
// **どこへ出すために、何を変換したのか**まで言えて初めて意味を持つ。
//
// ここでは変換を5通り、出す場所を5通り作って、その総当たりを測る。
// 実時間も乱数も使わないので、何度走らせても同じ表になる。
package xss

import (
	"net/url"
	"strings"
)

// #region place

// Place は文字列を出す場所。危ない文字がここで決まる。
type Place int

const (
	// HTMLText は本文。<div>ここ</div>
	HTMLText Place = iota
	// QuotedAttr は引用符で囲んだ属性値。<img alt="ここ">
	QuotedAttr
	// BareAttr は引用符を書かなかった属性値。<img alt=ここ>
	BareAttr
	// JSString はスクリプトの中の文字列。<script>var s = "ここ"</script>
	JSString
	// URLValue はリンク先。<a href="ここ">
	URLValue
)

func (p Place) String() string {
	switch p {
	case QuotedAttr:
		return "属性値(引用符あり)"
	case BareAttr:
		return "属性値(引用符なし)"
	case JSString:
		return "スクリプトの中の文字列"
	case URLValue:
		return "リンク先"
	default:
		return "本文"
	}
}

// Places は測る対象の一覧。
func Places() []Place { return []Place{HTMLText, QuotedAttr, BareAttr, JSString, URLValue} }

// Render は、その場所へ値を埋めた結果の断片を返す。
func Render(p Place, v string) string {
	switch p {
	case QuotedAttr:
		return `<img alt="` + v + `">`
	case BareAttr:
		return `<img alt=` + v + `>`
	case JSString:
		return `<script>var s = "` + v + `";</script>`
	case URLValue:
		return `<a href="` + v + `">link</a>`
	default:
		return `<div>` + v + `</div>`
	}
}

// #endregion place

// #region escape

// Escape は変換のやり方。
type Escape int

const (
	// NoEscape は何もしない。
	NoEscape Escape = iota
	// HTMLEscape は & < > " ' を実体参照にする。いちばん広く使われる。
	HTMLEscape
	// JSEscape はスクリプトの中の文字列として安全にする。
	JSEscape
	// URLEscape は URL の一部として安全にする。
	URLEscape
	// SchemeCheck はリンク先のスキームを許可制にする。
	SchemeCheck
)

func (e Escape) String() string {
	switch e {
	case HTMLEscape:
		return "HTML エスケープ"
	case JSEscape:
		return "JS 文字列エスケープ"
	case URLEscape:
		return "URL エンコード"
	case SchemeCheck:
		return "スキーム検査"
	default:
		return "何もしない"
	}
}

// Escapes は測る対象の一覧。
func Escapes() []Escape { return []Escape{NoEscape, HTMLEscape, JSEscape, URLEscape, SchemeCheck} }

var htmlRepl = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

var jsRepl = strings.NewReplacer(
	`\`, `\\`, `"`, `\"`, "'", `\'`, "\n", `\n`, "\r", `\r`,
	// スクリプトの中では、文字列の中にいても </script> で外に出られてしまう。
	// だから < > も逃がす。実体参照ではなく、JS の書き方で書く。
	"<", `\u003C`, ">", `\u003E`)

// 許可するスキーム。相対 URL も通す。
var okScheme = map[string]bool{"http": true, "https": true, "mailto": true}

// Apply は変換を当てる。
func Apply(e Escape, v string) string {
	switch e {
	case HTMLEscape:
		return htmlRepl.Replace(v)
	case JSEscape:
		return jsRepl.Replace(v)
	case URLEscape:
		return url.QueryEscape(v)
	case SchemeCheck:
		if safeURL(v) {
			return v
		}
		return "#" // 相対 URL にして無害化する
	default:
		return v
	}
}

// safeURL は、そのリンク先が許可したスキームか。
//
// 実行できるスキーム(javascript: など)を弾くのが目的になる。
func safeURL(v string) bool {
	s := strings.TrimSpace(v)
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return true // スキームが無い = 相対 URL
	}
	// スキームの中に / や ? が来ていたら、それは区切りであってスキームではない。
	if j := strings.IndexAny(s, "/?#"); j >= 0 && j < i {
		return true
	}
	return okScheme[strings.ToLower(s[:i])]
}

// #endregion escape

// #region outcome

// Outcome は、その組み合わせで何が起きたか。
type Outcome int

const (
	// Executed は攻撃者のコードが動く。
	Executed Outcome = iota
	// Neutralized は無害な文字列として出る。
	Neutralized
	// Mangled は無害だが、まともな値としても壊れている。
	Mangled
)

func (o Outcome) String() string {
	switch o {
	case Executed:
		return "動く"
	case Mangled:
		return "壊れる"
	default:
		return "無害"
	}
}

// Attack は攻撃に使う文字列と、それがどこを狙っているか。
type Attack struct {
	Name    string
	Payload string
	// Targets はこの文字列が刺さる場所。
	Targets []Place
}

// Attacks は測るときの攻撃。それぞれ狙う場所が違う。
func Attacks() []Attack {
	return []Attack{
		{"タグを開く", `<script>alert(1)</script>`, []Place{HTMLText}},
		{"引用符を閉じて属性を足す", `" onerror="alert(1)`, []Place{QuotedAttr}},
		{"空白で属性を足す", `x onerror=alert(1)`, []Place{BareAttr}},
		{"文字列を閉じる", `";alert(1);//`, []Place{JSString}},
		{"スクリプトを閉じる", `</script><script>alert(1)</script>`, []Place{JSString}},
		{"実行するスキーム", `javascript:alert(1)`, []Place{URLValue}},
	}
}

// Check は、その場所にその変換を当てたとき、その攻撃がどうなるかを返す。
//
// 判定は素朴に、変換後の文字列に「外へ出るための文字」が残っているかで見る。
// 本物のブラウザの構文解析ではないが、どの変換がどの場所で足りないかは、
// これで十分に分かれる。
func Check(p Place, e Escape, a Attack) Outcome {
	v := Apply(e, a.Payload)

	// その場所を狙った攻撃でなければ、そもそも刺さらない。
	hit := false
	for _, t := range a.Targets {
		if t == p {
			hit = true
		}
	}
	if !hit {
		return Neutralized
	}

	switch p {
	case HTMLText:
		if strings.Contains(v, "<") {
			return Executed
		}
	case QuotedAttr:
		if strings.Contains(v, `"`) {
			return Executed
		}
	case BareAttr:
		// 引用符で囲っていないので、空白1つで次の属性になってしまう。
		// HTML エスケープは空白を変換しないので、ここでは効かない。
		if strings.ContainsAny(v, " \t\n") {
			return Executed
		}
	case JSString:
		// 文字列を閉じる " か、スクリプトを閉じる < が残っていれば出られる。
		// ここだけは \" のように逃がされた引用符を数えてはいけない。
		// 属性値では逆で、HTML に \ の逃がしは無いので " はそのまま閉じる。
		if hasBareQuote(v) || strings.Contains(v, "<") {
			return Executed
		}
	case URLValue:
		if safeURL(v) {
			break
		}
		return Executed
	}

	// 無害にはなったが、値として無事とは限らない。
	// その場所のための変換でないのに何かを変えたなら、出したいものが壊れている。
	// 例えば HTML エスケープをスクリプトの中に当てると、攻撃は止まるが
	// 文字列の中に &quot; がそのまま見えることになる。
	if v != a.Payload && e != Right(p) {
		return Mangled
	}
	return Neutralized
}

// hasBareQuote は、逃がされていない " が残っているか。
//
// \" は文字列を閉じないので、数えない。\\ は \ そのものなので、
// その次の " は逃がされていないことになる。
func hasBareQuote(v string) bool {
	esc := false
	for _, c := range v {
		switch {
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case c == '"':
			return true
		}
	}
	return false
}

// #endregion outcome

// #region score

// Score は、その変換がその場所で攻撃を何件止め、値を壊さなかったか。
func Score(p Place, e Escape) (stopped, total, intact int) {
	for _, a := range Attacks() {
		hit := false
		for _, t := range a.Targets {
			if t == p {
				hit = true
			}
		}
		if !hit {
			continue
		}
		total++
		switch Check(p, e, a) {
		case Neutralized:
			stopped++
			intact++
		case Mangled:
			stopped++
		}
	}
	return
}

// Right は、その場所で使うべき変換。
//
// 「エスケープする」ではなく「**この場所のための**変換をする」という形にしないと、
// 場所が変わったときに黙って外れる。
func Right(p Place) Escape {
	switch p {
	case JSString:
		return JSEscape
	case URLValue:
		return SchemeCheck
	default:
		return HTMLEscape
	}
}

// FixableByEscaping は、その場所が変換だけで守れるか。
//
// 1 か所だけ false になる。引用符を書かなかった属性値は、
// **値を壊さずに守れる変換が存在しない**。空白1つで次の属性になってしまうので、
// 空白を潰すしかなく、潰すと普通の値も壊れる。
// ここは変換で直すところではなく、**引用符を書く**ところになる。
func FixableByEscaping(p Place) bool { return p != BareAttr }

// #endregion score
