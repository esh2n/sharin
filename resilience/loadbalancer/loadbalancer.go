// Package loadbalancer は負荷分散の代表的な選び方を最小構成で実装する。
//
// 複数の同等なバックエンドに、どのリクエストをどこへ振るか。素朴には順番に
// 回す(ラウンドロビン)か、今いちばん空いている所へ送る(最少接続)。だが
// 最少接続は全接続数を正確に知る必要があり、分散した振り分け役が同じ「最少」を
// 見て一斉に殺到する群集効果(herd)を起こす。そこで P2C(power of two choices):
// 無作為に 2 台だけ選び、そのうち軽い方へ送る。全体を知らずとも偏りが劇的に
// 減る、という確率のいたずらを使う。一貫ハッシュは、同じキーを常に同じ台へ
// 振り、台の増減で振り先が最小限しか動かないようにする。
package loadbalancer

import "sort"

// #region rand

// Rand は決定的な擬似乱数源(テスト再現性のため実 rand を使わない)。
type Rand struct{ state uint64 }

// NewRand は seed から擬似乱数源を作る。
func NewRand(seed uint64) *Rand { return &Rand{state: seed*2862933555777941757 + 1} }

// intn は [0, n) の擬似乱数を返す。
func (r *Rand) intn(n int) int {
	if n <= 0 {
		return 0
	}
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return int((r.state >> 33) % uint64(n))
}

// #endregion rand

// Backend は振り分け先の 1 台。active は処理中(in-flight)のリクエスト数。
type Backend struct {
	ID     string
	active int
}

// Active は処理中のリクエスト数を返す。
func (b *Backend) Active() int { return b.active }

// Strategy は振り分けの方式。
type Strategy int

const (
	RoundRobin     Strategy = iota // 順番に回す。状態を見ない
	LeastConn                      // 最少接続。全台を見て最も空いた台へ
	P2C                            // 2 台を無作為に選び、軽い方へ
	ConsistentHash                 // キーのハッシュで常に同じ台へ
)

func (s Strategy) String() string {
	switch s {
	case RoundRobin:
		return "round-robin"
	case LeastConn:
		return "least-conn"
	case P2C:
		return "p2c"
	case ConsistentHash:
		return "consistent-hash"
	}
	return "unknown"
}

// Balancer は複数のバックエンドへリクエストを振り分ける。
type Balancer struct {
	backends []*Backend
	strategy Strategy
	rr       int   // ラウンドロビンのカーソル
	rand     *Rand // P2C 用
	ring     []vnode
}

// vnode は一貫ハッシュのリング上の仮想ノード。
type vnode struct {
	hash    uint32
	backend int
}

// New は台数分のバックエンドを持つ Balancer を作る。r は P2C の乱数源。
func New(ids []string, strategy Strategy, r *Rand) *Balancer {
	bs := make([]*Backend, len(ids))
	for i, id := range ids {
		bs[i] = &Backend{ID: id}
	}
	b := &Balancer{backends: bs, strategy: strategy, rand: r}
	if strategy == ConsistentHash {
		b.buildRing(replicas)
	}
	return b
}

// Backends は内部のバックエンド一覧を返す(観測用)。
func (b *Balancer) Backends() []*Backend { return b.backends }

// #region pick

// Pick は方式に従って振り先のインデックスを選ぶ。
// key は一貫ハッシュでのみ使う(他方式は無視)。
func (b *Balancer) Pick(key string) int {
	n := len(b.backends)
	if n == 0 {
		return -1
	}
	switch b.strategy {
	case RoundRobin:
		i := b.rr % n
		b.rr++
		return i

	case LeastConn:
		// 全台を見て、最も active が少ない台。同数なら若い番号。
		best := 0
		for i := 1; i < n; i++ {
			if b.backends[i].active < b.backends[best].active {
				best = i
			}
		}
		return best

	case P2C:
		if n == 1 {
			return 0
		}
		// 異なる 2 台を無作為に選び、active の少ない方(同数なら先に引いた方)。
		i := b.rand.intn(n)
		j := b.rand.intn(n - 1)
		if j >= i {
			j++ // i を除いた n-1 台から選ぶ
		}
		if b.backends[j].active < b.backends[i].active {
			return j
		}
		return i

	case ConsistentHash:
		return b.pickRing(key)
	}
	return 0
}

// Acquire は i 番の台の処理中カウントを 1 増やす(リクエスト受付)。
func (b *Balancer) Acquire(i int) { b.backends[i].active++ }

// Release は i 番の台の処理中カウントを 1 減らす(処理完了)。
func (b *Balancer) Release(i int) {
	if b.backends[i].active > 0 {
		b.backends[i].active--
	}
}

// #endregion pick

// #region consistent

// replicas は 1 台あたりのリング上の仮想ノード数。多いほど分散が均等になる。
const replicas = 64

// hash32 は FNV-1a による 32bit ハッシュ(外部依存を避け自前で書く)。
func hash32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// buildRing は各台を replicas 個の仮想ノードとしてリング上に配置し、ソートする。
func (b *Balancer) buildRing(rep int) {
	b.ring = b.ring[:0]
	for i, be := range b.backends {
		for v := 0; v < rep; v++ {
			key := be.ID + "#" + itoa(v)
			b.ring = append(b.ring, vnode{hash: hash32(key), backend: i})
		}
	}
	sort.Slice(b.ring, func(x, y int) bool { return b.ring[x].hash < b.ring[y].hash })
}

// pickRing は key のハッシュ以上で最も近い仮想ノードの台を返す(円環なので回り込む)。
func (b *Balancer) pickRing(key string) int {
	if len(b.ring) == 0 {
		return 0
	}
	h := hash32(key)
	// h 以上の最初の点を二分探索。無ければ先頭(円環の回り込み)。
	idx := sort.Search(len(b.ring), func(i int) bool { return b.ring[i].hash >= h })
	if idx == len(b.ring) {
		idx = 0
	}
	return b.ring[idx].backend
}

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// #endregion consistent
