// Package sameorigin は、ブラウザの同一生成元ポリシーと CORS を最小構成で実装する。
//
// この仕組みが分かりにくいのは、名前が「同一生成元でないと駄目」に見えるからだ。
// 実際に既定で止まるのは**読み取りだけ**で、**送信は通る**。フォームの送信も
// 画像の読み込みもスクリプトの取得も、別のオリジンへ普通に飛ぶ。
// 応答を JavaScript から読めないだけになる。
//
// ここが分かると、2つの話が同じ性質の裏表だと見える。
//
//	CORS   読めないのを、サーバの許しで開ける仕組み
//	CSRF   送れることを、そのまま悪用する攻撃
//
// 実時間も乱数も使わない。同じ入力なら何度でも同じ結果になる。
package sameorigin

import (
	"net/url"
	"strconv"
	"strings"
)

// #region origin

// Origin は生成元。スキーム・ホスト・ポートの3つ組で、1つでも違えば別物になる。
type Origin struct {
	Scheme string
	Host   string
	Port   int
}

// 既定ポート。URL に書かれていなければこれが補われる。
var defaultPort = map[string]int{"http": 80, "https": 443}

// Parse は URL を生成元にする。
//
// ホストとスキームは小文字に畳む。ポートは省略されていれば既定を補う。
// パスやクエリは生成元に含まれない。ここを含めないのが要点で、
// 同じホストの別ページは互いに同じ生成元になる。
func Parse(raw string) (Origin, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Origin{}, err
	}
	o := Origin{
		Scheme: strings.ToLower(u.Scheme),
		Host:   strings.ToLower(u.Hostname()),
	}
	if p := u.Port(); p != "" {
		// url.Parse がここまでで数字であることを保証している。
		o.Port, _ = strconv.Atoi(p)
	} else {
		o.Port = defaultPort[o.Scheme]
	}
	return o, nil
}

// Same は2つが同じ生成元か。3つ組がすべて一致したときだけ真になる。
func Same(a, b Origin) bool {
	return a.Scheme == b.Scheme && a.Host == b.Host && a.Port == b.Port
}

// String は `Origin` ヘッダに載る形。既定ポートは省く。
func (o Origin) String() string {
	if o.Port == defaultPort[o.Scheme] {
		return o.Scheme + "://" + o.Host
	}
	return o.Scheme + "://" + o.Host + ":" + strconv.Itoa(o.Port)
}

// #endregion origin

// #region access

// Access は、あるやり方で別の生成元へ触ったときに何が通るか。
type Access struct {
	// Send は要求が相手に届くか。既定ではほとんど届く。
	Send bool
	// Read は応答を JavaScript から読めるか。既定ではほとんど読めない。
	Read bool
	// Cookie は相手のクッキーが自動で付くか。CSRF はここに乗る。
	Cookie bool
}

// CrossOrigin は、代表的なやり方ごとに既定で何が通るかを返す。
//
// 表にすると分かるが、Send はほぼ全部 true で、Read はほぼ全部 false になる。
// 「送れるのに読めない」というのがこの仕組みの実態だ。
func CrossOrigin(kind string) Access {
	switch kind {
	case "form": // <form method=POST action=別オリジン>
		return Access{Send: true, Read: false, Cookie: true}
	case "img": // <img src=別オリジン>
		return Access{Send: true, Read: false, Cookie: true}
	case "script": // <script src=別オリジン>
		return Access{Send: true, Read: false, Cookie: true}
	case "link": // <a href=別オリジン>
		return Access{Send: true, Read: false, Cookie: true}
	case "fetch": // fetch(別オリジン)。既定では credentials を付けない
		return Access{Send: true, Read: false, Cookie: false}
	case "fetch-credentials": // fetch(別オリジン, {credentials:"include"})
		return Access{Send: true, Read: false, Cookie: true}
	default:
		return Access{}
	}
}

// Kinds は CrossOrigin が答えられるやり方。
func Kinds() []string {
	return []string{"form", "img", "script", "link", "fetch", "fetch-credentials"}
}

// #endregion access

// #region preflight

// Request はブラウザが出そうとしている要求のうち、判定に使う部分。
type Request struct {
	Method      string
	ContentType string
	Headers     []string // 独自に足したヘッダの名前
}

