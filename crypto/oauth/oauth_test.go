package oauth

import "testing"

var secret = []byte("shared-signing-secret")

// newFlow は認可サーバと資源サーバ(同じ秘密鍵)を用意する。
func newFlow() (*AuthServer, *ResourceServer) {
	as := NewAuthServer(secret, 10, 100, 42) // codeTTL=10, tokenTTL=100
	rs := NewResourceServer(secret)
	return as, rs
}

// TestHappyPath はこの章の主眼。パスワードを渡さず、コード→トークンの
// 交換を経て、scope に応じたアクセスができることを固定する。
func TestHappyPath(t *testing.T) {
	as, rs := newFlow()

	// クライアントは検証子を作り、そのハッシュ(challenge)だけを預ける。
	verifier := "random-verifier-xyz"
	challenge := Challenge(verifier)

	// フロントチャネル: ユーザ認証後、短命のコードが返る。
	code := as.Authorize("app-1", "alice", "photo:read", challenge)

	// バックチャネル: コードと検証子の原本を出してトークンに交換。
	token, err := as.Exchange("app-1", code, verifier)
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}

	// 資源サーバは photo:read を許可、photo:write は拒否(最小権限)。
	if !rs.Validate(token, "photo:read") {
		t.Fatal("token should grant photo:read")
	}
	if rs.Validate(token, "photo:write") {
		t.Fatal("token must NOT grant photo:write")
	}
}

// TestCodeSingleUse は認可コードが一度きりで、再利用が弾かれることを示す。
func TestCodeSingleUse(t *testing.T) {
	as, _ := newFlow()
	v := "verifier-1"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(v))

	if _, err := as.Exchange("app-1", code, v); err != nil {
		t.Fatalf("first exchange should succeed: %v", err)
	}
	// 二度目は使用済みで拒否。
	if _, err := as.Exchange("app-1", code, v); err != ErrBadCode {
		t.Fatalf("replayed code should be ErrBadCode, got %v", err)
	}
}

// TestPKCEStolenCodeUseless はこの章のもう一つの主眼。攻撃者がコードを
// 横取りしても、検証子の原本を持たないのでトークンに交換できない。
func TestPKCEStolenCodeUseless(t *testing.T) {
	as, _ := newFlow()
	verifier := "the-real-verifier"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(verifier))

	// 攻撃者はコードは奪えたが、検証子を知らない(推測で攻める)。
	if _, err := as.Exchange("app-1", code, "guessed-verifier"); err != ErrBadVerifier {
		t.Fatalf("stolen code without verifier should be ErrBadVerifier, got %v", err)
	}
	// 空の検証子でも当然だめ。
	if _, err := as.Exchange("app-1", code, ""); err != ErrBadVerifier {
		t.Fatalf("empty verifier should be ErrBadVerifier, got %v", err)
	}
}

func TestCodeExpiry(t *testing.T) {
	as, _ := newFlow()
	v := "v"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(v))
	as.Advance(11) // codeTTL=10 を超える
	if _, err := as.Exchange("app-1", code, v); err != ErrBadCode {
		t.Fatalf("expired code should be ErrBadCode, got %v", err)
	}
}

func TestClientMismatch(t *testing.T) {
	as, _ := newFlow()
	v := "v"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(v))
	// 別のクライアントがコードを使おうとする。
	if _, err := as.Exchange("app-evil", code, v); err != ErrClientMismatch {
		t.Fatalf("wrong client should be ErrClientMismatch, got %v", err)
	}
}

func TestTokenExpiry(t *testing.T) {
	as, rs := newFlow()
	v := "v"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(v))
	token, _ := as.Exchange("app-1", code, v)
	if !rs.Validate(token, "photo:read") {
		t.Fatal("fresh token should validate")
	}
	rs.Advance(101) // tokenTTL=100 を超える
	if rs.Validate(token, "photo:read") {
		t.Fatal("expired token must not validate")
	}
}

// TestTokenTamperDetected は署名されたトークンの改ざんが検出されることを示す。
func TestTokenTamperDetected(t *testing.T) {
	as, rs := newFlow()
	v := "v"
	code := as.Authorize("app-1", "alice", "photo:read", Challenge(v))
	token, _ := as.Exchange("app-1", code, v)

	// scope を write に書き換えても、署名が合わないので弾かれる。
	tampered := ""
	for _, c := range token {
		if c == 'r' {
			tampered += "w"
		} else {
			tampered += string(c)
		}
	}
	if tampered != token && rs.Validate(tampered, "photo:read") {
		t.Fatal("tampered token must be rejected")
	}
}

func TestMalformedTokenRejected(t *testing.T) {
	_, rs := newFlow()
	for _, bad := range []string{"", "no-dot", "a|b|c", "a|b|c.deadbeef", "sub|scope|notanumber.00"} {
		if rs.Validate(bad, "photo:read") {
			t.Fatalf("malformed token %q must be rejected", bad)
		}
	}
}

func TestChallengeDeterministic(t *testing.T) {
	if Challenge("abc") != Challenge("abc") {
		t.Fatal("challenge must be deterministic")
	}
	if Challenge("abc") == Challenge("abd") {
		t.Fatal("different verifiers must give different challenges")
	}
}
