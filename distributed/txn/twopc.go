// Package txn は複数サービスにまたがる更新を「全部成功か、全部無かったことにするか」で
// 揃える 2 つの方法——2 相コミット(2PC)と Saga——をモデル化する。
//
// 1 台の DB なら MVCC や WAL がトランザクションの原子性を守る。だが「在庫サービスの引き当て」
// と「決済サービスの課金」のように、別々のシステムにまたがる更新には、単一の commit が無い。
// 片方だけ成功した中途半端な状態をどう避けるかが主題になる。
//
//   - 2PC(twopc.go): 調整役が全員に「準備できるか」を聞き(prepare)、全員 Yes のときだけ
//     commit を送る。1 人でも No なら全員 abort。強い原子性が得られるが、参加者は prepare で
//     資源をロックしたまま調整役の決定を待つ——調整役が落ちるとロックを抱えて動けない
//     (ブロッキング)。
//   - Saga(saga.go): ロックを持たない。各ステップをローカルに即コミットし、途中で失敗したら
//     完了済みステップの「補償(打ち消し)」を逆順に実行する。止まらないが、途中状態が外から
//     見える(原子性は諦め、結果整合で辻褄を合わせる)。
package txn

import (
	"errors"
	"fmt"
)

// ErrAborted は 2PC が abort で終わったことを表す。
var ErrAborted = errors.New("txn: トランザクションは中止された")

// #region participant

// Vote は prepare への返答。
type Vote int

const (
	VoteYes Vote = iota // 準備完了。commit が来たら必ず遂行できる状態で待つ
	VoteNo              // 遂行できない(残高不足など)。全体を中止させる
)

// PState は参加者から見たトランザクションの状態。
type PState int

const (
	PIdle     PState = iota
	PPrepared        // Yes と答えた。資源をロックして決定を待っている
	PCommitted
	PAborted
)

func (s PState) String() string {
	switch s {
	case PPrepared:
		return "prepared"
	case PCommitted:
		return "committed"
	case PAborted:
		return "aborted"
	default:
		return "idle"
	}
}

// Participant は 2PC の参加者。1 つの口座(残高)を持つサービスを模す。
// Prepare で引き落とし可能かを検査し、可能なら金額をロックして Yes と答える。
// 以降 Commit / Abort が来るまで、そのロックは解けない。
type Participant struct {
	Name    string
	balance int64
	locked  int64
	state   PState
	crashed bool
}

// NewParticipant は初期残高つきの参加者を作る。
func NewParticipant(name string, balance int64) *Participant {
	return &Participant{Name: name, balance: balance}
}

// Balance は確定済み残高。Locked は prepare でロック中の額。State は現在の状態。
func (p *Participant) Balance() int64 { return p.balance }
func (p *Participant) Locked() int64  { return p.locked }
func (p *Participant) State() PState  { return p.state }

// Crash は参加者の停止を模す(prepare に答えられなくなる)。
func (p *Participant) Crash()   { p.crashed = true }
func (p *Participant) Recover() { p.crashed = false }

// Prepare は「amount を引き落とす準備ができるか」に答える。できるなら金額をロックして
// Yes。ロックした資源は、調整役の決定(Commit/Abort)が届くまで他の用途に使えない。
// ここが 2PC の約束の重さで、Yes と答えた参加者は「commit が来たら必ず遂行できる」
// 状態を維持し続ける義務を負う。
func (p *Participant) Prepare(amount int64) Vote {
	if p.crashed || p.balance < amount {
		return VoteNo
	}
	p.balance -= amount
	p.locked += amount
	p.state = PPrepared
	return VoteYes
}

// Commit はロック分を確定する(引き落とし完了)。
func (p *Participant) Commit() {
	p.locked = 0
	p.state = PCommitted
}

// Abort はロック分を残高へ戻す。prepare していなければ何もしない。
func (p *Participant) Abort() {
	p.balance += p.locked
	p.locked = 0
	p.state = PAborted
}

// #endregion participant

// #region coordinator

// Decision は調整役の最終決定。
type Decision int

const (
	DecisionNone Decision = iota // まだ決めていない(または決定を配る前に落ちた)
	DecisionCommit
	DecisionAbort
)

func (d Decision) String() string {
	switch d {
	case DecisionCommit:
		return "commit"
	case DecisionAbort:
		return "abort"
	default:
		return "none"
	}
}

// Coordinator は 2PC の調整役。
type Coordinator struct {
	participants []*Participant
	decision     Decision
}

// NewCoordinator は参加者を束ねる調整役を作る。
func NewCoordinator(ps ...*Participant) *Coordinator {
	return &Coordinator{participants: ps}
}

// Decision は現在の決定。
func (c *Coordinator) Decision() Decision { return c.decision }

// Run は 2PC を最後まで実行する。
//
//	phase 1(prepare): 全参加者に「amount を引けるか」を聞く。1 人でも No なら決定は abort。
//	phase 2(decide) : 決定を全員に配る。commit なら全員確定、abort なら Yes と答えて
//	                  ロックしていた参加者も全員巻き戻す。
//
// 全員 Yes のときだけ commit になるので、「一部だけ引き落とされた」状態は決して確定しない。
func (c *Coordinator) Run(amount int64) (Decision, error) {
	votes := make([]Vote, len(c.participants))
	c.decision = DecisionCommit
	for i, p := range c.participants {
		votes[i] = p.Prepare(amount)
		if votes[i] == VoteNo {
			c.decision = DecisionAbort
		}
	}
	for i, p := range c.participants {
		if c.decision == DecisionCommit {
			p.Commit()
		} else if votes[i] == VoteYes {
			p.Abort() // Yes と答えてロックしていた参加者を解放する
		}
	}
	if c.decision == DecisionAbort {
		return c.decision, ErrAborted
	}
	return c.decision, nil
}

// RunPrepareOnly は phase 1 だけ実行して止まる——「決定を配る前に調整役が落ちた」を模す。
// Yes と答えた参加者は prepared のままロックを抱え、自分では commit も abort も決められない。
// これが 2PC のブロッキング問題で、調整役が復旧するまで資源は塞がったままになる。
func (c *Coordinator) RunPrepareOnly(amount int64) []Vote {
	votes := make([]Vote, len(c.participants))
	for i, p := range c.participants {
		votes[i] = p.Prepare(amount)
	}
	c.decision = DecisionNone
	return votes
}

// Blocked は決定が届かないまま prepared でロックを抱えている参加者の一覧。
func (c *Coordinator) Blocked() []string {
	var out []string
	for _, p := range c.participants {
		if p.State() == PPrepared {
			out = append(out, fmt.Sprintf("%s(locked=%d)", p.Name, p.Locked()))
		}
	}
	return out
}

// #endregion coordinator
