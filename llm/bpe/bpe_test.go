package bpe

import (
	"reflect"
	"strings"
	"testing"
)

func TestChunksRoundTrip(t *testing.T) {
	cases := []string{
		"low lower lowest",
		"  double  space",
		"猫が好き 犬も好き",
		"",
		" ",
		"mixed 日本語 and english",
	}
	for _, c := range cases {
		got := strings.Join(chunks(c), "")
		if got != c {
			t.Errorf("chunks round trip: %q -> %q", c, got)
		}
	}
}

func TestTrainMergeOrder(t *testing.T) {
	// "low" 系のペア数: (l,o)=5, (o,w)=5 で同数。タイブレークは辞書順で (l,o) が先。
	// 2 手目は lo と w が隣接して最多になり (lo,w) が併合される。
	corpus := "low low low lower lower newest"
	tok := Train(corpus, 2)
	want := []Pair{{"l", "o"}, {"lo", "w"}}
	if !reflect.DeepEqual(tok.Merges(), want) {
		t.Fatalf("merges = %v, want %v", tok.Merges(), want)
	}
}

func TestTrainStopsWhenNoPairs(t *testing.T) {
	// 1 文字だけのコーパスはペアが無いので併合 0 で止まる。
	if got := Train("a", 10).Merges(); len(got) != 0 {
		t.Fatalf("merges = %v, want empty", got)
	}
	// "ab ab" は (a,b) と (空白,ab) の 2 併合で尽きる。要求 10 でも 2 で止まる。
	if got := Train("ab ab", 10).Merges(); len(got) != 2 {
		t.Fatalf("merges = %v, want 2 merges", got)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	corpus := "low low low lower lower newest 猫が好き 猫が寝る"
	tok := Train(corpus, 30)
	for _, text := range []string{
		"low lower",
		"lowest",
		"猫が好き",
		" 猫が寝る low",
		corpus,
	} {
		got := tok.Decode(tok.Encode(text))
		if got != text {
			t.Errorf("round trip: %q -> %q", text, got)
		}
	}
}

func TestUnknownRune(t *testing.T) {
	tok := Train("low lower", 5)
	ids := tok.Encode("boxer")
	hasUnk := false
	for _, id := range ids {
		if id == 0 {
			hasUnk = true
		}
	}
	if !hasUnk {
		t.Fatalf("encode of unseen runes should contain unk id 0: %v", ids)
	}
	if !strings.Contains(tok.Decode(ids), "�") {
		t.Fatalf("decode of unk should contain replacement char: %q", tok.Decode(ids))
	}
}

func TestDecodeOutOfRange(t *testing.T) {
	tok := Train("ab", 1)
	if got := tok.Decode([]int{-1, 99999}); got != "��" {
		t.Fatalf("decode out of range = %q", got)
	}
}

func TestMergesCompress(t *testing.T) {
	corpus := "the cat sat on the mat the cat ran to the cat"
	raw := Train(corpus, 0)
	merged := Train(corpus, 30)
	nRaw := len(raw.Encode(corpus))
	nMerged := len(merged.Encode(corpus))
	if nMerged >= nRaw {
		t.Fatalf("merges should compress: %d merges -> %d tokens, 0 merges -> %d tokens", 30, nMerged, nRaw)
	}
}

func TestDeterminism(t *testing.T) {
	corpus := "aa ab ba bb aa ab 猫が good goods"
	a := Train(corpus, 20)
	b := Train(corpus, 20)
	if !reflect.DeepEqual(a.Merges(), b.Merges()) {
		t.Fatalf("merges differ between runs")
	}
	if !reflect.DeepEqual(a.Vocab(), b.Vocab()) {
		t.Fatalf("vocab differs between runs")
	}
}

func TestVocabSize(t *testing.T) {
	corpus := "abc abd"
	tok := Train(corpus, 3)
	// <unk> + 基底シンボル(a,b,c,d,空白) + 併合トークン数
	want := 1 + 5 + len(tok.Merges())
	if tok.VocabSize() != want {
		t.Fatalf("vocab size = %d, want %d (vocab %v)", tok.VocabSize(), want, tok.Vocab())
	}
}

func TestTokens(t *testing.T) {
	tok := Train("low low low lower lower newest", 2)
	// 2 併合(lo, low)後の "low lower": "low"=[low], " lower"=[" ", "low", "e", "r"]
	got := tok.Tokens("low lower")
	want := []string{"low", " ", "low", "e", "r"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
}

func TestEmptyInput(t *testing.T) {
	tok := Train("", 5)
	if tok.VocabSize() != 1 {
		t.Fatalf("empty corpus vocab = %d, want 1 (<unk> only)", tok.VocabSize())
	}
	if got := tok.Encode(""); len(got) != 0 {
		t.Fatalf("encode empty = %v", got)
	}
	if got := tok.Decode(nil); got != "" {
		t.Fatalf("decode nil = %q", got)
	}
}
