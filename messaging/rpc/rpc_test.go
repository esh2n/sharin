package rpc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// newPair は in-memory 接続でサーバとクライアントを繋ぐ。
func newPair(t *testing.T, register func(*Server)) *Client {
	t.Helper()
	c1, c2 := net.Pipe()
	srv := NewServer()
	register(srv)
	go srv.Serve(c2)
	client := NewClient(c1)
	t.Cleanup(func() { client.Close() })
	return client
}

func TestCallEcho(t *testing.T) {
	client := newPair(t, func(s *Server) {
		s.Register("echo", func(_ context.Context, b []byte) ([]byte, error) { return b, nil })
	})
	got, err := client.Call(context.Background(), "echo", []byte("ping"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(got) != "ping" {
		t.Fatalf("echo = %q, want ping", got)
	}
}

func TestCallComputes(t *testing.T) {
	client := newPair(t, func(s *Server) {
		s.Register("add", func(_ context.Context, b []byte) ([]byte, error) {
			x, n := binary.Uvarint(b)
			y, _ := binary.Uvarint(b[n:])
			return binary.AppendUvarint(nil, x+y), nil
		})
	})
	body := binary.AppendUvarint(nil, 20)
	body = binary.AppendUvarint(body, 22)
	got, err := client.Call(context.Background(), "add", body)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if sum, _ := binary.Uvarint(got); sum != 42 {
		t.Fatalf("add = %d, want 42", sum)
	}
}

func TestRemoteError(t *testing.T) {
	client := newPair(t, func(s *Server) {
		s.Register("boom", func(_ context.Context, _ []byte) ([]byte, error) {
			return nil, errors.New("kaboom")
		})
	})
	_, err := client.Call(context.Background(), "boom", nil)
	var re *RemoteError
	if !errors.As(err, &re) {
		t.Fatalf("expected RemoteError, got %v", err)
	}
	if re.Status != statusHandlerError || re.Msg != "kaboom" {
		t.Fatalf("RemoteError = %+v", re)
	}
}

func TestUnknownMethod(t *testing.T) {
	client := newPair(t, func(s *Server) {})
	_, err := client.Call(context.Background(), "nope", nil)
	var re *RemoteError
	if !errors.As(err, &re) || re.Status != statusUnknownMethod {
		t.Fatalf("expected unknown-method RemoteError, got %v", err)
	}
}

// 1本の接続に多重化した並行呼び出しが、それぞれ正しい応答に相関することを確かめる。
func TestConcurrentCallsCorrelate(t *testing.T) {
	client := newPair(t, func(s *Server) {
		s.Register("echo", func(_ context.Context, b []byte) ([]byte, error) { return b, nil })
	})
	const N = 50
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprintf("req-%d", i)
			got, err := client.Call(context.Background(), "echo", []byte(want))
			if err != nil {
				errs[i] = err
				return
			}
			if string(got) != want {
				errs[i] = fmt.Errorf("got %q, want %q", got, want)
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
}

// RPC はローカル呼び出しと違い、応答が返らないことがある。ctx のキャンセルで待つのをやめる。
func TestContextCancelStopsWaiting(t *testing.T) {
	block := make(chan struct{})
	client := newPair(t, func(s *Server) {
		s.Register("block", func(_ context.Context, _ []byte) ([]byte, error) {
			<-block // 応答を返さずに詰まる
			return nil, nil
		})
	})
	defer close(block)

	ctx, cancel := context.WithCancel(context.Background())
	type res struct {
		err error
	}
	rc := make(chan res, 1)
	go func() {
		_, err := client.Call(ctx, "block", nil)
		rc <- res{err}
	}()
	time.Sleep(10 * time.Millisecond) // 呼び出しを in-flight にする
	cancel()

	select {
	case r := <-rc:
		if !errors.Is(r.err, context.Canceled) {
			t.Fatalf("Call err = %v, want context.Canceled", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Call did not return after cancel")
	}
}

func TestCallOnClosedClient(t *testing.T) {
	client := newPair(t, func(s *Server) {})
	client.Close()
	time.Sleep(10 * time.Millisecond) // readLoop がエラーを検知するのを待つ
	if _, err := client.Call(context.Background(), "echo", nil); err == nil {
		t.Fatal("Call on closed client should error")
	}
}
