package embed

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) <= 1e-9 }

func TestMeanPool(t *testing.T) {
	// トークン埋め込みの平均が文の埋め込みになる。
	tokens := [][]float64{
		{1, 0, 0},
		{0, 2, 0},
		{2, 1, 3},
	}
	got := MeanPool(tokens)
	want := []float64{1, 1, 1}
	for i := range want {
		if !approx(got[i], want[i]) {
			t.Fatalf("meanpool = %v, want %v", got, want)
		}
	}
	if got := MeanPool(nil); got != nil {
		t.Fatalf("empty pool = %v", got)
	}
}

func TestCosineSimilarity(t *testing.T) {
	// 同じ向きは 1、直交は 0、逆向きは -1。長さには依存しない。
	a := []float64{1, 0}
	if s := Cosine(a, []float64{5, 0}); !approx(s, 1) {
		t.Fatalf("same direction = %f, want 1", s)
	}
	if s := Cosine(a, []float64{0, 3}); !approx(s, 0) {
		t.Fatalf("orthogonal = %f, want 0", s)
	}
	if s := Cosine(a, []float64{-2, 0}); !approx(s, -1) {
		t.Fatalf("opposite = %f, want -1", s)
	}
	// ゼロベクトルは 0 を返す(NaN を出さない)。
	if s := Cosine(a, []float64{0, 0}); s != 0 {
		t.Fatalf("zero vector = %f, want 0", s)
	}
}

func TestNormalizeUnit(t *testing.T) {
	// 正規化後は長さ 1。正規化した内積 = コサイン類似度。
	v := Normalize([]float64{3, 4})
	if !approx(v[0], 0.6) || !approx(v[1], 0.8) {
		t.Fatalf("normalize = %v", v)
	}
	if Normalize([]float64{0, 0})[0] != 0 {
		t.Fatal("zero normalize should stay zero")
	}
}

func buildStore() *Store {
	s := NewStore()
	// 語彙は決定的な埋め込みを持つ、というおもちゃの前提。
	s.Add("cat", []float64{0.9, 0.1, 0})
	s.Add("kitten", []float64{0.8, 0.2, 0}) // cat に近い
	s.Add("dog", []float64{0.7, 0.3, 0.2})
	s.Add("car", []float64{0, 0.1, 0.95}) // 全く別方向
	return s
}

func TestSearchRanksByCosine(t *testing.T) {
	s := buildStore()
	// "cat" のクエリに最も近いのは cat 自身、次に kitten。car は最下位。
	hits := s.Search([]float64{0.9, 0.1, 0}, 3)
	if len(hits) != 3 {
		t.Fatalf("hits = %d", len(hits))
	}
	if hits[0].Key != "cat" {
		t.Fatalf("top = %q, want cat", hits[0].Key)
	}
	if hits[1].Key != "kitten" {
		t.Fatalf("second = %q, want kitten", hits[1].Key)
	}
	// スコアは降順。
	for i := 1; i < len(hits); i++ {
		if hits[i].Score > hits[i-1].Score {
			t.Fatalf("not sorted: %v", hits)
		}
	}
	// car は cat と直交気味なので、上位3件から漏れるほど低い。
	last := s.Search([]float64{0.9, 0.1, 0}, 4)[3]
	if last.Key != "car" {
		t.Fatalf("worst match = %q, want car", last.Key)
	}
}

func TestSearchTopKClamped(t *testing.T) {
	s := buildStore()
	// k が語彙数を超えても全件返すだけ(パニックしない)。
	if len(s.Search([]float64{1, 0, 0}, 100)) != 4 {
		t.Fatal("k over size should return all")
	}
	if len(s.Search([]float64{1, 0, 0}, 0)) != 0 {
		t.Fatal("k=0 should return none")
	}
}

func TestExactSearchOpsIsLinear(t *testing.T) {
	// 総当たり検索は全ベクトルとの比較 = 件数に線形。
	// ANN(近似)がなぜ要るかの土台: 件数が billion になると総当たりは無理。
	s := NewStore()
	for i := 0; i < 1000; i++ {
		s.Add(string(rune(i)), []float64{float64(i), 1})
	}
	_, ops := s.SearchCounted([]float64{1, 1}, 5)
	if ops != 1000 {
		t.Fatalf("exact search ops = %d, want 1000", ops)
	}
}
