// Package scaletrigger は、外部の滞留を見てレプリカ数を決めるときに、
// 「何が取れて何が取れないか」を最小構成で実装する。
//
// [カスタム指標とKEDA](custom-metrics)では、待ち行列の長さを目標にすると
// 必要な数がその場で出ることを見た。だが実物では、そもそも「待ち行列の長さ」として
// 何が取れるかが、相手の作りで変わる。
//
//   - ログ型(Kafka 等): 読んでも消えない。書いた位置と読んだ位置の差(ラグ)が取れる
//   - キュー型(SQS 等): 読んだら消える。残っている数しか取れず、位置の差は存在しない
//
// さらにキュー型には「見えている数」と「取り出されたがまだ確定していない数」があり、
// 前者だけで判断すると縮めすぎる。
//
// そして、いくら滞留が大きくてもレプリカを増やせば増えるとは限らない。
// ログ型では同時に読める数がパーティション数で頭打ちになる。
//
// 実時間も乱数も使わない。件数はすべて整数で数える。
package scaletrigger

// #region log

// Log は読んでも消えない配信元(Kafka のトピックなど)。
//
// 書いた位置と読んだ位置を別々に持つので、その差が「まだ処理していない量」になる。
type Log struct {
	written   int // 書き込まれた総数
	committed int // 読み終えた位置
}

// Append は n 件を末尾に足す。
func (l *Log) Append(n int) { l.written += n }

// Commit は n 件を読み終えたことにする。書いた位置は動かない。
func (l *Log) Commit(n int) {
	l.committed += n
	if l.committed > l.written {
		l.committed = l.written
	}
}

// Written は書き込まれた総数。
func (l *Log) Written() int { return l.written }

// Committed は読み終えた位置。
func (l *Log) Committed() int { return l.committed }

// Lag は書いた位置と読んだ位置の差。ログ型でしか取れない。
//
// 読んでも消えないので、過去に何件流れたかと、どこまで読んだかが両方残る。
// この 2 つがあるから引き算ができる。
func (l *Log) Lag() int { return l.written - l.committed }

// #endregion log

// #region queue

// Queue は読んだら消える配信元(SQS などの分配キュー)。
//
// 取り出された件数は「処理中」に移り、確定(ack)で消え、失敗(nack)で戻る。
// 書いた位置という概念が無いので、ラグは定義できない。
type Queue struct {
	visible  int // まだ誰も取り出していない数
	inFlight int // 取り出されたが、まだ確定していない数
}

// Enqueue は n 件を入れる。
func (q *Queue) Enqueue(n int) { q.visible += n }

// Receive は n 件を取り出す。取り出した分は見えなくなり、処理中へ移る。
func (q *Queue) Receive(n int) int {
	if n > q.visible {
		n = q.visible
	}
	q.visible -= n
	q.inFlight += n
	return n
}

// Ack は n 件を確定させる。ここで初めて消える。
func (q *Queue) Ack(n int) {
	if n > q.inFlight {
		n = q.inFlight
	}
	q.inFlight -= n
}

// Nack は n 件を戻す。処理に失敗したものは、また見えるようになる。
func (q *Queue) Nack(n int) {
	if n > q.inFlight {
		n = q.inFlight
	}
	q.inFlight -= n
	q.visible += n
}

// Visible は取り出せる数。KEDA の SQS スケーラが既定で見るのはこちら。
func (q *Queue) Visible() int { return q.visible }

// InFlight は処理中の数。
func (q *Queue) InFlight() int { return q.inFlight }

// Outstanding は「まだ終わっていない総数」。見えている数と処理中の数の合計。
//
// 見えている数だけを見ると、処理中が多いときに「もう空いた」と誤読する。
func (q *Queue) Outstanding() int { return q.visible + q.inFlight }

// #endregion queue

// #region desired

// Desired は滞留から必要なレプリカ数を返す。
//
// KEDA の lagThreshold と同じ式で、ceil(滞留 ÷ 1 レプリカが引き受ける量)。
// 現在のレプリカ数が式に出てこないので、何個で動いていても同じ滞留からは同じ答えが出る。
func Desired(backlog, perReplica int) int {
	if perReplica <= 0 || backlog <= 0 {
		return 0
	}
	return (backlog + perReplica - 1) / perReplica
}

// #endregion desired

// #region partition

// Topic は分割された配信元。同時に読める数がパーティション数で決まる。
type Topic struct {
	// Partitions は分割数。1 パーティションを同時に読めるのは 1 レプリカだけ。
	Partitions int
	// PerReplica は 1 レプリカが 1 単位時間に捌ける件数。
	PerReplica int
}

// EffectiveReplicas は、実際に仕事をするレプリカ数を返す。
//
// パーティションを超えたぶんは、割り当てが無いので何もしない。
func (t Topic) EffectiveReplicas(replicas int) int {
	if replicas > t.Partitions {
		return t.Partitions
	}
	if replicas < 0 {
		return 0
	}
	return replicas
}

// Idle は割り当てが無く遊ぶレプリカ数。
func (t Topic) Idle(replicas int) int { return replicas - t.EffectiveReplicas(replicas) }

// Throughput は 1 単位時間に捌ける件数。
func (t Topic) Throughput(replicas int) int {
	return t.EffectiveReplicas(replicas) * t.PerReplica
}

// MaxUseful は、これ以上増やしても捌ける量が増えないレプリカ数。
func (t Topic) MaxUseful() int { return t.Partitions }

// #endregion partition

// #region drain

// Drain は滞留が捌けるまでの単位時間を返す。到着が無い前提の見積もり。
//
// 捌ける量が 0 なら永遠に終わらないので -1 を返す。
func Drain(backlog int, t Topic, replicas int) int {
	th := t.Throughput(replicas)
	if th <= 0 {
		return -1
	}
	return (backlog + th - 1) / th
}

// #endregion drain
