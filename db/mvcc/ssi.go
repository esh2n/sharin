package mvcc

// ssi.go はスナップショット分離(SI)の残された穴——write skew——を塞ぐ。
//
// SI は lost update を first-committer-wins で防ぐが、「別々のキーに書く」異常は
// すり抜ける。古典例が当直医のシフトだ。規則は「最低 1 人は当直」。alice と bob が
// 当直中(oncall:alice=yes, oncall:bob=yes)で、2 人が同時に「相手が残るなら自分は
// 抜けよう」と考える。
//
//	T1: alice を読む=yes, bob を読む=yes → 2 人いるので oncall:alice=no と書く
//	T2: alice を読む=yes, bob を読む=yes → 2 人いるので oncall:bob=no と書く
//
// 2 本は別々のキーに書くので書き込み競合は無く、SI では両方コミットできてしまう。
// 結果、当直はゼロ。これが write skew——それぞれは正しい判断なのに、直列に実行したら
// 決して起きない状態になる。
//
// 防ぐには「読んだ値が、自分のコミットまでの間に書き換えられていないか」を見ればよい。
// T1 が読んだ bob を T2 が書き換えてコミットしていたら、T1 の判断は古い世界のものだった
// ことになる。ここでは読み集合の検証としてそれを実装する。実際の PostgreSQL の SSI は
// rw-antidependency の環を検出するより精密な方法で、偽陽性を減らしている。

// #region ssi

// validateReads は、読んだ各キーについて「スナップショット以降にコミットされた版」が
// 無いかを確かめる。あれば、自分の読みは古く、その上で行った判断は直列化できない
// 可能性があるので中止する(ErrRWConflict)。
//
// 自分が書くキーの後発コミットは検証 1(first-committer-wins)が既に弾いているので、
// ここで効くのは「読んだだけのキー」——write skew がまさにこれに当たる。
func (t *Txn) validateReads() error {
	for key := range t.reads {
		if vs := t.store.versions[key]; len(vs) > 0 && vs[len(vs)-1].CommitTS > t.snapshotTS {
			return ErrRWConflict
		}
	}
	return nil
}

// #endregion ssi
