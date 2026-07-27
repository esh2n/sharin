package hash

import (
	"bytes"
	"testing"
)

func TestDeterministic(t *testing.T) {
	a := Sum([]byte("hello"))
	b := Sum([]byte("hello"))
	if !Equal(a, b) {
		t.Fatal("same input must give same digest")
	}
}

// TestAvalanche は 1 ビットの差で指紋が総崩れになる(なだれ効果)ことを確かめる。
func TestAvalanche(t *testing.T) {
	a := Sum([]byte("hello"))
	b := Sum([]byte("hellp")) // 1 文字違い
	if Equal(a, b) {
		t.Fatal("1-bit change must change digest")
	}
	// 半分近くのビットが変わるのが理想。ここでは最低 1/4 は変わることを要求。
	diff := 0
	for i := 0; i < Size; i++ {
		x := a[i] ^ b[i]
		for x != 0 {
			diff += int(x & 1)
			x >>= 1
		}
	}
	if diff < Size*8/4 {
		t.Fatalf("weak avalanche: only %d bits changed", diff)
	}
}

func TestFixedSizeAcrossLengths(t *testing.T) {
	for _, in := range []string{"", "a", "block-exactly-16", "a much longer message than one block"} {
		d := Sum([]byte(in))
		if len(d) != Size {
			t.Fatalf("digest of %q has size %d want %d", in, len(d), Size)
		}
	}
}

// TestLengthExtension はこの章の主眼。素朴な H(secret‖msg) を認証符号にすると、
// 秘密を知らない攻撃者が末尾を継ぎ足した正しい指紋を偽造できることを示す。
func TestLengthExtension(t *testing.T) {
	secret := []byte("supersecretkey")
	msg := []byte("amount=100&to=alice")

	// サーバは tag = H(secret‖msg) を「正しさの証」として付ける。
	tag := Sum(append(append([]byte{}, secret...), msg...))

	// 攻撃者は secret を知らない。だが tag と secret‖msg の長さは推測できる。
	ext := []byte("&to=attacker")
	forged, glue := Extend(tag, len(secret)+len(msg), ext)

	// 攻撃者が作れる「見かけ上のメッセージ」は msg‖glue‖ext。
	// サーバが H(secret‖その) を計算すると、偽造した forged と一致してしまう。
	forgedMsg := append(append(append([]byte{}, msg...), glue...), ext...)
	actual := Sum(append(append([]byte{}, secret...), forgedMsg...))

	if !Equal(forged, actual) {
		t.Fatal("length extension should reproduce a valid tag without the secret")
	}
	// 継ぎ足した内容が確かに含まれる(攻撃が意味を持つ)。
	if !bytes.Contains(forgedMsg, ext) {
		t.Fatal("forged message must contain the injected extension")
	}
}

// TestHMACResistsExtension は、HMAC なら同じ拡張攻撃が効かないことを示す。
func TestHMACResistsExtension(t *testing.T) {
	secret := []byte("supersecretkey")
	msg := []byte("amount=100&to=alice")
	tag := HMAC(secret, msg)

	// 攻撃者が HMAC の tag を内部状態とみなして拡張しても、
	// 外側のハッシュが包んでいるので、末尾を継ぎ足した正しい tag は作れない。
	ext := []byte("&to=attacker")
	forged, glue := Extend(tag, len(secret)+len(msg), ext)
	forgedMsg := append(append(append([]byte{}, msg...), glue...), ext...)
	actual := HMAC(secret, forgedMsg)

	if Equal(forged, actual) {
		t.Fatal("HMAC must NOT be forgeable by length extension")
	}
}

func TestHMACDeterministicAndKeyed(t *testing.T) {
	m := []byte("message")
	if !Equal(HMAC([]byte("k1"), m), HMAC([]byte("k1"), m)) {
		t.Fatal("same key+msg must match")
	}
	if Equal(HMAC([]byte("k1"), m), HMAC([]byte("k2"), m)) {
		t.Fatal("different keys must differ")
	}
}

func TestHMACLongKey(t *testing.T) {
	// ブロックより長い鍵は縮められる。落ちずに決定的であればよい。
	longKey := bytes.Repeat([]byte("x"), 100)
	a := HMAC(longKey, []byte("m"))
	b := HMAC(longKey, []byte("m"))
	if !Equal(a, b) {
		t.Fatal("long-key HMAC must be deterministic")
	}
}

// TestPasswordSaltMatters は、塩が違えば同じパスワードでも別の指紋になることを示す
// (レインボーテーブルが効かない)。
func TestPasswordSaltMatters(t *testing.T) {
	pw := []byte("hunter2")
	h1 := HashPassword(pw, []byte("saltA"), 100)
	h2 := HashPassword(pw, []byte("saltB"), 100)
	if Equal(h1, h2) {
		t.Fatal("same password with different salt must differ")
	}
}

func TestPasswordVerify(t *testing.T) {
	pw := []byte("hunter2")
	salt := []byte("salt")
	stored := HashPassword(pw, salt, 500)
	if !VerifyPassword(stored, pw, salt, 500) {
		t.Fatal("correct password must verify")
	}
	if VerifyPassword(stored, []byte("wrong"), salt, 500) {
		t.Fatal("wrong password must fail")
	}
	// 反復回数が違えば別物(検証条件の一部)。
	if VerifyPassword(stored, pw, salt, 499) {
		t.Fatal("different iteration count must fail")
	}
}

func TestEqualConstantTimeShape(t *testing.T) {
	a := Sum([]byte("x"))
	b := a
	if !Equal(a, b) {
		t.Fatal("identical digests must be equal")
	}
	b[0] ^= 1
	if Equal(a, b) {
		t.Fatal("one-byte difference must be unequal")
	}
}
