package dns

import (
	"encoding/binary"
	"net"
	"testing"
)

// mockDNS はローカルの UDP サーバで、どんな質問にも固定の A レコードを返す。
// 実際の 8.8.8.8 に問い合わせるとテストがネットワーク依存で不安定になるので、
// リゾルバの「UDP で送って受ける」経路だけをローカルで検証する。
func mockDNS(t *testing.T, ip [4]byte) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			resp := buildAnswer(buf[:n], ip)
			pc.WriteTo(resp, addr)
		}
	}()
	return pc.LocalAddr().String()
}

// buildAnswer は受け取った質問に、固定 IP の A レコード応答を組む。
func buildAnswer(query []byte, ip [4]byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	binary.BigEndian.PutUint16(resp[2:4], 0x8180) // 応答フラグ
	binary.BigEndian.PutUint16(resp[6:8], 1)      // ANCOUNT=1
	// 回答: 名前ポインタ(0xc00c) + A + IN + TTL + RDLENGTH=4 + IP
	resp = append(resp, 0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x04)
	resp = append(resp, ip[0], ip[1], ip[2], ip[3])
	return resp
}

func TestResolve(t *testing.T) {
	addr := mockDNS(t, [4]byte{1, 2, 3, 4})
	r := NewResolver(addr)

	ips, err := r.Resolve("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || ips[0] != "1.2.3.4" {
		t.Errorf("Resolve = %v, want [1.2.3.4]", ips)
	}
}

func TestResolveIDIncrements(t *testing.T) {
	addr := mockDNS(t, [4]byte{5, 6, 7, 8})
	r := NewResolver(addr)
	// 2回引いても両方成功する(IDが変わっても応答を正しく照合できる)。
	for i := 0; i < 2; i++ {
		if _, err := r.Resolve("example.com"); err != nil {
			t.Fatalf("%d回目: %v", i, err)
		}
	}
}

func TestResolveDialError(t *testing.T) {
	r := NewResolver("256.256.256.256:53") // 不正なアドレス
	if _, err := r.Resolve("example.com"); err == nil {
		t.Error("不正なサーバアドレスはエラーになるべき")
	}
}
