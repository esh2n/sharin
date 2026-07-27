// Package invertedindex は全文検索の心臓部である転置インデックスを最小構成で作る。
//
// 素朴に「全文書を頭から走査して語を探す」と、文書数×文書長の時間がかかる。検索エンジンは
// 逆向きの表を先に作っておく。文書 → 語の表(元の文書)ではなく、語 → 文書リストの表
// (転置インデックス)だ。検索時は語で表を引くだけで、その語を含む文書が即座に手に入る。
//
// このパッケージは 3 段で組む:
//  1. トークン化と索引構築(index.go): 文書を語に割り、語 → ポスティングリストの表を作る
//  2. ブール検索(index.go): AND はポスティングリストの積、OR は和
//  3. ランキング(rank.go): TF-IDF と BM25 で「どの文書がより関連するか」を点数づけする
package invertedindex

import (
	"sort"
	"strings"
	"unicode"
)

// #region index

// Posting は 1 つの語が 1 つの文書に現れた記録。TF(その文書内の出現回数)を持つ。
type Posting struct {
	DocID int
	TF    int
}

// Index は転置インデックス。語 → ポスティングリスト(その語を含む文書と出現回数の列)。
type Index struct {
	postings map[string][]Posting
	docLen   []int // 文書ごとの語数(ランキングで使う)
	docs     []string
}

// Tokenize は文書を語の列に割る。英数字の連なりを 1 語とし、小文字に揃える。
// 実物はステミング(walked→walk)やストップワード除去もするが、ここでは省く。
func Tokenize(text string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// Build は文書集合から転置インデックスを作る。各文書をトークン化し、
// 語ごとに「どの文書に何回現れたか」を記録する。DocID は与えた順の添字。
func Build(docs []string) *Index {
	idx := &Index{postings: map[string][]Posting{}, docs: docs}
	for id, doc := range docs {
		tokens := Tokenize(doc)
		idx.docLen = append(idx.docLen, len(tokens))
		tf := map[string]int{}
		for _, t := range tokens {
			tf[t]++
		}
		terms := make([]string, 0, len(tf))
		for t := range tf {
			terms = append(terms, t)
		}
		sort.Strings(terms) // 決定的に(マップ順に依存しない)
		for _, t := range terms {
			idx.postings[t] = append(idx.postings[t], Posting{DocID: id, TF: tf[t]})
		}
	}
	return idx
}

// Postings は語のポスティングリスト(DocID 昇順)。無ければ nil。
func (idx *Index) Postings(term string) []Posting {
	return idx.postings[strings.ToLower(term)]
}

// NumDocs は文書数。DocLen は文書の語数。Doc は元の文書。
func (idx *Index) NumDocs() int      { return len(idx.docs) }
func (idx *Index) DocLen(id int) int { return idx.docLen[id] }
func (idx *Index) Doc(id int) string { return idx.docs[id] }

// DF は語を含む文書数(document frequency)。ランキングの材料になる。
func (idx *Index) DF(term string) int { return len(idx.Postings(term)) }

// #endregion index

// #region boolean

// SearchAND は全ての語を含む文書を返す。ポスティングリストの積(intersection)。
// 各リストは DocID 昇順なので、マージ走査で線形に交差できる——ここが転置インデックスの
// 検索が速い理由で、文書本文には一切触れない。
func (idx *Index) SearchAND(terms ...string) []int {
	if len(terms) == 0 {
		return nil
	}
	result := docIDs(idx.Postings(terms[0]))
	for _, t := range terms[1:] {
		result = intersect(result, docIDs(idx.Postings(t)))
		if len(result) == 0 {
			return nil // 早期終了: 積がもう空
		}
	}
	return result
}

// SearchOR はいずれかの語を含む文書を返す。ポスティングリストの和(union)。
func (idx *Index) SearchOR(terms ...string) []int {
	seen := map[int]bool{}
	for _, t := range terms {
		for _, p := range idx.Postings(t) {
			seen[p.DocID] = true
		}
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func docIDs(ps []Posting) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.DocID
	}
	return out
}

// intersect は昇順リスト 2 本のマージ走査。両方に現れる ID だけ残す。
func intersect(a, b []int) []int {
	var out []int
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

// #endregion boolean
