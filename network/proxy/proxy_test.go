package proxy

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
)

// backend は「受けたリクエストに自分の名前を書いて返す」テスト用のミニ HTTP サーバ。
func backend(t *testing.T, name string) (addr string, seen *int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	count := 0
	var mu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buf := make([]byte, 4096)
				n, _ := conn.Read(buf)
				mu.Lock()
				count++
				mu.Unlock()
				// リクエストに含まれる X-Forwarded-For をボディに反映して返す。
				xff := extractHeader(string(buf[:n]), "X-Forwarded-For")
				body := fmt.Sprintf("from %s (xff=%s)", name, xff)
				fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
			}(c)
		}
	}()
	return ln.Addr().String(), &count
}

func extractHeader(raw, key string) string {
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(key)+":") {
			return strings.TrimSpace(line[len(key)+1:])
		}
	}
	return ""
}

func request(t *testing.T, addr, path string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", path)
	data, _ := io.ReadAll(conn)
	return string(data)
}

func TestReverseProxyForwards(t *testing.T) {
	backendAddr, count := backend(t, "backend-1")
	p := New([]string{backendAddr})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go p.Serve(ln)

	resp := request(t, ln.Addr().String(), "/")
	if !strings.Contains(resp, "from backend-1") {
		t.Errorf("バックエンドに転送されていない: %q", resp)
	}
	if *count != 1 {
		t.Errorf("バックエンドが受けた回数 = %d, want 1", *count)
	}
}

func TestAddsForwardedForHeader(t *testing.T) {
	backendAddr, _ := backend(t, "b")
	p := New([]string{backendAddr})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go p.Serve(ln)

	resp := request(t, ln.Addr().String(), "/")
	// プロキシがクライアントの IP を X-Forwarded-For として付けているはず。
	if !strings.Contains(resp, "xff=127.0.0.1") {
		t.Errorf("X-Forwarded-For が付いていない: %q", resp)
	}
}

func TestRoundRobinLoadBalance(t *testing.T) {
	addr1, c1 := backend(t, "b1")
	addr2, c2 := backend(t, "b2")
	p := New([]string{addr1, addr2})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go p.Serve(ln)

	// 4回叩くと、ラウンドロビンで各バックエンドに2回ずつ振り分けられる。
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		resp := request(t, ln.Addr().String(), "/")
		if strings.Contains(resp, "from b1") {
			seen["b1"]++
		} else if strings.Contains(resp, "from b2") {
			seen["b2"]++
		}
	}
	if *c1 != 2 || *c2 != 2 {
		t.Errorf("分散が偏っている: b1=%d, b2=%d", *c1, *c2)
	}
	if seen["b1"] != 2 || seen["b2"] != 2 {
		t.Errorf("レスポンスの分散が偏っている: %v", seen)
	}
}

func TestNoBackends(t *testing.T) {
	if _, err := newChecked(nil); err == nil {
		t.Error("バックエンド0個はエラーになるべき")
	}
}

func TestBackendDown(t *testing.T) {
	// 存在しないバックエンドに向けると 502 を返す。
	p := New([]string{"127.0.0.1:1"}) // 繋がらないポート
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go p.Serve(ln)

	resp := request(t, ln.Addr().String(), "/")
	if !strings.Contains(resp, "502") {
		t.Errorf("バックエンド不通は 502 を返すべき: %q", resp)
	}
}
