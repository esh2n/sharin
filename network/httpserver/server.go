package httpserver

import (
	"bufio"
	"net"
)

// #region server
// Handler はリクエストを受けてレスポンスを返す関数。
type Handler func(*Request) *Response

// Server は「メソッド + パス → ハンドラ」の対応表を持つ最小の HTTP サーバ。
type Server struct {
	routes map[string]Handler // キーは "GET /hello" のような文字列
}

// NewServer は空のサーバを返す。
func NewServer() *Server {
	return &Server{routes: map[string]Handler{}}
}

// Handle はメソッドとパスにハンドラを登録する。
func (s *Server) Handle(method, path string, h Handler) {
	s.routes[method+" "+path] = h
}

// ListenAndServe は addr で待ち受けて Serve する。
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve は接続を受け付け続け、1接続ごとに goroutine で処理する。
// これが「サーバ」の本体: Accept でクライアントとの TCP 接続を得て、handleConn に渡す。
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err // リスナが閉じられたら終了
		}
		go s.handleConn(conn)
	}
}

// #endregion server

// #region handle
// handleConn は1つの TCP 接続を処理する。
// 生のバイトストリーム(conn)から HTTP リクエストをパースし、
// ルートを引いてハンドラを呼び、レスポンスを書き戻す。
// このミニ実装は1接続1リクエストで閉じる(keep-alive はしない)。
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	req, err := ParseRequest(bufio.NewReader(conn))
	if err != nil {
		// パースできない = 壊れたリクエスト。400 を返す。
		Text(400, "bad request").Write(conn)
		return
	}

	h, ok := s.routes[req.Method+" "+req.Path]
	if !ok {
		Text(404, "not found").Write(conn)
		return
	}

	resp := h(req)
	if resp == nil {
		resp = Text(500, "handler returned no response")
	}
	resp.Write(conn)
}

// #endregion handle
