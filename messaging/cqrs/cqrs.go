// Package cqrs は CQRS(コマンドとクエリの責務分離)を最小構成で実装する。
//
// 普通のアプリは 1 つのモデルで読み書きの両方をこなす。だが読みと書きは要求が
// 違う。書きは正しさ(検証・整合性)が命で、読みは速さと使いやすい形が命だ。
// 1 つのモデルで両方を最適化しようとすると、どちらも中途半端になる。CQRS は
// これを分ける。書き込み側(コマンド)は状態を検証して変更し、その結果を
// イベントとして流す。読み取り側(クエリ)は、そのイベントを畳んで、用途ごとに
// 都合のよい形の読みモデル(射影)を作る。1 つのイベント列から、注文状況の一覧、
// 顧客ごとの購入額、日次売上といった別々のビューを、それぞれ独立に組み立てられる。
// 代償は結果整合。読みモデルは書きに少し遅れて追いつくので、一瞬古い値を返す。
package cqrs

import "errors"

// #region events

// EventKind は注文に起きた出来事の種類。
type EventKind int

const (
	Placed    EventKind = iota // 注文された
	Paid                       // 支払われた
	Cancelled                  // 取り消された
)

// Event は書き込み側が流し、読み取り側が畳む出来事。
type Event struct {
	Kind     EventKind
	OrderID  string
	Customer string
	Amount   int
	Version  int
}

// Store はイベントの追記専用ログ(書き込み側と読み取り側をつなぐ)。
type Store struct{ events []Event }

func (s *Store) append(e Event) Event {
	e.Version = len(s.events) + 1
	s.events = append(s.events, e)
	return e
}

// Events は全イベントを返す。
func (s *Store) Events() []Event { return s.events }

// #endregion events

// #region write

var (
	// ErrExists は既に存在する注文を再度作ろうとしたとき。
	ErrExists = errors.New("cqrs: order already exists")
	// ErrNotFound は無い注文を操作しようとしたとき。
	ErrNotFound = errors.New("cqrs: order not found")
	// ErrNotPayable は支払えない状態(既に支払済み/取消済み)。
	ErrNotPayable = errors.New("cqrs: order not payable")
	// ErrPaidCannotCancel は支払済みは取り消せない。
	ErrPaidCannotCancel = errors.New("cqrs: paid order cannot be cancelled")
)

// WriteSide はコマンドを受け、検証して、イベントを流す(書き込みモデル)。
// 状態はイベントから毎回導く(検証のためだけに使う最小の再構成)。
type WriteSide struct{ store *Store }

// NewWriteSide は書き込み側を作る。
func NewWriteSide(store *Store) *WriteSide { return &WriteSide{store: store} }

// statusOf は id の現在状態をイベントから導く(空文字なら未存在)。
func (w *WriteSide) statusOf(id string) string {
	st := ""
	for _, e := range w.store.events {
		if e.OrderID != id {
			continue
		}
		switch e.Kind {
		case Placed:
			st = "placed"
		case Paid:
			st = "paid"
		case Cancelled:
			st = "cancelled"
		}
	}
	return st
}

// Place は注文を作る。既存 ID は拒否。
func (w *WriteSide) Place(id, customer string, amount int) error {
	if w.statusOf(id) != "" {
		return ErrExists
	}
	w.store.append(Event{Kind: Placed, OrderID: id, Customer: customer, Amount: amount})
	return nil
}

// Pay は注文を支払う。placed の注文だけが支払える。
func (w *WriteSide) Pay(id string) error {
	switch w.statusOf(id) {
	case "":
		return ErrNotFound
	case "placed":
		w.store.append(Event{Kind: Paid, OrderID: id})
		return nil
	default:
		return ErrNotPayable
	}
}

// Cancel は注文を取り消す。支払済みは取り消せない。
func (w *WriteSide) Cancel(id string) error {
	switch w.statusOf(id) {
	case "":
		return ErrNotFound
	case "paid":
		return ErrPaidCannotCancel
	default:
		w.store.append(Event{Kind: Cancelled, OrderID: id})
		return nil
	}
}

// #endregion write

// #region read

// ReadSide は同じイベント列から複数の読みモデル(射影)を組み立てる。
// cursor までを処理済みとし、CatchUp で新しいイベントに追いつく。
type ReadSide struct {
	store  *Store
	cursor int // どのバージョンまで処理したか

	// 用途別の読みモデル。それぞれ同じイベントを別の形に畳む。
	Status  map[string]string // 注文ID → 状態(注文状況の一覧)
	Spend   map[string]int    // 顧客 → 支払済み合計(顧客ごとの購入額)
	Revenue int               // 総売上(支払済みの合計)
	orders  map[string]order  // 射影が必要とする注文明細(内部)
}

type order struct {
	customer string
	amount   int
}

// NewReadSide は空の読みモデルで読み取り側を作る。
func NewReadSide(store *Store) *ReadSide {
	return &ReadSide{
		store:  store,
		Status: make(map[string]string),
		Spend:  make(map[string]int),
		orders: make(map[string]order),
	}
}

// CatchUp は未処理のイベントを畳んで読みモデルを最新にする。
// これを呼ぶまで読みモデルは書き込みに遅れる(結果整合)。
func (r *ReadSide) CatchUp() {
	for _, e := range r.store.events {
		if e.Version <= r.cursor {
			continue
		}
		r.apply(e)
		r.cursor = e.Version
	}
}

// Lag は読みモデルが書き込みにどれだけ遅れているか(未処理イベント数)。
func (r *ReadSide) Lag() int { return len(r.store.events) - r.cursor }

func (r *ReadSide) apply(e Event) {
	switch e.Kind {
	case Placed:
		r.Status[e.OrderID] = "placed"
		r.orders[e.OrderID] = order{customer: e.Customer, amount: e.Amount}
	case Paid:
		r.Status[e.OrderID] = "paid"
		o := r.orders[e.OrderID]
		r.Spend[o.customer] += o.amount // 顧客ごとの購入額に反映
		r.Revenue += o.amount           // 総売上に反映
	case Cancelled:
		r.Status[e.OrderID] = "cancelled"
	}
}

// #endregion read
