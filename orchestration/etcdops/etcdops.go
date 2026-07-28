// Package etcdops は etcd の履歴と容量の扱いを最小構成で実装する。
//
// [API サーバと informer](apiserver)の章で、写しが古すぎると差分で追いつけず、
// 全件を取り直すことになると書いた。その「古すぎる」を作っているのが、この層で
// 定期的に走っている圧縮になる。
//
// etcd はキーの履歴を持つ。書くたびに版が1つ増え、古い版もしばらく残る。
// 残っているから、写しは「版5まで見た。それ以降をくれ」と言える。履歴があることが、
// watch が成立する条件そのものになっている。
//
// だが履歴は増え続ける。放っておけば容量を使い切る。だから捨てる。捨てると、
// そこより古い版を見ていた写しは追いつけなくなる。同じ性質の表と裏になっている。
//
// そしてもう1段ややこしいところがある。捨ててもファイルは小さくならない。
// 空いた場所が中で余るだけで、返すには別の操作が要る。この二段構えを知らないと、
// 「圧縮したのに容量が減らない」で止まる。
//
// 容量を使い切ると、書き込みが止まる。読み出しは通るが、書けない。設定を変えるのも
// 書き込みなので、そのままでは自分で抜け出せない。抜けるには決まった順番の3手が要る。
package etcdops

import "sort"

// #region store

// Event は1件の変更。写しはこれを順に受け取って追いつく。
type Event struct {
	Rev   int
	Key   string
	Value string
	Del   bool
}

type version struct {
	rev   int
	value string
	del   bool
}

// Store は履歴を持つ置き場。
type Store struct {
	rev     int
	history map[string][]version
	// compactedAt より古い版は捨てられている。
	compactedAt int

	// logical は今も意味のあるデータの量。
	logical int
	// physical はファイルとして確保している量。圧縮しても減らない。
	physical int
	// quota を超えると書き込みが止まる。
	quota int
	// alarm が立っている間は書き込みを受け付けない。
	alarm bool

	Log []string
}

// New は容量の上限を決めて置き場を作る。
func New(quota int) *Store {
	return &Store{history: map[string][]version{}, quota: quota}
}

// Rev は今の版を返す。
func (s *Store) Rev() int { return s.rev }

// CompactedAt は、どこまで捨てたかを返す。
func (s *Store) CompactedAt() int { return s.compactedAt }

// Logical と Physical は、それぞれ意味のある量とファイルの量を返す。
func (s *Store) Logical() int  { return s.logical }
func (s *Store) Physical() int { return s.physical }

// Alarm は書き込みが止まっているかを返す。
func (s *Store) Alarm() bool { return s.alarm }

// ReadOnly は今書けない状態かを返す。
func (s *Store) ReadOnly() bool { return s.alarm }

// #endregion store

// #region write

// ErrNoSpace は容量を使い切って書けないこと、ErrCompacted は古すぎて追えないこと。
type Err string

func (e Err) Error() string { return string(e) }

const (
	ErrNoSpace   = Err("容量を使い切っている。書き込みは受け付けない")
	ErrCompacted = Err("その版はもう捨てられている")
)

// Put は値を書き、新しい版を返す。
//
// 上書きでも古い版が残るのが肝になる。だから物理的な量は、書いた回数ぶん増える。
// 論理的な量はキーの数ぶんしか増えない。この差が、そのまま履歴の重さになる。
func (s *Store) Put(key, value string) (int, error) {
	if s.alarm {
		return 0, ErrNoSpace
	}
	s.rev++
	if len(s.history[key]) == 0 {
		s.logical++
	}
	s.history[key] = append(s.history[key], version{rev: s.rev, value: value})
	s.physical++
	s.checkQuota()
	return s.rev, nil
}

// Delete は削除の版を積む。削除も履歴なので、量は増える。
func (s *Store) Delete(key string) (int, error) {
	if s.alarm {
		return 0, ErrNoSpace
	}
	if len(s.history[key]) == 0 {
		return s.rev, nil
	}
	s.rev++
	if !s.history[key][len(s.history[key])-1].del {
		s.logical--
	}
	s.history[key] = append(s.history[key], version{rev: s.rev, del: true})
	s.physical++
	s.checkQuota()
	return s.rev, nil
}

func (s *Store) checkQuota() {
	if s.quota > 0 && s.physical > s.quota && !s.alarm {
		s.alarm = true
		s.logf("容量の上限を超えた。書き込みを止める(読み出しは通る)")
	}
}

// #endregion write

// #region read

