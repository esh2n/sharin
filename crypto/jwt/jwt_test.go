package jwt

import (
	"strings"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	secret := []byte("my-secret-key")
	token, err := Sign(Claims{Sub: "user-42", Exp: time.Now().Add(time.Hour).Unix()}, secret)
	if err != nil {
		t.Fatal(err)
	}
	// JWT は "header.payload.signature" の3パート(ドット区切り)。
	if strings.Count(token, ".") != 2 {
		t.Errorf("JWT は3パートのはず: %q", token)
	}

	claims, err := Verify(token, secret)
	if err != nil {
		t.Fatalf("正しいトークンが検証に落ちた: %v", err)
	}
	if claims.Sub != "user-42" {
		t.Errorf("Sub = %q, want user-42", claims.Sub)
	}
}

// 署名の肝: 秘密鍵を知らない者はトークンを改竄できない。
func TestTamperedPayloadFails(t *testing.T) {
	secret := []byte("secret")
	token, _ := Sign(Claims{Sub: "alice", Exp: future()}, secret)

	// ペイロードだけ別人に差し替える(署名はそのまま)。
	parts := strings.Split(token, ".")
	forged := encodeSegment([]byte(`{"sub":"admin","exp":9999999999}`))
	tampered := parts[0] + "." + forged + "." + parts[2]

	if _, err := Verify(tampered, secret); err == nil {
		t.Error("改竄されたペイロードが検証に通ってしまった")
	}
}

func TestWrongSecretFails(t *testing.T) {
	token, _ := Sign(Claims{Sub: "bob", Exp: future()}, []byte("real-secret"))
	if _, err := Verify(token, []byte("guessed-secret")); err == nil {
		t.Error("違う秘密鍵で検証が通ってしまった")
	}
}

func TestExpiredTokenFails(t *testing.T) {
	secret := []byte("s")
	token, _ := Sign(Claims{Sub: "carol", Exp: time.Now().Add(-time.Hour).Unix()}, secret)
	if _, err := Verify(token, secret); err == nil {
		t.Error("期限切れトークンが通ってしまった")
	}
}

func TestMalformedToken(t *testing.T) {
	secret := []byte("s")
	for _, bad := range []string{"", "onlyonepart", "two.parts", "a.b.c.d"} {
		if _, err := Verify(bad, secret); err == nil {
			t.Errorf("不正な形式 %q が通ってしまった", bad)
		}
	}
}

// alg=none 攻撃: ヘッダのアルゴリズムを none に書き換えて署名を空にする既知の攻撃。
// 実装がヘッダの alg を鵜呑みにすると通ってしまう。ここでは HS256 固定なので防げること。
func TestAlgNoneRejected(t *testing.T) {
	secret := []byte("s")
	header := encodeSegment([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := encodeSegment([]byte(`{"sub":"attacker","exp":9999999999}`))
	forged := header + "." + payload + "."
	if _, err := Verify(forged, secret); err == nil {
		t.Error("alg=none 攻撃が通ってしまった")
	}
}

func future() int64 { return time.Now().Add(time.Hour).Unix() }
