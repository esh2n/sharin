// Package cipher は対称鍵のブロック暗号と、その使い方(暗号利用モード)を
// 最小構成で実装する。
//
// ブロック暗号は、固定長のブロック 1 つを鍵で可逆に混ぜる関数だ。ここでは
// Feistel 構造(左右に分けて片側だけ混ぜるのを繰り返す)で 8 バイトの
// ブロック暗号を自作する。だが本題はその先にある。長いデータをブロックに
// 切って 1 つずつ暗号化するとき、どう繋ぐか。素朴に独立して暗号化する ECB は、
// 同じ平文ブロックが同じ暗号文ブロックになるので模様が透けて漏れる。CBC は
// 前のブロックを混ぜ、CTR は鍵で作った擬似乱数列と平文を XOR する。さらに、
// 暗号化しただけでは改ざんを防げない(CTR はビットを反転すると平文も反転する)。
// だから認証を足した AEAD(ここでは暗号化してから HMAC)が要る。
package cipher

import (
	"encoding/binary"
	"math/bits"

	"github.com/esh2n/sharin/crypto/hash"
)

// #region block

const (
	// BlockSize はブロック暗号が一度に処理するバイト数。
	BlockSize = 8
	// rounds は Feistel の段数。多いほど混ざる。
	rounds = 16
)

// Cipher は 8 バイトブロックの Feistel 暗号。
type Cipher struct{ rk []uint32 }

// NewCipher は鍵から段ごとの鍵(ラウンド鍵)を導いて暗号を作る。
func NewCipher(key []byte) *Cipher { return &Cipher{rk: expandKey(key)} }

// expandKey は鍵を畳んで種を作り、そこから rounds 個のラウンド鍵を生成する。
func expandKey(key []byte) []uint32 {
	var seed uint64 = 1469598103934665603 // FNV offset
	for _, b := range key {
		seed = (seed ^ uint64(b)) * 1099511628211
	}
	rk := make([]uint32, rounds)
	for i := range rk {
		seed = seed*6364136223846793005 + 1442695040888963407
		rk[i] = uint32(seed >> 32)
	}
	return rk
}

// feistelF は片側 32bit を鍵で混ぜる関数(可逆でなくてよいのが Feistel の妙)。
func feistelF(r, k uint32) uint32 {
	x := (r ^ k) * 2654435761
	x = bits.RotateLeft32(x+0x9e3779b9, 7)
	x ^= bits.RotateLeft32(x, 11)
	return x
}

// encryptBlock は 1 ブロックを暗号化する。
// 各段で (L,R) → (R, L ⊕ F(R,k))。F が可逆でなくても全体は必ず戻せる。
func (c *Cipher) encryptBlock(dst, src []byte) {
	l := binary.LittleEndian.Uint32(src[0:])
	r := binary.LittleEndian.Uint32(src[4:])
	for i := 0; i < rounds; i++ {
		l, r = r, l^feistelF(r, c.rk[i])
	}
	binary.LittleEndian.PutUint32(dst[0:], l)
	binary.LittleEndian.PutUint32(dst[4:], r)
}

// decryptBlock は暗号化の逆。ラウンド鍵を逆順に辿る。
func (c *Cipher) decryptBlock(dst, src []byte) {
	l := binary.LittleEndian.Uint32(src[0:])
	r := binary.LittleEndian.Uint32(src[4:])
	for i := rounds - 1; i >= 0; i-- {
		l, r = r^feistelF(l, c.rk[i]), l
	}
	binary.LittleEndian.PutUint32(dst[0:], l)
	binary.LittleEndian.PutUint32(dst[4:], r)
}

// #endregion block

// #region modes

// pkcs7Pad は末尾を BlockSize の倍数に詰める。詰めた分の値をそのバイトに書く
// (例: 3 バイト詰めるなら 03 03 03)。ちょうど倍数でも 1 ブロック足す。
func pkcs7Pad(data []byte) []byte {
	n := BlockSize - len(data)%BlockSize
	out := make([]byte, len(data)+n)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(n)
	}
	return out
}

// pkcs7Unpad は詰め物を外す。壊れていれば false。
func pkcs7Unpad(data []byte) ([]byte, bool) {
	if len(data) == 0 || len(data)%BlockSize != 0 {
		return nil, false
	}
	n := int(data[len(data)-1])
	if n < 1 || n > BlockSize {
		return nil, false
	}
	for i := len(data) - n; i < len(data); i++ {
		if data[i] != byte(n) {
			return nil, false
		}
	}
	return data[:len(data)-n], true
}

