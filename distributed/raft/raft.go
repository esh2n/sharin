// Package raft は Raft 合意アルゴリズムの最小実装。
//
// 設計の肝は「純粋状態機械」。ノードは Step(Message) と Tick() だけで動く関数の塊で、
// 内部で時計を読んだり通信したりしない。時間の経過も外から Tick() で刻む。だから
// テストが完全に決定的になり、選挙の競合や分断も1プロセスで再現できる(etcd/raft と同じ発想)。
package raft

import (
	"errors"
	"math/rand"
	"sort"
)

// ErrNotLeader はリーダー以外に書き込みを提案したときに返る。
var ErrNotLeader = errors.New("raft: このノードはリーダーではない")

// progress はリーダーが各追従者について持つ複製の進み具合。
type progress struct {
	next  uint64 // 次に送るエントリの位置
	match uint64 // 複製が確認できた最大位置
}

// Config は Raft ノードの初期設定。
type Config struct {
	ID            uint64     // 自分のノードID(1始まり。0は「不明」を表す予約)
	Peers         []uint64   // 起動時のメンバ全員(自分を含む)
	ElectionTick  int        // 選挙タイムアウトの基準(この Tick 数リーダーから音沙汰が無ければ選挙)
	HeartbeatTick int        // リーダーが心拍を送る間隔(Tick 数)
	Rand          *rand.Rand // 選挙タイムアウトの散らしに使う乱数源(テストで固定できるよう外から注入)
}

// Raft は1ノードの合意状態機械。
type Raft struct {
	id    uint64
	state State
	term  uint64 // currentTerm: 単調増加する論理時刻。全メッセージに乗る
	vote  uint64 // votedFor: この任期で投票した相手(0=まだ)
	lead  uint64 // 現在のリーダー(分かっていれば)
	log   *raftLog

	peers map[uint64]struct{} // 現在のメンバ集合。ログの構成変更から常に再計算する

	// リーダー専用
	prog map[uint64]*progress
	// 候補者専用
	votes map[uint64]bool

	// 時間(すべて Tick 数。実時間には依存しない)
	electionElapsed  int
	heartbeatElapsed int
	electionTimeout  int // 基準値
	heartbeatTimeout int
	randElection     int // 基準 + ランダム。split vote を避けるため毎回散らす
	rng              *rand.Rand

	pendingConf uint64 // 未適用の構成変更エントリの位置(0=なし)。同時に1件だけ許す

	msgs []Message // Step/Tick 中に溜める送信待ちメッセージ
}

// NewRaft はノードを1つ作る(起動直後は追従者)。
func NewRaft(c Config) *Raft {
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(int64(c.ID)))
	}
	l := newLog()
	l.snapshot.Conf = append([]uint64(nil), c.Peers...) // 初期メンバをスナップショット基底に置く
	r := &Raft{
		id:               c.ID,
		log:              l,
		peers:            map[uint64]struct{}{},
		prog:             map[uint64]*progress{},
		votes:            map[uint64]bool{},
		electionTimeout:  c.ElectionTick,
		heartbeatTimeout: c.HeartbeatTick,
		rng:              c.Rand,
	}
	r.recomputeMembership()
	r.becomeFollower(0, 0)
	return r
}

// --- 参照系(テスト・デモ用) ---

func (r *Raft) ID() uint64        { return r.id }
func (r *Raft) State() State      { return r.state }
func (r *Raft) Term() uint64      { return r.term }
func (r *Raft) Leader() uint64    { return r.lead }
func (r *Raft) Committed() uint64 { return r.log.committed }
func (r *Raft) LastIndex() uint64 { return r.log.lastIndex() }

