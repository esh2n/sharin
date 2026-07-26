// Package replication は単一リーダーのログシッピング複製の最小実装。
//
// リーダー(primary)が書き込みログを持ち、それをフォロワー(replica)へ順番に流す。
// レコードはべき等(絶対値で置く。wal.Set と同じ考え)なので、フォロワーは受け取った順に
// 適用するだけでよい。
//
// この実装で目に見えるようにするのは3つ:
//
//   - 耐久性ポリシー(Async / Quorum / Sync): 書き込みを「確定」と見なすのに
//     何台の複製を待つか。速さと無損失のトレードオフ。
//   - 複製ラグと stale read: 遅れているレプリカから読むと、自分の書き込みが見えない。
//   - フェイルオーバー時のデータ損失窓: Async でリーダーが落ち、遅れたレプリカを
//     昇格させると、未複製の確定済み書き込みが消える。
//
// ネットワークや goroutine は使わず、明示的な操作で駆動する純粋なモデルにしてあるので、
// テストもデモも決定的に再現できる(distributed/raft の cluster.go と同じ方針)。
package replication

import (
	"errors"
	"fmt"
)

// Durability はリーダーが書き込みを「確定(クライアントへ ack)」と見なす条件。
type Durability int

const (
	// Async は複製を待たず即確定する。速いが、リーダー故障で未複製ぶんが消えうる。
	Async Durability = iota
	// Quorum は過半数のノード(リーダー込み)が持ったら確定する(準同期)。
	// 少数の遅い/落ちたレプリカを許容しつつ、昇格先が最新なら無損失。
	Quorum
	// Sync は全ノードが持ったら確定する。無損失だが、1台でも落ちると書き込みが止まる。
	Sync
)

func (d Durability) String() string {
	switch d {
	case Async:
		return "async"
	case Quorum:
		return "quorum"
	case Sync:
		return "sync"
	default:
		return fmt.Sprintf("Durability(%d)", int(d))
	}
}

// Record は1件のべき等な書き込み。「+100する」ではなく「値を V にする」と絶対値で持つ。
type Record struct {
	Offset int // リーダーログ内の1始まり位置(LSN)
	Key    string
	Value  int64
}

// node はリーダーにもレプリカにもなる1台。log はそのノードが持つ複製ログ。
type node struct {
	id    int
	log   []Record
	store map[string]int64
}

func newNode(id int) *node {
	return &node{id: id, store: map[string]int64{}}
}

// apply は1件を末尾に足して store に反映する(べき等)。
func (n *node) apply(r Record) {
	n.log = append(n.log, r)
	n.store[r.Key] = r.Value
}

// applied はこのノードが適用済みの最大 offset(= ログ長)。
func (n *node) applied() int { return len(n.log) }

// Cluster は1台のリーダーと複数のレプリカからなる複製系。
type Cluster struct {
	durability Durability
	primary    *node
	replicas   []*node
	reachable  map[int]bool // レプリカ id -> リーダーと繋がっているか
	committed  int          // 耐久性条件を満たした最大 offset(確定済み)
}

// NewCluster はレプリカ replicas 台の複製系を作る。id は primary=0、レプリカ=1..replicas。
func NewCluster(replicas int, d Durability) (*Cluster, error) {
	if replicas < 1 {
		return nil, errors.New("replication: replicas must be >= 1")
	}
	c := &Cluster{
		durability: d,
		primary:    newNode(0),
		reachable:  map[int]bool{},
	}
	for i := 1; i <= replicas; i++ {
		c.replicas = append(c.replicas, newNode(i))
		c.reachable[i] = true
	}
	return c, nil
}

// nodes はリーダー込みの総台数。
func (c *Cluster) nodes() int { return 1 + len(c.replicas) }

// #region durability
// need は現在の耐久性ポリシーで確定に必要な「そのレコードを持つノード数」(リーダー込み)。
func (c *Cluster) need() int {
	switch c.durability {
	case Async:
		return 1 // リーダーだけでよい
	case Quorum:
		return c.nodes()/2 + 1
	case Sync:
		return c.nodes()
	default:
		return c.nodes()
	}
}

// #endregion durability

// find は id のレプリカを返す(無ければ nil)。
func (c *Cluster) find(id int) *node {
	for _, rp := range c.replicas {
		if rp.id == id {
			return rp
		}
	}
	return nil
}

// shipTo はレプリカに欠けているぶんをリーダーログの末尾まで流す。
func (c *Cluster) shipTo(rp *node) {
	for i := rp.applied(); i < c.primary.applied(); i++ {
		rp.apply(c.primary.log[i])
	}
}

// acksFor は offset off のレコードを持つノード数(リーダー込み)。
// off は低いほど多くのノードが持つので、off について非増加。
func (c *Cluster) acksFor(off int) int {
	acks := 1 // リーダーは常に持つ
	for _, rp := range c.replicas {
		if rp.applied() >= off {
			acks++
		}
	}
	return acks
}

