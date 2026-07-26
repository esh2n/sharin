package raft

// State はノードの役割。Raft では常にこの3つのいずれか。
type State int

const (
	Follower     State = iota // 追従者。リーダーの指示を受ける
	PreCandidate              // 仮候補者。任期を上げる前に「勝てそうか」を仮投票で確かめている
	Candidate                 // 候補者。選挙中(自分に投票を集めている)
	Leader                    // 指導者。クライアントの書き込みを受け付ける唯一のノード
)

func (s State) String() string {
	switch s {
	case Follower:
		return "Follower"
	case PreCandidate:
		return "PreCandidate"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// EntryType はログエントリの種類。
type EntryType int

const (
	EntryNormal     EntryType = iota // 通常の書き込み(状態機械へ適用する)
	EntryConfChange                  // メンバ構成の変更(ノードの追加/削除)
)

// Entry はレプリケーションログの1件。
// (Term, Index) の組がログ上の位置を一意に決め、Raft の安全性の土台になる。
type Entry struct {
	Term  uint64    // このエントリが作られた任期
	Index uint64    // ログ先頭からの通し番号(1始まり)
	Type  EntryType // 通常書き込みか構成変更か
	Data  []byte    // ペイロード(状態機械が解釈する。Raft は中身を見ない)
}

// Snapshot はある地点までのログを畳んだ「状態の写し」。
// ログが無限に伸びないよう、適用済みの前半をこれ1枚に置き換える(ログ圧縮)。
type Snapshot struct {
	// この写しが含む最後のエントリの位置。これ以前のログは捨ててよい。
	LastIndex uint64
	LastTerm  uint64
	// 写しを撮った時点のメンバ構成(スナップショットだけ受け取ったノードが構成を復元できるように)。
	Conf []uint64
	// 状態機械が直列化した状態そのもの。Raft は中身を見ない。
	Data []byte
}

// ConfChangeType は構成変更の向き。
type ConfChangeType int

const (
	ConfAddNode    ConfChangeType = iota // ノードを1台加える
	ConfRemoveNode                       // ノードを1台外す
)

// ConfChange は EntryConfChange の Data に入るペイロード。
// 「1回に1台だけ」変える単一サーバ変更方式(Ongaro の学位論文の推奨)を採る。
type ConfChange struct {
	Type   ConfChangeType
	NodeID uint64
}
