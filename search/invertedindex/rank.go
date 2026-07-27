package invertedindex

import (
	"math"
	"sort"
)

// rank.go はブール検索の「含む/含まない」の先——どの文書がより関連するか——を点数づけする。
//
// 直感は 2 つの頻度の掛け算にある。TF(term frequency): その文書に何度も出る語は、その文書に
// とって重要。IDF(inverse document frequency): どの文書にも出る語(the や です)は判別に
// 役立たず、珍しい語ほど効く。TF-IDF はこの 2 つの積で、BM25 はその実戦版だ。
// BM25 は TF の効きに飽和(何十回出ても頭打ち)を入れ、文書の長さで正規化する。

// Hit はランキング結果の 1 件。
type Hit struct {
	DocID int
	Score float64
}

// #region tfidf

// IDF は語の珍しさ。log(N / df)。全文書に出る語は 0 に近づき、珍しい語ほど大きい。
func (idx *Index) IDF(term string) float64 {
	df := idx.DF(term)
	if df == 0 {
		return 0
	}
	return math.Log(float64(idx.NumDocs()) / float64(df))
}

// SearchTFIDF はクエリの各語について TF × IDF を文書ごとに足し込み、スコア降順で返す。
// ポスティングリストを走査するだけで、全文書を見ない(スコアが付くのは語を含む文書だけ)。
func (idx *Index) SearchTFIDF(terms ...string) []Hit {
	scores := map[int]float64{}
	for _, t := range terms {
		idf := idx.IDF(t)
		for _, p := range idx.Postings(t) {
			scores[p.DocID] += float64(p.TF) * idf
		}
	}
	return sortHits(scores)
}

// #endregion tfidf

// #region bm25

// BM25 のパラメータ。k1 は TF の飽和の強さ(大きいほど TF が効き続ける)、
// b は文書長正規化の強さ(1 で完全正規化、0 で無視)。1.2 / 0.75 が定番の既定値。
const (
	k1 = 1.2
	b  = 0.75
)

// SearchBM25 は BM25 でスコアづけする。TF-IDF との違いは 2 つ。
//
//  1. TF の飽和: TF-IDF は出現 100 回なら 100 倍効くが、BM25 は tf/(tf+k1) の形で頭打ちに
//     なる。「2 回出る」と「100 回出る」の差は、「0 回」と「2 回」の差より小さい。
//  2. 文書長の正規化: 長い文書は偶然どんな語でも含みやすい。平均文書長との比で TF を
//     割り引き、短い文書での出現を相対的に重く見る。
//
// IDF も +0.5 平滑化入りの形を使う(BM25 の標準形)。
func (idx *Index) SearchBM25(terms ...string) []Hit {
	n := float64(idx.NumDocs())
	if n == 0 {
		return nil
	}
	var totalLen float64
	for i := 0; i < idx.NumDocs(); i++ {
		totalLen += float64(idx.DocLen(i))
	}
	avgLen := totalLen / n

	scores := map[int]float64{}
	for _, t := range terms {
		df := float64(idx.DF(t))
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, p := range idx.Postings(t) {
			tf := float64(p.TF)
			norm := 1 - b + b*float64(idx.DocLen(p.DocID))/avgLen
			scores[p.DocID] += idf * (tf * (k1 + 1)) / (tf + k1*norm)
		}
	}
	return sortHits(scores)
}

// #endregion bm25

// sortHits はスコア降順(同点は DocID 昇順)に整列する。決定的な順序にするため。
func sortHits(scores map[int]float64) []Hit {
	hits := make([]Hit, 0, len(scores))
	for id, s := range scores {
		hits = append(hits, Hit{DocID: id, Score: s})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].DocID < hits[j].DocID
	})
	return hits
}
