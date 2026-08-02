// Package csrf は、別のサイトから勝手に送られる要求と、その止め方を実装する。
//
// [送れるのに読めない](same-origin)で見たとおり、別の生成元へ要求は届くし、
// クッキーも自動で付く。止まるのは応答の読み取りだけになる。
// つまり**実行が目的で、応答が要らない用件**は、そのまま通ってしまう。
// 送金や退会や設定変更は、まさにそれに当たる。
//
// 止め方は2系統ある。どちらも同じ穴を塞いでいるようで、塞ぐ場所が違う。
//
//	トークン    サーバが出す合言葉。送れるが読めないので、攻撃者は付けられない
//	SameSite   ブラウザ側で、クッキーを付けるのをやめる
//
// 実時間も乱数も使わない。合言葉は擬似乱数の整数 LCG から作るので、
// 同じ入力なら何度でも同じ値になる。
package csrf

import (
	"strconv"
	"strings"
)

// #region token

// 擬似乱数の定数。実時間も乱数源も使わないので、何度走らせても同じ値になる。
const (
	lcgMul = 6364136223846793005
	lcgAdd = 1442695040888963407
)

// Issue はセッションに紐づく合言葉を出す。
//
// 実物は署名や乱数で作るが、ここでは決定的に導出する。要点は値の作り方ではなく、
// **攻撃者がこの値を知りようがない**ことのほうにある。
// 出すのはサーバで、載るのはページの中。そして別の生成元からはページを読めない。
func Issue(session string) string {
	var s uint64 = 1469598103934665603
	for _, c := range []byte(session) {
		s = s*lcgMul + lcgAdd + uint64(c)
	}
	return strconv.FormatUint(s>>16, 36)
}

// Valid は送られてきた合言葉が、そのセッションのものか。
func Valid(session, token string) bool { return token != "" && Issue(session) == token }

// #endregion token

// #region samesite

// SameSite はクッキーに付ける印。ブラウザが「別サイトからのとき付けるか」を決める。
type SameSite int

const (
	// None は昔からの振る舞い。別サイトからでも付く。
	None SameSite = iota
	// Lax は既定。書き換えない移動(リンクを踏む等)でだけ付く。
	Lax
	// Strict は別サイトからは一切付けない。
	Strict
)

func (s SameSite) String() string {
	switch s {
	case Lax:
		return "Lax"
	case Strict:
		return "Strict"
	default:
		return "None"
	}
}

// Nav はブラウザから見た要求の出かた。
type Nav struct {
	// CrossSite は別サイトから出た要求か。
	CrossSite bool
	// TopLevel はアドレス欄が変わる移動か。リンクやフォームの送信が該当する。
	// 画像の読み込みや fetch は該当しない。
	TopLevel bool
	// Method は HTTP のメソッド。
	Method string
}

// Sends は、その印のクッキーがこの要求に付くか。
//
// Lax の線は「**書き換えないはずの移動**」で引かれている。リンクを踏んだ GET は
// 付き、フォームの POST は付かない。検索から飛んできたときにログイン状態が
// 消えていると困る、という都合と、送信を止めたい都合の折り合いになる。
func Sends(s SameSite, n Nav) bool {
	if !n.CrossSite {
		return true
	}
	switch s {
	case Strict:
		return false
	case Lax:
		return n.TopLevel && safeMethod(n.Method)
	default:
		return true
	}
}

// safeMethod は、決まりの上で「状態を変えない」ことになっているメソッドか。
func safeMethod(m string) bool {
	switch strings.ToUpper(m) {
	case "GET", "HEAD", "OPTIONS", "TRACE":
		return true
	}
	return false
}

// #endregion samesite

// #region defense

// Defense はサーバ側に置いた守り。
type Defense struct {
	// Token は合言葉を確かめるか。
	Token bool
	// Cookie に付ける印。
	SameSite SameSite
	// Origin は Origin ヘッダが自分のものか確かめるか。
	Origin bool
}

