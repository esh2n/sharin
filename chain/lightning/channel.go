// Package lightning は Lightning Network(決済チャネル)の最小実装。
//
// ブロックチェーンは 1 秒に数件しかさばけない。コーヒー 1 杯の支払いを毎回チェーンに
// 載せるのは遅く高い。Lightning の発想はこうだ——2 者が最初に一度だけ資金をチェーン上の
// 2-of-2 マルチシグにロックし(open)、以降の残高のやり取りは「署名済みの残高割り当て
// (commitment)」を 2 人で交換するだけでオフチェーンに行う。チェーンに触れるのは
// 開くときと閉じるときの 2 回きり。だから瞬時・ほぼ無料で、何千回でも送り合える。
//
// だが「紙の残高表を書き換えるだけ」には落とし穴がある。古い残高表(自分が有利だった頃)
// をチェーンに提出して過去に巻き戻せてしまうと、オフチェーンの意味がない。これを封じるのが
// リボケーション(revocation)とペナルティで、penalty.go で扱う。多段の送金を仲介者を
// 信頼せずに繋ぐ HTLC は htlc.go で扱う。
package lightning

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// ChanState はチャネルの状態。
type ChanState int

const (
	StateOpen               ChanState = iota // 稼働中(オフチェーン更新を受け付ける)
	StateClosedCooperative                   // 双方合意で最新残高どおりに閉じた
	StateClosedUnilateral                    // 片側が最新 commitment を提出して閉じた
	StateDisputed                            // revoked な古い commitment が提出された(係争中)
	StateClosedPenalty                       // 不正提出を相手がペナルティで没収して閉じた
	StateClosedExpiredCheat                  // 誰も咎めず、不正提出が確定してしまった(監視失敗)
)

func (s ChanState) String() string {
	switch s {
	case StateClosedCooperative:
		return "closed-cooperative"
	case StateClosedUnilateral:
		return "closed-unilateral"
	case StateDisputed:
		return "disputed"
	case StateClosedPenalty:
		return "closed-penalty"
	case StateClosedExpiredCheat:
		return "closed-expired-cheat"
	default:
		return "open"
	}
}

var (
	ErrChannelClosed  = errors.New("lightning: チャネルは開いていない")
	ErrUnknownParty   = errors.New("lightning: このチャネルの参加者ではない")
	ErrInsufficient   = errors.New("lightning: 残高が足りない")
	ErrInvalidCommit  = errors.New("lightning: その commitment は提出できない")
	ErrNoDispute      = errors.New("lightning: 係争中の commitment が無い")
	ErrNotVictim      = errors.New("lightning: ペナルティを行使できるのは被害者(相手)だけ")
	ErrBadSecret      = errors.New("lightning: リボケーション秘密が一致しない")
	ErrWindowClosed   = errors.New("lightning: 異議申立て期間が過ぎている")
	ErrHTLCPreimage   = errors.New("lightning: preimage が hash と一致しない")
	ErrHTLCDone       = errors.New("lightning: その HTLC は既に確定している")
	ErrHTLCNotExpired = errors.New("lightning: タイムアウト前なので失効できない")
	ErrNoChannel      = errors.New("lightning: 経路上に存在しないチャネルがある")
)

// #region model

// Commitment は「チャネルの資金を今どう割るか」のスナップショット。
// 更新のたびに番号が 1 つ増える。過去の commitment は revoke され、提出すると罰される。
type Commitment struct {
	Number   uint64
	BalanceA uint64
	BalanceB uint64
	// RevocationSecret は、この commitment が次で置き換えられたとき相手に渡す秘密。
	// これを握った側は、相手がこの(古い)状態を提出したら全額を没収できる。
	RevocationSecret string
}

// Channel は 2 者間の決済チャネル。資金はチェーン上で一度ロックされ、以降の残高更新は
// commitment の差し替えとしてオフチェーンで進む。
type Channel struct {
	A, B     string
	Capacity uint64

	commitments []Commitment    // index == 番号。末尾が最新
	revoked     map[uint64]bool // 置き換え済み(=相手が秘密を握っている)番号
	htlcs       []*HTLC         // 進行中/確定した HTLC(htlc.go)

	state          ChanState
	finalA, finalB uint64 // 閉じたときの最終残高

	clock         int
	disputeWindow int
	dispute       *dispute
}

