// Package rsa は RSA 公開鍵暗号を「小さな素数」で自作したもの。
//
// 公開鍵暗号の魔法は「暗号化に使う鍵(公開鍵)と、復号に使う鍵(秘密鍵)が別」なこと。
// 誰でも暗号化できるが、復号できるのは秘密鍵を持つ人だけ。この非対称性が、
// 「鍵を安全に配る」問題(共通鍵暗号の弱点)を解く。
//
// RSA の安全性は「大きな数の素因数分解は難しい」に依っている。
// ここでは仕組みを見るために極小の素数を使う(実物は数百桁)。だから安全ではない、教材専用。
package rsa

// #region keys
// PublicKey は公開鍵 (e, n)。暗号化と署名検証に使う。誰に配ってもよい。
type PublicKey struct {
	E, N int64
}

// PrivateKey は秘密鍵 (d, n)。復号と署名に使う。絶対に他人に渡さない。
type PrivateKey struct {
	D, N int64
}

// GenKeyPair は2つの素数 p, q から鍵ペアを作る。
//
//	n   = p·q                     ← 公開する法(modulus)
//	φ   = (p-1)(q-1)              ← n のオイラー関数(秘密)
//	e   = φ と互いに素な公開指数   ← 慣例の 65537。φ が小さければ互いに素な小さい奇数
//	d   = e の逆元 (e·d ≡ 1 mod φ) ← 秘密指数。φ を知らないと求まらない
//
// 「n だけ知っていても d は求まらない(φ を知るには n の素因数分解が要る)」が安全性の核。
func GenKeyPair(p, q int64) (PublicKey, PrivateKey) {
	if !isPrime(p) || !isPrime(q) || p == q || p < 5 || q < 5 {
		panic("rsa: p, q must be distinct primes >= 5")
	}
	n := p * q
	phi := (p - 1) * (q - 1)

	// e は φ と互いに素な公開指数。慣例の 65537 が使えなければ小さい候補を探す。
	e := int64(65537)
	if e >= phi || gcd(e, phi) != 1 {
		e = 3
		for gcd(e, phi) != 1 {
			e += 2
		}
	}
	d := modInverse(e, phi) // e の逆元
	return PublicKey{E: e, N: n}, PrivateKey{D: d, N: n}
}

// #endregion keys

// #region cipher
// Encrypt は公開鍵で暗号化する: c = m^e mod n。
// m は 0 <= m < n の整数(実物はメッセージをこの範囲のブロックに切る)。
func Encrypt(pub PublicKey, m int64) int64 {
	return modExp(m, pub.E, pub.N)
}

// Decrypt は秘密鍵で復号する: m = c^d mod n。
// e で上げたものを d で戻せるのは、e·d ≡ 1 mod φ という関係(オイラーの定理)による。
func Decrypt(priv PrivateKey, c int64) int64 {
	return modExp(c, priv.D, priv.N)
}

// #endregion cipher

// #region sign
// Sign は秘密鍵で署名する: s = m^d mod n。
// 暗号化(公開鍵で上げる)と鍵の向きが逆。「秘密鍵を持つ本人しか作れない値」を作る。
// 実物はメッセージそのものでなくハッシュに署名する(長さ固定 + 改竄検出のため)。
func Sign(priv PrivateKey, m int64) int64 {
	return modExp(m, priv.D, priv.N)
}

// Verify は公開鍵で署名を検証する: s^e mod n == m なら本物。
// 誰でも(公開鍵だけで)検証できるが、作れるのは秘密鍵の持ち主だけ。これが「署名」。
func Verify(pub PublicKey, m, sig int64) bool {
	return modExp(sig, pub.E, pub.N) == m
}

// #endregion sign

// #region math
// modExp は (base^exp) mod m を高速に計算する(二乗法)。
// 素朴に base^exp を計算すると桁が爆発するので、掛けるたびに mod を取る。
func modExp(base, exp, m int64) int64 {
	if m == 1 {
		return 0
	}
	result := int64(1)
	base %= m
	for exp > 0 {
		if exp&1 == 1 { // exp の最下位ビットが立っていれば result に掛ける
			result = result * base % m
		}
		exp >>= 1
		base = base * base % m
	}
	return result
}

// gcd はユークリッドの互除法で最大公約数を返す。
func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// modInverse は a の mod m における逆元(a·x ≡ 1 mod m)を拡張ユークリッド法で求める。
func modInverse(a, m int64) int64 {
	g, x, _ := extGCD(a, m)
	if g != 1 {
		panic("rsa: modular inverse does not exist")
	}
	return (x%m + m) % m
}

// extGCD は ax + by = gcd(a,b) の (gcd, x, y) を返す。
func extGCD(a, b int64) (int64, int64, int64) {
	if b == 0 {
		return a, 1, 0
	}
	g, x1, y1 := extGCD(b, a%b)
	return g, y1, x1 - (a/b)*y1
}

// isPrime は素朴な素数判定(教材用の小さな数向け)。
func isPrime(n int64) bool {
	if n < 2 {
		return false
	}
	for i := int64(2); i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// #endregion math