// Attempt は1回の要求。
type Attempt struct {
	Name string
	Nav  Nav
	// Origin は要求元の生成元。
	Origin string
	// KnowsToken は送り手が合言葉を知っているか。
	// 正規の画面は知っている。攻撃者のページは読めないので知らない。
	// ただし XSS が刺さると読めてしまう。
	KnowsToken bool
}

// Result は判定。
type Result struct {
	// CookieSent はクッキーが付いたか。付かなければ、そもそも本人と見なされない。
	CookieSent bool
	// Accepted は要求が受け付けられたか。
	Accepted bool
	// Reason は止めた側の理由。
	Reason string
}

// Check は、その守りでその要求が通るかを判定する。
//
// 順番に意味がある。クッキーが付かなければ、合言葉を見るまでもなく他人の要求だ。
func Check(d Defense, session string, a Attempt) Result {
	if !Sends(d.SameSite, a.Nav) {
		return Result{Reason: "SameSite=" + d.SameSite.String() + " でクッキーが付かない"}
	}
	r := Result{CookieSent: true}
	if d.Origin && a.Nav.CrossSite {
		r.Reason = "Origin が自分のものでない"
		return r
	}
	// 合言葉を確かめるのは、状態を変える要求だけにする。読むだけの GET にも
	// 求めると、他サイトからのリンク流入がすべて弾かれてしまう。
	// ここが後で効いてくる。**GET で状態を変える設計だと、この守りの外に出る**。
	if d.Token && !safeMethod(a.Nav.Method) {
		tok := ""
		if a.KnowsToken {
			tok = Issue(session)
		}
		if !Valid(session, tok) {
			r.Reason = "合言葉が無いか、合っていない"
			return r
		}
	}
	r.Accepted = true
	r.Reason = "通った"
	return r
}

// #endregion defense

// #region matrix

// Attempts は測るときの筋書き。攻撃と正規の両方を含める。
//
// 守りは「攻撃を止める」だけでなく「正規を通す」必要がある。
// 片方だけ見ると、全部止める守りがいちばん強く見えてしまう。
func Attempts() []Attempt {
	const me, evil = "https://bank.example", "https://evil.test"
	return []Attempt{
		{Name: "正規の画面から POST", Nav: Nav{Method: "POST"}, Origin: me, KnowsToken: true},
		{Name: "他サイトからリンクで流入", Nav: Nav{CrossSite: true, TopLevel: true, Method: "GET"}, Origin: evil},
		{Name: "攻撃: 隠しフォームで POST", Nav: Nav{CrossSite: true, TopLevel: true, Method: "POST"}, Origin: evil},
		{Name: "攻撃: img で GET", Nav: Nav{CrossSite: true, Method: "GET"}, Origin: evil},
		{Name: "攻撃: XSS で合言葉を読んで POST", Nav: Nav{Method: "POST"}, Origin: me, KnowsToken: true},
	}
}

// IsAttack はその筋書きが攻撃側か。
func IsAttack(a Attempt) bool { return strings.HasPrefix(a.Name, "攻撃") }

// Defenses は測るときの守り。
func Defenses() []struct {
	Name string
	D    Defense
} {
	return []struct {
		Name string
		D    Defense
	}{
		{"何もしない", Defense{SameSite: None}},
		{"合言葉だけ", Defense{Token: true, SameSite: None}},
		{"SameSite=Lax だけ", Defense{SameSite: Lax}},
		{"SameSite=Strict だけ", Defense{SameSite: Strict}},
		{"合言葉 + Lax", Defense{Token: true, SameSite: Lax}},
		{"Origin 検査 + Lax", Defense{Origin: true, SameSite: Lax}},
	}
}

// Score は、その守りで攻撃を何件止め、正規を何件通したか。
func Score(d Defense, session string) (blocked, attacks, passed, legit int) {
	for _, a := range Attempts() {
		r := Check(d, session, a)
		if IsAttack(a) {
			attacks++
			if !r.Accepted {
				blocked++
			}
			continue
		}
		legit++
		if r.Accepted {
			passed++
		}
	}
	return
}

// #endregion matrix
