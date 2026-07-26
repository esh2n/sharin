package rpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

// ステータスコード(statusOK は codec.go)。
const (
	statusUnknownMethod = 1
	statusHandlerError  = 2
)

// RemoteError はサーバ側で起きたエラーを、呼び出し側へ運ぶ。
type RemoteError struct {
	Status uint64
	Msg    string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("rpc: remote error (status %d): %s", e.Status, e.Msg)
}

// --- サーバ ---

// Handler はメソッド1つの処理。引数バイト列を受け、結果バイト列かエラーを返す。
type Handler func(ctx context.Context, body []byte) ([]byte, error)

// Server はメソッド名 → Handler の対応表を持つ。
type Server struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewServer は空のサーバを作る。
func NewServer() *Server {
	return &Server{handlers: map[string]Handler{}}
}

// Register はメソッドを登録する。
func (s *Server) Register(method string, h Handler) {
	s.mu.Lock()
	s.handlers[method] = h
	s.mu.Unlock()
}

// Serve は接続からフレームを読み、要求ごとに処理して応答を返し続ける。
// 各要求は並行に処理するので、応答は要求の順とは限らない(だから ID で相関する)。
// 接続への書き込みだけは writeMu で直列化する(フレームが混ざらないように)。
func (s *Server) Serve(conn net.Conn) error {
	var writeMu sync.Mutex
	var wg sync.WaitGroup
	for {
		frame, err := readFrame(conn)
		if err != nil {
			wg.Wait()
			if err == io.EOF {
				return nil
			}
			return err
		}
		call, err := decodeCall(frame)
		if err != nil {
			wg.Wait()
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep := s.dispatch(call)
			writeMu.Lock()
			_ = writeFrame(conn, encodeReply(rep))
			writeMu.Unlock()
		}()
	}
}

func (s *Server) dispatch(call Call) Reply {
	s.mu.RLock()
	h := s.handlers[call.Method]
	s.mu.RUnlock()
	if h == nil {
		return Reply{ID: call.ID, Status: statusUnknownMethod, Body: []byte("unknown method: " + call.Method)}
	}
	body, err := h(context.Background(), call.Body)
	if err != nil {
		return Reply{ID: call.ID, Status: statusHandlerError, Body: []byte(err.Error())}
	}
	return Reply{ID: call.ID, Status: statusOK, Body: body}
}

// --- クライアント ---

// Client は1本の接続に要求を多重化し、応答を ID で呼び出し元へ振り分ける。
type Client struct {
	conn    net.Conn
	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]chan Reply
	err     error
	closed  chan struct{}
}

// NewClient は接続を受け取り、応答を読む goroutine を起動する。
func NewClient(conn net.Conn) *Client {
	c := &Client{
		conn:    conn,
		pending: map[uint64]chan Reply{},
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// readLoop は応答フレームを読み、ID で待っている呼び出し元へ渡し続ける。
func (c *Client) readLoop() {
	for {
		frame, err := readFrame(c.conn)
		if err != nil {
			c.fail(err)
			return
		}
		rep, err := decodeReply(frame)
		if err != nil {
			c.fail(err)
			return
		}
		c.mu.Lock()
		ch := c.pending[rep.ID]
		delete(c.pending, rep.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- rep // buffered(1) なのでブロックしない
		}
	}
}

// fail は接続が壊れたことを全呼び出し元へ知らせる。
func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
		close(c.closed)
	}
	c.mu.Unlock()
}

// Call はメソッドを呼ぶ。応答が返るか、ctx が切れるか、接続が壊れるまで待つ。
func (c *Client) Call(ctx context.Context, method string, body []byte) ([]byte, error) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return nil, c.err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	c.writeMu.Lock()
	err := writeFrame(c.conn, encodeCall(Call{ID: id, Method: method, Body: body}))
	c.writeMu.Unlock()
	if err != nil {
		c.drop(id)
		return nil, err
	}

	select {
	case rep := <-ch:
		if rep.Status != statusOK {
			return nil, &RemoteError{Status: rep.Status, Msg: string(rep.Body)}
		}
		return rep.Body, nil
	case <-ctx.Done():
		// タイムアウト/キャンセル。待つのをやめる(サーバ側は動き続けるかもしれない)。
		c.drop(id)
		return nil, ctx.Err()
	case <-c.closed:
		return nil, c.err
	}
}

func (c *Client) drop(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Close は接続を閉じ、読み取り goroutine を終わらせる。
func (c *Client) Close() error {
	return c.conn.Close()
}
