package dns

import (
	"fmt"
	"net"
	"time"
)

// #region resolver
// Resolver は DNS サーバに UDP で問い合わせて名前を解決する。
type Resolver struct {
	// Server は問い合わせ先(例: "8.8.8.8:53" = Google Public DNS)。
	Server  string
	Timeout time.Duration
	// nextID は問い合わせIDの採番用。応答を照合するために毎回変える。
	nextID uint16
}

// NewResolver は指定した DNS サーバを使うリゾルバを返す。
func NewResolver(server string) *Resolver {
	return &Resolver{Server: server, Timeout: 3 * time.Second, nextID: 1}
}

// Resolve は name の A レコード(IPv4)を引く。
// 1. UDP ソケットを開く 2. 問い合わせを送る 3. 応答を受け取る 4. パースする。
func (r *Resolver) Resolve(name string) ([]string, error) {
	id := r.nextID
	r.nextID++

	conn, err := net.Dial("udp", r.Server)
	if err != nil {
		return nil, fmt.Errorf("dns: dial %s: %w", r.Server, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(r.Timeout))

	query := BuildQuery(id, name)
	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("dns: send query: %w", err)
	}

	// DNS 応答は 512 バイト以内(UDP の古い上限)に収まる前提で受ける。
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("dns: read response: %w", err)
	}
	return ParseResponse(buf[:n], id)
}

// #endregion resolver
