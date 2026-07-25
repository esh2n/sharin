package httpserver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
)

// ローカルの実ポートでサーバを起動し、生の TCP で HTTP を喋って検証する。
func startServer(t *testing.T) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.Handle("GET", "/hello", func(req *Request) *Response {
		return Text(200, "hello, "+req.Query("name"))
	})
	s.Handle("POST", "/echo", func(req *Request) *Response {
		return Text(200, string(req.Body))
	})
	go s.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return s, ln.Addr().String()
}

// rawRoundTrip は生の HTTP テキストを送って生のレスポンスを受け取る。
func rawRoundTrip(t *testing.T, addr, request string) string {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestServerGET(t *testing.T) {
	_, addr := startServer(t)
	resp := rawRoundTrip(t, addr, "GET /hello?name=world HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(resp, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("ステータス行が違う: %q", resp)
	}
	if !strings.HasSuffix(resp, "hello, world") {
		t.Errorf("ボディが違う: %q", resp)
	}
}

func TestServerPOST(t *testing.T) {
	_, addr := startServer(t)
	body := "payload"
	req := fmt.Sprintf("POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	resp := rawRoundTrip(t, addr, req)
	if !strings.HasSuffix(resp, body) {
		t.Errorf("echo が返らない: %q", resp)
	}
}

func TestServerNotFound(t *testing.T) {
	_, addr := startServer(t)
	resp := rawRoundTrip(t, addr, "GET /nope HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(resp, "HTTP/1.1 404 Not Found\r\n") {
		t.Errorf("404 になるべき: %q", resp)
	}
}

func TestServerMethodMismatch(t *testing.T) {
	_, addr := startServer(t)
	// /hello は GET のみ。POST は 404(このミニ実装ではメソッド+パスで一意)。
	resp := rawRoundTrip(t, addr, "POST /hello HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(resp, "HTTP/1.1 404") {
		t.Errorf("メソッド不一致は 404: %q", resp)
	}
}

func TestServerMalformedRequest(t *testing.T) {
	_, addr := startServer(t)
	resp := rawRoundTrip(t, addr, "GARBAGE\r\n\r\n")
	if !strings.HasPrefix(resp, "HTTP/1.1 400 Bad Request\r\n") {
		t.Errorf("壊れたリクエストは 400: %q", resp)
	}
}

func TestQuery(t *testing.T) {
	req, _ := ParseRequest(bufio.NewReader(strings.NewReader("GET /p?a=1&b=hello HTTP/1.1\r\n\r\n")))
	if req.Path != "/p" {
		t.Errorf("Path はクエリを除くべき: %q", req.Path)
	}
	if req.Query("a") != "1" || req.Query("b") != "hello" {
		t.Errorf("クエリ解釈が違う: a=%q b=%q", req.Query("a"), req.Query("b"))
	}
	if req.Query("missing") != "" {
		t.Error("無いクエリは空文字のはず")
	}
}
