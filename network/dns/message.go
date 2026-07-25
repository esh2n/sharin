// Package dns は DNS リゾルバの最小実装(A レコードの問い合わせ)。
//
// ブラウザに "example.com" と打つと、まず「その名前の IP は?」を DNS サーバに聞く。
// この問い合わせは UDP で送るバイナリのメッセージで、HTTP のようなテキストではない。
// この章ではそのメッセージを手で組み立て(encode)、返ってきたバイト列を手で解く(decode)。
package dns

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// #region encode
// encodeName はドメイン名を DNS のラベル形式にする。
// "www.example.com" → [3]www[7]example[3]com[0]。
// 各ラベルの前に「そのラベルの長さ」を1バイト置き、最後を 0 で終える。
func encodeName(name string) []byte {
	var buf []byte
	for _, label := range strings.Split(name, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0) // ルート(空ラベル)で終端
	return buf
}

// BuildQuery は A レコードの問い合わせメッセージを組み立てる。
// 構成: 12バイトのヘッダ + 質問セクション(名前 + QTYPE + QCLASS)。
func BuildQuery(id uint16, name string) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], id)     // ID: 応答を照合するための番号
	binary.BigEndian.PutUint16(msg[2:4], 0x0100) // フラグ: RD=1(再帰的に解決してほしい)
	binary.BigEndian.PutUint16(msg[4:6], 1)      // QDCOUNT: 質問1つ
	// ANCOUNT/NSCOUNT/ARCOUNT は 0 のまま。

	msg = append(msg, encodeName(name)...)
	msg = binary.BigEndian.AppendUint16(msg, 1) // QTYPE = A(IPv4 アドレス)
	msg = binary.BigEndian.AppendUint16(msg, 1) // QCLASS = IN(インターネット)
	return msg
}

// #endregion encode

// #region decode
// ParseResponse は応答メッセージから A レコードの IP を取り出す。
// wantID は送った問い合わせの ID。一致しなければ別の問い合わせの応答なので弾く。
func ParseResponse(msg []byte, wantID uint16) ([]string, error) {
	if len(msg) < 12 {
		return nil, fmt.Errorf("dns: response too short (%d bytes)", len(msg))
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	if id != wantID {
		return nil, fmt.Errorf("dns: id mismatch: got %#x, want %#x", id, wantID)
	}
	qdcount := binary.BigEndian.Uint16(msg[4:6])
	ancount := binary.BigEndian.Uint16(msg[6:8])

	// 質問セクションを読み飛ばす(名前 + QTYPE 2B + QCLASS 2B)。
	off := 12
	for i := 0; i < int(qdcount); i++ {
		n, err := skipName(msg, off)
		if err != nil {
			return nil, err
		}
		off = n + 4 // QTYPE + QCLASS
	}

	// 回答レコードを読む。
	var ips []string
	for i := 0; i < int(ancount); i++ {
		n, err := skipName(msg, off) // 回答の名前(たいていポインタ圧縮)
		if err != nil {
			return nil, err
		}
		off = n
		if off+10 > len(msg) {
			return nil, fmt.Errorf("dns: truncated answer header")
		}
		typ := binary.BigEndian.Uint16(msg[off : off+2])
		rdlength := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		off += 10
		if off+rdlength > len(msg) {
			return nil, fmt.Errorf("dns: truncated rdata")
		}
		if typ == 1 && rdlength == 4 { // A レコード = 4バイトの IPv4
			ip := msg[off : off+4]
			ips = append(ips, fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]))
		}
		off += rdlength
	}
	return ips, nil
}

// skipName は off から名前を読み飛ばし、その次のオフセットを返す。
// DNS はメッセージを縮めるため「名前をポインタで参照する」圧縮を使う(上位2bitが 11)。
// ポインタに当たったら、そこで名前は終わり(2バイトぶん進めて返す)。
func skipName(msg []byte, off int) (int, error) {
	for {
		if off >= len(msg) {
			return 0, fmt.Errorf("dns: name runs past end of message")
		}
		length := int(msg[off])
		switch {
		case length == 0:
			return off + 1, nil // ルートラベル。名前の終わり
		case length&0xc0 == 0xc0:
			// 圧縮ポインタ(2バイト)。ここで名前は終端とみなす。
			return off + 2, nil
		default:
			off += 1 + length // 通常ラベル: 長さ + 中身を飛ばす
		}
	}
}

// #endregion decode
