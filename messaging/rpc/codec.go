// Package rpc は最小の RPC(遠隔手続き呼び出し)実装。
//
// RPC は「関数を呼ぶように、別プロセスの処理を呼ぶ」仕組み。だが実体はネットワーク越しの
// メッセージ交換で、ローカルの関数呼び出しとは決定的に違う。この実装で扱うのは3つ:
//
//   - シリアライズ: 型のある要求/応答を、バイト列にして戻す(protobuf 風のタグ + varint)。
//   - フレーミング: バイトストリームには境界が無い。長さを前置きして「1メッセージ」を切り出す。
//   - 相関(correlation): 1本の接続に要求を多重化し、応答を要求IDで突き合わせる。
//     応答は送った順に返るとは限らない。
//
// そして RPC は「ローカル呼び出しではない」——ネットワークエラー・タイムアウト・部分故障が
// 起きる。再送すると処理が二重に走りうるので、受け手は冪等であるべき(→ message-queue の章)。
//
// codec.go はその土台のシリアライズとフレーミングを担う。
package rpc

import (
	"encoding/binary"
	"errors"
	"io"
)

// --- varint / フレーミング ---
// バイトストリームには「ここで1メッセージ」という区切りが無い。長さ(uvarint)を前置きし、
// その分だけ読むことで1フレームを切り出す。これをしないと、2つの送信がくっついたり
// 途中で切れたりする(TCP はバイトの列で、メッセージの列ではない)。

// #region framing
// writeFrame は payload の前に長さ(uvarint)を書いて1フレームとして送る。
func writeFrame(w io.Writer, payload []byte) error {
	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], uint64(len(payload)))
	if _, err := w.Write(hdr[:n]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame は1フレーム(長さ + payload)を読み出す。
func readFrame(r io.Reader) ([]byte, error) {
	br, ok := r.(io.ByteReader)
	if !ok {
		br = &byteReader{r: r}
	}
	n, err := binary.ReadUvarint(br)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// #endregion framing

// byteReader は io.Reader を1バイトずつ読めるようにする薄いラッパ。
type byteReader struct{ r io.Reader }

func (b *byteReader) ReadByte() (byte, error) {
	var p [1]byte
	_, err := io.ReadFull(b.r, p[:])
	return p[0], err
}

// --- protobuf 風の値エンコード ---
// 各フィールドを [タグ(フィールド番号)] [値] の並びで書く。整数は varint(小さい値ほど短い)、
// 文字列/バイト列は [長さ][中身] で書く。デコード側はタグを見てフィールドを振り分ける。

const (
	fieldID     = 1 // 要求/応答の相関ID
	fieldMethod = 2 // メソッド名(要求)
	fieldStatus = 2 // ステータス(応答) ※要求とは別メッセージなので番号は再利用
	fieldBody   = 3 // 本体(引数 or 結果)
)

func appendUvarint(b []byte, field int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(field))
	return binary.AppendUvarint(b, v)
}

func appendBytes(b []byte, field int, v []byte) []byte {
	b = binary.AppendUvarint(b, uint64(field))
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

// Call は1回の要求。ID で応答と突き合わせる。
type Call struct {
	ID     uint64
	Method string
	Body   []byte
}

func encodeCall(c Call) []byte {
	var b []byte
	b = appendUvarint(b, fieldID, c.ID)
	b = appendBytes(b, fieldMethod, []byte(c.Method))
	b = appendBytes(b, fieldBody, c.Body)
	return b
}

func decodeCall(data []byte) (Call, error) {
	var c Call
	d := decoder{data: data}
	for d.more() {
		field, err := d.uvarint()
		if err != nil {
			return Call{}, err
		}
		switch int(field) {
		case fieldID:
			if c.ID, err = d.uvarint(); err != nil {
				return Call{}, err
			}
		case fieldMethod:
			s, err := d.bytes()
			if err != nil {
				return Call{}, err
			}
			c.Method = string(s)
		case fieldBody:
			if c.Body, err = d.bytes(); err != nil {
				return Call{}, err
			}
		default:
			return Call{}, errors.New("rpc: unknown field in call")
		}
	}
	return c, nil
}

// Reply は1回の応答。Status=0 が成功、それ以外はエラー(Body にメッセージ)。
type Reply struct {
	ID     uint64
	Status uint64
	Body   []byte
}

const statusOK = 0

func encodeReply(r Reply) []byte {
	var b []byte
	b = appendUvarint(b, fieldID, r.ID)
	b = appendUvarint(b, fieldStatus, r.Status)
	b = appendBytes(b, fieldBody, r.Body)
	return b
}

func decodeReply(data []byte) (Reply, error) {
	var r Reply
	d := decoder{data: data}
	for d.more() {
		field, err := d.uvarint()
		if err != nil {
			return Reply{}, err
		}
		switch int(field) {
		case fieldID:
			if r.ID, err = d.uvarint(); err != nil {
				return Reply{}, err
			}
		case fieldStatus:
			if r.Status, err = d.uvarint(); err != nil {
				return Reply{}, err
			}
		case fieldBody:
			if r.Body, err = d.bytes(); err != nil {
				return Reply{}, err
			}
		default:
			return Reply{}, errors.New("rpc: unknown field in reply")
		}
	}
	return r, nil
}

// decoder は appendUvarint/appendBytes の逆。data を先頭から読み進める。
type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) more() bool { return d.pos < len(d.data) }

func (d *decoder) uvarint() (uint64, error) {
	v, n := binary.Uvarint(d.data[d.pos:])
	if n <= 0 {
		return 0, errors.New("rpc: bad uvarint")
	}
	d.pos += n
	return v, nil
}

func (d *decoder) bytes() ([]byte, error) {
	n, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	if d.pos+int(n) > len(d.data) {
		return nil, errors.New("rpc: truncated bytes")
	}
	out := d.data[d.pos : d.pos+int(n)]
	d.pos += int(n)
	return out, nil
}
