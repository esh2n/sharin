// Package hash は暗号学的ハッシュと、その上に立つ HMAC・パスワードハッシュを
// 最小構成で実装する。
//
// ハッシュは任意長の入力を固定長の指紋に潰す一方向関数だ。同じ入力は同じ指紋、
// 1 ビット変えれば指紋は総崩れ(なだれ効果)、指紋から入力は戻せない。ここでは
// Merkle–Damgård 構成(ブロックごとに内部状態を混ぜていく)で小さなハッシュを
// 自作する。そのうえで、この素朴な構成には長さ拡張攻撃という穴があること、
// つまり H(secret‖msg) をそのまま認証符号に使うと秘密鍵を知らずに正しい指紋を
// 偽造できてしまうことを実演し、それを塞ぐ HMAC を組み立てる。最後に、パスワードを
// 保存するには「速いハッシュ」がむしろ危険で、塩と反復が要ることを見る。
package hash

import (
	"encoding/binary"
	"math/bits"
)

// #region hash

const (
	// BlockSize は圧縮関数が一度に処理するバイト数。
	BlockSize = 16
	// Size は指紋のバイト数(32bit 語 4 つ = 128bit)。
	Size = 16
)

// iv は内部状態の初期値。
var iv = [4]uint32{0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476}

// consts は各ラウンドで混ぜる定数(なだれ効果を強める)。
var consts = [8]uint32{
	0x5a827999, 0x6ed9eba1, 0x8f1bbcdc, 0xca62c1d6,
	0x452821e6, 0x38d01377, 0xbe5466cf, 0x34e90c6c,
}

// compress は 16 バイトのブロック 1 つを内部状態に混ぜ込む。
// 最後に元の状態を足し戻す(Davies–Meyer 風)ので一方向になる。
func compress(s [4]uint32, block []byte) [4]uint32 {
	var w [4]uint32
	for i := 0; i < 4; i++ {
		w[i] = binary.LittleEndian.Uint32(block[i*4:])
	}
	a, b, c, d := s[0], s[1], s[2], s[3]
	for r := 0; r < 8; r++ {
		mixed := a + (b ^ c ^ d) + w[r%4] + consts[r]
		a = bits.RotateLeft32(mixed, 7)
		a, b, c, d = d, a, b, c // レジスタを回す
	}
	return [4]uint32{s[0] + a, s[1] + b, s[2] + c, s[3] + d}
}

// Sum は data の指紋を返す。
func Sum(data []byte) [Size]byte {
	s := iv
	padded := append(append([]byte{}, data...), padding(len(data))...)
	for i := 0; i < len(padded); i += BlockSize {
		s = compress(s, padded[i:i+BlockSize])
	}
	return toBytes(s)
}

func toBytes(s [4]uint32) [Size]byte {
	var out [Size]byte
	for i, w := range s {
		binary.LittleEndian.PutUint32(out[i*4:], w)
	}
	return out
}

func fromBytes(d [Size]byte) [4]uint32 {
	var s [4]uint32
	for i := range s {
		s[i] = binary.LittleEndian.Uint32(d[i*4:])
	}
	return s
}

// #endregion hash

// #region extension

// padding は msgLen バイトのメッセージの後ろに付く詰め物を返す。
// 0x80 の 1 バイト + 0 の並び + 末尾 8 バイトにビット長。これで全体が
// BlockSize の倍数になる。末尾に長さを書くのが Merkle–Damgård 強化。
func padding(msgLen int) []byte {
	// 0x80 と 8 バイト長を除いて、ブロック境界に揃うだけの 0 を入れる。
	zeros := (BlockSize - (msgLen+1+8)%BlockSize) % BlockSize
	pad := make([]byte, 1+zeros+8)
	pad[0] = 0x80
	binary.LittleEndian.PutUint64(pad[1+zeros:], uint64(msgLen)*8)
	return pad
}

// Extend は長さ拡張攻撃を実演する。
// 攻撃者は secret を知らなくても、H(secret‖orig) の値(digest)と
// secret‖orig の長さ(origLen)さえ分かれば、末尾に ext を継ぎ足した
// H(secret‖orig‖glue‖ext) を正しく計算できる。glue は元の詰め物。
//
// 素朴な構成では digest がそのまま内部状態なので、そこから処理を続けられる。
// これが「H(secret‖msg) を認証符号にしてはいけない」理由。
func Extend(digest [Size]byte, origLen int, ext []byte) (forged [Size]byte, glue []byte) {
	glue = padding(origLen)
	s := fromBytes(digest) // digest = secret‖orig‖glue 処理後の内部状態
	totalLen := origLen + len(glue) + len(ext)
	tail := append(append([]byte{}, ext...), padding(totalLen)...)
	for i := 0; i < len(tail); i += BlockSize {
		s = compress(s, tail[i:i+BlockSize])
	}
	return toBytes(s), glue
}

// #endregion extension

// #region hmac

// HMAC は鍵付きハッシュによる認証符号。長さ拡張攻撃に強い。
//
//	HMAC(k, m) = H((k⊕opad) ‖ H((k⊕ipad) ‖ m))
//
// 内側のハッシュをさらに外側で包むので、内側を拡張しても外側の指紋は偽造できない。
func HMAC(key, msg []byte) [Size]byte {
	// 鍵がブロックより長ければ一度ハッシュして縮める。
	if len(key) > BlockSize {
		k := Sum(key)
		key = k[:]
	}
	k := make([]byte, BlockSize)
	copy(k, key) // 短い鍵は 0 詰め

	ipad := make([]byte, BlockSize)
	opad := make([]byte, BlockSize)
	for i := range k {
		ipad[i] = k[i] ^ 0x36
		opad[i] = k[i] ^ 0x5c
	}
	inner := Sum(append(ipad, msg...))
	return Sum(append(opad, inner[:]...))
}

// Equal は 2 つの指紋を比べる(定数時間。早期 return でタイミングを漏らさない)。
func Equal(a, b [Size]byte) bool {
	var diff byte
	for i := 0; i < Size; i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// HashPassword は塩と反復でパスワードをハッシュする。
// 速いハッシュ 1 回では総当たりに弱い。塩でレインボーテーブルを無効にし、
// 反復で 1 回の検証を意図的に重くして総当たりの単価を上げる。
func HashPassword(password, salt []byte, iters int) [Size]byte {
	if iters < 1 {
		iters = 1
	}
	h := Sum(append(append([]byte{}, salt...), password...))
	for i := 1; i < iters; i++ {
		// 毎回 塩を混ぜ直しながら反復する。
		h = Sum(append(append([]byte{}, salt...), h[:]...))
	}
	return h
}

// VerifyPassword は保存済みハッシュと入力パスワードを定数時間で照合する。
func VerifyPassword(stored [Size]byte, password, salt []byte, iters int) bool {
	return Equal(stored, HashPassword(password, salt, iters))
}

// #endregion hmac
