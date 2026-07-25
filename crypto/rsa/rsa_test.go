package rsa

import "testing"

func TestGenKeyPair(t *testing.T) {
	// 小さな素数でも鍵ペアの関係(n=p*q, e*d ≡ 1 mod φ)が成り立つこと。
	pub, priv := GenKeyPair(61, 53) // 教科書によく出る例。n=3233
	if pub.N != 3233 {
		t.Errorf("N = %d, want 3233", pub.N)
	}
	// e*d mod φ(n) == 1 を確認。φ = (61-1)*(53-1) = 3120。
	phi := int64(60 * 52)
	if (pub.E*priv.D)%phi != 1 {
		t.Errorf("e*d mod φ = %d, want 1", (pub.E*priv.D)%phi)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	pub, priv := GenKeyPair(61, 53)
	for _, m := range []int64{0, 1, 42, 123, 3232} {
		c := Encrypt(pub, m)
		got := Decrypt(priv, c)
		if got != m {
			t.Errorf("Decrypt(Encrypt(%d)) = %d, want %d", m, got, m)
		}
	}
}

// 公開鍵で暗号化 → 秘密鍵でしか戻せない、が公開鍵暗号の肝。
func TestOnlyPrivateKeyDecrypts(t *testing.T) {
	pub, priv := GenKeyPair(61, 53)
	wrong := PrivateKey{D: priv.D + 1, N: priv.N} // 間違った秘密鍵
	m := int64(42)
	c := Encrypt(pub, m)
	if Decrypt(wrong, c) == m {
		t.Error("間違った秘密鍵で復号できてしまった")
	}
	if Decrypt(priv, c) != m {
		t.Error("正しい秘密鍵で復号できない")
	}
}

// 署名: 秘密鍵で署名 → 公開鍵で検証。暗号化とは鍵の使う向きが逆。
func TestSignVerify(t *testing.T) {
	pub, priv := GenKeyPair(61, 53)
	m := int64(100)
	sig := Sign(priv, m)
	if !Verify(pub, m, sig) {
		t.Error("正しい署名が検証に通らない")
	}
	if Verify(pub, m, sig+1) {
		t.Error("改竄された署名が検証に通ってしまった")
	}
	if Verify(pub, m+1, sig) {
		t.Error("別のメッセージの署名が検証に通ってしまった")
	}
}

func TestModExp(t *testing.T) {
	// 高速べき乗剰余: 7^13 mod 100 = 7 の手計算と一致すること。
	if got := modExp(7, 13, 100); got != 7 {
		t.Errorf("modExp(7,13,100) = %d, want 7", got)
	}
	if got := modExp(2, 10, 1000); got != 24 {
		t.Errorf("modExp(2,10,1000) = %d, want 24 (1024 mod 1000)", got)
	}
}

func TestGenKeyValidation(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("非素数や小さすぎる入力は panic すべき")
		}
	}()
	GenKeyPair(4, 6) // 素数でない
}
