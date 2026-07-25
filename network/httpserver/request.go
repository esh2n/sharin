// Package httpserver は HTTP/1.1 サーバを TCP ソケットから自作したもの。
//
// 普段フレームワークが隠しているが、HTTP は「TCP の上を流れる人間可読なテキスト」に
// すぎない。net.Listen("tcp") で生のバイトストリームを受け取り、
// リクエストのテキストを手でパースし、レスポンスのテキストを手で組み立てる。
// TCP が下で信頼できるバイト列を届けてくれる前提で、その上に載る HTTP を作る。
package httpserver

import (
	"bufio"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

// #region request
// Request は1つの HTTP リクエスト。
type Request struct {
	Method  string
	Path    string            // クエリ文字列を除いたパス
	Version string            // "HTTP/1.1"
	Headers map[string]string // ヘッダ名は Canonical 形(Content-Type など)
	Body    []byte
	query   map[string]string // ?a=1&b=2 を解釈したもの
}

// ParseRequest は1リクエスト分のバイト列を読んで Request に組み立てる。
// HTTP/1.1 の形は「リクエストライン → ヘッダ群 → 空行 → ボディ」。
func ParseRequest(r *bufio.Reader) (*Request, error) {
	// 1. リクエストライン: "GET /path?q=1 HTTP/1.1"
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("httpserver: read request line: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) != 3 {
		return nil, fmt.Errorf("httpserver: malformed request line %q", line)
	}
	req := &Request{
		Method:  parts[0],
		Version: parts[2],
		Headers: map[string]string{},
		query:   map[string]string{},
	}
	req.parseTarget(parts[1])

	// 2. ヘッダ群: 空行(\r\n)まで "Key: Value" が続く。
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("httpserver: read header: %w", err)
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break // 空行 = ヘッダの終わり
		}
		colon := strings.IndexByte(hline, ':')
		if colon < 0 {
			return nil, fmt.Errorf("httpserver: malformed header %q", hline)
		}
		// ヘッダ名は大小無視なので Canonical 形(Content-Type)に正規化して格納。
		name := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(hline[:colon]))
		value := strings.TrimSpace(hline[colon+1:])
		req.Headers[name] = value
	}

	// 3. ボディ: Content-Length があればそのバイト数だけ読む。
	if cl := req.Headers["Content-Length"]; cl != "" {
		n, err := strconv.Atoi(cl)
		if err != nil {
			return nil, fmt.Errorf("httpserver: bad Content-Length %q", cl)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("httpserver: read body: %w", err)
		}
		req.Body = body
	}
	return req, nil
}

// parseTarget は "/path?a=1&b=2" を Path とクエリに分ける。
func (req *Request) parseTarget(target string) {
	if i := strings.IndexByte(target, '?'); i >= 0 {
		req.Path = target[:i]
		for _, pair := range strings.Split(target[i+1:], "&") {
			if pair == "" {
				continue
			}
			k, v, _ := strings.Cut(pair, "=")
			req.query[k] = v
		}
	} else {
		req.Path = target
	}
}

// Query は ?key=value のクエリ値を返す(無ければ空文字)。
func (req *Request) Query(key string) string { return req.query[key] }

// #endregion request

// #region response
// Response は返す HTTP レスポンス。
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// statusText はステータスコードに対応する短い説明。
var statusText = map[int]string{
	200: "OK",
	400: "Bad Request",
	404: "Not Found",
	500: "Internal Server Error",
}

// Write はレスポンスを HTTP テキストとして書き出す。
// "HTTP/1.1 <code> <text>" → ヘッダ群 → 空行 → ボディ、という形を手で組む。
func (resp *Response) Write(w io.Writer) error {
	text := statusText[resp.Status]
	if text == "" {
		text = "Unknown"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP/1.1 %d %s\r\n", resp.Status, text)

	// Content-Length はボディ長から自動で付ける(受け手が本文の切れ目を知るため)。
	fmt.Fprintf(&sb, "Content-Length: %d\r\n", len(resp.Body))
	for k, v := range resp.Headers {
		fmt.Fprintf(&sb, "%s: %s\r\n", k, v)
	}
	sb.WriteString("\r\n") // ヘッダとボディの区切り
	sb.Write(resp.Body)

	_, err := io.WriteString(w, sb.String())
	return err
}

// Text は text/plain のレスポンスを作るヘルパー。
func Text(status int, body string) *Response {
	return &Response{
		Status:  status,
		Headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"},
		Body:    []byte(body),
	}
}

// #endregion response
