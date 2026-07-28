// Package quorum は、リーダーを置かない複製で「何台に聞けば古い値を見ないか」を扱う。
//
// [レプリケーション](replication)の章では、書き込みを受けるのはリーダー1台だった。
// リーダーが居れば最新は必ずリーダーにあるので、どこから読むかは速さの問題でしかない。
//
// リーダーを置かないと、その拠り所が消える。書き込みは複数の台に散り、
// どの台が最新を持っているかは、聞いてみるまで分からない。
//
// 答えは台数の勘定になる。担当が N 台、書きで返事を待つのが W 台、
// 読みで返事を待つのが R 台。R + W > N なら、読む集合と書く集合は
// 必ず1台以上を共有する。共有する台が最新を持っているので、
// R 台の中に必ず最新が混ざる。
//
// この実装で見えるようにするのは3つ:
//
//   - 重なりは台数の引き算で作れること(R + W > N)。
//   - 重なっても、どれが最新かは別に決めること(版番号と読み修復)。
//   - 緩めた quorum は可用性と引き換えに重なりを手放すこと(代役と受け渡し)。
//
// 実時間も乱数も使わない。版番号は単調に増える数え上げで、
// 「誰が先に返事するか」は読みの回数で回すので、結果は必ず再現する。
package quorum

import "strconv"

// #region config

// Config は複製の本数と、返事を待つ台数を決める。
type Config struct {
	// N は1つの key を担当する台数(複製の本数)。
	N int
	// R は読みで返事を待つ台数。
	R int
	// W は書きで返事を待つ台数。
	W int
	// Sloppy は、担当が落ちているとき担当外の台を代役に使うかどうか。
	Sloppy bool
	// ReadRepair は、読んだついでに古い台を直すかどうか。
	ReadRepair bool
}

// Overlaps は、読む集合と書く集合が必ず1台以上重なるかを返す。
//
// N 台のうち W 台が書き込みを持っている。残りの古い台は N - W 台しかない。
// R > N - W なら、R 台すべてを古い台で埋めることはできない。
// 移項すると R + W > N になる。これがこの章のすべてになる。
func (c Config) Overlaps() bool { return c.R+c.W > c.N }

// #endregion config

// #region value

// Value は1件の値。Stamp は書き込みごとに1つ増える版番号。
type Value struct {
	Data  string
	Stamp int
}

// Newer は2つの値のうち新しいほうを返す。版番号が大きいほうが勝つ。
//
// 重なりが保証するのは「最新を持つ台が返事の中に居る」ことだけで、
// 返ってきた値のどれが最新かは、この判定が決める。
func Newer(a, b Value) Value {
	if b.Stamp > a.Stamp {
		return b
	}
	return a
}

// #endregion value

// Hint は代役が預かっている書き込み。担当が戻ったら渡す。
type Hint struct {
	Owner string
	Key   string
	Value Value
}

// Node は1台。key ごとに1つの値と、預かりぶんを持つ。
type Node struct {
	Name  string
	data  map[string]Value
	hints []Hint
}

// Get はその台が持っている値を返す。
func (n *Node) Get(key string) (Value, bool) {
	v, ok := n.data[key]
	return v, ok
}

// Hints は預かっているぶんを返す。
func (n *Node) Hints() []Hint { return append([]Hint(nil), n.hints...) }

// put は書き込みを取り込む。古いものが後から届いても値は戻らない。
func (n *Node) put(key string, v Value) { n.data[key] = Newer(n.data[key], v) }

// Cluster は担当を持ち回る複製系。リーダーは居ない。
type Cluster struct {
	cfg   Config
	names []string
	nodes map[string]*Node
	down  map[string]bool

	stamp int
	reads int

	Log []string
}

// New は台数と設定から系を作る。
func New(cfg Config, names ...string) *Cluster {
	c := &Cluster{cfg: cfg, nodes: map[string]*Node{}, down: map[string]bool{}}
	c.names = append([]string(nil), names...)
	for _, n := range c.names {
		c.nodes[n] = &Node{Name: n, data: map[string]Value{}}
	}
	return c
}

// Names は台の一覧を返す。
func (c *Cluster) Names() []string { return append([]string(nil), c.names...) }

// Node は1台を返す。
func (c *Cluster) Node(name string) *Node { return c.nodes[name] }

// Config は設定を返す。
func (c *Cluster) Config() Config { return c.cfg }

// Kill は台を落とす。落ちた台は書きも読みも返事をしない。
func (c *Cluster) Kill(name string) {
	c.down[name] = true
	c.logf(name + " が落ちた")
}

// Revive は台を戻す。落ちている間の書き込みは持っていない。
func (c *Cluster) Revive(name string) {
	delete(c.down, name)
	c.logf(name + " が戻った(落ちていた間のぶんは持っていない)")
}

// IsDown は落ちているかを返す。
func (c *Cluster) IsDown(name string) bool { return c.down[name] }

// #region home

// Home は key を担当する N 台を返す。
//
// 輪の上で key の位置から順に N 台。誰が担当かは
// [コンシステントハッシュ](consistenthash)と同じ決め方で、key ごとに固定される。
func (c *Cluster) Home(key string) []string {
	start := int(hash(key) % uint32(len(c.names)))
	out := make([]string, 0, c.cfg.N)
	for i := 0; i < c.cfg.N && i < len(c.names); i++ {
		out = append(out, c.names[(start+i)%len(c.names)])
	}
	return out
}

// substitute は担当外から代役を1台選ぶ。輪の続きから、生きていてまだ使っていない台。
func (c *Cluster) substitute(key string, used map[string]bool) string {
	start := int(hash(key) % uint32(len(c.names)))
	for i := c.cfg.N; i < len(c.names); i++ {
		n := c.names[(start+i)%len(c.names)]
		if c.down[n] || used[n] {
			continue
		}
		return n
	}
	return ""
}