// 単純要求として通るメソッド。これ以外は先に問い合わせが要る。
var simpleMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true}

// 単純要求として通る Content-Type。JSON がここに無いのが効いてくる。
var simpleTypes = map[string]bool{
	"application/x-www-form-urlencoded": true,
	"multipart/form-data":               true,
	"text/plain":                        true,
}

// 単純要求として通るヘッダ。だいたいブラウザが勝手に付けるものだけになる。
var simpleHeaders = map[string]bool{
	"accept": true, "accept-language": true, "content-language": true,
	"content-type": true,
}

// NeedsPreflight は、送る前に許可を問い合わせる必要があるか。
//
// 「単純要求」に当てはまらなければ、ブラウザは本番の要求の前に OPTIONS を
// 投げて許可を確かめる。ここで大事なのは、単純要求の条件が
// **フォームで送れる範囲**とほぼ重なっていることだ。
// フォームで送れるものは昔から送れていたので、いまさら止められない。
func NeedsPreflight(r Request) bool {
	if !simpleMethods[strings.ToUpper(r.Method)] {
		return true
	}
	if ct := mediaType(r.ContentType); ct != "" && !simpleTypes[ct] {
		return true
	}
	for _, h := range r.Headers {
		if !simpleHeaders[strings.ToLower(h)] {
			return true
		}
	}
	return false
}

func mediaType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// #endregion preflight

// #region policy

// Policy はサーバが「どこに読ませるか」を決めた設定。
type Policy struct {
	// Allow は読ませる生成元。
	Allow []string
	// Wildcard は「どこでも読んでよい」。
	Wildcard bool
	// Credentials はクッキー付きの要求でも読ませるか。
	Credentials bool
	// Reflect は受け取った Origin をそのまま返す実装。事故のもとになる。
	Reflect bool
}

// Decision はサーバが返す答え。
type Decision struct {
	// AllowOrigin は Access-Control-Allow-Origin に載せる値。空なら返さない。
	AllowOrigin string
	// AllowCredentials は Access-Control-Allow-Credentials を返すか。
	AllowCredentials bool
	// Readable はブラウザが応答を読ませるか。
	Readable bool
	// Reason は判定の理由。
	Reason string
}

// Decide は、その生成元からの要求に何を返すかを決める。
//
// 仕様の要点が1つある。**`*` とクッキー付きは同時に成り立たない**。
// どこからでも読めて、しかもログイン済みの応答が読めるなら、
// 読めないことにしていた意味が消えるからだ。
func Decide(p Policy, origin string, withCookie bool) Decision {
	switch {
	case p.Wildcard && withCookie && p.Credentials:
		// ブラウザがここで止める。サーバが返しても読ませない。
		return Decision{AllowOrigin: "*", AllowCredentials: true, Readable: false,
			Reason: "* とクッキー付きは同時に使えない"}
	case p.Wildcard && withCookie:
		return Decision{AllowOrigin: "*", Readable: false,
			Reason: "クッキー付きなのに許可が * になっている"}
	case p.Wildcard:
		return Decision{AllowOrigin: "*", Readable: true, Reason: "どこからでも読める"}
	case p.Reflect:
		// 受け取った値をそのまま返すので、事実上どこからでも読める。
		// しかも Credentials を立てられるぶん、* より広い。
		return Decision{AllowOrigin: origin, AllowCredentials: p.Credentials,
			Readable: true, Reason: "Origin をそのまま返している(実質すべて許可)"}
	default:
		for _, a := range p.Allow {
			if a == origin {
				if withCookie && !p.Credentials {
					return Decision{AllowOrigin: a, Readable: false,
						Reason: "許可した相手だが、クッキー付きは許していない"}
				}
				return Decision{AllowOrigin: a, AllowCredentials: p.Credentials,
					Readable: true, Reason: "名指しで許可されている"}
			}
		}
		return Decision{Readable: false, Reason: "許可した一覧に無い"}
	}
}

// EffectivelyOpen は、その設定が実質すべての生成元に読ませるか。
//
// `Reflect` は一覧に何も書かれていなくても全開になる。設定を読んだだけでは
// 気づきにくいので、判定として名前を付けておく。
func (p Policy) EffectivelyOpen() bool { return p.Wildcard || p.Reflect }

// #endregion policy
