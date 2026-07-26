// Package pubsub はトピック型 Pub/Sub の最小実装。
//
// [queue](../queue/) が「1メッセージを1人が取る」点対点だったのに対し、Pub/Sub は
// 「1メッセージを購読者**全員**が受け取る」ファンアウト(one-to-many)。トピックごとに
// 追記ログを持ち、購読者はそれぞれ自分のカーソルで読む。だから:
//
//   - ファンアウト: 発行は1回。購読者はそれぞれ独立に全メッセージを読む。
//   - 独立カーソル: 遅い購読者がいても、他の購読者は止まらない(各自の位置で進む)。
//   - 購読開始位置: FromBeginning なら過去ぶんも再生、FromNow なら購読以降だけ。
//
// goroutine やネットワークは使わず、明示的な操作で駆動する純粋なモデル。
package pubsub

// Message はトピック上の1件。
type Message struct {
	Offset int
	Body   string
}

// Broker は複数トピックを持つ。トピックはそれぞれ追記専用ログ。
type Broker struct {
	topics map[string][]Message
}

// NewBroker は空のブローカを作る。
func NewBroker() *Broker {
	return &Broker{topics: map[string][]Message{}}
}

// Publish はトピックにメッセージを1件追記し、その offset を返す。
// トピックが無ければ作る。購読者全員がこの1件を(各自のペースで)受け取る。
func (b *Broker) Publish(topic, body string) int {
	log := b.topics[topic]
	off := len(log)
	b.topics[topic] = append(log, Message{Offset: off, Body: body})
	return off
}

// Len はトピックに積まれた総件数(未知トピックは 0)。
func (b *Broker) Len(topic string) int { return len(b.topics[topic]) }

// Topics は存在するトピック名の数(観測用)。
func (b *Broker) NumTopics() int { return len(b.topics) }

// StartAt は購読を始める位置。
type StartAt int

const (
	// FromBeginning は先頭から。過去のメッセージも再生される(durable topic)。
	FromBeginning StartAt = iota
	// FromNow は購読時点の末尾から。以降の新しいメッセージだけ届く(ephemeral)。
	FromNow
)

// #region subscribe
// Subscription は1つのトピックを、自分のカーソルで読む購読。
type Subscription struct {
	broker *Broker
	topic  string
	cursor int // 次に読む offset
}

// Subscribe はトピックの購読を作る。FromNow なら現在の末尾から始める。
func (b *Broker) Subscribe(topic string, start StartAt) *Subscription {
	cursor := 0
	if start == FromNow {
		cursor = len(b.topics[topic]) // 今ある分は飛ばす
	}
	return &Subscription{broker: b, topic: topic, cursor: cursor}
}

// #endregion subscribe

// #region poll
// Poll は未読を最大 max 件返し、カーソルを進める。購読者ごとに独立なので、
// ある購読者が読まなくても(遅くても)、他の購読者には影響しない。
func (s *Subscription) Poll(max int) []Message {
	log := s.broker.topics[s.topic]
	end := s.cursor + max
	if end > len(log) {
		end = len(log)
	}
	if s.cursor >= end {
		return nil
	}
	out := make([]Message, end-s.cursor)
	copy(out, log[s.cursor:end])
	s.cursor = end
	return out
}

// #endregion poll

// Backlog はこの購読の未読件数(遅れ具合)。
func (s *Subscription) Backlog() int {
	return len(s.broker.topics[s.topic]) - s.cursor
}

// Cursor は次に読む offset。
func (s *Subscription) Cursor() int { return s.cursor }
