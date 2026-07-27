// Package mvcc は MVCC(多版型同時実行制御)を最小構成でモデル化する。
//
// ロックで読み書きを直列化すると、読むだけのトランザクションが書き込みを待たせ、
// 書き込みが読み手を止める。MVCC の発想は、上書きせずに版を積むことだ。値を書き換える
// のではなく「新しい版」を追加し、各トランザクションは開始時点のタイムスタンプで
// 見える版だけを読む。読み手はロックを取らず、書き手と衝突しない。
//
// このパッケージは 3 段で組む:
//  1. 版の連なりとスナップショット読み(mvcc.go): key → 版のリスト。開始時点で見える版を選ぶ
//  2. コミットと first-committer-wins(mvcc.go): 同じキーへの並行書き込みは先勝ち。lost update を防ぐ
//  3. 直列化可能性(ssi.go): スナップショット分離でも起きる write skew を、読み集合の検証で防ぐ
package mvcc

import "errors"

var (
	ErrTxnDone       = errors.New("mvcc: トランザクションは終了済み")
	ErrWriteConflict = errors.New("mvcc: 書き込み競合(first-committer-wins で敗北)")
	ErrRWConflict    = errors.New("mvcc: 読んだ値が後から書き換えられた(直列化違反)")
)

// #region store

// Version は 1 つのキーの 1 つの版。commitTS の昇順に積まれる。
// 上書きはせず、新しい版を追加していくのが MVCC の要になる。
type Version struct {
	Value    string
	CommitTS uint64
}

// Store は MVCC ストア。キーごとの版の連なりと、論理タイムスタンプの発番器を持つ。
// タイムスタンプは「いつ始まったか」「いつコミットしたか」の全順序を与える。
type Store struct {
	versions map[string][]Version
	clock    uint64 // 論理時計。Begin と Commit で 1 つずつ進む
}

// NewStore は空のストアを作る。initial があれば TS=0 の初期版として入れる。
func NewStore(initial map[string]string) *Store {
	s := &Store{versions: map[string][]Version{}}
	for k, v := range initial {
		s.versions[k] = []Version{{Value: v, CommitTS: 0}}
	}
	return s
}

// visible は snapshotTS 時点で見える版(CommitTS <= snapshotTS の中で最新)を返す。
// これがスナップショット読みの全てで、後からコミットされた版は自然に無視される。
func (s *Store) visible(key string, snapshotTS uint64) (Version, bool) {
	vs := s.versions[key]
	for i := len(vs) - 1; i >= 0; i-- {
		if vs[i].CommitTS <= snapshotTS {
			return vs[i], true
		}
	}
	return Version{}, false
}

// Versions はキーの版の連なり(表示・検査用)。
func (s *Store) Versions(key string) []Version { return s.versions[key] }

// #endregion store

// #region txn

// IsolationLevel は分離レベル。Snapshot はスナップショット分離(SI)、
// Serializable は読み集合の検証を足した直列化可能(ssi.go)。
type IsolationLevel int

const (
	Snapshot IsolationLevel = iota
	Serializable
)

// Txn は 1 つのトランザクション。開始時点のスナップショット TS と、
// バッファした書き込み(コミットまでストアに触れない)、読んだキーの集合を持つ。
type Txn struct {
	store      *Store
	level      IsolationLevel
	snapshotTS uint64
	writes     map[string]string
	reads      map[string]bool // Serializable の検証で使う読み集合
	done       bool
}

// Begin はトランザクションを開始する。この瞬間の時計がスナップショット TS になり、
// 以降なにを読んでも「この時点の世界」が見える。
func (s *Store) Begin(level IsolationLevel) *Txn {
	s.clock++
	return &Txn{
		store:      s,
		level:      level,
		snapshotTS: s.clock,
		writes:     map[string]string{},
		reads:      map[string]bool{},
	}
}

// Get はキーを読む。自分の未コミット書き込みが最優先(read-your-writes)、
// 次にスナップショット時点で見える版。ロックは取らない——読み手は誰も待たせない。
func (t *Txn) Get(key string) (string, bool) {
	if v, ok := t.writes[key]; ok {
		return v, true
	}
	t.reads[key] = true
	v, ok := t.store.visible(key, t.snapshotTS)
	if !ok {
		return "", false
	}
	return v.Value, true
}

// Put は書き込みをバッファする。コミットまでストアには反映されず、他のトランザクション
// からは見えない。
func (t *Txn) Put(key, value string) error {
	if t.done {
		return ErrTxnDone
	}
	t.writes[key] = value
	return nil
}

// Abort は書き込みを捨てて終了する。バッファ方式なので、捨てるだけで何も汚れない。
func (t *Txn) Abort() {
	t.done = true
	t.writes = nil
}

// #endregion txn

// #region commit

// Commit は検証してから書き込みを版として積む。
//
// 検証 1(SI 共通): first-committer-wins。自分が書こうとしているキーに、スナップショット
// より後のコミットが既にあれば敗北(ErrWriteConflict)。これが lost update を防ぐ——
// 同じ残高を同時に更新した 2 本のうち、後からコミットした方は必ず気づかされる。
//
// 検証 2(Serializable のみ): 読んだキーが後から書き換えられていないか(ssi.go)。
//
// 通れば commit TS を発番し、全書き込みをその TS の版として積む(原子的に見える)。
func (t *Txn) Commit() error {
	if t.done {
		return ErrTxnDone
	}
	t.done = true

	// 検証 1: 書き込み競合(first-committer-wins)
	for key := range t.writes {
		if vs := t.store.versions[key]; len(vs) > 0 && vs[len(vs)-1].CommitTS > t.snapshotTS {
			return ErrWriteConflict
		}
	}
	// 検証 2: 直列化可能性(Serializable のみ)
	if t.level == Serializable {
		if err := t.validateReads(); err != nil {
			return err
		}
	}

	t.store.clock++
	commitTS := t.store.clock
	for key, value := range t.writes {
		t.store.versions[key] = append(t.store.versions[key], Version{Value: value, CommitTS: commitTS})
	}
	return nil
}

// #endregion commit