// EncryptECB は各ブロックを独立に暗号化する(最も素朴。模様が漏れる)。
func (c *Cipher) EncryptECB(plain []byte) []byte {
	p := pkcs7Pad(plain)
	out := make([]byte, len(p))
	for i := 0; i < len(p); i += BlockSize {
		c.encryptBlock(out[i:i+BlockSize], p[i:i+BlockSize])
	}
	return out
}

// DecryptECB は EncryptECB の逆。
func (c *Cipher) DecryptECB(ct []byte) ([]byte, bool) {
	if len(ct)%BlockSize != 0 {
		return nil, false
	}
	out := make([]byte, len(ct))
	for i := 0; i < len(ct); i += BlockSize {
		c.decryptBlock(out[i:i+BlockSize], ct[i:i+BlockSize])
	}
	return pkcs7Unpad(out)
}

// EncryptCBC は各ブロックを暗号化する前に、直前の暗号文ブロックと XOR する。
// 先頭は初期化ベクトル(iv)と XOR。同じ平文でも iv が違えば別の暗号文になる。
func (c *Cipher) EncryptCBC(plain, iv []byte) []byte {
	p := pkcs7Pad(plain)
	out := make([]byte, len(p))
	prev := make([]byte, BlockSize)
	copy(prev, iv)
	for i := 0; i < len(p); i += BlockSize {
		block := make([]byte, BlockSize)
		for j := 0; j < BlockSize; j++ {
			block[j] = p[i+j] ^ prev[j] // 直前の暗号文を混ぜる
		}
		c.encryptBlock(out[i:i+BlockSize], block)
		prev = out[i : i+BlockSize]
	}
	return out
}

// DecryptCBC は EncryptCBC の逆。
func (c *Cipher) DecryptCBC(ct, iv []byte) ([]byte, bool) {
	if len(ct)%BlockSize != 0 || len(ct) == 0 {
		return nil, false
	}
	out := make([]byte, len(ct))
	prev := make([]byte, BlockSize)
	copy(prev, iv)
	for i := 0; i < len(ct); i += BlockSize {
		dec := make([]byte, BlockSize)
		c.decryptBlock(dec, ct[i:i+BlockSize])
		for j := 0; j < BlockSize; j++ {
			out[i+j] = dec[j] ^ prev[j]
		}
		prev = ct[i : i+BlockSize]
	}
	return pkcs7Unpad(out)
}

// keystreamBlock は nonce と counter を暗号化して擬似乱数ブロックを作る。
func (c *Cipher) keystreamBlock(nonce []byte, counter uint32) []byte {
	in := make([]byte, BlockSize)
	copy(in, nonce) // 前半は nonce
	binary.LittleEndian.PutUint32(in[4:], counter)
	out := make([]byte, BlockSize)
	c.encryptBlock(out, in)
	return out
}

// CTR は平文を「鍵で作った擬似乱数列」と XOR するだけ(ストリーム暗号化)。
// 暗号化と復号が同じ操作になる。詰め物が要らず、長さもそのまま。
func (c *Cipher) CTR(data, nonce []byte) []byte {
	out := make([]byte, len(data))
	var counter uint32
	for i := 0; i < len(data); i += BlockSize {
		ks := c.keystreamBlock(nonce, counter)
		for j := 0; j < BlockSize && i+j < len(data); j++ {
			out[i+j] = data[i+j] ^ ks[j]
		}
		counter++
	}
	return out
}

// #endregion modes

// #region auth

// Seal は CTR で暗号化してから、nonce と暗号文に HMAC を付ける(encrypt-then-MAC)。
// 返すのは nonce ‖ 暗号文 ‖ tag。これで秘匿だけでなく改ざん検知もできる(AEAD 相当)。
func (c *Cipher) Seal(key, plain, nonce []byte) []byte {
	ct := c.CTR(plain, nonce)
	out := append(append([]byte{}, nonce...), ct...)
	tag := hash.HMAC(key, out)
	return append(out, tag[:]...)
}

// Open は tag を先に検証し、正しいときだけ復号する。改ざんは復号前に弾く。
func (c *Cipher) Open(key, sealed, nonce []byte) ([]byte, bool) {
	if len(sealed) < len(nonce)+hash.Size {
		return nil, false
	}
	body := sealed[:len(sealed)-hash.Size]
	var got [hash.Size]byte
	copy(got[:], sealed[len(sealed)-hash.Size:])
	want := hash.HMAC(key, body)
	if !hash.Equal(got, want) {
		return nil, false // 改ざんされている。復号すらしない
	}
	ct := body[len(nonce):]
	return c.CTR(ct, nonce), true
}

// #endregion auth
