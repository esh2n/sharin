package tls

import (
	"bytes"
	"testing"

	"github.com/esh2n/sharin/crypto/dh"
	"github.com/esh2n/sharin/crypto/rsa"
)

// setup は CA・サーバ証明書・サーバを用意する共通の下ごしらえ。
func setup() (*CA, *Server) {
	ca := NewCA(61, 53) // CA の鍵(n=3233)
	serverPub, serverPriv := rsa.GenKeyPair(67, 71)
	cert := ca.Issue("example.com", serverPub)
	return ca, NewServer(cert, serverPriv)
}

// TestHandshakeEstablishesSharedKey はこの章の主眼その 1。
// 正規のハンドシェイクで、クライアントとサーバが同じ鍵に到達し、
// その鍵で本文を安全にやり取りできることを固定する。
func TestHandshakeEstablishesSharedKey(t *testing.T) {
	ca, server := setup()

	client := NewClient("example.com", ca.Pub, dh.NewRand(1))
	sh, serverSession := server.Respond(client.Hello(), dh.NewRand(2))
	clientSession, err := client.Finish(sh)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if !clientSession.SameKey(serverSession) {
		t.Fatal("client and server derived different keys")
	}

	// 確立した鍵で本文を往復させる。
	msg := []byte("GET /secret HTTP/1.1")
	sealed := clientSession.Seal(msg)
	got, ok := serverSession.Open(sealed)
	if !ok || !bytes.Equal(got, msg) {
		t.Fatalf("record protection failed: ok=%v got=%q", ok, got)
	}
}

func TestCertVerification(t *testing.T) {
	ca, server := setup()
	if !VerifyCert(ca.Pub, server.cert) {
		t.Fatal("legit cert must verify under its CA")
	}
	// 別の CA では検証が通らない。
	other := NewCA(83, 79)
	if VerifyCert(other.Pub, server.cert) {
		t.Fatal("cert must NOT verify under a different CA")
	}
}

// TestRejectsUntrustedCA は、CA が信頼できない証明書を出しても弾くことを示す。
func TestRejectsUntrustedCA(t *testing.T) {
	trustedCA, _ := setup()
	// 攻撃者が自前の CA で "example.com" の証明書を勝手に発行。
	rogue := NewCA(83, 79)
	fakePub, fakePriv := rsa.GenKeyPair(67, 71)
	fakeCert := rogue.Issue("example.com", fakePub)
	rogueServer := NewServer(fakeCert, fakePriv)

	client := NewClient("example.com", trustedCA.Pub, dh.NewRand(1))
	sh, _ := rogueServer.Respond(client.Hello(), dh.NewRand(2))
	if _, err := client.Finish(sh); err != ErrBadCert {
		t.Fatalf("expected ErrBadCert, got %v", err)
	}
}

func TestRejectsWrongSubject(t *testing.T) {
	ca, server := setup()
	// クライアントは別ドメインへ接続したつもり。
	client := NewClient("evil.com", ca.Pub, dh.NewRand(1))
	sh, _ := server.Respond(client.Hello(), dh.NewRand(2))
	if _, err := client.Finish(sh); err != ErrBadName {
		t.Fatalf("expected ErrBadName, got %v", err)
	}
}

// TestMITMForgedKeyRejected はこの章の主眼その 2。中間者が DH 公開鍵を
// 自分のものにすり替えても、ハンドシェイク署名を偽造できないので弾かれる。
// 鍵交換に認証を足すと中間者攻撃が防げる、という [key-exchange] の続き。
func TestMITMForgedKeyRejected(t *testing.T) {
	ca, server := setup()
	client := NewClient("example.com", ca.Pub, dh.NewRand(1))
	sh, _ := server.Respond(client.Hello(), dh.NewRand(2))

	// Mallory が ServerHello の DH 公開鍵を自分のものにすり替える。
	// 証明書と署名はそのまま(署名は書き換えられない)。
	_, mPub := group.Generate(dh.NewRand(99))
	tampered := ServerHello{DHPub: mPub, Cert: sh.Cert, Sig: sh.Sig}

	if _, err := client.Finish(tampered); err != ErrBadSig {
		t.Fatalf("MITM should be rejected with ErrBadSig, got %v", err)
	}
}

func TestTamperedRecordRejected(t *testing.T) {
	ca, server := setup()
	client := NewClient("example.com", ca.Pub, dh.NewRand(1))
	sh, serverSession := server.Respond(client.Hello(), dh.NewRand(2))
	clientSession, err := client.Finish(sh)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	sealed := clientSession.Seal([]byte("transfer 100 dollars"))
	sealed[10] ^= 0x20 // 改ざん
	if _, ok := serverSession.Open(sealed); ok {
		t.Fatal("tampered record must be rejected")
	}
}
