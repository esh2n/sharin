package httpserver

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseRequestGET(t *testing.T) {
	raw := "GET /hello HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test\r\n\r\n"
	req, err := ParseRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "GET" || req.Path != "/hello" || req.Version != "HTTP/1.1" {
		t.Errorf("リクエストライン = %+v", req)
	}
	if req.Headers["Host"] != "example.com" || req.Headers["User-Agent"] != "test" {
		t.Errorf("ヘッダ = %v", req.Headers)
	}
	if len(req.Body) != 0 {
		t.Errorf("GET のボディは空のはず: %q", req.Body)
	}
}

func TestParseRequestPOSTWithBody(t *testing.T) {
	raw := "POST /submit HTTP/1.1\r\nContent-Length: 11\r\n\r\nhello world"
	req, err := ParseRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" {
		t.Errorf("Method = %q", req.Method)
	}
	if string(req.Body) != "hello world" {
		t.Errorf("Body = %q, want %q", req.Body, "hello world")
	}
}

func TestParseRequestHeaderCaseInsensitive(t *testing.T) {
	// ヘッダ名は大小無視が HTTP の仕様。Canonical 形に正規化して引けること。
	raw := "GET / HTTP/1.1\r\ncontent-type: text/plain\r\n\r\n"
	req, err := ParseRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if req.Headers["Content-Type"] != "text/plain" {
		t.Errorf("大小無視で引けるべき: %v", req.Headers)
	}
}

func TestParseRequestErrors(t *testing.T) {
	bad := []string{
		"",                                 // 空
		"GET\r\n\r\n",                      // リクエストラインの要素不足
		"GET / HTTP/1.1\r\nbadhdr\r\n\r\n", // コロンのないヘッダ
	}
	for _, raw := range bad {
		if _, err := ParseRequest(bufio.NewReader(strings.NewReader(raw))); err == nil {
			t.Errorf("ParseRequest(%q) はエラーになるべき", raw)
		}
	}
}

func TestResponseWrite(t *testing.T) {
	var sb strings.Builder
	resp := &Response{Status: 200, Headers: map[string]string{"Content-Type": "text/plain"}, Body: []byte("hi")}
	if err := resp.Write(&sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("ステータス行が違う: %q", out)
	}
	if !strings.Contains(out, "Content-Type: text/plain\r\n") {
		t.Errorf("ヘッダが無い: %q", out)
	}
	if !strings.Contains(out, "Content-Length: 2\r\n") {
		t.Errorf("Content-Length が自動付与されるべき: %q", out)
	}
	if !strings.HasSuffix(out, "\r\n\r\nhi") {
		t.Errorf("空行の後にボディが来るべき: %q", out)
	}
}
