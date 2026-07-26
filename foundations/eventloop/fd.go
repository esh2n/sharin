// Package eventloop は、1 本のスレッドで多数の接続をさばく I/O 多重化
// (epoll / select 風)を、syscall・実ソケット・実スレッド不使用で決定的に
// モデル化する。
//
// 素朴なサーバは 1 接続に 1 スレッド(またはプロセス)を割り当て、read が
// データを返すまでそのスレッドを止める(ブロックする)。接続が 1 万本なら
// スレッドも 1 万本——文脈切り替えとメモリで破綻する。event loop はこれを
// ひっくり返す: FD を全てノンブロッキングにし、「どれか準備できたら教えて」
// と OS(epoll)に一括で尋ね、準備できた FD だけを 1 本のスレッドで順に
// 処理する。Node.js / nginx / Redis の心臓部はこの形をしている。
//
// このパッケージは 3 段で構成する:
//   - fd.go     ノンブロッキング FD(read/write は即返る。EAGAIN 相当)
//   - poller.go epoll のモデル(多数の FD の readiness を 1 回で問い合わせる)
//   - loop.go   イベントループ(poll → ready を dispatch を回すリアクタ)
package eventloop

import "errors"

// #region fd

// ErrWouldBlock は、ノンブロッキング FD への読み書きが「今は進められない」
// ことを表す。実機の EAGAIN / EWOULDBLOCK に当たる。スレッドを止める代わりに
// 即座にこれを返し、呼び手は「準備できたら教えて」とイベントループに委ねる。
// このエラーが event loop の全ての出発点——決してブロックしない、という約束だ。
var ErrWouldBlock = errors.New("eventloop: would block")

// Interest は、その FD について通知してほしいイベントの種類。ビットマスク。
// 読み出せるデータを待つなら Readable、書き込む余地を待つなら Writable。
type Interest uint8

const (
	Readable Interest = 1 << iota // 読み出せるデータが届いている
	Writable                      // 送信バッファに書き込む余地がある
)

// Has は f を含むかを返す。
func (i Interest) Has(f Interest) bool { return i&f != 0 }

// String は "r" / "w" / "rw" / "-" で表す。
func (i Interest) String() string {
	s := ""
	if i.Has(Readable) {
		s += "r"
	}
	if i.Has(Writable) {
		s += "w"
	}
	if s == "" {
		return "-"
	}
	return s
}

// FD は 1 本の接続(ソケット)を表す疑似ファイルディスクリプタ。in は
// カーネル受信バッファ(到着済み・未読)、out は送信バッファの空き容量。
// 実メモリ・実ソケット無しで、ノンブロッキング I/O の意味論だけを取り出す。
type FD struct {
	id     int
	name   string
	in     []byte // 受信済みで未読のバイト列
	out    int    // 送信バッファの空き(あと何バイト書けるか)
	sent   []byte // これまで書き出したバイト列(観察用)
	closed bool
}

// ID は FD 番号を返す。
func (f *FD) ID() int { return f.id }

// Name は接続名を返す。
func (f *FD) Name() string { return f.name }

// Sent はこの FD にこれまで書き出した全バイトの写しを返す。
func (f *FD) Sent() []byte { return append([]byte(nil), f.sent...) }

// Buffered は受信バッファに残っている未読バイト数を返す(観察用)。
func (f *FD) Buffered() int { return len(f.in) }

// Read はノンブロッキング読み出し。到着済みバイトを最大 max だけ返す。
// 受信バッファが空なら、スレッドを止めず即 ErrWouldBlock を返す——ここが肝。
func (f *FD) Read(max int) ([]byte, error) {
	if f.closed {
		return nil, errors.New("eventloop: read on closed fd")
	}
	if len(f.in) == 0 {
		return nil, ErrWouldBlock // 実機の read() が EAGAIN を返す状況
	}
	n := max
	if n > len(f.in) {
		n = len(f.in)
	}
	b := append([]byte(nil), f.in[:n]...)
	f.in = f.in[n:]
	return b, nil
}

// Write はノンブロッキング書き込み。送信バッファの空き out までしか書けず、
// 空きが無ければ ErrWouldBlock。書けた分だけ返す(部分書き込みがありうる)。
// 「書ききれなかった残り」をどう扱うかが、後述のバックプレッシャの話になる。
func (f *FD) Write(data []byte) (int, error) {
	if f.closed {
		return 0, errors.New("eventloop: write on closed fd")
	}
	if f.out == 0 {
		return 0, ErrWouldBlock // 送信バッファが満杯。相手が受け取るまで書けない
	}
	n := len(data)
	if n > f.out {
		n = f.out
	}
	f.out -= n
	f.sent = append(f.sent, data[:n]...)
	return n, nil
}

// ready は、いま発火している Interest を返す。受信バッファに未読があれば
// Readable、送信バッファに空きがあれば Writable。条件が続く限り毎回報告される
// = level-triggered(epoll の既定)。
func (f *FD) ready() Interest {
	var r Interest
	if len(f.in) > 0 {
		r |= Readable
	}
	if f.out > 0 {
		r |= Writable
	}
	return r
}

// deliver は「ネットワークからバイトが到着した」を表す(受信バッファに積む)。
// 外の世界(loop.Deliver)から呼ぶ。
func (f *FD) deliver(b []byte) { f.in = append(f.in, b...) }

// drain は「送信バッファに空きが n だけ戻った」を表す(相手が受信した)。
func (f *FD) drain(n int) { f.out += n }

// #endregion fd
