// Package apiserver は Kubernetes の API サーバと informer を最小構成で
// 実装する。この編がずっと踏んできた土台にあたる。
//
// ここまでのすべての章は、ある仕掛けの上に成り立っていた。コントローラは
// 現状を知っている、という前提だ。調整ループは Pod の数を数えたし、
// スケジューラは空きを見たし、DaemonSet はノードの一覧を見た。だが、
// どうやって知るのか。
//
// 素朴には、毎回全件を問い合わせればよい。だが調整は頻繁に走るし、
// コントローラは何十個もある。全員が全件を毎秒問い合わせたら、API サーバが
// 潰れる。かといって、たまにしか問い合わせないと、反応が遅くなる。
//
// 答えは、手元に写しを持つことだった。最初に一度だけ全件を取り、以降は
// 変更だけを流してもらって写しを更新する。読むときは写しを見るので、
// API サーバには触らない。これが informer で、この編のコントローラが
// 「現状を数える」と言っていたときに実際に見ていたものになる。
//
// そして、変更を流してもらう仕組みには弱点がある。流れが切れると、切れて
// いる間の変更を取りこぼす。だから写しは古くなりうる。この編が
// level-triggered にこだわってきた理由が、ここで分かる。取りこぼしても
// 次に数え直せば追いつく、という設計でなければ、この土台の上では動けない。
package apiserver

import "sort"

// #region store

// Object は保存される1つの資源。Version は変更のたびに上がる。
type Object struct {
	Kind    string
	Name    string
	Value   string
	Version int
}

// Event は1回の変更。watch はこれを流す。
type Event struct {
	Type    string // "added" / "modified" / "deleted"
	Object  Object
	Version int // このイベント時点の全体の版
}

// Store は API サーバが持つ唯一の真実。ここが正で、写しは常にこの後を追う。
type Store struct {
	objs    map[string]*Object
	version int
	history []Event // 変更の履歴。watch が途中から流すために持つ

	Reads   int // 全件の読み出し回数(重い操作)
	Watches int // watch を張り直した回数
}

// NewStore は空の置き場を作る。
func NewStore() *Store { return &Store{objs: map[string]*Object{}} }

// Version は今の版を返す。
func (s *Store) Version() int { return s.version }

// Put は資源を作るか書き換える。版が上がり、履歴に積まれる。
func (s *Store) Put(kind, name, value string) Object {
	s.version++
	kt := "modified"
	o, ok := s.objs[kind+"/"+name]
	if !ok {
		kt = "added"
		o = &Object{Kind: kind, Name: name}
		s.objs[kind+"/"+name] = o
	}
	o.Value = value
	o.Version = s.version
	s.history = append(s.history, Event{Type: kt, Object: *o, Version: s.version})
	return *o
}

// Delete は資源を消す。これも版が上がり、履歴に積まれる。
func (s *Store) Delete(kind, name string) bool {
	o, ok := s.objs[kind+"/"+name]
	if !ok {
		return false
	}
	s.version++
	delete(s.objs, kind+"/"+name)
	s.history = append(s.history, Event{Type: "deleted", Object: *o, Version: s.version})
	return true
}

// List は全件を返す。重い操作で、写しを最初に埋めるときだけ呼ぶ。
func (s *Store) List(kind string) []Object {
	s.Reads++
	var keys []string
	for k, o := range s.objs {
		if o.Kind == kind {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]Object, 0, len(keys))
	for _, k := range keys {
		out = append(out, *s.objs[k])
	}
	return out
}

// Since は版 from より後の変更を返す。watch が途中から追いつくために使う。
//
// 履歴が残っている範囲でしか答えられない。古すぎる版から聞かれたら、
// 差分では答えられないので、全件を取り直してもらうしかない。
func (s *Store) Since(from int) ([]Event, bool) {
	if len(s.history) == 0 {
		return nil, true
	}
	oldest := s.history[0].Version
	if from < oldest-1 {
		return nil, false // 古すぎる。差分では追いつけない
	}
	var out []Event
	for _, e := range s.history {
		if e.Version > from {
			out = append(out, e)
		}
	}
	return out, true
}

// Compact は古い履歴を捨てる。無限には持てないので、いつか捨てる。
// 捨てた後は、それより古い版から追いつくことができなくなる。
func (s *Store) Compact(keep int) {
	if len(s.history) > keep {
		s.history = s.history[len(s.history)-keep:]
	}
}

// #endregion store

// #region informer

// Informer は手元の写し。読むときはここを見るので、API サーバに触らない。
type Informer struct {
	kind    string
	store   *Store
	cache   map[string]Object
	version int // どこまで追いついているか

	Connected bool // watch が繋がっているか
	Resyncs   int  // 全件を取り直した回数

	Log []string
}

// NewInformer は写しを作る。この時点ではまだ空で、Start で埋まる。
func NewInformer(s *Store, kind string) *Informer {
	return &Informer{kind: kind, store: s, cache: map[string]Object{}}
}

// Start は最初に一度だけ全件を取り、そこから watch を張る。
//
// この「一度だけ全件、以降は差分」が肝になる。毎回全件を取れば正確だが
// 重い。差分だけを追えば軽いが、始点が要る。だから最初に一度だけ全件を取る。
func (i *Informer) Start() {
	for _, o := range i.store.List(i.kind) {
		i.cache[o.Name] = o
	}
	i.version = i.store.Version()
	i.Connected = true
	i.store.Watches++
	i.Resyncs++
	i.logf("全件を読み込んで watch を張った(版 " + itoa(i.version) + ")")
}

// Disconnect は watch が切れた状態にする。以降の変更は届かなくなる。
func (i *Informer) Disconnect() {
	i.Connected = false
	i.logf("watch が切れた。この間の変更は届かない")
}

// Sync は届いた変更を写しに反映する。繋がっていなければ何もしない。
//
// 繋がっていない間に起きた変更は、ここでは反映されない。だから写しは
// 古くなる。読み手は古い写しを見て判断することになる。
func (i *Informer) Sync() {
	if !i.Connected {
		return
	}
	events, ok := i.store.Since(i.version)
	if !ok {
		// 履歴が捨てられていて追いつけない。全件を取り直すしかない。
		i.logf("履歴が古すぎて差分で追いつけない。全件を取り直す")
		i.cache = map[string]Object{}
		i.Start()
		return
	}
	for _, e := range events {
		if e.Type == "deleted" {
			delete(i.cache, e.Object.Name)
			continue
		}
		i.cache[e.Object.Name] = e.Object
	}
	i.version = i.store.Version()
}

// Reconnect は watch を張り直し、切れている間の変更に追いつく。
func (i *Informer) Reconnect() {
	i.Connected = true
	i.store.Watches++
	i.logf("watch を張り直した")
	i.Sync()
}

// List は写しから読む。API サーバには触らないので、何度呼んでも安い。
func (i *Informer) List() []Object {
	names := make([]string, 0, len(i.cache))
	for n := range i.cache {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Object, 0, len(names))
	for _, n := range names {
		out = append(out, i.cache[n])
	}
	return out
}

// Get は写しから1件返す。
func (i *Informer) Get(name string) (Object, bool) {
	o, ok := i.cache[name]
	return o, ok
}

// Stale は写しが最新から遅れているかを返す。
func (i *Informer) Stale() bool { return i.version < i.store.Version() }

// Lag は何版ぶん遅れているかを返す。
func (i *Informer) Lag() int { return i.store.Version() - i.version }

// #endregion informer

func (i *Informer) logf(msg string) { i.Log = append(i.Log, msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	k := len(buf)
	for n > 0 {
		k--
		buf[k] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[k:])
}
