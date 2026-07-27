// Package metrics はメトリクスの 3 つの型(カウンタ・ゲージ・ヒストグラム)を
// 最小構成で実装する。
//
// システムの健康は数字で見る。処理した総数、今この瞬間の同時接続数、そして
// 応答時間の分布。総数のような増える一方の値はカウンタ、同時接続数のような
// 上下する値はゲージ、応答時間のような「速いのも遅いのも混じる」値はヒストグラム。
// 特にヒストグラムが要だ。平均応答時間は速い大多数に引っ張られて、遅い少数
// (テールレイテンシ)を隠す。ユーザが怒るのはその遅い方だ。ヒストグラムは
// 値をバケットに数え、p99(下から 99% 目の値)のような分位点を推定できる。
// しかもバケットはマシン間で足し合わせられるので、全台の p99 を正しく出せる。
package metrics

// #region counter

// Counter は増える一方の値(処理総数、エラー総数など)。減らせない。
type Counter struct{ v float64 }

// Inc は 1 増やす。
func (c *Counter) Inc() { c.v++ }

// Add は d(非負)を足す。負を渡すとパニック(カウンタは減らせない)。
func (c *Counter) Add(d float64) {
	if d < 0 {
		panic("metrics: counter cannot decrease")
	}
	c.v += d
}

// Value は現在値を返す。
func (c *Counter) Value() float64 { return c.v }

// Gauge は上下する値(同時接続数、キュー長、メモリ使用量など)。
type Gauge struct{ v float64 }

// Set は値を x にする。
func (g *Gauge) Set(x float64) { g.v = x }

// Add は d 足す(負でもよい)。
func (g *Gauge) Add(d float64) { g.v += d }

// Sub は d 引く。
func (g *Gauge) Sub(d float64) { g.v -= d }

// Value は現在値を返す。
func (g *Gauge) Value() float64 { return g.v }

// #endregion counter

// #region histogram

// Histogram は値の分布を、上限の決まったバケットに数える(Prometheus 方式)。
// bounds は各バケットの上限(昇順)。末尾に暗黙の +Inf バケットが 1 つ付く。
type Histogram struct {
	bounds []float64 // バケットの上限(昇順)
	counts []uint64  // 各バケットの個数。len == len(bounds)+1(末尾は +Inf)
	sum    float64   // 全観測値の合計(平均のため)
	total  uint64    // 全観測数
}

// NewHistogram は上限の並びからヒストグラムを作る。
// 例: {10, 50, 100, 500} なら (-∞,10] (10,50] (50,100] (100,500] (500,+∞) の 5 バケット。
func NewHistogram(bounds []float64) *Histogram {
	b := make([]float64, len(bounds))
	copy(b, bounds)
	return &Histogram{bounds: b, counts: make([]uint64, len(bounds)+1)}
}

// Observe は値 x を該当バケットに 1 つ数える。
func (h *Histogram) Observe(x float64) {
	i := 0
	for i < len(h.bounds) && x > h.bounds[i] {
		i++ // x が上限を超える間だけ次のバケットへ
	}
	h.counts[i]++
	h.sum += x
	h.total++
}

// Count は全観測数を返す。
func (h *Histogram) Count() uint64 { return h.total }

// Sum は全観測値の合計を返す。
func (h *Histogram) Sum() float64 { return h.sum }

// Mean は平均を返す(観測なしは 0)。
func (h *Histogram) Mean() float64 {
	if h.total == 0 {
		return 0
	}
	return h.sum / float64(h.total)
}

// Buckets は (上限, 累積個数) の並びを返す(観測用。末尾 +Inf は上限を返さない)。
func (h *Histogram) Buckets() ([]float64, []uint64) {
	return h.bounds, h.counts
}

// #endregion histogram

// #region quantile

// Quantile は分位点 q(0..1)を、バケット内を線形補間して推定する。
// 生の値を保存しないので厳密値ではないが、バケットが細かいほど誤差は小さい。
// 例: q=0.99 なら「下から 99% 目」= p99。
func (h *Histogram) Quantile(q float64) float64 {
	if h.total == 0 {
		return 0
	}
	if q <= 0 {
		return 0
	}
	if q >= 1 {
		q = 1
	}
	// 下から rank 番目の値を探す。
	rank := q * float64(h.total)
	var cum uint64
	for i := 0; i < len(h.counts); i++ {
		next := cum + h.counts[i]
		if float64(next) >= rank {
			// このバケットに rank 番目がある。
			if i == len(h.bounds) {
				// +Inf バケット。上限が無いので最大の有限上限で頭打ち。
				return h.bounds[len(h.bounds)-1]
			}
			lower := 0.0
			if i > 0 {
				lower = h.bounds[i-1]
			}
			upper := h.bounds[i]
			// バケット内で rank の位置を線形補間。
			inBucket := rank - float64(cum)
			frac := inBucket / float64(h.counts[i])
			return lower + (upper-lower)*frac
		}
		cum = next
	}
	return h.bounds[len(h.bounds)-1]
}

// Merge は同じ bounds を持つ別のヒストグラムをバケットごとに足し込む。
// これがヒストグラムの肝。各マシンのバケットを足すだけで全台の分布になり、
// そこから全台の p99 を正しく出せる(各台の p99 を平均しても全体の p99 にならない)。
func (h *Histogram) Merge(o *Histogram) {
	if len(h.counts) != len(o.counts) {
		panic("metrics: histogram bounds mismatch")
	}
	for i := range h.counts {
		h.counts[i] += o.counts[i]
	}
	h.sum += o.sum
	h.total += o.total
}

// #endregion quantile
