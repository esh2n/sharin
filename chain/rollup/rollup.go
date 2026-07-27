package rollup

import (
	"errors"
	"fmt"
)

// #region rollup

// Mode はロールアップの検証方式。
type Mode int

const (
	Optimistic Mode = iota // 楽観的に受理し、challenge 期間内の fraud proof で覆す
	ZK                     // validity proof を commit 時に検証し、即確定する
)

func (m Mode) String() string {
	if m == ZK {
		return "zk"
	}
	return "optimistic"
}

// Status は記録されたバッチの状態。
type Status int

const (
	Pending  Status = iota // Optimistic: challenge 期間中(まだ覆りうる)
	Final                  // 確定(ZK は即、Optimistic は期間経過後)
	Reverted               // fraud proof で覆された
)

func (s Status) String() string {
	switch s {
	case Final:
		return "final"
	case Reverted:
		return "reverted"
	default:
		return "pending"
	}
}

// Record は L1 に記録された 1 バッチ。
type Record struct {
	Batch       Batch
	Status      Status
	CommittedAt int
}

var (
	ErrRootMismatch     = errors.New("rollup: PrevRoot が現在の確定 root と繋がらない")
	ErrInvalidProof     = errors.New("rollup: validity proof が無効(ZK)")
	ErrProofRequired    = errors.New("rollup: ZK モードでは proof が必須")
	ErrBadWitness       = errors.New("rollup: witness が batch の PrevRoot と一致しない")
	ErrNotChallengeable = errors.New("rollup: このバッチは challenge 対象でない(確定済み or 期間切れ)")
	ErrWrongMode        = errors.New("rollup: このモードでは使えない操作")
)

// Rollup は L1 側のロールアップコントラクト。取引を再実行せず、state root の列を記録する。
type Rollup struct {
	mode            Mode
	challengePeriod int // Optimistic の異議申立て可能期間(論理時間)
	clock           int
	genesisRoot     string
	records         []*Record
	sequencerBond   uint64 // sequencer が積んだ保証金。fraud が証明されると没収される
	slashed         uint64
}

// New は初期状態の root を起点にロールアップを作る。
// challengePeriod は Optimistic 用(ZK では使わない)。bond は sequencer の保証金。
func New(mode Mode, genesis *L2State, challengePeriod int, bond uint64) *Rollup {
	return &Rollup{
		mode:            mode,
		challengePeriod: challengePeriod,
		genesisRoot:     genesis.Root(),
		sequencerBond:   bond,
	}
}

// Now は現在の論理時間。
func (r *Rollup) Now() int { return r.clock }

// Tick は論理時間を進める(challenge 期間の経過を表す)。
func (r *Rollup) Tick(d int) { r.clock += d }

// CanonicalRoot は現在の確定 root。覆っていない最後のバッチの PostRoot、無ければ genesis。
func (r *Rollup) CanonicalRoot() string {
	for i := len(r.records) - 1; i >= 0; i-- {
		if r.records[i].Status != Reverted {
			return r.records[i].Batch.PostRoot
		}
	}
	return r.genesisRoot
}

// Records は記録されたバッチ列(表示・検査用)。
func (r *Rollup) Records() []*Record { return r.records }

// Slashed は没収された保証金の累計。
func (r *Rollup) Slashed() uint64 { return r.slashed }

// Commit はバッチを投稿する。モードにより挙動が違う:
//   - Optimistic: 再実行も検証もせず記録する(だから安い)。PostRoot の正しさは
//     challenge 期間中の fraud proof に委ねられる。状態は Pending。
//   - ZK: proof を検証し、有効なら即 Final。無効なら拒否(不正はそもそも入れない)。
func (r *Rollup) Commit(b Batch) error {
	if b.PrevRoot != r.CanonicalRoot() {
		return fmt.Errorf("%w: batch.PrevRoot=%s canonical=%s", ErrRootMismatch, b.PrevRoot, r.CanonicalRoot())
	}
	b.Index = len(r.records)

	if r.mode == ZK {
		if b.Proof == nil {
			return ErrProofRequired
		}
		if !b.Proof.Valid {
			return ErrInvalidProof // 嘘の PostRoot には有効な証明が作れない → ここで弾かれる
		}
		r.records = append(r.records, &Record{Batch: b, Status: Final, CommittedAt: r.clock})
		return nil
	}

	// Optimistic: 検証せず Pending で受理する。
	r.records = append(r.records, &Record{Batch: b, Status: Pending, CommittedAt: r.clock})
	return nil
}

// Challenge は Optimistic モードで、バッチ index の不正を fraud proof で告発する。
//
// 告発者は witness(そのバッチの開始状態そのもの)を提示する。L1 は
//
//	(1) witness が batch.PrevRoot と一致するか
//	(2) witness に Txs を適用した正しい PostRoot が、主張値と一致するか
//
// を確かめる。食い違えば不正が証明され、そのバッチ以降を巻き戻し保証金を没収する。
// 一致すれば(=正直だった)challenge は失敗する。
//
// 返り値: 不正が証明されて巻き戻したなら true。
func (r *Rollup) Challenge(index int, witness *L2State) (bool, error) {
	if r.mode != Optimistic {
		return false, ErrWrongMode
	}
	if index < 0 || index >= len(r.records) {
		return false, fmt.Errorf("rollup: index %d は範囲外", index)
	}
	rec := r.records[index]
	// 確定済み・期間切れ・すでに覆ったものは対象外。
	if rec.Status != Pending || r.clock-rec.CommittedAt >= r.challengePeriod {
		return false, ErrNotChallengeable
	}
	if witness.Root() != rec.Batch.PrevRoot {
		return false, ErrBadWitness
	}

	trueRoot := Execute(witness, rec.Batch.Txs).Root()
	if trueRoot == rec.Batch.PostRoot {
		return false, nil // 正直だった。challenge は不成立
	}

	// 不正確定: このバッチと、それに積み上がった後続を全て巻き戻す。
	for i := index; i < len(r.records); i++ {
		r.records[i].Status = Reverted
	}
	r.slashed += r.sequencerBond // 保証金を没収(不正のコストを sequencer に負わせる)
	return true, nil
}

// Finalize は challenge 期間を過ぎた Pending バッチを Final にする(Optimistic)。
// 一度 Final になると、もう覆せない。
func (r *Rollup) Finalize() {
	if r.mode != Optimistic {
		return
	}
	for _, rec := range r.records {
		if rec.Status == Pending && r.clock-rec.CommittedAt >= r.challengePeriod {
			rec.Status = Final
		}
	}
}

// #endregion rollup
