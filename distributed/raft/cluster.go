package raft

import (
	"math/rand"
	"sort"
)

// Cluster は複数の Raft ノードを1プロセス上で束ねる決定的シミュレータ。
// 実ネットワークの代わりにメモリ上でメッセージを配送し、分断や孤立を注入できる。
// 実時間に一切依存しない(Step/Tick 駆動)ので、選挙の競合も分断も確実に再現できる。
type Cluster struct {
	nodes   map[uint64]*Raft
	applied map[uint64][]Entry // 各ノードが状態機械へ適用済みのエントリ列(整合性検証用)
	// groups は到達可能な島の分割。同じ島の中だけメッセージが届く。
	// nil のときは全員1つの島(全通信可能)。
	groups [][]uint64
}

// NewCluster は n 台(ID=1..n)のクラスタを作る。seed で乱数を固定でき、テストが再現可能になる。
func NewCluster(n int, electionTick, heartbeatTick int, seed int64) *Cluster {
	peers := make([]uint64, n)
	for i := range peers {
		peers[i] = uint64(i + 1)
	}
	c := &Cluster{nodes: map[uint64]*Raft{}, applied: map[uint64][]Entry{}}
	for _, id := range peers {
		c.nodes[id] = NewRaft(Config{
			ID: id, Peers: peers,
			ElectionTick: electionTick, HeartbeatTick: heartbeatTick,
			// ノードごとに別種を与え、選挙タイムアウトがばらけて split vote を避けられるようにする
			Rand: rand.New(rand.NewSource(seed + int64(id))),
		})
	}
	return c
}

// Node は ID を指定してノードを取り出す。
func (c *Cluster) Node(id uint64) *Raft { return c.nodes[id] }

// Tick は全ノードの論理時計を1つ進め、その結果生じたメッセージを配送しきる。
func (c *Cluster) Tick() {
	for _, id := range c.ids() {
		c.nodes[id].Tick()
	}
	c.deliver()
}

// TickN は Tick を n 回まとめて進める。
func (c *Cluster) TickN(n int) {
	for range n {
		c.Tick()
	}
}

// deliver は届けられるメッセージが尽きるまで配送を繰り返す(1 Tick 内で決着させる)。
func (c *Cluster) deliver() {
	for range 10000 { // 無限ループ保険
		var batch []Message
		for _, id := range c.ids() {
			for _, m := range c.nodes[id].TakeMessages() {
				batch = append(batch, m)
			}
			c.drainApplied(id)
		}
		if len(batch) == 0 {
			return
		}
		for _, m := range batch {
			if c.reachable(m.From, m.To) {
				if dst := c.nodes[m.To]; dst != nil {
					dst.Step(m)
				}
			}
		}
	}
}

// drainApplied は確定エントリを取り出してそのノードの適用列に足す。
func (c *Cluster) drainApplied(id uint64) {
	for _, e := range c.nodes[id].TakeApplied() {
		if e.Type == EntryNormal && e.Data != nil {
			c.applied[id] = append(c.applied[id], e)
		}
	}
}

// Propose は現在のリーダーに書き込みを提案する。リーダー不在なら false。
func (c *Cluster) Propose(data []byte) bool {
	if l := c.Leader(); l != nil {
		if l.Propose(data) == nil {
			c.deliver()
			return true
		}
	}
	return false
}

// Leader は現在のリーダー(1台に収束していれば)を返す。いなければ nil。
func (c *Cluster) Leader() *Raft {
	var lead *Raft
	for _, id := range c.ids() {
		if n := c.nodes[id]; n.State() == Leader {
			if lead != nil && lead.Term() >= n.Term() {
				continue
			}
			lead = n
		}
	}
	return lead
}

// --- 分断の注入 ---

// Partition は与えた島に分割する。例: Partition([]uint64{1,2}, []uint64{3,4,5})。
func (c *Cluster) Partition(groups ...[]uint64) { c.groups = groups }

// Isolate は1台を他の全員から切り離す(それ以外は互いに通信可能)。
func (c *Cluster) Isolate(id uint64) {
	var rest []uint64
	for _, x := range c.ids() {
		if x != id {
			rest = append(rest, x)
		}
	}
	c.groups = [][]uint64{{id}, rest}
}

// Heal はすべての分断を取り除き、全員を再接続する。
func (c *Cluster) Heal() { c.groups = nil }

// reachable は from→to にメッセージが届くか(同じ島にいるか)。
func (c *Cluster) reachable(from, to uint64) bool {
	if c.groups == nil {
		return true
	}
	for _, g := range c.groups {
		var hasFrom, hasTo bool
		for _, id := range g {
			if id == from {
				hasFrom = true
			}
			if id == to {
				hasTo = true
			}
		}
		if hasFrom && hasTo {
			return true
		}
	}
	return false
}

func (c *Cluster) ids() []uint64 {
	out := make([]uint64, 0, len(c.nodes))
	for id := range c.nodes {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
