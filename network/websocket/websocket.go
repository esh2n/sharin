// Package websocket は WebSocket の要である Upgrade ハンドシェイクと
// フレーミングを最小構成で実装する。
//
// HTTP は要求と応答が対になる。クライアントが尋ね、サーバが答える。サーバの
// 都合で勝手にデータを送りつけることはできない。チャットや通知のように、
// サーバ発の即時配信が要る場面ではこれが困る。WebSocket は、まず普通の HTTP
// リクエストとして始まり、Upgrade ヘッダで「この接続を WebSocket に昇格させて
// くれ」と頼む。サーバが受理すると、以後その 1 本の TCP 接続は要求応答の型を
// 脱ぎ、両側がいつでもメッセージを送れる全二重の通路になる。やり取りは
// フレームという単位で、先頭に FIN・オペコード・マスクの有無・長さが並ぶ。
// クライアントからサーバへのフレームは必ずマスク(鍵で XOR)する。これは
// 中継プロキシのキャッシュを汚染する攻撃を防ぐための決まりだ。
package websocket

// #region handshake

// magicGUID は Upgrade の応答鍵を作るための固定文字列(RFC 6455)。
const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Accept は Sec-WebSocket-Key から Sec-WebSocket-Accept を計算する。
// key に magicGUID を連結し、SHA-1 して base64 する。サーバがこの値を返せる
// ことが「WebSocket を理解している」証明になり、偶然の昇格を防ぐ。
func Accept(key string) string {
	sum := sha1Sum([]byte(key + magicGUID))
	return base64Encode(sum[:])
}

// #endregion handshake

// #region frame

// オペコードはフレームの種類。
const (
	OpContinuation byte = 0x0
	OpText         byte = 0x1
	OpBinary       byte = 0x2
	OpClose        byte = 0x8
	OpPing         byte = 0x9
	OpPong         byte = 0xa
)

// Frame は WebSocket の 1 フレーム。
type Frame struct {
	Fin     bool    // このメッセージの最終フレームか
	Opcode  byte    // 種類(text/binary/close/ping/pong)
	Masked  bool    // ペイロードがマスクされているか(client→server は必須)
	MaskKey [4]byte // マスク鍵
	Payload []byte  // 本体(マスク前の平文で保持)
}

// Encode はフレームをバイト列にする。マスク指定があればペイロードを鍵で XOR する。
func Encode(f Frame) []byte {
	var b []byte

	b0 := f.Opcode & 0x0f
	if f.Fin {
		b0 |= 0x80
	}
	b = append(b, b0)

	// 長さの符号化。125 以下は 1 バイト、65535 以下は 126+2 バイト、以上は 127+8 バイト。
	n := len(f.Payload)
	maskBit := byte(0)
	if f.Masked {
		maskBit = 0x80
	}
	switch {
	case n <= 125:
		b = append(b, maskBit|byte(n))
	case n <= 0xffff:
		b = append(b, maskBit|126, byte(n>>8), byte(n))
	default:
		b = append(b, maskBit|127)
		for s := 56; s >= 0; s -= 8 {
			b = append(b, byte(n>>uint(s)))
		}
	}

	if f.Masked {
		b = append(b, f.MaskKey[0], f.MaskKey[1], f.MaskKey[2], f.MaskKey[3])
		for i, p := range f.Payload {
			b = append(b, p^f.MaskKey[i%4]) // 鍵で XOR してマスク
		}
	} else {
		b = append(b, f.Payload...)
	}
	return b
}

// #endregion frame

// #region decode

// ErrShort はバイト列が 1 フレームに満たないとき。
type ErrShort struct{}

func (ErrShort) Error() string { return "websocket: short frame" }

// Decode はバイト列の先頭から 1 フレームを読み、消費バイト数とともに返す。
// マスクされていれば鍵で外し、Payload には平文を入れる。
func Decode(b []byte) (Frame, int, error) {
	if len(b) < 2 {
		return Frame{}, 0, ErrShort{}
	}
	var f Frame
	f.Fin = b[0]&0x80 != 0
	f.Opcode = b[0] & 0x0f
	f.Masked = b[1]&0x80 != 0

	n := int(b[1] & 0x7f)
	pos := 2
	switch n {
	case 126:
		if len(b) < pos+2 {
			return Frame{}, 0, ErrShort{}
		}
		n = int(b[pos])<<8 | int(b[pos+1])
		pos += 2
	case 127:
		if len(b) < pos+8 {
			return Frame{}, 0, ErrShort{}
		}
		n = 0
		for i := 0; i < 8; i++ {
			n = n<<8 | int(b[pos+i])
		}
		pos += 8
	}

	if f.Masked {
		if len(b) < pos+4 {
			return Frame{}, 0, ErrShort{}
		}
		copy(f.MaskKey[:], b[pos:pos+4])
		pos += 4
	}
	if len(b) < pos+n {
		return Frame{}, 0, ErrShort{}
	}

	f.Payload = make([]byte, n)
	if f.Masked {
		for i := 0; i < n; i++ {
			f.Payload[i] = b[pos+i] ^ f.MaskKey[i%4] // マスクを外す
		}
	} else {
		copy(f.Payload, b[pos:pos+n])
	}
	return f, pos + n, nil
}

// #endregion decode
