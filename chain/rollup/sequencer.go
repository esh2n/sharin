package rollup

// #region sequencer

// Sequencer は L2 でオフチェーン実行し、L1 に投稿するバッチを組む主体。
// 内部に現在の L2 状態を持ち、取引を適用しては次のバッチを作る。
type Sequencer struct {
	state *L2State
}

// NewSequencer は初期状態から sequencer を作る。
func NewSequencer(initial *L2State) *Sequencer {
	return &Sequencer{state: initial.clone()}
}

// State は sequencer が主張する現在の L2 状態。
func (s *Sequencer) State() *L2State { return s.state }

// Propose は正直にバッチを組む。取引を実行し、その正しい結果を PostRoot として主張する。
// 返り値の witness は「このバッチの開始状態」で、後で challenge する側が使う
// (L1 は状態そのものを持たないので、検証には witness の提示が要る)。
func (s *Sequencer) Propose(txs []Tx) (Batch, *L2State) {
	pre := s.state.clone()
	post := Execute(pre, txs)
	b := Batch{
		PrevRoot: pre.Root(),
		PostRoot: post.Root(),
		Txs:      txs,
		Proof:    Prove(pre, txs, post.Root()),
	}
	s.state = post
	return b, pre
}

// ProposeFraud は不正なバッチを組む。取引は Txs のとおりだが、PostRoot に嘘の状態
// (claimedPost、例えば自分の残高を水増しした状態)を主張する。正直に計算した proof は
// この嘘に対して無効になる(ZK では commit 時に弾かれる。Optimistic では通ってしまい、
// challenge で暴かれるのを待つ)。
func (s *Sequencer) ProposeFraud(txs []Tx, claimedPost *L2State) (Batch, *L2State) {
	pre := s.state.clone()
	b := Batch{
		PrevRoot: pre.Root(),
		PostRoot: claimedPost.Root(),
		Txs:      txs,
		Proof:    Prove(pre, txs, claimedPost.Root()), // 嘘なので Valid=false になる
	}
	s.state = claimedPost.clone() // sequencer は嘘の状態を「本当の状態」として持ち回る
	return b, pre
}

// #endregion sequencer
