package idgen

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// #region ulid
// crockford は ULID が使う base32 アルファベット。
// 紛らわしい I, L, O, U を除いてあり、大文字小文字の区別もない。
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID は「48bit ミリ秒 + 80bit 乱数」を Crockford base32 で26文字にしたIDを返す。
// 中身は UUIDv7 とほぼ同じ配合で、表現(短くて人間が扱いやすい文字列)が違う。
func NewULID(now time.Time, rand io.Reader) (string, error) {
	ms := uint64(now.UnixMilli())
	if ms >= 1<<48 {
		return "", errors.New("idgen: timestamp exceeds 48 bits")
	}
	var random [10]byte
	if _, err := io.ReadFull(rand, random[:]); err != nil {
		return "", fmt.Errorf("idgen: read random: %w", err)
	}

	var out [26]byte
	// 時刻部: 48bit を5bitずつ10文字に(50bit分の器に上位2bitはゼロ詰め)。
	for i := 9; i >= 0; i-- {
		out[i] = crockford[ms&31]
		ms >>= 5
	}
	// 乱数部: 80bit を5bitずつ16文字に。
	for i := 0; i < 16; i++ {
		out[10+i] = crockford[take5(random[:], i*5)]
	}
	return string(out[:]), nil
}

// take5 はバイト列の bitOffset ビット目から5ビットを取り出す。
func take5(b []byte, bitOffset int) int {
	byteIdx := bitOffset / 8
	v := int(b[byteIdx]) << 8
	if byteIdx+1 < len(b) {
		v |= int(b[byteIdx+1])
	}
	return (v >> (11 - bitOffset%8)) & 31
}

// #endregion ulid
