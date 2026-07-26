package queue

// IdempotentSink は Key で重複を捨てる冪等な受け皿。at-least-once で同じメッセージが
// 再配送されても、2回目以降は副作用を起こさない。これで「取りこぼさない(at-least-once)」
// を保ったまま「重複しない」を実現し、**実質1回(effectively-once)**にする。
//
// 冪等キー(Key)は「この処理は済んだか」を判定する印。実務では処理結果の書き込み先で
// UNIQUE 制約にする/処理済みIDの表を持つ、などで同じことをする。
type IdempotentSink struct {
	seen       map[string]struct{}
	Delivered  []Message // 実際に副作用を起こした(重複でない)メッセージ
	Duplicates int       // 重複として捨てた回数
}

// NewIdempotentSink は空の冪等な受け皿を作る。
func NewIdempotentSink() *IdempotentSink {
	return &IdempotentSink{seen: map[string]struct{}{}}
}

// #region dedup
// Handle は1件を受ける。初見なら副作用を記録し、既見(重複)なら捨てる。
// Consumer の handle にそのまま渡せる。
func (s *IdempotentSink) Handle(m Message) {
	if _, dup := s.seen[m.Key]; dup {
		s.Duplicates++
		return // すでに処理済みのキー。再配送なので副作用は起こさない
	}
	s.seen[m.Key] = struct{}{}
	s.Delivered = append(s.Delivered, m)
}

// #endregion dedup
