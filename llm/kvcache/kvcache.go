// Package kvcache は LLM の推論高速化の 2 本柱、KV キャッシュと
// speculative decoding を最小構成で実装する。
//
// 生成は 1 トークンずつ進み、各ステップで過去全トークンの K/V が要る。
// これを毎回作り直すとステップあたりのコストが系列長に比例して伸びる(合計二次)。
// K/V は位置ごとに不変なので、一度作って保存すれば新トークンぶんだけ作れば
// よくなる(合計線形)。これが KV キャッシュ。
//
// speculative decoding は別の軸の高速化で、軽いドラフトモデルに数トークン
// 先読みさせ、本命モデルは 1 回のパスでまとめて検証する。本命の選択と一致した
// 分だけ一気に進み、生成結果は本命だけで生成した場合と完全に一致する。
package kvcache

import "math"

// #region cache

// KV は 1 位置ぶんのキー・バリューベクトル。
type KV struct{ K, V []float64 }

// Cache は位置順に並んだ K/V の列。生成が進むたび末尾に足すだけで、
// 過去の行は二度と計算し直さない。
type Cache struct{ rows []KV }

func (c *Cache) Append(kv KV) { c.rows = append(c.rows, kv) }
func (c *Cache) Len() int     { return len(c.rows) }

// #endregion cache

// Model は 1 ヘッドの causal attention だけでできた決定的なおもちゃの言語モデル。
// K/V 射影の回数を数えることで、キャッシュの効果を実測できるようにしている。
type Model struct {
	dim   int
	vocab int
	wk    []float64
	wv    []float64
}

// NewModel は dim 次元・語彙 vocab の決定的モデルを作る。
func NewModel(dim, vocab int) *Model {
	m := &Model{dim: dim, vocab: vocab}
	m.wk = seededVec(dim, 17)
	m.wv = seededVec(dim, 29)
	return m
}

// emb はトークン ID から決定的な埋め込みを作る。
func (m *Model) emb(tok int) []float64 { return seededVec(m.dim, uint64(1000+tok)) }

// project は 1 位置ぶんの K/V を作る(要素積の簡略射影)。ここの呼び出し回数が
// 「K/V を何回作ったか」の計測点になる。
func (m *Model) project(e []float64) KV {
	k := make([]float64, m.dim)
	v := make([]float64, m.dim)
	for i := range e {
		k[i] = e[i] * m.wk[i]
		v[i] = e[i] * m.wv[i]
	}
	return KV{K: k, V: v}
}

// attend は末尾トークンを query に、キャッシュ全行へ注目して次トークンを選ぶ。
func (m *Model) attend(cache *Cache, lastTok int) int {
	q := m.emb(lastTok)
	scores := make([]float64, cache.Len())
	maxS := math.Inf(-1)
	for i, kv := range cache.rows {
		s := dot(q, kv.K) / math.Sqrt(float64(m.dim))
		scores[i] = s
		if s > maxS {
			maxS = s
		}
	}
	sum := 0.0
	for i := range scores {
		scores[i] = math.Exp(scores[i] - maxS)
		sum += scores[i]
	}
	out := make([]float64, m.dim)
	for i, kv := range cache.rows {
		w := scores[i] / sum
		for j := range out {
			out[j] += w * kv.V[j]
		}
	}
	// 語彙全体と照らして最も近いトークンを選ぶ(greedy)。
	best, bestS := 0, math.Inf(-1)
	for t := 0; t < m.vocab; t++ {
		if s := dot(out, m.emb(t)); s > bestS {
			best, bestS = t, s
		}
	}
	return best
}

// #region generate

// GenerateNoCache はキャッシュを使わず、毎ステップ文脈全体の K/V を作り直す。
// 戻り値は (生成込みの列, K/V 射影の回数)。射影回数は系列長の二次で伸びる。
func (m *Model) GenerateNoCache(prompt []int, n int) ([]int, int) {
	seq := append([]int(nil), prompt...)
	ops := 0
	for step := 0; step < n; step++ {
		cache := &Cache{}
		for _, tok := range seq {
			cache.Append(m.project(m.emb(tok)))
			ops++
		}
		seq = append(seq, m.attend(cache, seq[len(seq)-1]))
	}
	return seq, ops
}

// GenerateWithCache は K/V を一度だけ作って使い回す。毎ステップの追加射影は
// 新しく文脈に入ったトークンの 1 回だけで、射影回数は線形になる。
// 生成列は GenerateNoCache と完全に一致する。
func (m *Model) GenerateWithCache(prompt []int, n int) ([]int, int) {
	seq := append([]int(nil), prompt...)
	cache := &Cache{}
	ops := 0
	for step := 0; step < n; step++ {
		// キャッシュに無い位置(初回はプロンプト全部、以降は直前の 1 個)だけ射影。
		for i := cache.Len(); i < len(seq); i++ {
			cache.Append(m.project(m.emb(seq[i])))
			ops++
		}
		seq = append(seq, m.attend(cache, seq[len(seq)-1]))
	}
	return seq, ops
}

// #endregion generate

// #region speculative

// Fn は「これまでの列 → 次のトークン」の greedy なモデル。
type Fn func(prefix []int) int

// GenerateGreedy は target だけで n トークン生成する(比較の基準)。
func GenerateGreedy(target Fn, prompt []int, n int) []int {
	seq := append([]int(nil), prompt...)
	for i := 0; i < n; i++ {
		seq = append(seq, target(seq))
	}
	return seq
}

// Speculative は draft に gamma トークン先読みさせ、target の 1 パスでまとめて
// 検証する。一致した分は一気に採用し、最初の不一致は target の訂正で置き換える。
// 全部一致なら同じパスからボーナスの 1 トークンも得る。
// 生成列は GenerateGreedy(target, ...) と必ず一致し、節約されるのは
// target のパス数(戻り値の 2 つ目)だけ。
func Speculative(target, draft Fn, prompt []int, n, gamma int) ([]int, int) {
	seq := append([]int(nil), prompt...)
	if gamma < 1 {
		return seq, 0
	}
	passes := 0
	remaining := n
	for remaining > 0 {
		// ドラフトが gamma 個、自分の予想で先読みする(軽い逐次計算)。
		prop := make([]int, 0, gamma)
		cur := append([]int(nil), seq...)
		for i := 0; i < gamma; i++ {
			t := draft(cur)
			prop = append(prop, t)
			cur = append(cur, t)
		}
		// 本命は 1 パスで gamma+1 位置ぶんの自分の選択を出す(実物は並列に一括)。
		passes++
		allMatched := true
		for i := 0; i < len(prop) && remaining > 0; i++ {
			want := target(seq)
			seq = append(seq, want) // 一致なら prop[i] と同じ値、不一致なら訂正
			remaining--
			if prop[i] != want {
				allMatched = false
				break
			}
		}
		if allMatched && remaining > 0 {
			seq = append(seq, target(seq)) // 同じパスから得られるボーナス
			remaining--
		}
	}
	return seq, passes
}

// #endregion speculative

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// seededVec は seed から決定的に [-1, 1) の値を敷いたベクトルを作る。
func seededVec(dim int, seed uint64) []float64 {
	out := make([]float64, dim)
	s := seed*6364136223846793005 + 1442695040888963407
	for i := range out {
		s = s*6364136223846793005 + 1442695040888963407
		out[i] = float64(s>>40)/float64(1<<23) - 1
	}
	return out
}
