// Package moe は Mixture of Experts(専門家の混合)を最小構成で実装する。
//
// FFN はパラメータの大半を占める(transformer 編)。MoE はこの FFN を複数の
// 「expert」に分割し、トークンごとにルータが上位 k 個だけを選んで通す。
// 総パラメータは expert 数ぶん巨大になるが、1 トークンあたりの計算は k 個ぶん
// で済む。「大きいのに速い」の正体は、この総量とアクティブ量の分離にある。
// 学習では特定の expert への集中(崩壊)を防ぐため、負荷分散の補助損失を足す。
package moe

import (
	"errors"
	"math"
	"sort"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region config

// Config は MoE 層の構成。
type Config struct {
	DModel   int // 埋め込み次元
	DHidden  int // expert FFN の中間次元
	NExperts int // expert の総数
	TopK     int // 1 トークンが使う expert 数
}

// TotalParams はルータ + 全 expert のパラメータ数(メモリに載る総量)。
func (c Config) TotalParams() int {
	return c.DModel*c.NExperts + c.NExperts*c.expertParams()
}

// ActiveParams は 1 トークンの計算に使われるパラメータ数(計算コスト側)。
// 総量が NExperts 倍でも、こちらは TopK 倍にしかならない。
func (c Config) ActiveParams() int {
	return c.DModel*c.NExperts + c.TopK*c.expertParams()
}

func (c Config) expertParams() int { return 2 * c.DModel * c.DHidden }

// #endregion config

// MoE はルータと expert FFN 群を持つ 1 層。
type MoE struct {
	cfg     Config
	router  *tensor.Tensor // (DModel, NExperts) トークン → expert スコア
	experts []*ffn
}

type ffn struct {
	w1 *tensor.Tensor // (DModel, DHidden)
	w2 *tensor.Tensor // (DHidden, DModel)
}

// New は構成を検証して MoE 層を作る。重みは決定的な擬似乱数。
func New(cfg Config) (*MoE, error) {
	if cfg.DModel <= 0 || cfg.DHidden <= 0 || cfg.NExperts <= 0 {
		return nil, errors.New("moe: sizes must be positive")
	}
	if cfg.TopK < 1 || cfg.TopK > cfg.NExperts {
		return nil, errors.New("moe: TopK must be in [1, NExperts]")
	}
	m := &MoE{cfg: cfg, router: randMatrix(cfg.DModel, cfg.NExperts, 7)}
	for e := 0; e < cfg.NExperts; e++ {
		m.experts = append(m.experts, &ffn{
			w1: randMatrix(cfg.DModel, cfg.DHidden, uint64(400+e)),
			w2: randMatrix(cfg.DHidden, cfg.DModel, uint64(500+e)),
		})
	}
	return m, nil
}

// #region route

// Assignment は 1 トークンに割り当てられた expert と混合の重み。
type Assignment struct {
	Expert int
	Weight float32
}

// Route は 1 トークンの埋め込みからルータのスコアを出し、上位 TopK 個を選ぶ。
// 重みは選ばれた TopK 個のスコアだけで softmax を取り直す(選外は 0 扱い)。
func (m *MoE) Route(x []float32) []Assignment {
	scores := make([]float32, m.cfg.NExperts)
	for e := 0; e < m.cfg.NExperts; e++ {
		s := float32(0)
		for i, v := range x {
			s += v * m.router.At(i, e)
		}
		scores[e] = s
	}
	order := make([]int, len(scores))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })

	top := order[:m.cfg.TopK]
	maxS := scores[top[0]]
	sum := float32(0)
	weights := make([]float32, len(top))
	for i, e := range top {
		weights[i] = float32(math.Exp(float64(scores[e] - maxS)))
		sum += weights[i]
	}
	out := make([]Assignment, len(top))
	for i, e := range top {
		out[i] = Assignment{Expert: e, Weight: weights[i] / sum}
	}
	return out
}

// #endregion route

// #region forward

// Stats は 1 回の Forward で各 expert が受け取ったトークン数。
type Stats struct {
	TokensPerExpert []int
}

// Forward は各トークンをルーティングし、選ばれた expert の出力を重み付きで
// 混ぜる。計算されるのはトークンごとに TopK 個の expert だけ。
func (m *MoE) Forward(x *tensor.Tensor) (*tensor.Tensor, *Stats) {
	out := tensor.New(x.Rows, x.Cols)
	stats := &Stats{TokensPerExpert: make([]int, m.cfg.NExperts)}
	for r := 0; r < x.Rows; r++ {
		row := make([]float32, x.Cols)
		for c := 0; c < x.Cols; c++ {
			row[c] = x.At(r, c)
		}
		for _, a := range m.Route(row) {
			stats.TokensPerExpert[a.Expert]++
			y := m.expertRow(a.Expert, row)
			for c := 0; c < x.Cols; c++ {
				out.Set(r, c, out.At(r, c)+a.Weight*y[c])
			}
		}
	}
	return out, stats
}

// #endregion forward

// ExpertForward は指定 expert の FFN を全行に適用する(検証用)。
func (m *MoE) ExpertForward(e int, x *tensor.Tensor) *tensor.Tensor {
	h := tensor.GELU(tensor.MatMul(x, m.experts[e].w1))
	return tensor.MatMul(h, m.experts[e].w2)
}

func (m *MoE) expertRow(e int, row []float32) []float32 {
	x := tensor.New(1, len(row))
	copy(x.Data, row)
	y := m.ExpertForward(e, x)
	return y.Data
}

// #region balance

// LoadBalanceLoss は Switch Transformer 流の負荷分散損失の簡略版。
// f_e = expert e に流れたトークンの割合として N·Σ f_e² を返す。
// 均等なら 1(最小)、1 つに集中すると N(最大)。学習ではこれを本来の損失に
// 足して、ルータが特定の expert に固執する崩壊を防ぐ。
func LoadBalanceLoss(s *Stats) float32 {
	total := 0
	for _, n := range s.TokensPerExpert {
		total += n
	}
	if total == 0 {
		return 0
	}
	sum := float32(0)
	for _, n := range s.TokensPerExpert {
		f := float32(n) / float32(total)
		sum += f * f
	}
	return float32(len(s.TokensPerExpert)) * sum
}

// #endregion balance

// randMatrix は seed から決定的に [-0.5, 0.5) の値を敷いた行列を作る。
func randMatrix(rows, cols int, seed uint64) *tensor.Tensor {
	out := tensor.New(rows, cols)
	s := seed*6364136223846793005 + 1442695040888963407
	for i := range out.Data {
		s = s*6364136223846793005 + 1442695040888963407
		out.Data[i] = float32(s>>40)/float32(1<<24) - 0.5
	}
	return out
}