type dispute struct {
	cheater    string
	commitment Commitment
	deadline   int
}

// #endregion model

// revocationSecret は番号ごとの秘密を決定的に導く(本物は乱数 + ハッシュチェーン)。
func revocationSecret(a, b string, n uint64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("rev:%s:%s:%d", a, b, n)))
	return hex.EncodeToString(h[:])[:12]
}

// Open はチェーン上の資金ロック(funding)を模し、初期残高でチャネルを開く。
// これがチェーンに触れる 1 回目。以降 Pay はチェーンに触れない。
func Open(a, b string, fundA, fundB uint64) *Channel {
	genesis := Commitment{
		Number:           0,
		BalanceA:         fundA,
		BalanceB:         fundB,
		RevocationSecret: revocationSecret(a, b, 0),
	}
	return &Channel{
		A:             a,
		B:             b,
		Capacity:      fundA + fundB,
		commitments:   []Commitment{genesis},
		revoked:       map[uint64]bool{},
		state:         StateOpen,
		disputeWindow: 3,
	}
}

// Current は最新の commitment。
func (c *Channel) Current() Commitment { return c.commitments[len(c.commitments)-1] }

// Commitment は番号 n の(過去の)commitment を返す。古い状態を提出する実験に使う。
func (c *Channel) Commitment(n uint64) (Commitment, bool) {
	if n >= uint64(len(c.commitments)) {
		return Commitment{}, false
	}
	return c.commitments[n], true
}

// Balances は現在のオフチェーン残高。チェーンには載っていない。
func (c *Channel) Balances() (uint64, uint64) {
	cur := c.Current()
	return cur.BalanceA, cur.BalanceB
}

// State は現在のチャネル状態。
func (c *Channel) State() ChanState { return c.state }

// Final は閉じたあとの最終残高。
func (c *Channel) Final() (uint64, uint64) { return c.finalA, c.finalB }

// Now は論理時計。Tick で進める(異議申立て期間・HTLC の失効に使う)。
func (c *Channel) Now() int   { return c.clock }
func (c *Channel) Tick(d int) { c.clock += d }

// other は相手方を返す。
func (c *Channel) other(p string) string {
	if p == c.A {
		return c.B
	}
	return c.A
}

// #region pay

// advance は新しい残高で次の commitment を作り、直前を revoke する。
// Pay も HTLC の確定も、内部的にはこの「番号を 1 つ進める」操作に還元される。
func (c *Channel) advance(na, nb uint64) {
	cur := c.Current()
	c.revoked[cur.Number] = true // 直前の状態は以後 revoked(相手が秘密を握る)
	n := cur.Number + 1
	c.commitments = append(c.commitments, Commitment{
		Number:           n,
		BalanceA:         na,
		BalanceB:         nb,
		RevocationSecret: revocationSecret(c.A, c.B, n),
	})
}

// Pay は from から相手へ amount をオフチェーンで送る。新しい commitment を作るだけで、
// チェーンには一切触れない。これを何千回でも瞬時に繰り返せるのが Lightning の値打ち。
func (c *Channel) Pay(from string, amount uint64) error {
	if c.state != StateOpen {
		return ErrChannelClosed
	}
	if from != c.A && from != c.B {
		return ErrUnknownParty
	}
	na, nb := c.Balances()
	if from == c.A {
		if na < amount {
			return ErrInsufficient
		}
		na, nb = na-amount, nb+amount
	} else {
		if nb < amount {
			return ErrInsufficient
		}
		na, nb = na+amount, nb-amount
	}
	c.advance(na, nb)
	return nil
}

// #endregion pay

// CloseCooperative は双方合意で最新残高どおりに閉じる(チェーンに触れる 2 回目)。
// 争いが無ければこれが普通の閉じ方で、最新の commitment がそのまま決済になる。
func (c *Channel) CloseCooperative() error {
	if c.state != StateOpen {
		return ErrChannelClosed
	}
	a, b := c.Balances()
	c.finalize(a, b, StateClosedCooperative)
	return nil
}

// finalize は最終残高と状態を確定する。
func (c *Channel) finalize(a, b uint64, s ChanState) {
	c.finalA, c.finalB, c.state = a, b, s
}
