package websocket

import (
	"bytes"
	"testing"
)

// TestAcceptRFCExample はこの章の主眼その 1。RFC 6455 の例と一致することを固定する。
// SHA-1 と base64 を自作しているので、実物と同じ Accept 値が出る。
func TestAcceptRFCExample(t *testing.T) {
	// RFC 6455 の例。
	got := Accept("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("Accept mismatch:\n got  %q\n want %q", got, want)
	}
}

func TestSHA1KnownVectors(t *testing.T) {
	// "abc" の SHA-1(既知ベクトル)。
	got := sha1Sum([]byte("abc"))
	want := []byte{
		0xa9, 0x99, 0x3e, 0x36, 0x47, 0x06, 0x81, 0x6a, 0xba, 0x3e,
		0x25, 0x71, 0x78, 0x50, 0xc2, 0x6c, 0x9c, 0xd0, 0xd8, 0x9d,
	}
	if !bytes.Equal(got[:], want) {
		t.Fatalf("sha1(abc): got %x want %x", got, want)
	}
	// 空文字列の SHA-1。
	empty := sha1Sum(nil)
	wantEmpty := []byte{
		0xda, 0x39, 0xa3, 0xee, 0x5e, 0x6b, 0x4b, 0x0d, 0x32, 0x55,
		0xbf, 0xef, 0x95, 0x60, 0x18, 0x90, 0xaf, 0xd8, 0x07, 0x09,
	}
	if !bytes.Equal(empty[:], wantEmpty) {
		t.Fatalf("sha1(''): got %x want %x", empty, wantEmpty)
	}
}

func TestBase64(t *testing.T) {
	cases := map[string]string{
		"":       "",
		"f":      "Zg==",
		"fo":     "Zm8=",
		"foo":    "Zm9v",
		"foobar": "Zm9vYmFy",
	}
	for in, want := range cases {
		if got := base64Encode([]byte(in)); got != want {
			t.Fatalf("base64(%q): got %q want %q", in, got, want)
		}
	}
}

// TestFrameRoundTrip はフレームの符号化・復号が一致することを確かめる。
func TestFrameRoundTrip(t *testing.T) {
	cases := []Frame{
		{Fin: true, Opcode: OpText, Payload: []byte("hello")},
		{Fin: true, Opcode: OpBinary, Payload: []byte{0, 1, 2, 255}},
		{Fin: false, Opcode: OpText, Payload: []byte("partial")},
		{Fin: true, Opcode: OpClose, Payload: nil},
	}
	for _, in := range cases {
		enc := Encode(in)
		out, n, err := Decode(enc)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if n != len(enc) {
			t.Fatalf("consumed %d want %d", n, len(enc))
		}
		if out.Fin != in.Fin || out.Opcode != in.Opcode || !bytes.Equal(out.Payload, in.Payload) {
			t.Fatalf("roundtrip mismatch: in=%+v out=%+v", in, out)
		}
	}
}

// TestMaskingHidesPlaintext はこの章の主眼その 2。クライアント→サーバの
// フレームはマスクされ、ワイヤ上のバイト列に平文がそのまま現れないこと、
// 復号で正しく平文に戻ることを固定する。
func TestMaskingHidesPlaintext(t *testing.T) {
	plaintext := []byte("secret message")
	f := Frame{Fin: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{0x12, 0x34, 0x56, 0x78}, Payload: plaintext}
	enc := Encode(f)

	// ワイヤ上に平文がそのまま出ない(マスクされている)。
	if bytes.Contains(enc, plaintext) {
		t.Fatal("masked frame must not contain plaintext on the wire")
	}

	// 復号すると平文に戻る。
	out, _, err := Decode(enc)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !bytes.Equal(out.Payload, plaintext) {
		t.Fatalf("unmask failed: got %q want %q", out.Payload, plaintext)
	}
	if !out.Masked {
		t.Fatal("decoded frame should report masked")
	}
}

// TestExtendedLength は 125 を超えるペイロードの長さ符号化(126 パス)を確かめる。
func TestExtendedLength(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 300) // 126 パス(2 バイト長)
	f := Frame{Fin: true, Opcode: OpBinary, Payload: big}
	enc := Encode(f)
	// 2 バイト目の下位 7 ビットが 126 を示す。
	if enc[1]&0x7f != 126 {
		t.Fatalf("expected extended length marker 126, got %d", enc[1]&0x7f)
	}
	out, n, err := Decode(enc)
	if err != nil || n != len(enc) || !bytes.Equal(out.Payload, big) {
		t.Fatalf("extended-length roundtrip failed: err=%v", err)
	}
}

func TestDecodeShortFrame(t *testing.T) {
	if _, _, err := Decode([]byte{0x81}); err == nil {
		t.Fatal("1-byte input should error")
	}
	// マスクありと言いつつ鍵が足りない。
	if _, _, err := Decode([]byte{0x81, 0x85, 0x00}); err == nil {
		t.Fatal("truncated masked frame should error")
	}
}
