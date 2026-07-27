package cipher

import (
	"bytes"
	"testing"
)

var key = []byte("my-secret-key-123")

func TestBlockRoundTrip(t *testing.T) {
	c := NewCipher(key)
	src := []byte("8bytes!!")
	enc := make([]byte, BlockSize)
	dec := make([]byte, BlockSize)
	c.encryptBlock(enc, src)
	c.decryptBlock(dec, enc)
	if !bytes.Equal(dec, src) {
		t.Fatalf("roundtrip failed: %q -> %q -> %q", src, enc, dec)
	}
	if bytes.Equal(enc, src) {
		t.Fatal("ciphertext equals plaintext (not encrypting)")
	}
}

func TestECBRoundTrip(t *testing.T) {
	c := NewCipher(key)
	msg := []byte("hello world, this is a longer message")
	ct := c.EncryptECB(msg)
	got, ok := c.DecryptECB(ct)
	if !ok || !bytes.Equal(got, msg) {
		t.Fatalf("ECB roundtrip failed: ok=%v got=%q", ok, got)
	}
}

// TestECBLeaksPatterns はこの章の主眼その 1。ECB は同じ平文ブロックが
// 同じ暗号文ブロックになるので、繰り返し模様が暗号文に透ける。
func TestECBLeaksPatterns(t *testing.T) {
	c := NewCipher(key)
	// 同じ 8 バイトブロックを 3 回繰り返した平文。
	block := []byte("AAAAAAAA")
	plain := append(append(append([]byte{}, block...), block...), block...)

	ecb := c.EncryptECB(plain)
	// 先頭 3 ブロックの暗号文が全部同じ = 模様が漏れている。
	b0 := ecb[0:BlockSize]
	b1 := ecb[BlockSize : 2*BlockSize]
	b2 := ecb[2*BlockSize : 3*BlockSize]
	if !bytes.Equal(b0, b1) || !bytes.Equal(b1, b2) {
		t.Fatal("ECB should produce identical ciphertext for identical plaintext blocks")
	}

	// CBC なら同じ平文ブロックでも暗号文がばらける。
	iv := []byte("iv-8byte")
	cbc := c.EncryptCBC(plain, iv)
	c0 := cbc[0:BlockSize]
	c1 := cbc[BlockSize : 2*BlockSize]
	if bytes.Equal(c0, c1) {
		t.Fatal("CBC should NOT leak repeated blocks")
	}
}

func TestCBCRoundTrip(t *testing.T) {
	c := NewCipher(key)
	iv := []byte("iv-8byte")
	msg := []byte("cbc chains each block to the previous one")
	ct := c.EncryptCBC(msg, iv)
	got, ok := c.DecryptCBC(ct, iv)
	if !ok || !bytes.Equal(got, msg) {
		t.Fatalf("CBC roundtrip failed: ok=%v got=%q", ok, got)
	}
}

func TestCBCDifferentIVDiffersCiphertext(t *testing.T) {
	c := NewCipher(key)
	msg := []byte("same plaintext")
	a := c.EncryptCBC(msg, []byte("iv-aaaaa"))
	b := c.EncryptCBC(msg, []byte("iv-bbbbb"))
	if bytes.Equal(a, b) {
		t.Fatal("different IVs must give different ciphertext")
	}
}

func TestCTRRoundTripAndArbitraryLength(t *testing.T) {
	c := NewCipher(key)
	nonce := []byte("nonce123")
	for _, msg := range [][]byte{[]byte("x"), []byte("not a block multiple!!"), []byte("")} {
		ct := c.CTR(msg, nonce)
		got := c.CTR(ct, nonce) // CTR は暗号化=復号
		if !bytes.Equal(got, msg) {
			t.Fatalf("CTR roundtrip failed for %q", msg)
		}
		if len(ct) != len(msg) {
			t.Fatalf("CTR must preserve length: %d != %d", len(ct), len(msg))
		}
	}
}

// TestCTRMalleable はこの章の主眼その 2。暗号化しただけでは改ざんを防げない。
// CTR は平文とキーストリームの XOR なので、暗号文のビットを反転すると
// 復号後の平文の同じ位置がそのまま反転する。攻撃者は中身を狙って書き換えられる。
func TestCTRMalleable(t *testing.T) {
	c := NewCipher(key)
	nonce := []byte("nonce123")
	plain := []byte("balance=0000100") // 送金額 100
	ct := c.CTR(plain, nonce)

	// 攻撃者は鍵を知らないが、暗号文の該当バイトを XOR で書き換える。
	// '1' (0x31) を '9' (0x39) にしたい → 0x31^0x39 = 0x08 を XOR。
	pos := bytes.IndexByte(plain, '1')
	tampered := append([]byte{}, ct...)
	tampered[pos] ^= '1' ^ '9'

	got := c.CTR(tampered, nonce)
	if got[pos] != '9' {
		t.Fatalf("expected tampered byte to become '9', got %q", got[pos])
	}
	// 復号は成功してしまう(検知できない)。これが暗号化だけの危うさ。
	if !bytes.Contains(got, []byte("balance=0000900")) {
		t.Fatalf("attacker rewrote plaintext undetected: %q", got)
	}
}

// TestSealOpenDetectsTampering は、encrypt-then-MAC(AEAD 相当)なら
// 同じ改ざんが復号前に弾かれることを示す。
func TestSealOpenDetectsTampering(t *testing.T) {
	c := NewCipher(key)
	nonce := []byte("nonce123")
	plain := []byte("balance=0000100")

	sealed := c.Seal(key, plain, nonce)

	// まず正規のものは開ける。
	got, ok := c.Open(key, sealed, nonce)
	if !ok || !bytes.Equal(got, plain) {
		t.Fatalf("Open of untampered failed: ok=%v got=%q", ok, got)
	}

	// 暗号文部分を 1 ビット改ざんすると、tag 検証で弾かれる。
	tampered := append([]byte{}, sealed...)
	tampered[len(nonce)+3] ^= 0x08
	if _, ok := c.Open(key, tampered, nonce); ok {
		t.Fatal("Open must reject tampered ciphertext")
	}
	// tag 自体を弄っても弾かれる。
	tampered2 := append([]byte{}, sealed...)
	tampered2[len(tampered2)-1] ^= 0x01
	if _, ok := c.Open(key, tampered2, nonce); ok {
		t.Fatal("Open must reject tampered tag")
	}
}

func TestUnpadRejectsGarbage(t *testing.T) {
	c := NewCipher(key)
	// 長さがブロック非整数倍の暗号文。
	if _, ok := c.DecryptECB([]byte("123")); ok {
		t.Fatal("non-block-multiple ciphertext must be rejected")
	}
	if _, ok := c.DecryptCBC([]byte("123"), []byte("iv-8byte")); ok {
		t.Fatal("non-block-multiple ciphertext must be rejected")
	}
}
