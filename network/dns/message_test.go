package dns

import (
	"bytes"
	"testing"
)

func TestEncodeName(t *testing.T) {
	// "www.example.com" は [3]www[7]example[3]com[0] とラベル長プレフィックスで並ぶ。
	got := encodeName("www.example.com")
	want := []byte{3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
	if !bytes.Equal(got, want) {
		t.Errorf("encodeName = %v, want %v", got, want)
	}
}

func TestBuildQuery(t *testing.T) {
	msg := BuildQuery(0x1234, "example.com")

	// ヘッダ12バイト: ID, フラグ(RD=1), QDCOUNT=1, 残り0。
	if msg[0] != 0x12 || msg[1] != 0x34 {
		t.Errorf("ID が違う: %x %x", msg[0], msg[1])
	}
	if msg[2] != 0x01 || msg[3] != 0x00 {
		t.Errorf("フラグ(RD=1)が違う: %x %x", msg[2], msg[3])
	}
	if msg[4] != 0x00 || msg[5] != 0x01 {
		t.Errorf("QDCOUNT が 1 でない")
	}
	// 質問セクションの末尾は QTYPE=A(1), QCLASS=IN(1)。
	tail := msg[len(msg)-4:]
	want := []byte{0, 1, 0, 1}
	if !bytes.Equal(tail, want) {
		t.Errorf("QTYPE/QCLASS = %v, want %v", tail, want)
	}
}

func TestParseResponse(t *testing.T) {
	// ID=0x1234 の A レコード応答を手で組む。回答は 93.184.216.34。
	resp := []byte{
		0x12, 0x34, // ID
		0x81, 0x80, // フラグ(応答, RD, RA)
		0x00, 0x01, // QDCOUNT=1
		0x00, 0x01, // ANCOUNT=1
		0x00, 0x00, 0x00, 0x00,
		// 質問: example.com A IN
		7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0,
		0x00, 0x01, 0x00, 0x01,
		// 回答: 名前はポインタ圧縮(0xc00c = オフセット12), A, IN, TTL, RDLENGTH=4, IP
		0xc0, 0x0c,
		0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x3c,
		0x00, 0x04,
		93, 184, 216, 34,
	}

	ips, err := ParseResponse(resp, 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || ips[0] != "93.184.216.34" {
		t.Errorf("IP = %v, want [93.184.216.34]", ips)
	}
}

func TestParseResponseIDMismatch(t *testing.T) {
	resp := []byte{0x00, 0x01, 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}
	if _, err := ParseResponse(resp, 0x1234); err == nil {
		t.Error("ID が一致しなければエラーになるべき(他の問い合わせの応答を誤受信)")
	}
}

func TestParseResponseTruncated(t *testing.T) {
	if _, err := ParseResponse([]byte{0x12, 0x34}, 0x1234); err == nil {
		t.Error("短すぎる応答はエラーになるべき")
	}
}
