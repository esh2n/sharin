package rpc

import (
	"bytes"
	"strings"
	"testing"
)

func TestFramingSplitsMessages(t *testing.T) {
	var buf bytes.Buffer
	payloads := [][]byte{
		[]byte("hello"),
		[]byte(""),
		[]byte(strings.Repeat("x", 300)), // 長さが多バイト varint になる
		[]byte("last"),
	}
	for _, p := range payloads {
		if err := writeFrame(&buf, p); err != nil {
			t.Fatalf("writeFrame: %v", err)
		}
	}
	// 1本のストリームから、境界どおりに1フレームずつ切り出せる。
	for i, want := range payloads {
		got, err := readFrame(&buf)
		if err != nil {
			t.Fatalf("readFrame[%d]: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("frame[%d] = %q, want %q", i, got, want)
		}
	}
	if _, err := readFrame(&buf); err == nil {
		t.Fatal("expected error at end of stream")
	}
}

func TestCallRoundTrip(t *testing.T) {
	in := Call{ID: 42, Method: "user.Get", Body: []byte("payload")}
	out, err := decodeCall(encodeCall(in))
	if err != nil {
		t.Fatalf("decodeCall: %v", err)
	}
	if out.ID != in.ID || out.Method != in.Method || !bytes.Equal(out.Body, in.Body) {
		t.Fatalf("round trip mismatch: %+v vs %+v", out, in)
	}
}

func TestReplyRoundTrip(t *testing.T) {
	in := Reply{ID: 7, Status: 2, Body: []byte("boom")}
	out, err := decodeReply(encodeReply(in))
	if err != nil {
		t.Fatalf("decodeReply: %v", err)
	}
	if out.ID != in.ID || out.Status != in.Status || !bytes.Equal(out.Body, in.Body) {
		t.Fatalf("round trip mismatch: %+v vs %+v", out, in)
	}
}

func TestDecodeCallTruncated(t *testing.T) {
	full := encodeCall(Call{ID: 1, Method: "m", Body: []byte("abc")})
	// 途中で切ると、長さぶんのバイトが足りずエラーになる。
	if _, err := decodeCall(full[:len(full)-2]); err == nil {
		t.Fatal("expected truncation error")
	}
}

func TestDecodeUnknownField(t *testing.T) {
	// フィールド番号 9 は未知。
	bad := appendUvarint(nil, 9, 123)
	if _, err := decodeCall(bad); err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestDecodeReplyErrors(t *testing.T) {
	if _, err := decodeReply(appendUvarint(nil, 9, 1)); err == nil {
		t.Fatal("expected unknown-field error in reply")
	}
	full := encodeReply(Reply{ID: 1, Status: 0, Body: []byte("abcd")})
	if _, err := decodeReply(full[:len(full)-2]); err == nil {
		t.Fatal("expected truncation error in reply")
	}
}

func TestRemoteErrorMessage(t *testing.T) {
	e := &RemoteError{Status: 2, Msg: "boom"}
	if got := e.Error(); !strings.Contains(got, "boom") || !strings.Contains(got, "status 2") {
		t.Fatalf("RemoteError.Error() = %q", got)
	}
}
