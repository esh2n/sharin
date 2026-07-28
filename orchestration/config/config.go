// Package config は Kubernetes の ConfigMap と Secret を最小構成で実装する。
//
// 設定をイメージに焼き込むと、環境ごとに別のイメージが要る。開発と本番で
// 中身の違うイメージを配ると、本番で動くものをテストしたことにならない。
// だから設定は外に出して、同じイメージに別の設定を渡す。
//
// 出し方は2つある。環境変数として渡すか、ファイルとして見せるか。この2つは
// 見た目が似ているのに、更新のときの振る舞いがまったく違う。環境変数は
// プロセスの起動時に渡されるので、後から変えられない。ファイルは実体が
// 差し替わるので、後から変わる。
//
// この違いが、設定を変えたのに反映されない、という形で表に出る。ConfigMap を
// 書き換えたのに古い値で動き続ける Pod は、たいてい環境変数で受け取っている。
// 反映するには作り直すしかなく、そのことが設定の外出しの一部になっている。
//
// Secret は ConfigMap とほとんど同じ形をしている。違うのは扱いの慎重さで、
// 仕組みとしての秘匿はほとんど無い。名前が示すほど守ってくれない、という
// ことを知らずに使うほうが危ない。
package config

import "sort"

// #region store

// Kind は設定の種類。仕組みはほぼ同じで、扱いの慎重さだけが違う。
type Kind int

const (
	// ConfigMap は秘密でない設定。
	ConfigMap Kind = iota
	// Secret は秘密の設定。保存形式は違うが、暗号ではない。
	Secret
)

func (k Kind) String() string {
	if k == Secret {
		return "Secret"
	}
	return "ConfigMap"
}

// Entry は1つの設定。名前と、鍵と値の集まりを持つ。
type Entry struct {
	Name    string
	Kind    Kind
	data    map[string]string
	Version int // 書き換えるたびに上がる(観測用)
}

// Get は鍵の値を返す。
func (e *Entry) Get(key string) string { return e.data[key] }

// Keys は鍵を名前順に返す。
func (e *Entry) Keys() []string {
	out := make([]string, 0, len(e.data))
	for k := range e.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Store は設定の置き場。
type Store struct {
	entries map[string]*Entry
	Log     []string
}

// NewStore は空の置き場を作る。
func NewStore() *Store { return &Store{entries: map[string]*Entry{}} }

// Put は設定を作るか、書き換える。書き換えると版が上がる。
func (s *Store) Put(name string, kind Kind, data map[string]string) *Entry {
	e, ok := s.entries[name]
	if !ok {
		e = &Entry{Name: name, Kind: kind, data: map[string]string{}}
		s.entries[name] = e
	}
	for k, v := range data {
		e.data[k] = v
	}
	e.Version++
	s.logf(kind.String() + " " + name + " を更新(版 " + itoa(e.Version) + ")")
	return e
}

// Get は設定を返す。
func (s *Store) Get(name string) *Entry { return s.entries[name] }

// #endregion store

// #region mount

// Source は設定の受け取り方。ここが更新の振る舞いを決める。
type Source int

const (
	// EnvVar は環境変数として渡す。プロセスの起動時に決まり、後から変わらない。
	EnvVar Source = iota
	// FileMount はファイルとして見せる。実体が差し替わるので後から変わる。
	FileMount
)

func (s Source) String() string {
	if s == FileMount {
		return "ファイル"
	}
	return "環境変数"
}

// Ref は「どの設定を、どう受け取るか」の指定。
type Ref struct {
	Entry  string
	Source Source
}

// Pod は設定を受け取って動くプロセス。
type Pod struct {
	Name string
	refs []Ref

	// env は起動時に写し取った値。以降、置き場が変わっても変わらない。
	env map[string]string
	// bornVersion は起動時に見えていた版(観測用)。
	bornVersion map[string]int

	store *Store
}

// #endregion mount

// #region read

// Cluster は置き場と Pod をまとめて持つ。
type Cluster struct {
	Store *Store
	pods  map[string]*Pod
	Log   []string
}

// New は空のクラスタを作る。
func New() *Cluster { return &Cluster{Store: NewStore(), pods: map[string]*Pod{}} }

// Start は Pod を起動する。この瞬間に、環境変数として渡す分が写し取られる。
//
// 写し取るという一語が、この章のほとんどを説明する。環境変数はプロセスに
// 渡された時点で値が確定し、置き場が後から変わっても届かない。届けるには
// プロセスを作り直すしかない。
func (c *Cluster) Start(name string, refs ...Ref) *Pod {
	p := &Pod{Name: name, refs: refs, env: map[string]string{},
		bornVersion: map[string]int{}, store: c.Store}
	for _, r := range refs {
		e := c.Store.Get(r.Entry)
		if e == nil {
			continue
		}
		p.bornVersion[r.Entry] = e.Version
		if r.Source == EnvVar {
			for _, k := range e.Keys() {
				p.env[k] = e.Get(k) // ここで写し取る
			}
		}
	}
	c.pods[name] = p
	c.logf(name + " を起動(環境変数は起動時の値を写し取る)")
	return p
}

// Restart は Pod を作り直す。写し取り直すので、環境変数にも今の値が入る。
func (c *Cluster) Restart(name string) *Pod {
	p, ok := c.pods[name]
	if !ok {
		return nil
	}
	c.logf(name + " を作り直す")
	return c.Start(name, p.refs...)
}

// Pods は Pod を名前順に返す。
func (c *Cluster) Pods() []*Pod {
	names := make([]string, 0, len(c.pods))
	for n := range c.pods {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Pod, len(names))
	for i, n := range names {
		out[i] = c.pods[n]
	}
	return out
}

// Read は Pod から見えている値を返す。
//
// 環境変数なら起動時に写し取った値、ファイルなら今の置き場の値。
// 同じ設定を同じ Pod が読んでいるのに、受け取り方だけで結果が変わる。
func (p *Pod) Read(entry, key string) string {
	for _, r := range p.refs {
		if r.Entry != entry {
			continue
		}
		if r.Source == EnvVar {
			return p.env[key]
		}
		if e := p.store.Get(entry); e != nil {
			return e.Get(key) // 実体を見に行くので、今の値が返る
		}
	}
	return ""
}

// Stale は Pod が古い値を持ったままかを返す。
// 環境変数で受け取っていて、置き場のほうが新しければ古い。
func (p *Pod) Stale() bool {
	for _, r := range p.refs {
		if r.Source != EnvVar {
			continue
		}
		e := p.store.Get(r.Entry)
		if e != nil && e.Version > p.bornVersion[r.Entry] {
			return true
		}
	}
	return false
}

// Sources は Pod がどの設定をどう受け取っているかを返す。
func (p *Pod) Sources() []Ref { return append([]Ref(nil), p.refs...) }

// #endregion read

func (s *Store) logf(msg string)   { s.Log = append(s.Log, msg) }
func (c *Cluster) logf(msg string) { c.Log = append(c.Log, msg) }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
