package lightning

// penalty.go は Lightning の安全性の核——「古い残高表を出して過去に巻き戻す」不正を、
// リボケーションとペナルティで割に合わなくする仕組み。
//
// オフチェーンの commitment は署名済みなので、どちらの当事者も「自分が持っている
// commitment」をいつでもチェーンに提出して一方的に閉じられる(unilateral close)。
// 問題は、自分が有利だった過去の commitment を出せてしまうこと。例えば A が B に
// 支払い続けて残高が減ったあと、A が「まだ払う前」の古い状態を提出すれば、払ったはずの
// 金が戻ってしまう。
//
// 封じ方はこうだ。状態を 1 つ進めるたびに、双方は直前の commitment のリボケーション秘密を
// 相手に渡す。つまり「古い状態を出したら、その秘密で全額没収してよい」という許諾を
// 相手に与える。だから古い状態の提出は、成功すれば元本、失敗(=相手に気づかれる)すれば
// 全額没収という一方的に不利な賭けになる。正直でいるのが最も得、という均衡が生まれる。

// #region penalty

// Broadcast は by が commitment comm をチェーンに提出して一方的に閉じようとする操作。
//
//   - 最新の commitment なら、正当な unilateral close(最新残高で確定)。
//   - revoked な古い commitment なら、不正の疑い。即確定はせず係争(Disputed)に入り、
//     相手が異議申立て期間内に Penalize できる。誰も咎めなければ、期間経過で不正が通る。
func (c *Channel) Broadcast(by string, comm Commitment) error {
	if c.state != StateOpen {
		return ErrChannelClosed
	}
	if by != c.A && by != c.B {
		return ErrUnknownParty
	}
	cur := c.Current()

	if comm.Number == cur.Number {
		// 最新状態の提出。正当な一方的クローズ。
		c.finalize(cur.BalanceA, cur.BalanceB, StateClosedUnilateral)
		return nil
	}
	if comm.Number < cur.Number && c.revoked[comm.Number] {
		// 古い(revoked)状態の提出 = 過去への巻き戻しを狙った不正。
		// 相手が咎める猶予(異議申立て期間)を置く。
		c.dispute = &dispute{
			cheater:    by,
			commitment: comm,
			deadline:   c.clock + c.disputeWindow,
		}
		c.state = StateDisputed
		return nil
	}
	return ErrInvalidCommit
}

// Penalize は係争中に、被害者(不正提出者の相手)がリボケーション秘密を示して全額を没収する。
//
// 検査は 2 つ。(1) 行使者が被害者本人か。(2) 示した秘密が、提出された古い commitment の
// リボケーション秘密と一致するか。両方満たし、かつ期間内なら、チャネルの全資金が被害者に渡る。
// これが「不正の期待値をマイナスにする」罰で、Lightning がオフチェーン更新を安全にする要。
func (c *Channel) Penalize(by, secret string) error {
	if c.state != StateDisputed {
		return ErrNoDispute
	}
	d := c.dispute
	victim := c.other(d.cheater)
	if by != victim {
		return ErrNotVictim
	}
	if c.clock > d.deadline {
		return ErrWindowClosed
	}
	if secret != d.commitment.RevocationSecret {
		return ErrBadSecret
	}
	// 被害者が全額を没収する。
	if victim == c.A {
		c.finalize(c.Capacity, 0, StateClosedPenalty)
	} else {
		c.finalize(0, c.Capacity, StateClosedPenalty)
	}
	return nil
}

// FinalizeDispute は異議申立て期間が過ぎた係争を確定させる。誰も Penalize しなければ、
// 不正提出者の古い残高がそのまま通ってしまう(StateClosedExpiredCheat)。
//
// これが「自分のチャネルを見張っていないと損をする」現実で、常時監視を肩代わりする
// 第三者サービス(watchtower)が生まれた理由でもある。
func (c *Channel) FinalizeDispute() {
	if c.state != StateDisputed {
		return
	}
	if c.clock < c.dispute.deadline {
		return // まだ期間内。咎める余地が残っている
	}
	comm := c.dispute.commitment
	c.finalize(comm.BalanceA, comm.BalanceB, StateClosedExpiredCheat)
}

// #endregion penalty
