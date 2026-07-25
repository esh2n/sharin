// Package proxy は HTTP リバースプロキシ + ラウンドロビン負荷分散の最小実装。
// ネットワーク編の集大成。
//
// リバースプロキシは「クライアントとバックエンドの間に立つ代理人」。
// クライアントからのリクエストを受け、裏のサーバ(バックエンド)に転送し、
// その応答をクライアントに返す。クライアントはプロキシと話しているつもりだが、
// 実際に処理しているのは裏のサーバ。この1枚を挟むだけで、負荷分散・SSL終端・
// キャッシュ・レート制限といった機能を1箇所に集約できる。
//
// L4(TCP をそのまま中継)と L7(HTTP を理解して転送)の違いがこの実装の主題。
// ここでは L7: リクエストを解釈し、X-Forwarded-For を足してから転送する。
package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"

	"github.com/esh2n/sharin/network/httpserver"
)

// #region proxy
// Proxy はバックエンド群への振り分けを行うリバースプロキシ。
type Proxy struct {
	backends []string
	next     atomic.Uint64 // ラウンドロビンの次番号
}

// New はバックエンド一覧を持つプロキシを返す(空ならパニック)。
func New(backends []string) *Proxy {
	p, err := newChecked(backends)
	if err != nil {
		panic(err)
	}
	return p
}

func newChecked(backends []string) (*Proxy, error) {
	if len(backends) == 0 {
		return nil, errors.New("proxy: need at least one backend")
	}
	return &Proxy{backends: backends}, nil
}

// pick はラウンドロビンで次のバックエンドを選ぶ。
// atomic なので複数接続が同時に来ても番号が重複しない。
func (p *Proxy) pick() string {
	i := p.next.Add(1) - 1
	return p.backends[i%uint64(len(p.backends))]
}

// #endregion proxy

// #region serve
// Serve は接続を受け付け、1接続ごとに handle する。構造は httpserver と同じ Accept ループ。
func (p *Proxy) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go p.handle(conn)
	}
}

// handle は1リクエストを受けて、選んだバックエンドに転送する。
func (p *Proxy) handle(client net.Conn) {
	defer client.Close()

	// L7 プロキシなので、まずリクエストを「理解」する。
	req, err := httpserver.ParseRequest(bufio.NewReader(client))
	if err != nil {
		httpserver.Text(400, "bad request").Write(client)
		return
	}

	// クライアントの IP を X-Forwarded-For に記録する。
	// これがないと、バックエンドから見た送信元は常にプロキシになり、
	// 「本当のクライアントが誰か」が失われる。プロキシの重要な責務。
	clientIP := hostOf(client.RemoteAddr().String())

	backend := p.pick()
	if err := p.forward(backend, req, clientIP, client); err != nil {
		httpserver.Text(502, "bad gateway: "+err.Error()).Write(client)
	}
}

// #endregion serve

// #region forward
// forward は選んだバックエンドに接続し、リクエストを組み直して送り、応答をそのまま
// クライアントへ中継する。
func (p *Proxy) forward(backend string, req *httpserver.Request, clientIP string, client net.Conn) error {
	bconn, err := net.Dial("tcp", backend)
	if err != nil {
		return fmt.Errorf("dial backend: %w", err)
	}
	defer bconn.Close()

	// リクエストを再構築してバックエンドへ送る。X-Forwarded-For を付ける。
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s %s\r\n", req.Method, req.Path, req.Version)
	for k, v := range req.Headers {
		fmt.Fprintf(&sb, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&sb, "X-Forwarded-For: %s\r\n", clientIP)
	sb.WriteString("\r\n")
	if _, err := io.WriteString(bconn, sb.String()); err != nil {
		return fmt.Errorf("write to backend: %w", err)
	}
	if len(req.Body) > 0 {
		bconn.Write(req.Body)
	}

	// バックエンドの応答をそのままクライアントへ流す(ストリーム中継)。
	if _, err := io.Copy(client, bconn); err != nil {
		return fmt.Errorf("relay response: %w", err)
	}
	return nil
}

// hostOf は "127.0.0.1:54321" から "127.0.0.1" を取り出す。
func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// #endregion forward
