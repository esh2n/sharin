// Package queue はログ型メッセージキューの最小実装。
//
// Kafka のように、メッセージは追記専用の**ログ**に積まれ、消費者は自分の**オフセット**
// (どこまで読んだか)を持って順番に読む。ブローカはメッセージを消さない——消費者が
// オフセットを進めるだけ。
//
// この実装で目に見えるようにするのは配送保証(delivery semantics):
//
//   - at-most-once: 先にオフセットを確定してから処理する。処理前に落ちるとメッセージは
//     再配送されず**失われる**(重複はしないが取りこぼす)。
//   - at-least-once: 先に処理してからオフセットを確定する。確定前に落ちると次回**再配送**
//     され、同じメッセージを2回処理しうる(取りこぼさないが**重複**する)。
//   - 実質1回(effectively-once): at-least-once + **冪等な消費者**。メッセージのキーで
//     重複を捨てれば、再配送されても副作用は1回きりになる。
//
// 分散環境では「取りこぼさない」と「重複しない」を同時に完全達成するのは難しい。実務は
// たいてい at-least-once + 冪等で「実質1回」に寄せる。goroutine やネットワークは使わず、
// 明示的な操作で駆動する純粋なモデルなので、クラッシュ(確定前の中断)も決定的に再現できる。
package queue

import "errors"

// Message はログ上の1件。Key は冪等キー(同じ Key の再処理は重複と見なす)。
type Message struct {
	Offset int
	Key    string
	Body   string
}

// Broker は追記専用のログを持つ。メッセージは消えない。
type Broker struct {
	log []Message
}

// NewBroker は空のブローカを作る。
func NewBroker() *Broker { return &Broker{} }

// Publish はメッセージを1件追記し、その offset(0始まり)を返す。
func (b *Broker) Publish(key, body string) int {
	off := len(b.log)
	b.log = append(b.log, Message{Offset: off, Key: key, Body: body})
	return off
}

// Len はログに積まれた総件数。
func (b *Broker) Len() int { return len(b.log) }

// fetch は committed 以降のメッセージを最大 max 件返す。
func (b *Broker) fetch(committed, max int) []Message {
	if committed < 0 {
		committed = 0
	}
	end := committed + max
	if end > len(b.log) {
		end = len(b.log)
	}
	if committed >= end {
		return nil
	}
	out := make([]Message, end-committed)
	copy(out, b.log[committed:end])
	return out
}

// Semantics は配送保証。
type Semantics int

const (
	// AtMostOnce は「確定 → 処理」。処理前クラッシュで取りこぼす(重複なし)。
	AtMostOnce Semantics = iota
	// AtLeastOnce は「処理 → 確定」。確定前クラッシュで再配送・重複しうる(取りこぼしなし)。
	AtLeastOnce
)

func (s Semantics) String() string {
	switch s {
	case AtMostOnce:
		return "at-most-once"
	case AtLeastOnce:
		return "at-least-once"
	default:
		return "unknown"
	}
}

// Consumer は1つのブローカを、自分のオフセットで読む消費者。
// handle は各メッセージの処理(副作用)。処理が成功したら true を返す想定。
type Consumer struct {
	broker    *Broker
	committed int // 次に読む offset(ここまで確定済み)
	semantics Semantics
	handle    func(Message)
}

// NewConsumer は消費者を作る。handle は nil 不可。
func NewConsumer(b *Broker, s Semantics, handle func(Message)) (*Consumer, error) {
	if b == nil {
		return nil, errors.New("queue: broker is nil")
	}
	if handle == nil {
		return nil, errors.New("queue: handle is nil")
	}
	return &Consumer{broker: b, semantics: s, handle: handle}, nil
}

// Committed は確定済みオフセット(次に読む位置)。
func (c *Consumer) Committed() int { return c.committed }

// #region poll
// Poll は未読を最大 max 件、配送保証に従って処理する。処理した件数を返す。
//
//	AtMostOnce : オフセットを先に進めてから処理する(処理前に落ちると取りこぼす)
//	AtLeastOnce: 処理してからオフセットを進める(確定前に落ちると再配送される)
func (c *Consumer) Poll(max int) int {
	batch := c.broker.fetch(c.committed, max)
	if c.semantics == AtMostOnce {
		c.committed += len(batch) // 先に確定
		for _, m := range batch {
			c.handle(m) // ここで落ちると、この batch は再配送されない=消失
		}
		return len(batch)
	}
	// AtLeastOnce
	for _, m := range batch {
		c.handle(m) // 先に処理
	}
	c.committed += len(batch) // 後で確定。ここに来る前に落ちると再配送=重複
	return len(batch)
}

// #endregion poll

// PollCrash は「max 件を処理したが、オフセットを確定する前にクラッシュした」状況を模す。
// AtLeastOnce では処理だけ済んで確定が飛ぶので、次の Poll でこの batch が再配送される。
// AtMostOnce では確定は処理より先なので、処理された batch は確定済み(=取りこぼしは
// 「処理の前」に落ちたときに起きる。こちらは CrashBeforeHandle で模す)。
func (c *Consumer) PollCrash(max int) int {
	batch := c.broker.fetch(c.committed, max)
	if c.semantics == AtMostOnce {
		// 確定は済ませるが処理の途中で落ちる → この batch は再配送されず消失。
		c.committed += len(batch)
		return 0 // 1件も処理し切れなかった
	}
	// AtLeastOnce: 処理はするがオフセット確定をしないまま落ちる。
	for _, m := range batch {
		c.handle(m)
	}
	return len(batch) // 処理はしたが未確定 → 次の Poll で再配送される
}
