// Package tls は TLS 風のハンドシェイクを最小構成で組む。これまでの暗号部品
// (鍵交換・署名・ハッシュ・AEAD)を 1 本の流れに束ねる総まとめの章。
//
// 安全な通信路を張るには、3 つを同時に満たす必要がある。盗聴されない(秘匿)、
// 相手が本物だと分かる(認証)、途中で書き換えられない(完全性)。鍵交換だけ
// では認証がなく中間者に破られる。そこで TLS は、証明書で相手を認証してから
// Diffie–Hellman で鍵を作り、その鍵で本文を AEAD で守る。ここでは、
//   - crypto/dh    … 一時鍵で共有秘密を作る(前方秘匿性)
//   - crypto/rsa   … 証明書とハンドシェイクの署名(相手認証)
//   - crypto/hash  … トランスクリプトのハッシュと鍵導出(HMAC)
//   - crypto/cipher… 導出鍵で本文を暗号化+認証(AEAD)
//
// を組み合わせ、正規の接続が成立すること、そして中間者が署名を偽造できず
// はじかれることを確かめる。
package tls

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/esh2n/sharin/crypto/cipher"
	"github.com/esh2n/sharin/crypto/dh"
	"github.com/esh2n/sharin/crypto/hash"
	"github.com/esh2n/sharin/crypto/rsa"
)

// group は鍵交換の公開パラメータ(教科書用の小さな素数)。
var group = dh.Params{P: big.NewInt(2147483647), G: big.NewInt(5)}

// #region certificate

// pubBytes は RSA 公開鍵を署名対象のバイト列にする。
func pubBytes(p rsa.PublicKey) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:], uint64(p.E))
	binary.LittleEndian.PutUint64(b[8:], uint64(p.N))
	return b
}

// digestInt はデータをハッシュし、玩具 RSA が扱える [0, n) の整数に落とす。
func digestInt(n int64, parts ...[]byte) int64 {
	var buf []byte
	for _, p := range parts {
		buf = append(buf, p...)
	}
	d := hash.Sum(buf)
	v := int64(binary.LittleEndian.Uint32(d[:4]))
	v %= n
	if v < 0 {
		v += n
	}
	return v
}

// Certificate は「この主体名は、この公開鍵の持ち主だ」という CA の保証。
type Certificate struct {
	Subject   string        // 例 "example.com"
	ServerPub rsa.PublicKey // サーバの署名用公開鍵
	Sig       int64         // CA が (Subject, ServerPub) に付けた署名
}

// CA は認証局。自分の秘密鍵で証明書に署名する。
type CA struct {
	Pub  rsa.PublicKey
	priv rsa.PrivateKey
}

// NewCA は認証局を作る。
func NewCA(p, q int64) *CA {
	pub, priv := rsa.GenKeyPair(p, q)
	return &CA{Pub: pub, priv: priv}
}

// Issue は subject と公開鍵を束ねて証明書に署名する。
func (ca *CA) Issue(subject string, serverPub rsa.PublicKey) Certificate {
	m := digestInt(ca.Pub.N, []byte(subject), pubBytes(serverPub))
	return Certificate{Subject: subject, ServerPub: serverPub, Sig: rsa.Sign(ca.priv, m)}
}

// VerifyCert は証明書が caPub の CA によって署名されたものかを確かめる。
func VerifyCert(caPub rsa.PublicKey, cert Certificate) bool {
	m := digestInt(caPub.N, []byte(cert.Subject), pubBytes(cert.ServerPub))
	return rsa.Verify(caPub, m, cert.Sig)
}

// #endregion certificate

// #region handshake

// ClientHello はクライアントが最初に送るもの。一時 DH 公開鍵と乱数。
type ClientHello struct {
	DHPub  *big.Int
	Random []byte
}

// ServerHello はサーバの応答。一時 DH 公開鍵・証明書・ハンドシェイク署名。
// 署名は「この DH 公開鍵は確かに証明書の持ち主が出した」ことを証す。
type ServerHello struct {
	DHPub *big.Int
	Cert  Certificate
	Sig   int64
}

// Session は確立後の共有鍵。以後の本文はこの鍵で守る。
type Session struct{ key []byte }

// transcript はハンドシェイクの記録(署名と鍵導出の入力)。
func transcript(clientPub, serverPub *big.Int, random []byte) []byte {
	var b []byte
	b = append(b, clientPub.Bytes()...)
	b = append(b, random...)
	b = append(b, serverPub.Bytes()...)
	return b
}