// Get は今の値を返す。書き込みが止まっていても読める。
func (s *Store) Get(key string) (string, bool) {
	vs := s.history[key]
	if len(vs) == 0 {
		return "", false
	}
	last := vs[len(vs)-1]
	if last.del {
		return "", false
	}
	return last.value, true
}

// GetAt は指定した版の時点の値を返す。捨てた版より古ければ読めない。
func (s *Store) GetAt(key string, rev int) (string, bool, error) {
	if rev < s.compactedAt {
		return "", false, ErrCompacted
	}
	var cur version
	found := false
	for _, v := range s.history[key] {
		if v.rev > rev {
			break
		}
		cur, found = v, true
	}
	if !found || cur.del {
		return "", false, nil
	}
	return cur.value, true, nil
}

// Since は from より後の変更を順に返す。捨てた版より古ければ追えない。
//
// [informer](apiserver) が写しを保つときに呼ぶのがこれになる。ここが失敗したら、
// 差分では追いつけないので全件を取り直すしかない。
func (s *Store) Since(from int) ([]Event, error) {
	if from < s.compactedAt {
		return nil, ErrCompacted
	}
	var out []Event
	keys := make([]string, 0, len(s.history))
	for k := range s.history {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range s.history[k] {
			if v.rev > from {
				out = append(out, Event{Rev: v.rev, Key: k, Value: v.value, Del: v.del})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rev < out[j].Rev })
	return out, nil
}

// #endregion read

// #region maintain

// Compact は rev より古い版を捨てる。
//
// 捨てるのは履歴だけで、ファイルは小さくならない。空いた場所は中で余るだけになる。
// これが「圧縮したのに容量が減らない」の正体で、返すには Defrag が要る。
func (s *Store) Compact(rev int) int {
	if rev <= s.compactedAt {
		return 0
	}
	dropped := 0
	for k, vs := range s.history {
		var keep []version
		for i, v := range vs {
			// その版の時点の値を読めるように、境界の1つ手前は残す。
			isLast := i == len(vs)-1
			if v.rev < rev && !isLast && vs[i+1].rev <= rev {
				dropped++
				continue
			}
			keep = append(keep, v)
		}
		s.history[k] = keep
	}
	s.compactedAt = rev
	s.logf("版 " + itoa(rev) + " より古い履歴を " + itoa(dropped) + " 件捨てた(ファイルの大きさは変わらない)")
	return dropped
}

// Defrag は余っている場所を実際に返す。
//
// 実物ではこの間そのメンバーが応答しない。1台ずつ順に行うことになる。
func (s *Store) Defrag() int {
	before := s.physical
	live := 0
	for _, vs := range s.history {
		live += len(vs)
	}
	s.physical = live
	freed := before - s.physical
	s.logf("余っていた " + itoa(freed) + " を返した(この間そのメンバーは応答しない)")
	return freed
}

// Disarm は書き込みの停止を解除する。
//
// 空きが戻っていなければ、解除しても次の書き込みでまた止まる。
// だから順番が決まっている。捨てる、返す、そして解除する。
func (s *Store) Disarm() bool {
	if !s.alarm {
		return true
	}
	if s.quota > 0 && s.physical > s.quota {
		s.logf("解除したが、空きが戻っていないので次の書き込みでまた止まる")
		s.alarm = false
		return false
	}
	s.alarm = false
	s.logf("書き込みを再開できる状態になった")
	return true
}

// #endregion maintain

// #region snapshot

// Snapshot は今の状態の写しを取る。履歴は運ばず、今の値だけを運ぶ。
type Snapshot struct {
	Rev  int
	Data map[string]string
}

// Take は写しを取る。書き込みが止まっていても取れる。
func (s *Store) Take() Snapshot {
	snap := Snapshot{Rev: s.rev, Data: map[string]string{}}
	for k := range s.history {
		if v, ok := s.Get(k); ok {
			snap.Data[k] = v
		}
	}
	return snap
}

// Restore は写しから新しい置き場を作る。
//
// 履歴は運ばれないので、復元した直後は誰も過去を追えない。写しを持っていた
// informer は、全件を取り直すことになる。
func Restore(snap Snapshot, quota int) *Store {
	s := New(quota)
	s.rev = snap.Rev
	s.compactedAt = snap.Rev
	keys := make([]string, 0, len(snap.Data))
	for k := range snap.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.history[k] = []version{{rev: snap.Rev, value: snap.Data[k]}}
		s.logical++
		s.physical++
	}
	s.logf("写しから復元した(版 " + itoa(snap.Rev) + " から。それより前の履歴は無い)")
	return s
}

// #endregion snapshot

func (s *Store) logf(msg string) { s.Log = append(s.Log, msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
