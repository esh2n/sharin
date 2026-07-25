// Package idgen は分散ID発番方式(UUIDv4 / UUIDv7 / ULID / Snowflake)の最小実装。
//
// どの方式も材料は「時刻・乱数・ノードID」の3つで、その配合が違うだけ。
// 配合の違いが「調整不要か」「ソート可能か」「何ビットか」を決める。
package idgen

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// #region uuidv4
// NewUUIDv4 は128bit中122bitが純粋な乱数のUUIDを返す。時刻もノードIDも使わない。
// rand には crypto/rand.Reader を渡す(テストでは固定バイト列を注入する)。
func NewUUIDv4(rand io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return "", fmt.Errorf("idgen: read random: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return formatUUID(b), nil
}

// #endregion uuidv4

// #region uuidv7
// NewUUIDv7 は先頭48bitにミリ秒タイムスタンプを置くUUIDを返す(RFC 9562)。
// 生成時刻順に文字列比較でもソートできるのが v4 との違い。
func NewUUIDv7(now time.Time, rand io.Reader) (string, error) {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	if ms >= 1<<48 {
		return "", errors.New("idgen: timestamp exceeds 48 bits")
	}
	// 先頭6バイト = 48bit ミリ秒(big endian)。上位バイトが先頭に来るから辞書順=時刻順になる。
	for i := 0; i < 6; i++ {
		b[i] = byte(ms >> (40 - 8*i))
	}
	if _, err := io.ReadFull(rand, b[6:]); err != nil {
		return "", fmt.Errorf("idgen: read random: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return formatUUID(b), nil
}

// #endregion uuidv7

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
