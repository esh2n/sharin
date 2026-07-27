package rollup

// #region batch

// Batch は sequencer が L1 に投稿する 1 単位。
//
// 中身は「どの状態(PrevRoot)から始めて、これらの取引(Txs)を実行し、
// この状態(PostRoot)になった」という主張だ。L1 はこの主張を再実行せずに記録する。
// PostRoot が本当に正しいかは、Optimistic なら後で fraud proof が、
// ZK なら添えられた Proof が担保する。
//
// Txs を丸ごと L1 に載せるのは、誰でも再実行して検証できるようにするため
// (data availability)。L1 に取引データが無いと、fraud proof を作れない。
type Batch struct {
	Index    int
	PrevRoot string // 開始状態の root(直前バッチの PostRoot と繋がる)
	PostRoot string // 主張する結果状態の root
	Txs      []Tx   // 実行した取引(calldata として L1 に載る)
	Proof    *Proof // ZK モードでのみ添付。Optimistic では nil
}

// Proof は ZK ロールアップの validity proof を模したもの。
//
// 本物は「PostRoot が Txs を PrevRoot に適用した正しい結果である」ことを、
// 状態そのものを明かさずに検証できる暗号学的証明。ここではその暗号は自作せず、
// 「正直な prover だけが正しい PostRoot に対して有効な証明を作れる」という
// 性質だけをモデル化する: Valid は prover が正直に計算した真偽で、
// 嘘の PostRoot に対しては有効な証明を作れない(Valid=false になる)。
type Proof struct {
	Valid bool
}

// Prove は正直な prover として、この主張に対する証明を作る。
// 主張(PostRoot)が実際の実行結果と一致するときだけ Valid=true。
// 不正な sequencer はここで嘘の PostRoot を主張しても、有効な証明を作れない。
func Prove(pre *L2State, txs []Tx, claimedPostRoot string) *Proof {
	trueRoot := Execute(pre, txs).Root()
	return &Proof{Valid: trueRoot == claimedPostRoot}
}

// #endregion batch