// recomputeCommitted は耐久性条件を満たす最大 offset まで committed を進める。
// acksFor は off について非増加なので、上から下へ探して最初に満たす所が確定境界。
func (c *Cluster) recomputeCommitted() {
	need := c.need()
	for off := c.primary.applied(); off > c.committed; off-- {
		if c.acksFor(off) >= need {
			c.committed = off
			return
		}
	}
}

// Write はリーダーに1件書き込み、到達可能なレプリカへ複製し、
// この書き込みが確定したか(耐久性条件を満たしたか)と、その offset を返す。
func (c *Cluster) Write(key string, value int64) (committed bool, offset int) {
	off := c.primary.applied() + 1
	c.primary.apply(Record{Offset: off, Key: key, Value: value})
	for _, rp := range c.replicas {
		if c.reachable[rp.id] {
			c.shipTo(rp)
		}
	}
	c.recomputeCommitted()
	return c.committed >= off, off
}

// Disconnect はレプリカをリーダーから切り離す(以降 Write は届かない=ラグが開く)。
func (c *Cluster) Disconnect(id int) error {
	if c.find(id) == nil {
		return fmt.Errorf("replication: unknown replica %d", id)
	}
	c.reachable[id] = false
	return nil
}

// Connect はレプリカを再接続し、欠けているぶんを一気に追いつかせる。
// 追いついた結果として確定が進むこともあるので committed を計算し直す。
func (c *Cluster) Connect(id int) error {
	rp := c.find(id)
	if rp == nil {
		return fmt.Errorf("replication: unknown replica %d", id)
	}
	c.reachable[id] = true
	c.shipTo(rp)
	c.recomputeCommitted()
	return nil
}

// Promote はレプリカ id を新リーダーに昇格させる(フェイルオーバー)。
// 昇格したレプリカが持たない offset はこの世から消える。返り値は「消えた確定済み書き込みの数」
// = データ損失窓。Async では未複製の確定ぶんがここで失われる。
func (c *Cluster) Promote(id int) (lostCommitted int, err error) {
	rp := c.find(id)
	if rp == nil {
		return 0, fmt.Errorf("replication: unknown replica %d", id)
	}
	// #region loss
	// 昇格したノードが持たない offset は消える。確定済みだったぶんの数がデータ損失窓。
	lost := c.committed - rp.applied()
	if lost < 0 {
		lost = 0
	}
	// #endregion loss
	// 昇格したノードを新リーダーにし、他のレプリカはその配下に残す。
	others := make([]*node, 0, len(c.replicas)-1)
	for _, other := range c.replicas {
		if other.id != rp.id {
			others = append(others, other)
		}
	}
	c.primary = rp
	c.replicas = others
	c.reachable = map[int]bool{}
	// 新リーダーの持たない offset は消えた。確定境界も新リーダーのログ長まで切り下げる。
	c.committed = rp.applied()
	// 他のレプリカを新リーダーに合わせる(新リーダーのログが正。長い側は切り詰める)。
	for _, other := range c.replicas {
		c.reachable[other.id] = true
		if other.applied() > c.primary.applied() {
			other.log = other.log[:c.primary.applied()]
			other.store = map[string]int64{}
			for _, r := range other.log {
				other.store[r.Key] = r.Value
			}
		}
		c.shipTo(other)
	}
	c.recomputeCommitted()
	return lost, nil
}

// --- 観測用 ---

// Committed は確定済み(耐久性条件を満たした)最大 offset。
func (c *Cluster) Committed() int { return c.committed }

// LeaderOffset はリーダーログの末尾 offset。
func (c *Cluster) LeaderOffset() int { return c.primary.applied() }

// Lag はレプリカがリーダーからどれだけ遅れているか(未受信レコード数)。
func (c *Cluster) Lag(id int) (int, error) {
	rp := c.find(id)
	if rp == nil {
		return 0, fmt.Errorf("replication: unknown replica %d", id)
	}
	return c.primary.applied() - rp.applied(), nil
}

// LeaderValue はリーダーが持つ key の値と、存在したか。
func (c *Cluster) LeaderValue(key string) (int64, bool) {
	v, ok := c.primary.store[key]
	return v, ok
}

// ReplicaValue はレプリカ id が持つ key の値と、存在したか。
// 遅れているレプリカからの読みは stale(自分の書き込みが見えない)になりうる。
func (c *Cluster) ReplicaValue(id int, key string) (int64, bool, error) {
	rp := c.find(id)
	if rp == nil {
		return 0, false, fmt.Errorf("replication: unknown replica %d", id)
	}
	v, ok := rp.store[key]
	return v, ok, nil
}

// Replicas はレプリカの id 一覧(昇格後は減る)。
func (c *Cluster) Replicas() []int {
	ids := make([]int, len(c.replicas))
	for i, rp := range c.replicas {
		ids[i] = rp.id
	}
	return ids
}
