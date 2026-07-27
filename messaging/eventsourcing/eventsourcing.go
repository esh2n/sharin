// Package eventsourcing はイベントソーシングを最小構成で実装する。
//
// 普通のアプリは「現在の状態」を保存する。口座なら残高の数字を上書きしていく。
// だがこれは過去を捨てている。いつ・なぜその残高になったのかは残らない。
// イベントソーシングは発想を逆にする。保存するのは、起きた出来事(イベント)の
// 連なりだ。「1000 入金」「300 出金」…という追記専用の並びが唯一の真実で、
// 現在の残高は、その並びを頭から畳み込めば(リプレイすれば)いつでも導ける。
// 履歴が丸ごと残るので、監査も、過去のある時点の状態への遡りもできる。
// イベントが増えると毎回全部を畳むのは重いので、途中の状態をスナップショットに
// 取り、それ以降のイベントだけを畳んで速くする。
package eventsourcing

import "errors"

// #region event

// EventType は出来事の種類。
type EventType int

const (
	Deposited EventType = iota // 入金
	Withdrawn                  // 出金
)

func (t EventType) String() string {
	if t == Deposited {
		return "Deposited"
	}
	return "Withdrawn"
}

// Event は起きた 1 つの出来事。追記されたら二度と書き換えない(不変)。
type Event struct {
	Type    EventType
	Amount  int
	Version int // 何番目の出来事か(1 始まり)
}

// Store はイベントの追記専用ログ。上書き・削除はしない。
type Store struct{ events []Event }

// Append はイベントを末尾に追記し、通し番号(Version)を振る。
func (s *Store) Append(t EventType, amount int) Event {
	e := Event{Type: t, Amount: amount, Version: len(s.events) + 1}
	s.events = append(s.events, e)
	return e
}

// Events は全イベントを返す(監査ログそのもの)。
func (s *Store) Events() []Event { return s.events }

// EventsAfter は version より後のイベントだけを返す(スナップショット併用)。
func (s *Store) EventsAfter(version int) []Event {
	var out []Event
	for _, e := range s.events {
		if e.Version > version {
			out = append(out, e)
		}
	}
	return out
}

// #endregion event

// #region replay

// Account は口座の状態。これは保存されるものではなく、イベントから導かれる射影。
type Account struct {
	Balance int
	Version int // どのイベントまで反映したか
}

// Apply は 1 つのイベントを状態に適用し、新しい状態を返す(不変。元は変えない)。
func (a Account) Apply(e Event) Account {
	switch e.Type {
	case Deposited:
		a.Balance += e.Amount
	case Withdrawn:
		a.Balance -= e.Amount
	}
	a.Version = e.Version
	return a
}

// Replay はイベント列を頭から畳み込んで現在状態を作る。
// これがイベントソーシングの核心。状態は保存せず、常にイベントから導く。
func Replay(events []Event) Account {
	var a Account
	for _, e := range events {
		a = a.Apply(e)
	}
	return a
}

// StateAt は version の時点までを畳んだ、過去のある時点の状態を返す(タイムトラベル)。
func StateAt(events []Event, version int) Account {
	var a Account
	for _, e := range events {
		if e.Version > version {
			break
		}
		a = a.Apply(e)
	}
	return a
}

// #endregion replay

// #region command

// ErrInsufficient は残高不足で出金できないとき。
var ErrInsufficient = errors.New("eventsourcing: insufficient balance")

// ErrInvalidAmount は金額が正でないとき。
var ErrInvalidAmount = errors.New("eventsourcing: amount must be positive")

// Deposit は入金コマンドを検証し、正しければイベントを Store に追記する。
// コマンド(意図)は検証を通ってはじめてイベント(事実)になる。
func Deposit(store *Store, amount int) (Event, error) {
	if amount <= 0 {
		return Event{}, ErrInvalidAmount
	}
	return store.Append(Deposited, amount), nil
}

// Withdraw は出金コマンドを検証する。現在の残高(イベントを畳んで導く)を超える
// 出金は拒否し、イベントを残さない。不正な意図はイベントにしない。
func Withdraw(store *Store, amount int) (Event, error) {
	if amount <= 0 {
		return Event{}, ErrInvalidAmount
	}
	current := Replay(store.Events())
	if amount > current.Balance {
		return Event{}, ErrInsufficient
	}
	return store.Append(Withdrawn, amount), nil
}

// #endregion command

// #region snapshot

// Snapshot はある時点の状態の写し。これ以降のイベントだけ畳めば現在に追いつける。
type Snapshot struct {
	Balance int
	Version int
}

// TakeSnapshot は現在状態からスナップショットを作る。
func TakeSnapshot(a Account) Snapshot {
	return Snapshot{Balance: a.Balance, Version: a.Version}
}

// RestoreFrom はスナップショットに、それ以降のイベントだけを畳んで現在状態を復元する。
// 全イベントを畳み直す必要がなく、長い履歴でも速い。
func RestoreFrom(snap Snapshot, laterEvents []Event) Account {
	a := Account{Balance: snap.Balance, Version: snap.Version}
	for _, e := range laterEvents {
		if e.Version <= snap.Version {
			continue // スナップショットに既に含まれる
		}
		a = a.Apply(e)
	}
	return a
}

// #endregion snapshot
