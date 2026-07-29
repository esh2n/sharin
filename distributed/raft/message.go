package raft

// MsgType はノード間・ノード内で流れるメッセージの種類。
// Raft の論文は RPC 2種(RequestVote / AppendEntries)+ InstallSnapshot だが、
// ここでは etcd/raft 流に「すべてをメッセージにして Step() に流す」設計にする。
// ローカル駆動(選挙開始・書き込み提案)もメッセージで表すと、状態機械が1本の
// 入口(Step)だけを持つ純粋関数になり、決定的にテストできる。
type MsgType int

const (
	MsgHup         MsgType = iota // ローカル: 選挙タイマ満了。選挙を始めよ
	MsgProp                       // ローカル: クライアントからの書き込み提案
	MsgPreVote                    // 仮投票要求。任期を上げずに「勝てそうか」だけ先に問う
	MsgPreVoteResp                // 仮投票応答
	MsgVote                       // RequestVote 要求(候補者→他ノード)
	MsgVoteResp                   // RequestVote 応答
	MsgApp                        // AppendEntries 要求(リーダー→追従者。心拍もこれで兼ねる)
	MsgAppResp                    // AppendEntries 応答
	MsgSnap                       // InstallSnapshot 要求(遅れすぎた追従者へ写しを送る)
	MsgSnapResp                   // InstallSnapshot 応答
)

func (t MsgType) String() string {
	switch t {
	case MsgHup:
		return "MsgHup"
	case MsgProp:
		return "MsgProp"
	case MsgPreVote:
		return "MsgPreVote"
	case MsgPreVoteResp:
		return "MsgPreVoteResp"
	case MsgVote:
		return "MsgVote"
	case MsgVoteResp:
		return "MsgVoteResp"
	case MsgApp:
		return "MsgApp"
	case MsgAppResp:
		return "MsgAppResp"
	case MsgSnap:
		return "MsgSnap"
	case MsgSnapResp:
		return "MsgSnapResp"
	default:
		return "Unknown"
	}
}

// Message は1通の通信。フィールドは種類ごとに使う部分だけ埋める
// (Go の zero value を利用。C の union のように厳密には分けない — 教科書として読みやすさ優先)。
type Message struct {
	Type MsgType
	From uint64 // 送り主のノードID
	To   uint64 // 宛先のノードID
	Term uint64 // 送り主の現在の任期。Raft の全メッセージに任期が乗る(古い者を弾くため)

	// --- MsgVote / MsgVoteResp ---
	LastLogIndex uint64 // 候補者のログ末尾の位置(投票者が「自分より新しいか」を判定)
	LastLogTerm  uint64
	Reject       bool // 投票拒否 / AppendEntries 不一致。true=断る

	// --- MsgApp / MsgAppResp ---
	PrevLogIndex uint64  // 送るエントリ群の直前の位置。ここが一致しないと受け取らない(整合性チェック)
	PrevLogTerm  uint64  // 同上の任期
	Entries      []Entry // 複製するエントリ(心拍のときは空)
	Commit       uint64  // リーダーの commitIndex(追従者はここまで適用してよいと知る)
	Index        uint64  // 応答用: 成功なら追従者のログ末尾 / 拒否なら次に試すべき位置のヒント

	// --- MsgSnap ---
	Snapshot Snapshot

	// --- MsgProp ---
	Data []byte // 提案されたエントリの中身
}
