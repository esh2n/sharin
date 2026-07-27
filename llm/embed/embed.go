// Package embed は文の埋め込みとベクトル検索を最小構成で実装する。
//
// LLM の隠れ状態は「意味のベクトル」で、近い意味の文は近いベクトルになる。
// この性質を使うのが埋め込み検索(意味検索)で、クエリをベクトルにして、
// 蓄えたベクトルの中からコサイン類似度が高いものを返す。RAG(検索して
// 文脈に足す)や推薦、重複検出の土台になる。ここでは、トークン埋め込みを
// 平均して文ベクトルにするプーリング、コサイン類似度、総当たり検索を作り、
// 件数が増えると総当たりが破綻して近似最近傍(ANN)が要る理由まで示す。
package embed

import (
	"math"
	"sort"
)

// #region vector

// MeanPool はトークン埋め込みの列を平均して 1 本の文ベクトルにする。
// 最も素朴なプーリング。実物は attention 重み付き平均や [CLS] トークンも使う。
func MeanPool(tokens [][]float64) []float64 {
	if len(tokens) == 0 {
		return nil
	}
	dim := len(tokens[0])
	out := make([]float64, dim)
	for _, t := range tokens {
		for i := range out {
			out[i] += t[i]
		}
	}
	for i := range out {
		out[i] /= float64(len(tokens))
	}
	return out
}

// Cosine はコサイン類似度 a·b / (|a||b|)。向きだけを見て長さに依存しない。
// 同じ向き 1、直交 0、逆向き -1。ゼロベクトルは 0 を返す。
func Cosine(a, b []float64) float64 {
	dot, na, nb := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Normalize は長さ 1 のベクトルにする。正規化済みベクトルどうしの内積は
// コサイン類似度に等しくなるので、大量検索では前もって正規化しておくと速い。
func Normalize(v []float64) []float64 {
	n := 0.0
	for _, x := range v {
		n += x * x
	}
	out := make([]float64, len(v))
	if n == 0 {
		return out
	}
	inv := 1 / math.Sqrt(n)
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

// #endregion vector

// #region store

// Hit は検索結果の 1 件(キーと類似度スコア)。
type Hit struct {
	Key   string
	Score float64
}

// entry は蓄えた 1 件の埋め込み。
type entry struct {
	key string
	vec []float64
}

// Store は埋め込みの集まり。総当たりでコサイン類似度の上位を返す。
type Store struct {
	items []entry
}

// NewStore は空のストアを作る。
func NewStore() *Store { return &Store{} }

// Add はキーと埋め込みを 1 件追加する。
func (s *Store) Add(key string, vec []float64) {
	s.items = append(s.items, entry{key: key, vec: vec})
}

// Search はクエリに最も近い上位 k 件をコサイン類似度の降順で返す。
func (s *Store) Search(query []float64, k int) []Hit {
	hits, _ := s.SearchCounted(query, k)
	return hits
}

// SearchCounted は Search に加え、比較したベクトル数(= 総当たりの計算量)を返す。
// 蓄積件数にそのまま比例することを見せるための計測点。
func (s *Store) SearchCounted(query []float64, k int) ([]Hit, int) {
	hits := make([]Hit, len(s.items))
	for i, e := range s.items {
		hits[i] = Hit{Key: e.key, Score: Cosine(query, e.vec)}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].Score > hits[b].Score })
	if k > len(hits) {
		k = len(hits)
	}
	if k < 0 {
		k = 0
	}
	return hits[:k], len(s.items)
}

// #endregion store