// Members は現在のメンバ集合(ソート済み)。
func (r *Raft) Members() []uint64 {
	out := make([]uint64, 0, len(r.peers))
	for id := range r.peers {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// --- 駆動系(ドライバが呼ぶ) ---

// TakeMessages は溜まった送信待ちメッセージを取り出す(呼ぶと空になる)。
func (r *Raft) TakeMessages() []Message {
	m := r.msgs
	r.msgs = nil
	return m
}

// TakeApplied は「確定したがまだ渡していない」エントリを取り出し、applied を進める。
// 返ったエントリを状態機械へ適用するのは呼び出し側の責任。
func (r *Raft) TakeApplied() []Entry {
	ents := r.log.nextApplyable()
	if len(ents) > 0 {
		r.log.appliedTo(ents[len(ents)-1].Index)
		r.applyConfChanges(ents)
	}
	return ents
}

func (r *Raft) send(m Message) {
	m.From = r.id
	if m.Term == 0 {
		m.Term = r.term // 既に設定済み(仮投票の未来任期など)ならそれを尊重する
	}
	r.msgs = append(r.msgs, m)
}

// --- 状態遷移 ---

func (r *Raft) becomeFollower(term, lead uint64) {
	r.state = Follower
	r.reset(term)
	r.lead = lead
}

// becomePreCandidate は任期を上げずに仮投票フェーズに入る。
// 仮投票で過半数の感触が得られてから初めて本番の立候補(任期+1)へ進む。
// これにより、分断から復帰したノードや外されたノードが、無駄に任期を吊り上げて
// 正当なリーダーを引きずり下ろす「割り込み」を防ぐ。
func (r *Raft) becomePreCandidate() {
	r.state = PreCandidate
	// 任期も vote も変えない(仮投票が失敗しても元の状態に戻れる)
	r.votes = map[uint64]bool{r.id: true}
	r.lead = 0
	r.electionElapsed = 0
	r.randElection = r.electionTimeout + r.rng.Intn(r.electionTimeout)
}

func (r *Raft) becomeCandidate() {
	r.state = Candidate
	r.reset(r.term + 1) // 任期を1つ上げて立候補
	r.vote = r.id       // まず自分に投票
	r.votes = map[uint64]bool{r.id: true}
	r.lead = 0
}

func (r *Raft) becomeLeader() {
	r.state = Leader
	r.reset(r.term)
	r.lead = r.id
	// 各追従者の複製状態を初期化。next は楽観的に自分の末尾+1から始める
	last := r.log.lastIndex()
	for id := range r.peers {
		r.prog[id] = &progress{next: last + 1, match: 0}
	}
	r.prog[r.id].match = last
	// 就任直後に空(no-op)エントリを1件付ける。これで前任期のエントリも間接的に確定できる(図8対策)
	r.appendEntry(Entry{Type: EntryNormal})
	r.bcastAppend()
}

// reset は任期切り替え時の共通処理(タイマ・投票のリセット)。
func (r *Raft) reset(term uint64) {
	if term != r.term {
		r.term = term
		r.vote = 0
	}
	r.electionElapsed = 0
	r.heartbeatElapsed = 0
	r.randElection = r.electionTimeout + r.rng.Intn(r.electionTimeout) // [base, 2*base)
	r.votes = map[uint64]bool{}
	r.pendingConf = 0
}

// --- Tick: 論理時計を1つ進める ---

func (r *Raft) Tick() {
	switch r.state {
	case Leader:
		r.heartbeatElapsed++
		if r.heartbeatElapsed >= r.heartbeatTimeout {
			r.heartbeatElapsed = 0
			r.bcastAppend() // 心拍(空 or 追いつかせる AppendEntries)
		}
	default:
		r.electionElapsed++
		if r.electionElapsed >= r.randElection {
			r.electionElapsed = 0
			r.Step(Message{Type: MsgHup, To: r.id, From: r.id}) // 選挙開始
		}
	}
}

// --- Step: すべての入力の唯一の入口 ---

func (r *Raft) Step(m Message) error {
	// (1) 任期の大小で普遍的に処理。相手が新しければ自分は追従者に落ちる
	switch {
	case m.Term == 0:
		// ローカルメッセージ(MsgHup / MsgProp)。任期比較しない
	case m.Term > r.term:
		switch {
		case m.Type == MsgPreVote:
			// 仮投票は「未来の任期」を名乗るだけ。実際にはまだ任期は上がっていないので降格しない
		case m.Type == MsgPreVoteResp && !m.Reject:
			// 自分の仮投票が通った。任期は startCampaign で上げるのでここでは触らない
		case (m.Type == MsgVote) && r.lead != 0 && r.electionElapsed < r.electionTimeout:
			// リーダーリース: 現リーダーから最近声を聞いている間は割り込みの投票要求を無視する。
			// 外されたノードや分断復帰ノードがクラスタを乱すのを防ぐ核心。降格もしない
			return nil
		default:
			lead := m.From
			if m.Type == MsgVote {
				lead = 0 // 投票要求の送り主がリーダーとは限らない
			}
			r.becomeFollower(m.Term, lead)
		}
	case m.Term < r.term:
		// 古いメッセージは無視。ただし相手に「今の任期」を教えて追従者に落とす
		if m.Type == MsgApp || m.Type == MsgVote || m.Type == MsgPreVote || m.Type == MsgSnap {
			r.send(Message{To: m.From, Type: respType(m.Type), Term: r.term, Reject: true})
		}
		return nil
	}

	switch m.Type {
	case MsgHup:
		r.hup()
	case MsgProp:
		return r.propose(m.Data, EntryNormal)
	case MsgPreVote, MsgVote:
		r.handleVoteRequest(m)
	case MsgPreVoteResp, MsgVoteResp:
		r.handleVoteResp(m)
	case MsgApp:
		r.handleAppend(m)
	case MsgAppResp:
		r.handleAppendResp(m)
	case MsgSnap:
		r.handleSnapshot(m)
	case MsgSnapResp:
		r.handleAppendResp(m) // 写し適用後は AppendResp と同じく複製前進として扱う
	}
	return nil
}

func respType(t MsgType) MsgType {
	switch t {
	case MsgApp:
		return MsgAppResp
	case MsgVote:
		return MsgVoteResp
	case MsgPreVote:
		return MsgPreVoteResp
	case MsgSnap:
		return MsgSnapResp
	}
	return t
}

// --- 選挙 ---

// hup は選挙タイマ満了で呼ばれ、まず仮投票フェーズに入る。
func (r *Raft) hup() {
	r.becomePreCandidate()
	if g, _ := r.poll(); g >= r.quorum() { // 単独ノードなら自分の1票で足りる
		r.startCampaign()
		return
	}
	r.bcastVote(MsgPreVote, r.term+1) // 仮投票は「1つ上の任期」で勝てるか問う
}

// startCampaign は仮投票を通過して本番の立候補(任期+1)に進む。
func (r *Raft) startCampaign() {
	r.becomeCandidate()
	if g, _ := r.poll(); g >= r.quorum() {
		r.becomeLeader()
		return
	}
	r.bcastVote(MsgVote, r.term)
}

func (r *Raft) bcastVote(t MsgType, term uint64) {
	li, lt := r.log.lastIndex(), r.log.lastTerm()
	for id := range r.peers {
		if id == r.id {
			continue
		}
		r.send(Message{To: id, Type: t, Term: term, LastLogIndex: li, LastLogTerm: lt})
	}
}

// poll は現在集まっている(仮)投票の賛成数・反対数を数える。
func (r *Raft) poll() (grant, reject int) {
	for _, ok := range r.votes {
		if ok {
			grant++
		} else {
			reject++
		}
	}
	return
}

func (r *Raft) handleVoteRequest(m Message) {
	isPre := m.Type == MsgPreVote
	rt := MsgVoteResp
	if isPre {
		rt = MsgPreVoteResp
	}
	// 投票の条件:
	//   (a) 本投票: この任期でまだ誰にも入れていない or 既にこの候補者に入れた
	//       仮投票: 相手が名乗る未来任期は自分より先なので、投票済みは関係ない
	//   (b) 候補者のログが自分と同じか新しい(古い者をリーダーにしない=安全性の核)
	canVote := isPre || r.vote == 0 || r.vote == m.From
	if canVote && r.logUpToDate(m.LastLogIndex, m.LastLogTerm) {
		// 応答には相手の名乗る任期をそのまま返す(候補者が自分のフェーズと突き合わせられる)
		r.send(Message{To: m.From, Type: rt, Term: m.Term, Reject: false})
		if !isPre {
			r.vote = m.From
			r.electionElapsed = 0 // 本投票したら選挙タイマをリセット(正当な候補者を邪魔しない)
		}
	} else {
		r.send(Message{To: m.From, Type: rt, Term: r.term, Reject: true})
	}
}

// logUpToDate は候補者のログが自分以上に新しいか(任期優先、同任期なら長い方が新しい)。
// この1行が「古いログの持ち主をリーダーにしない」を保証し、確定済みエントリが消えるのを防ぐ。
// #region uptodate
func (r *Raft) logUpToDate(index, term uint64) bool {
	myTerm := r.log.lastTerm()
	return term > myTerm || (term == myTerm && index >= r.log.lastIndex())
}

// #endregion uptodate

func (r *Raft) handleVoteResp(m Message) {
	// PreCandidate は仮投票の、Candidate は本投票の集計をする
	if r.state != PreCandidate && r.state != Candidate {
		return
	}
	r.votes[m.From] = !m.Reject
	grant, reject := r.poll()
	switch {
	case grant >= r.quorum():
		if r.state == PreCandidate {
			r.startCampaign() // 仮投票通過 → 本番へ
		} else {
			r.becomeLeader() // 本投票通過 → リーダー就任
		}
	case reject >= r.quorum():
		r.becomeFollower(r.term, r.lead) // 過半数に拒否された。諦めて追従者へ
	}
}

// --- ログ複製 ---

func (r *Raft) bcastAppend() {
	for id := range r.peers {
		if id != r.id {
			r.sendAppend(id)
		}
	}
}

func (r *Raft) sendAppend(to uint64) {
	pr := r.prog[to]
	prevIndex := pr.next - 1
	prevTerm, ok := r.log.term(prevIndex)
	if !ok {
		// 送りたい起点が既に圧縮済み。エントリでは追いつけないので写しを送る
		r.send(Message{To: to, Type: MsgSnap, Snapshot: r.log.snapshot})
		return
	}
	ents := r.log.slice(prevIndex, r.log.lastIndex())
	r.send(Message{
		To: to, Type: MsgApp,
		PrevLogIndex: prevIndex, PrevLogTerm: prevTerm,
		Entries: ents, Commit: r.log.committed,
	})
}

func (r *Raft) handleAppend(m Message) {
	r.lead = m.From
	r.electionElapsed = 0 // リーダーから声が来た。選挙タイマをリセット
	last, ok := r.log.maybeAppend(m.PrevLogIndex, m.PrevLogTerm, m.Commit, m.Entries)
	if !ok {
		// 不一致。ヒント(自分の末尾+1)を返してリーダーに前を試させる
		r.send(Message{To: m.From, Type: MsgAppResp, Reject: true, Index: r.log.lastIndex() + 1})
		return
	}
	r.recomputeMembership() // 構成変更エントリを受け取ったかもしれない
	r.send(Message{To: m.From, Type: MsgAppResp, Reject: false, Index: last})
}

func (r *Raft) handleAppendResp(m Message) {
	if r.state != Leader {
		return
	}
	pr := r.prog[m.From]
	if pr == nil {
		return // 既に外したノードからの応答
	}
	if m.Reject {
		// 追従者のヒントまで next を下げて再送(1つずつより速い)
		if m.Index > 0 && m.Index <= pr.next {
			pr.next = m.Index
		} else if pr.next > 1 {
			pr.next--
		}
		r.sendAppend(m.From)
		return
	}
	if m.Index > pr.match {
		pr.match = m.Index
		pr.next = m.Index + 1
		if r.maybeCommit() {
			r.bcastAppend() // commit が進んだら全員に知らせる
		}
	}
}

// maybeCommit は過半数に複製された位置まで commitIndex を進める。
// ただし「現在の任期のエントリ」でしか前進させない(図8: 前任期のエントリを
// 数だけで確定すると後で覆る危険がある)。
// #region commit
func (r *Raft) maybeCommit() bool {
	// 全メンバの複製済み位置を集める(自分は末尾まで持っている)
	matches := make([]uint64, 0, len(r.peers))
	for id := range r.peers {
		if id == r.id {
			matches = append(matches, r.log.lastIndex())
		} else if pr := r.prog[id]; pr != nil {
			matches = append(matches, pr.match)
		} else {
			matches = append(matches, 0)
		}
	}
	// 降順に並べ、過半数番目の値 = 「過半数が到達している最大位置」
	sort.Slice(matches, func(i, j int) bool { return matches[i] > matches[j] })
	mci := matches[r.quorum()-1]
	if mci <= r.log.committed {
		return false
	}
	if t, ok := r.log.term(mci); !ok || t != r.term {
		return false // 現任期のエントリでなければ確定しない(図8対策)
	}
	r.log.commitTo(mci)
	return true
}

// #endregion commit

// --- スナップショット受信(追従者側) ---

func (r *Raft) handleSnapshot(m Message) {
	r.lead = m.From
	r.electionElapsed = 0
	s := m.Snapshot
	if s.LastIndex <= r.log.committed {
		// 既にそこまで持っている。今の末尾を返すだけ
		r.send(Message{To: m.From, Type: MsgSnapResp, Index: r.log.lastIndex()})
		return
	}
	r.log.restore(s)
	r.recomputeMembership()
	r.send(Message{To: m.From, Type: MsgSnapResp, Index: r.log.lastIndex()})
}

// --- 書き込み提案 ---

// Propose はクライアントの書き込みをログに積む(リーダーのみ)。
func (r *Raft) Propose(data []byte) error { return r.propose(data, EntryNormal) }

func (r *Raft) propose(data []byte, t EntryType) error {
	if r.state != Leader {
		return ErrNotLeader
	}
	r.appendEntry(Entry{Type: t, Data: data})
	r.bcastAppend()
	return nil
}

func (r *Raft) appendEntry(e Entry) {
	e.Term = r.term
	e.Index = r.log.lastIndex() + 1
	r.log.append(e)
	r.prog[r.id].match = e.Index
	r.prog[r.id].next = e.Index + 1
	r.maybeCommit() // 単独ノードなら即確定
}

// --- 構成変更(単一サーバ方式) ---

// ProposeConfChange はメンバを1台追加/削除する変更をログに積む(リーダーのみ)。
// 同時に進行できる構成変更は1件だけ(直前のが確定するまで次を受けない)。
func (r *Raft) ProposeConfChange(cc ConfChange) error {
	if r.state != Leader {
		return ErrNotLeader
	}
	if r.pendingConf > r.log.applied {
		return errors.New("raft: 構成変更が進行中")
	}
	r.appendEntry(Entry{Type: EntryConfChange, Data: encodeConfChange(cc)})
	r.pendingConf = r.log.lastIndex()
	r.recomputeMembership() // 構成はログに載った瞬間に採用する(論文どおり。append 時反映)
	if r.state == Leader {
		r.ensureProgress()
		r.bcastAppend()
	}
	return nil
}

// applyConfChanges は確定して状態機械へ渡す段でメンバを最終確定させる
// (append 時に既に peers へ反映済み。ここでは自分が外れたリーダーの降格などを扱う)。
func (r *Raft) applyConfChanges(ents []Entry) {
	r.recomputeMembership()
	if r.state == Leader {
		if _, ok := r.peers[r.id]; !ok {
			r.becomeFollower(r.term, 0) // 自分が外された。リーダーを降りる
		} else {
			r.ensureProgress()
		}
	}
}

// recomputeMembership はメンバ集合を「スナップショット基底 + ログ上の全構成変更」から作り直す。
// truncate で構成変更が消えても自動的に正しくなるのが利点。
func (r *Raft) recomputeMembership() {
	members := map[uint64]struct{}{}
	for _, id := range r.log.snapshot.Conf {
		members[id] = struct{}{}
	}
	for i := r.log.firstIndex(); i <= r.log.lastIndex(); i++ {
		e := r.log.entries[i-r.log.firstIndex()]
		if e.Type != EntryConfChange {
			continue
		}
		cc := decodeConfChange(e.Data)
		switch cc.Type {
		case ConfAddNode:
			members[cc.NodeID] = struct{}{}
		case ConfRemoveNode:
			delete(members, cc.NodeID)
		}
	}
	r.peers = members
}

// ensureProgress はメンバ増減に合わせてリーダーの複製状態を過不足なく整える。
func (r *Raft) ensureProgress() {
	for id := range r.peers {
		if r.prog[id] == nil {
			r.prog[id] = &progress{next: r.log.lastIndex() + 1, match: 0}
		}
	}
	for id := range r.prog {
		if _, ok := r.peers[id]; !ok {
			delete(r.prog, id)
		}
	}
}

// --- スナップショット作成(アプリ主導) ---

// Snapshot は applied 位置までのログを畳んで捨てる(ログ圧縮)。data は状態機械の直列化。
func (r *Raft) Snapshot(data []byte) {
	idx := r.log.applied
	term, ok := r.log.term(idx)
	if !ok {
		return
	}
	r.log.compact(idx, term, r.Members(), data)
}

func (r *Raft) quorum() int { return len(r.peers)/2 + 1 }