func deriveKey(shared *big.Int, ts []byte) []byte {
	k := hash.HMAC(shared.Bytes(), ts)
	return k[:]
}

// Client はハンドシェイクを始める側の状態。
type Client struct {
	subject string
	caPub   rsa.PublicKey
	dhPriv  *big.Int
	hello   ClientHello
}

// NewClient は subject へ接続するクライアントを作り、ClientHello を用意する。
func NewClient(subject string, caPub rsa.PublicKey, r *dh.Rand) *Client {
	priv, pub := group.Generate(r)
	random := []byte("client-random-01")
	return &Client{
		subject: subject,
		caPub:   caPub,
		dhPriv:  priv,
		hello:   ClientHello{DHPub: pub, Random: random},
	}
}

// Hello はサーバへ送る ClientHello を返す。
func (c *Client) Hello() ClientHello { return c.hello }

// Server はハンドシェイクに応じる側。証明書と、それに対応する RSA 秘密鍵を持つ。
type Server struct {
	cert    Certificate
	rsaPriv rsa.PrivateKey
}

// NewServer はサーバを作る(証明書と署名用秘密鍵は対応している必要がある)。
func NewServer(cert Certificate, rsaPriv rsa.PrivateKey) *Server {
	return &Server{cert: cert, rsaPriv: rsaPriv}
}

// Respond は ClientHello を受け、ServerHello と自分側の Session を返す。
// 一時 DH 鍵を作り、トランスクリプトに自分の RSA 秘密鍵で署名する。
func (s *Server) Respond(ch ClientHello, r *dh.Rand) (ServerHello, Session) {
	priv, pub := group.Generate(r)
	ts := transcript(ch.DHPub, pub, ch.Random)
	sig := rsa.Sign(s.rsaPriv, digestInt(s.cert.ServerPub.N, ts))

	shared := group.Shared(priv, ch.DHPub)
	return ServerHello{DHPub: pub, Cert: s.cert, Sig: sig}, Session{key: deriveKey(shared, ts)}
}

var (
	// ErrBadCert は証明書が信頼する CA の署名を持たないとき。
	ErrBadCert = errors.New("tls: certificate not signed by trusted CA")
	// ErrBadName は証明書の主体名が接続先と一致しないとき。
	ErrBadName = errors.New("tls: certificate subject mismatch")
	// ErrBadSig はハンドシェイク署名が検証できないとき(中間者の疑い)。
	ErrBadSig = errors.New("tls: handshake signature invalid")
)

// Finish はクライアントが ServerHello を検証し、Session を確立する。
// 証明書の署名・主体名・ハンドシェイク署名の 3 つを確かめてから鍵を導く。
// どれか 1 つでも欠ければ接続を拒否する(ここで中間者をはじく)。
func (c *Client) Finish(sh ServerHello) (Session, error) {
	if !VerifyCert(c.caPub, sh.Cert) {
		return Session{}, ErrBadCert
	}
	if sh.Cert.Subject != c.subject {
		return Session{}, ErrBadName
	}
	ts := transcript(c.hello.DHPub, sh.DHPub, c.hello.Random)
	if !rsa.Verify(sh.Cert.ServerPub, digestInt(sh.Cert.ServerPub.N, ts), sh.Sig) {
		return Session{}, ErrBadSig // DH 公開鍵が本人のものと証明できない
	}
	shared := group.Shared(c.dhPriv, sh.DHPub)
	return Session{key: deriveKey(shared, ts)}, nil
}

// #endregion handshake

// #region record

// nonce は本文保護に使う 8 バイト(教科書用の固定値。実物はレコードごとに変える)。
var nonce = []byte("tls-non1")

// Seal は確立した鍵で本文を暗号化+認証する(AEAD)。
func (se Session) Seal(plain []byte) []byte {
	c := cipher.NewCipher(se.key)
	return c.Seal(se.key, plain, nonce)
}

// Open は Seal の逆。改ざんされていれば false。
func (se Session) Open(sealed []byte) ([]byte, bool) {
	c := cipher.NewCipher(se.key)
	return c.Open(se.key, sealed, nonce)
}

// SameKey は 2 つの Session が同じ鍵を共有しているかを返す(検証用)。
func (se Session) SameKey(other Session) bool {
	if len(se.key) != len(other.key) {
		return false
	}
	var diff byte
	for i := range se.key {
		diff |= se.key[i] ^ other.key[i]
	}
	return diff == 0
}

// #endregion record
