package dns

import "encoding/binary"

// #region size

// UDPLimit は UDP で送れる DNS メッセージの上限(RFC 1035)。
//
// この 512 という数字が、DNS のフォーマットをほぼ全部決めている。
// テキストではなくバイナリなのも、名前をポインタで指すのも、ここに収めるため。
const UDPLimit = 512

// QuerySize は問い合わせメッセージのバイト数を返す。
// ヘッダ12 + 名前 + QTYPE 2 + QCLASS 2。
func QuerySize(name string) int {
	return 12 + len(encodeName(name)) + 4
}

// AnswerSize は A レコード1件のバイト数を返す。
//
// 名前のあとに TYPE 2 + CLASS 2 + TTL 4 + RDLENGTH 2 + IPv4 4 が続く。
// 圧縮すると名前が2バイトのポインタになるので、合計は 16 で固定になる。
// 名前をそのまま書くと、名前が長いほどレコードも太る。
func AnswerSize(name string, compress bool) int {
	n := 2 // 圧縮ポインタ
	if !compress {
		n = len(encodeName(name))
	}
	return n + 2 + 2 + 4 + 2 + 4
}

// Capacity は 512 バイトに入る A レコードの件数を返す。
//
// 圧縮ありなら 1件 16 バイト固定なので、名前が長くなっても
// 減るのは質問セクションのぶんだけになる。
func Capacity(name string, compress bool) int {
	return (UDPLimit - QuerySize(name)) / AnswerSize(name, compress)
}

// #endregion size

// #region response

// BuildResponse は回答つきの応答メッセージを組み立てる。
//
// compress が false なら、回答ごとに名前をそのまま書き直す。
// 512 バイトに収まらなくなったらそこで打ち切り、TC ビットを立てる。
// TC=1 は「入りきらなかったので TCP でやり直して」の合図になる。
func BuildResponse(id uint16, name string, ips [][4]byte, compress bool) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], id)
	binary.BigEndian.PutUint16(msg[2:4], 0x8180) // 応答, RD, RA
	binary.BigEndian.PutUint16(msg[4:6], 1)      // QDCOUNT

	// 質問の名前は必ずオフセット12から始まる。回答はここを指せばいい。
	const qnameOffset = 12
	qname := encodeName(name)
	msg = append(msg, qname...)
	msg = binary.BigEndian.AppendUint16(msg, 1) // QTYPE = A
	msg = binary.BigEndian.AppendUint16(msg, 1) // QCLASS = IN

	n := 0
	for _, ip := range ips {
		if len(msg)+AnswerSize(name, compress) > UDPLimit {
			msg[2] |= 0x02 // TC = 1
			break
		}
		if compress {
			msg = binary.BigEndian.AppendUint16(msg, 0xc000|qnameOffset)
		} else {
			msg = append(msg, qname...)
		}
		msg = binary.BigEndian.AppendUint16(msg, 1)  // TYPE = A
		msg = binary.BigEndian.AppendUint16(msg, 1)  // CLASS = IN
		msg = binary.BigEndian.AppendUint32(msg, 60) // TTL
		msg = binary.BigEndian.AppendUint16(msg, 4)  // RDLENGTH
		msg = append(msg, ip[:]...)
		n++
	}
	binary.BigEndian.PutUint16(msg[6:8], uint16(n)) // ANCOUNT
	return msg
}

// Truncated は TC ビットが立っているかを返す。
// 立っていたら、同じ問い合わせを TCP で投げ直すことになる。
func Truncated(msg []byte) bool {
	return len(msg) >= 12 && msg[2]&0x02 != 0
}

// #endregion response
