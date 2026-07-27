package websocket

import "math/bits"

// sha1Sum は SHA-1(160bit)を素朴に実装する。WebSocket の Accept 計算で
// 使われる実際のハッシュ。Merkle–Damgård 構成で、80 ラウンドの圧縮を回す。
func sha1Sum(data []byte) [20]byte {
	h0, h1, h2, h3, h4 := uint32(0x67452301), uint32(0xEFCDAB89), uint32(0x98BADCFE), uint32(0x10325476), uint32(0xC3D2E1F0)

	// 前処理: 0x80 を足し、長さ(ビット)を末尾 8 バイトに置いて 64 の倍数にする。
	ml := uint64(len(data)) * 8
	msg := append([]byte{}, data...)
	msg = append(msg, 0x80)
	for len(msg)%64 != 56 {
		msg = append(msg, 0)
	}
	for s := 56; s >= 0; s -= 8 {
		msg = append(msg, byte(ml>>uint(s)))
	}

	var w [80]uint32
	for c := 0; c < len(msg); c += 64 {
		chunk := msg[c : c+64]
		for i := 0; i < 16; i++ {
			w[i] = uint32(chunk[i*4])<<24 | uint32(chunk[i*4+1])<<16 | uint32(chunk[i*4+2])<<8 | uint32(chunk[i*4+3])
		}
		for i := 16; i < 80; i++ {
			w[i] = bits.RotateLeft32(w[i-3]^w[i-8]^w[i-14]^w[i-16], 1)
		}

		a, b, cc, d, e := h0, h1, h2, h3, h4
		for i := 0; i < 80; i++ {
			var f, k uint32
			switch {
			case i < 20:
				f = (b & cc) | (^b & d)
				k = 0x5A827999
			case i < 40:
				f = b ^ cc ^ d
				k = 0x6ED9EBA1
			case i < 60:
				f = (b & cc) | (b & d) | (cc & d)
				k = 0x8F1BBCDC
			default:
				f = b ^ cc ^ d
				k = 0xCA62C1D6
			}
			tmp := bits.RotateLeft32(a, 5) + f + e + k + w[i]
			e, d, cc, b, a = d, cc, bits.RotateLeft32(b, 30), a, tmp
		}
		h0, h1, h2, h3, h4 = h0+a, h1+b, h2+cc, h3+d, h4+e
	}

	var out [20]byte
	for i, h := range []uint32{h0, h1, h2, h3, h4} {
		out[i*4] = byte(h >> 24)
		out[i*4+1] = byte(h >> 16)
		out[i*4+2] = byte(h >> 8)
		out[i*4+3] = byte(h)
	}
	return out
}

// base64Encode は標準の base64(パディング付き)でバイト列を符号化する。
func base64Encode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	for i := 0; i < len(data); i += 3 {
		var n uint32
		rem := len(data) - i
		n = uint32(data[i]) << 16
		if rem > 1 {
			n |= uint32(data[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(data[i+2])
		}
		out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f])
		if rem > 1 {
			out = append(out, alphabet[(n>>6)&0x3f])
		} else {
			out = append(out, '=')
		}
		if rem > 2 {
			out = append(out, alphabet[n&0x3f])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}
