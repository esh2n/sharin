// Package oauth は OAuth 2.0 の認可コードフロー(PKCE 付き)を最小構成で実装する。
//
// 「このアプリにあなたの写真へのアクセスを許可しますか」。第三者アプリに自分の
// パスワードを渡すのは危険だ。パスワードを知られれば全権限を握られ、変更もできず、
// 範囲も限定できない。OAuth は代わりにトークンを渡す。ユーザは認可サーバでログインし、
// アプリには「写真を読む」だけの権限を持つトークンが渡る。パスワードはアプリに触れない。
//
// 肝は 2 つ。認可コードフローは、ブラウザ経由(フロントチャネル)には短命の「コード」
// だけを流し、トークン本体はアプリとサーバの直接通信(バックチャネル)で交換する。
// トークンがブラウザ履歴やリダイレクト URL に残らない。PKCE は、秘密を持てない
// 公開クライアント(SPA やモバイル)向けに、コードを横取りされても使えなくする。
// クライアントは検証子を作り、そのハッシュを先に預ける。コード交換時に検証子の
// 原本を示せた者だけがトークンを得られる。
package oauth

import (
	"errors"
	"strings"

	"github.com/esh2n/sharin/crypto/hash"
)

// #region token

// hexBytes は指紋を 16 進文字列にする。
func hexBytes(b [hash.Size]byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, hash.Size*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0xf]
	}
	return string(out)
}

// issueToken は subject と scope と有効期限を HMAC で署名したトークンを作る。
// 形式は "sub|scope|exp.署名"。署名があるので中身を書き換えると検出できる。
func issueToken(secret []byte, sub, scope string, exp int) string {
	body := sub + "|" + scope + "|" + itoa(exp)
	tag := hash.HMAC(secret, []byte(body))
	return body + "." + hexBytes(tag)
}

// parseToken は署名を検証し、正しければ (sub, scope, exp) を返す。
func parseToken(secret []byte, token string) (sub, scope string, exp int, ok bool) {
	dot := strings.LastIndexByte(token, '.')
	if dot < 0 {
		return "", "", 0, false
	}
	body, sig := token[:dot], token[dot+1:]
	want := hexBytes(hash.HMAC(secret, []byte(body)))
	if !constEq(sig, want) {
		return "", "", 0, false // 署名が合わない = 改ざん
	}
	parts := strings.Split(body, "|")
	if len(parts) != 3 {
		return "", "", 0, false
	}
	e, ok := atoi(parts[2])
	if !ok {
		return "", "", 0, false
	}
	return parts[0], parts[1], e, true
}

// ResourceServer は保護された API。トークンを検証してアクセスを判断する。
type ResourceServer struct {
	secret []byte
	now    int
}

// NewResourceServer は認可サーバと同じ秘密鍵を持つ資源サーバを作る。
func NewResourceServer(secret []byte) *ResourceServer { return &ResourceServer{secret: secret} }

// Advance は論理時計を進める。
func (rs *ResourceServer) Advance(d int) { rs.now += d }

// Validate はトークンの署名・有効期限・scope を確かめる。
// requiredScope を含まなければ拒否する(最小権限)。
func (rs *ResourceServer) Validate(token, requiredScope string) bool {
	_, scope, exp, ok := parseToken(rs.secret, token)
	if !ok || rs.now >= exp {
		return false
	}
	return hasScope(scope, requiredScope)
}

func hasScope(granted, required string) bool {
	for _, s := range strings.Fields(granted) {
		if s == required {
			return true
		}
	}
	return false
}

// #endregion token

// #region flow

// Challenge は PKCE の code_challenge を検証子から作る(そのハッシュの 16 進)。
// クライアントは検証子を秘密に持ち、この challenge だけを認可要求で先に預ける。
func Challenge(verifier string) string {
	return hexBytes(hash.Sum([]byte(verifier)))
}

// grant は認可コードに紐づく保留中の許可。
type grant struct {
	clientID  string
	sub       string
	scope     string
	challenge string // PKCE の code_challenge
	exp       int    // コードの有効期限
	used      bool   // 一度使ったら無効(再利用防止)
}

var (
	// ErrBadCode は未知・失効・再利用されたコード。
	ErrBadCode = errors.New("oauth: invalid or expired code")
	// ErrBadVerifier は PKCE 検証子が challenge と一致しないとき。
	ErrBadVerifier = errors.New("oauth: pkce verifier mismatch")
	// ErrClientMismatch はコード発行時と交換時でクライアントが違うとき。
	ErrClientMismatch = errors.New("oauth: client mismatch")
)

// AuthServer は認可サーバ。ユーザ認証の後にコードを発行し、コードをトークンに交換する。
type AuthServer struct {
	secret   []byte
	now      int
	codeTTL  int
	tokenTTL int
	codes    map[string]*grant
	ids      *idGen
}

// NewAuthServer は認可サーバを作る。secret はトークン署名と資源サーバで共有する。
func NewAuthServer(secret []byte, codeTTL, tokenTTL int, seed uint64) *AuthServer {
	return &AuthServer{
		secret:   secret,
		codeTTL:  codeTTL,
		tokenTTL: tokenTTL,
		codes:    make(map[string]*grant),
		ids:      &idGen{state: seed},
	}
}

// Advance は論理時計を進める。
func (s *AuthServer) Advance(d int) { s.now += d }

// Authorize はユーザ認証済みとして認可コードを発行する。
// ここで受け取るのは challenge だけ。検証子そのものは受け取らない(横取り対策)。
// フロントチャネル(ブラウザのリダイレクト)で返るのはこの短命なコードだけ。
func (s *AuthServer) Authorize(clientID, sub, scope, challenge string) string {
	code := s.ids.next()
	s.codes[code] = &grant{
		clientID:  clientID,
		sub:       sub,
		scope:     scope,
		challenge: challenge,
		exp:       s.now + s.codeTTL,
	}
	return code
}

// Exchange はバックチャネルでコードをトークンに交換する。
// コードが有効で、クライアントが一致し、PKCE 検証子のハッシュが預けた
// challenge と一致したときだけトークンを発行する。コードは一度で使い切る。
func (s *AuthServer) Exchange(clientID, code, verifier string) (string, error) {
	g, ok := s.codes[code]
	if !ok || g.used || s.now >= g.exp {
		return "", ErrBadCode
	}
	if g.clientID != clientID {
		return "", ErrClientMismatch
	}
	if Challenge(verifier) != g.challenge {
		return "", ErrBadVerifier // 検証子の原本を示せない = 横取りしただけの者
	}
	g.used = true // 再利用を防ぐ
	return issueToken(s.secret, g.sub, g.scope, s.now+s.tokenTTL), nil
}

// #endregion flow
