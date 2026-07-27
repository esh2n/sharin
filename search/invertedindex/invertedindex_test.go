package invertedindex

import (
	"reflect"
	"testing"
)

var corpus = []string{
	"the quick brown fox jumps over the lazy dog", // 0
	"a quick brown dog runs in the park",          // 1
	"the lazy cat sleeps all day",                 // 2
	"fox and cat play in the park",                // 3
	"dog dog dog barks at the fox",                // 4
}

func TestTokenize(t *testing.T) {
	got := Tokenize("Hello, World! 123 Go-lang")
	want := []string{"hello", "world", "123", "go", "lang"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tokenize want %v got %v", want, got)
	}
	if Tokenize("") != nil {
		t.Fatal("空文字列は nil")
	}
}

func TestPostings(t *testing.T) {
	idx := Build(corpus)
	ps := idx.Postings("dog")
	// dog は文書 0, 1, 4 に。文書 4 は 3 回。
	want := []Posting{{DocID: 0, TF: 1}, {DocID: 1, TF: 1}, {DocID: 4, TF: 3}}
	if !reflect.DeepEqual(ps, want) {
		t.Fatalf("postings(dog) want %v got %v", want, ps)
	}
	if idx.DF("dog") != 3 || idx.DF("cat") != 2 || idx.DF("zebra") != 0 {
		t.Fatalf("DF が不正: dog=%d cat=%d zebra=%d", idx.DF("dog"), idx.DF("cat"), idx.DF("zebra"))
	}
	// 大文字で引いても同じ(正規化)。
	if !reflect.DeepEqual(idx.Postings("DOG"), want) {
		t.Fatal("大文字クエリが正規化されていない")
	}
}

func TestSearchAND(t *testing.T) {
	idx := Build(corpus)
	if got := idx.SearchAND("quick", "dog"); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("AND(quick,dog) want [0 1] got %v", got)
	}
	if got := idx.SearchAND("fox", "cat"); !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("AND(fox,cat) want [3] got %v", got)
	}
	if got := idx.SearchAND("fox", "zebra"); got != nil {
		t.Fatalf("AND に無い語 want nil got %v", got)
	}
	if got := idx.SearchAND(); got != nil {
		t.Fatalf("空 AND want nil got %v", got)
	}
}

func TestSearchOR(t *testing.T) {
	idx := Build(corpus)
	if got := idx.SearchOR("cat", "fox"); !reflect.DeepEqual(got, []int{0, 2, 3, 4}) {
		t.Fatalf("OR(cat,fox) want [0 2 3 4] got %v", got)
	}
	if got := idx.SearchOR("zebra"); len(got) != 0 {
		t.Fatalf("OR に無い語 want empty got %v", got)
	}
}

func TestIDFOrdering(t *testing.T) {
	idx := Build(corpus)
	// the は 5 文書中 5 つに出る → IDF 0。park は 2 つ → 正の値。
	if idx.IDF("the") != 0 {
		t.Fatalf("IDF(the) want 0 got %f", idx.IDF("the"))
	}
	if idx.IDF("park") <= idx.IDF("the") {
		t.Fatal("珍しい語の IDF が大きくなっていない")
	}
	if idx.IDF("zebra") != 0 {
		t.Fatalf("無い語の IDF want 0 got %f", idx.IDF("zebra"))
	}
}

func TestSearchTFIDF(t *testing.T) {
	idx := Build(corpus)
	hits := idx.SearchTFIDF("dog")
	if len(hits) != 3 {
		t.Fatalf("TFIDF(dog) want 3 hits got %d", len(hits))
	}
	// dog を 3 回含む文書 4 が先頭。
	if hits[0].DocID != 4 {
		t.Fatalf("TFIDF(dog) top want doc4 got doc%d", hits[0].DocID)
	}
	// 全文書に出る the はスコア 0 だが結果には載る(IDF=0 の掛け算)。
	theHits := idx.SearchTFIDF("the")
	for _, h := range theHits {
		if h.Score != 0 {
			t.Fatalf("TFIDF(the) score want 0 got %f", h.Score)
		}
	}
}

func TestSearchBM25(t *testing.T) {
	idx := Build(corpus)
	hits := idx.SearchBM25("dog")
	if len(hits) != 3 {
		t.Fatalf("BM25(dog) want 3 hits got %d", len(hits))
	}
	if hits[0].DocID != 4 {
		t.Fatalf("BM25(dog) top want doc4 got doc%d", hits[0].DocID)
	}
	// スコアは降順。
	for i := 1; i < len(hits); i++ {
		if hits[i].Score > hits[i-1].Score {
			t.Fatal("BM25 スコアが降順でない")
		}
	}
	if got := idx.SearchBM25("zebra"); len(got) != 0 {
		t.Fatalf("BM25 無い語 want empty got %v", got)
	}
	if got := Build(nil).SearchBM25("x"); got != nil {
		t.Fatalf("空インデックス want nil got %v", got)
	}
}

func TestBM25Saturation(t *testing.T) {
	// TF 飽和: dog×3 の文書 4 は、dog×1 の文書 1 の 3 倍にはならない(頭打ち)。
	idx := Build(corpus)
	hits := idx.SearchBM25("dog")
	byDoc := map[int]float64{}
	for _, h := range hits {
		byDoc[h.DocID] = h.Score
	}
	if byDoc[4] >= byDoc[1]*3 {
		t.Fatalf("TF 飽和が効いていない: doc4=%f doc1=%f", byDoc[4], byDoc[1])
	}
	if byDoc[4] <= byDoc[1] {
		t.Fatalf("TF が多い方が高スコアのはず: doc4=%f doc1=%f", byDoc[4], byDoc[1])
	}
}

func TestBM25LengthNormalization(t *testing.T) {
	// 同じ「apple 1 回」でも、短い文書の方がスコアが高い(長さ正規化)。
	idx := Build([]string{
		"apple", // 0: 1 語
		"apple with many other words in a very long sentence", // 1: 10 語
	})
	hits := idx.SearchBM25("apple")
	if len(hits) != 2 || hits[0].DocID != 0 {
		t.Fatalf("短い文書が先頭のはず: %v", hits)
	}
}

func TestMultiTermRanking(t *testing.T) {
	idx := Build(corpus)
	// fox と park の両方に触れる文書 3 が、片方だけの文書より上に来る。
	hits := idx.SearchBM25("fox", "park")
	if hits[0].DocID != 3 {
		t.Fatalf("BM25(fox,park) top want doc3 got doc%d", hits[0].DocID)
	}
}

func TestAccessors(t *testing.T) {
	idx := Build(corpus)
	if idx.NumDocs() != 5 {
		t.Fatalf("NumDocs want 5 got %d", idx.NumDocs())
	}
	if idx.DocLen(0) != 9 {
		t.Fatalf("DocLen(0) want 9 got %d", idx.DocLen(0))
	}
	if idx.Doc(2) != corpus[2] {
		t.Fatal("Doc(2) が元文書と違う")
	}
}