// hash は key から輪の位置を出す(FNV-1a)。
func hash(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h
}

// #endregion home

// #region put

// WriteResult は書き込みの結果。
type WriteResult struct {
	// OK は W 台ぶんの返事が集まったか。
	OK bool
	// Acks は返事の数。代役ぶんも数える。
	Acks int
	// Stored は担当のうち実際に値を持った台。
	Stored []string
	// Substitutes は代役として預かった台。担当ではない。
	Substitutes []string
	// Stamp はこの書き込みの版番号。
	Stamp int
}

// Put は key に値を書く。担当 N 台すべてに送り、W 台の返事で成功とする。
//
// 落ちている担当は飛ばす。Sloppy なら担当外の台を代役に立てて、
// そこに預かってもらう。代役ぶんも返事に数えるので、担当が落ちていても
// 書き込みは通る。ただし値は担当の上に無い。
func (c *Cluster) Put(key, data string) WriteResult {
	c.stamp++
	v := Value{Data: data, Stamp: c.stamp}
	home := c.Home(key)

	used := map[string]bool{}
	for _, n := range home {
		used[n] = true
	}

	res := WriteResult{Stamp: v.Stamp}
	for _, n := range home {
		if !c.down[n] {
			c.nodes[n].put(key, v)
			res.Stored = append(res.Stored, n)
			res.Acks++
			continue
		}
		if !c.cfg.Sloppy {
			continue
		}
		s := c.substitute(key, used)
		if s == "" {
			continue
		}
		used[s] = true
		c.nodes[s].hints = append(c.nodes[s].hints, Hint{Owner: n, Key: key, Value: v})
		res.Substitutes = append(res.Substitutes, s)
		res.Acks++
	}

	res.OK = res.Acks >= c.cfg.W
	if res.OK {
		c.logf(key + "=" + data + " を書いた(返事 " + itoa(res.Acks) + "/" + itoa(c.cfg.W) + ")")
	} else {
		// 足りなくても、書けてしまった台の値はそのまま残る。
		c.logf(key + "=" + data + " は返事が足りない(" + itoa(res.Acks) + "/" + itoa(c.cfg.W) + ")が、書けた台には残る")
	}
	return res
}

// #endregion put

// #region get

// ReadResult は読みの結果。
type ReadResult struct {
	// OK は R 台ぶんの返事が集まったか。
	OK bool
	// Asked は実際に返事をもらった台。
	Asked []string
	// Value は返事の中でいちばん新しい値。
	Value Value
	// Found は値が見つかったか。
	Found bool
	// Repaired は読み修復で直した台。
	Repaired []string
}

// Get は key を読む。担当のうち R 台に聞いて、いちばん新しい値を返す。
//
// 誰が先に返事するかは決まっていないので、聞く順は読みの回数で回す。
// R + W > N なら、この順がどう回っても最新を持つ台が必ず混ざる。
func (c *Cluster) Get(key string) ReadResult {
	home := c.Home(key)
	c.reads++

	var asked []string
	var got []Value
	for i := 0; i < len(home) && len(asked) < c.cfg.R; i++ {
		n := home[(c.reads+i)%len(home)]
		if c.down[n] {
			continue
		}
		asked = append(asked, n)
		v, _ := c.nodes[n].Get(key)
		got = append(got, v)
	}

	res := ReadResult{Asked: asked}
	if len(asked) < c.cfg.R {
		c.logf(key + " は返事が足りない(" + itoa(len(asked)) + "/" + itoa(c.cfg.R) + ")ので読めない")
		return res
	}
	res.OK = true

	best := Value{}
	for _, v := range got {
		best = Newer(best, v)
	}
	if best.Stamp > 0 {
		res.Value, res.Found = best, true
	}

	if c.cfg.ReadRepair && res.Found {
		for i, n := range asked {
			if got[i].Stamp < best.Stamp {
				c.nodes[n].put(key, best)
				res.Repaired = append(res.Repaired, n)
			}
		}
	}
	c.logf(key + " を " + itoa(len(asked)) + " 台に聞いた → " + res.Value.Data + "(版 " + itoa(res.Value.Stamp) + ")")
	return res
}

// #endregion get

// #region handoff

// Handoff は、代役が預かっているぶんを戻ってきた担当へ渡す。
//
// これが済むまで、値は担当の上に無い。読みは担当にしか聞かないので、
// R + W > N でも古い値が返りうる。緩めた quorum が手放したのはこの重なりになる。
func (c *Cluster) Handoff() int {
	moved := 0
	for _, name := range c.names {
		n := c.nodes[name]
		var keep []Hint
		for _, h := range n.hints {
			if c.down[h.Owner] {
				keep = append(keep, h)
				continue
			}
			c.nodes[h.Owner].put(h.Key, h.Value)
			moved++
			c.logf(name + " が預かっていた " + h.Key + " を " + h.Owner + " へ渡した")
		}
		n.hints = keep
	}
	return moved
}

// #endregion handoff

// Stale は、担当のうち最新を持っていない台を返す(観測用)。
func (c *Cluster) Stale(key string) []string {
	best := Value{}
	for _, n := range c.Home(key) {
		v, _ := c.nodes[n].Get(key)
		best = Newer(best, v)
	}
	var out []string
	for _, n := range c.Home(key) {
		if v, _ := c.nodes[n].Get(key); v.Stamp < best.Stamp {
			out = append(out, n)
		}
	}
	return out
}

func (c *Cluster) logf(msg string) { c.Log = append(c.Log, msg) }

func itoa(n int) string { return strconv.Itoa(n) }
