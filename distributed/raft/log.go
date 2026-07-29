package raft

// raftLog はレプリケーションログ。1始まりの通し番号(Index)で並ぶ。
//
// スナップショットで前半を畳むので、実体は2つに分かれる:
//
//   - snapshot: Index ≤ snapshot.LastIndex を1枚に畳んだもの(実体は捨ててある)
//
//   - entries : Index > snapshot.LastIndex の生エントリ
//
//     Index:  1 2 3 4 5 6 7 8 9
//     └── snapshot ──┘└ entries ┘   (LastIndex=6 のとき)
//
// committed = 過半数に複製され「確定」した位置。applied = 状態機械へ渡し終えた位置。
// 常に applied ≤ committed ≤ lastIndex。
type raftLog struct {
	snapshot  Snapshot
	entries   []Entry // snapshot.LastIndex+1, +2, ... の順
	committed uint64
	applied   uint64
}

func newLog() *raftLog { return &raftLog{} }

// firstIndex は実体として残っている最小の Index(スナップショットの1つ後ろ)。
func (l *raftLog) firstIndex() uint64 { return l.snapshot.LastIndex + 1 }

// lastIndex はログ末尾の Index。エントリが無ければスナップショット境界。
func (l *raftLog) lastIndex() uint64 { return l.snapshot.LastIndex + uint64(len(l.entries)) }

// term は位置 i の任期を返す。ok=false は「圧縮済みで分からない/範囲外」。
func (l *raftLog) term(i uint64) (uint64, bool) {
	switch {
	case i == 0:
		return 0, true // 空ログの手前。Term 0 として扱う
	case i == l.snapshot.LastIndex:
		return l.snapshot.LastTerm, true
	case i < l.snapshot.LastIndex || i > l.lastIndex():
		return 0, false
	default:
		return l.entries[i-l.firstIndex()].Term, true
	}
}

// lastTerm はログ末尾の任期。選挙で「どちらが新しいか」の比較に使う。
func (l *raftLog) lastTerm() uint64 {
	t, _ := l.term(l.lastIndex())
	return t
}

// matchTerm は位置 i の任期が t と一致するか。AppendEntries の整合性チェックの核。
func (l *raftLog) matchTerm(i, t uint64) bool {
	lt, ok := l.term(i)
	return ok && lt == t
}

// slice は (lo, hi] のエントリを返す(lo は含まない)。範囲外は詰めて返す。
func (l *raftLog) slice(lo, hi uint64) []Entry {
	if lo < l.snapshot.LastIndex {
		lo = l.snapshot.LastIndex
	}
	if hi > l.lastIndex() {
		hi = l.lastIndex()
	}
	if lo >= hi {
		return nil
	}
	out := make([]Entry, hi-lo)
	copy(out, l.entries[lo-l.snapshot.LastIndex:hi-l.snapshot.LastIndex])
	return out
}

// append はリーダーが自分のログ末尾に新エントリを足す。付けた末尾 Index を返す。
func (l *raftLog) append(ents ...Entry) uint64 {
	l.entries = append(l.entries, ents...)
	return l.lastIndex()
}

// maybeAppend は追従者側の AppendEntries 処理。
// prevIndex/prevTerm が自分のログと一致して初めて ents を受け入れ、競合部分を上書きする。
// 一致しなければ (0,false) を返し、リーダーは1つ前を試す。
func (l *raftLog) maybeAppend(prevIndex, prevTerm, commit uint64, ents []Entry) (uint64, bool) {
	if !l.matchTerm(prevIndex, prevTerm) {
		return 0, false
	}
	// prevIndex の直後から ents を突き合わせる。食い違う最初の位置から上書き。
	for i, e := range ents {
		idx := prevIndex + 1 + uint64(i)
		if l.matchTerm(idx, e.Term) {
			continue // 既に同じものがある。冪等に飛ばす
		}
		// idx 以降を切り捨てて、残りの ents を貼り直す。
		l.truncateFrom(idx)
		l.append(ents[i:]...)
		break
	}
	last := prevIndex + uint64(len(ents))
	if commit > l.committed {
		l.commitTo(min(commit, last)) // 自分が持つ範囲を超えて commit してはいけない
	}
	return last, true
}

// truncateFrom は位置 idx 以降(idx を含む)を捨てる。競合エントリの上書きに使う。
func (l *raftLog) truncateFrom(idx uint64) {
	if idx <= l.snapshot.LastIndex {
		l.entries = nil
		return
	}
	cut := idx - l.firstIndex()
	if cut < uint64(len(l.entries)) {
		l.entries = l.entries[:cut]
	}
}

func (l *raftLog) commitTo(c uint64) {
	if c > l.committed {
		l.committed = c
	}
}

func (l *raftLog) appliedTo(a uint64) {
	if a > l.applied {
		l.applied = a
	}
}

// nextApplyable は「確定したがまだ状態機械に渡していない」エントリ (applied, committed]。
func (l *raftLog) nextApplyable() []Entry {
	if l.committed <= l.applied {
		return nil
	}
	return l.slice(l.applied, l.committed)
}

// restore はスナップショットを丸ごと受け入れる(遅れすぎた追従者が写しで追いつく)。
func (l *raftLog) restore(s Snapshot) {
	l.snapshot = s
	l.entries = nil
	l.committed = s.LastIndex
	l.applied = s.LastIndex
}

// compact は index までのログをスナップショットに畳んで実体を捨てる(ログ圧縮)。
func (l *raftLog) compact(index, term uint64, conf []uint64, data []byte) {
	keep := l.slice(index, l.lastIndex()) // index より後ろは残す
	l.snapshot = Snapshot{LastIndex: index, LastTerm: term, Conf: append([]uint64(nil), conf...), Data: data}
	l.entries = keep
}
