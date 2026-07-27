package dh

import (
	"math/big"
	"testing"
)

// 教科書用の小さな素数。実物は 2048bit 以上。
func testParams() Params {
	return Params{P: big.NewInt(2147483647), G: big.NewInt(5)} // P = 2^31-1(素数)
}

func TestModExpKnownValues(t *testing.T) {
	// 5^3 mod 23 = 125 mod 23 = 10。
	got := ModExp(big.NewInt(5), big.NewInt(3), big.NewInt(23))
	if got.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("5^3 mod 23: got %s want 10", got)
	}
	// 2^10 mod 1000 = 1024 mod 1000 = 24。
	got = ModExp(big.NewInt(2), big.NewInt(10), big.NewInt(1000))
	if got.Cmp(big.NewInt(24)) != 0 {
		t.Fatalf("2^10 mod 1000: got %s want 24", got)
	}
	// base^0 = 1。
	got = ModExp(big.NewInt(9999), big.NewInt(0), big.NewInt(7))
	if got.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("x^0: got %s want 1", got)
	}
}

// TestBothSidesDeriveSameSecret はこの章の主眼。Alice と Bob が公開鍵だけを
// 交換して、同じ共有秘密 g^(ab) mod p にたどり着くことを固定する。
func TestBothSidesDeriveSameSecret(t *testing.T) {
	pr := testParams()
	a, A := pr.Generate(NewRand(1)) // Alice
	b, B := pr.Generate(NewRand(2)) // Bob

	// Alice は Bob の公開鍵 B を、Bob は Alice の公開鍵 A を使う。
	aliceShared := pr.Shared(a, B)
	bobShared := pr.Shared(b, A)

	if aliceShared.Cmp(bobShared) != 0 {
		t.Fatalf("shared secrets differ: %s vs %s", aliceShared, bobShared)
	}
	// 共有秘密は公開鍵とは別物(秘密が実際に混ざっている)。
	if aliceShared.Cmp(A) == 0 || aliceShared.Cmp(B) == 0 {
		t.Fatal("shared secret must not equal a public key")
	}
}

func TestDifferentPrivatesDifferentSecret(t *testing.T) {
	pr := testParams()
	_, A := pr.Generate(NewRand(1))
	b1, _ := pr.Generate(NewRand(2))
	b2, _ := pr.Generate(NewRand(3))
	s1 := pr.Shared(b1, A)
	s2 := pr.Shared(b2, A)
	if s1.Cmp(s2) == 0 {
		t.Fatal("different private keys should yield different shared secrets")
	}
}

// TestPassiveEavesdropperCannotDerive は、公開鍵 A・B と素数・生成元を見ても、
// 秘密なしには共有秘密を作れないことを(小さな探索で)確かめる。
// 盗聴者は g^a と g^b は見えるが、そこから g^(ab) を直接は作れない
// (A*B mod P や A+B などの素朴な合成では一致しない)。
func TestPassiveEavesdropperCannotDerive(t *testing.T) {
	pr := testParams()
	a, A := pr.Generate(NewRand(7))
	_, B := pr.Generate(NewRand(8))
	real := pr.Shared(a, B)

	// 盗聴者が公開値だけで試しそうな素朴な合成は、どれも共有秘密に一致しない。
	guesses := []*big.Int{
		new(big.Int).Mod(new(big.Int).Mul(A, B), pr.P),
		new(big.Int).Mod(new(big.Int).Add(A, B), pr.P),
		ModExp(A, B, pr.P),
	}
	for i, g := range guesses {
		if g.Cmp(real) == 0 {
			t.Fatalf("naive guess %d unexpectedly matched the shared secret", i)
		}
	}
}

// TestMITMWithoutAuth はこの章のもう一つの主眼。認証がなければ、間に割り込む
// 中間者 Mallory が Alice とも Bob とも別々に鍵交換でき、両者を騙せることを示す。
// Alice が「Bob と共有した」と思う鍵は、実は Mallory と共有した鍵。
func TestMITMWithoutAuth(t *testing.T) {
	pr := testParams()
	a, A := pr.Generate(NewRand(1)) // Alice
	b, B := pr.Generate(NewRand(2)) // Bob
	m, M := pr.Generate(NewRand(9)) // Mallory(中間者)

	// Mallory は A・B を横取りし、自分の M を両者に渡す。
	// Alice は M を Bob の公開鍵だと思い込む。Bob も M を Alice のものと思う。
	aliceThinksShared := pr.Shared(a, M) // Alice ↔ Mallory
	bobThinksShared := pr.Shared(b, M)   // Bob ↔ Mallory

	// Mallory は両方の鍵を自分で計算できる(復号して読み、再暗号化して中継できる)。
	malloryWithAlice := pr.Shared(m, A)
	malloryWithBob := pr.Shared(m, B)

	if aliceThinksShared.Cmp(malloryWithAlice) != 0 {
		t.Fatal("Mallory should share a key with Alice")
	}
	if bobThinksShared.Cmp(malloryWithBob) != 0 {
		t.Fatal("Mallory should share a key with Bob")
	}
	// 肝心なのは、Alice と Bob は実は「同じ鍵」を共有していない。
	// 認証なしでは、相手が本当に Bob かを確かめる術がないので気づけない。
	if aliceThinksShared.Cmp(bobThinksShared) == 0 {
		t.Fatal("without MITM Alice and Bob would match; here they must not")
	}
}
