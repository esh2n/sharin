package regex

import (
	"errors"
	"strings"
	"testing"
)

// マッチ表: 各パターンについて、通る/通らない入力を並べる。
// NFA と DFA の両方で同じ結果になることを一括で確かめる。
var matchCases = []struct {
	pattern string
	ok      []string
	ng      []string
}{
	{"abc", []string{"abc"}, []string{"", "ab", "abcd", "abx"}},
	{"a|b", []string{"a", "b"}, []string{"", "ab", "c"}},
	{"a*", []string{"", "a", "aaaa"}, []string{"b", "ab"}},
	{"a+", []string{"a", "aaa"}, []string{"", "b"}},
	{"a?", []string{"", "a"}, []string{"aa", "b"}},
	{"ab*c", []string{"ac", "abc", "abbbc"}, []string{"", "ab", "abbb", "abcc"}},
	{"a(b|c)*d", []string{"ad", "abd", "acd", "abcbcd"}, []string{"a", "abce"}},
	{"(ab)+", []string{"ab", "abab", "ababab"}, []string{"", "a", "aba", "abb"}},
	{"a.c", []string{"abc", "axc", "a.c"}, []string{"ac", "abbc", "abcd"}},
	{".*", []string{"", "a", "hello world"}, []string{}},
	{"a.*z", []string{"az", "abcz", "a z z"}, []string{"a", "z", "abc"}},
	{"(a|b)*abb", []string{"abb", "aabb", "babb", "ababb"}, []string{"", "ab", "abbb"}},
	{"", []string{""}, []string{"a"}},
	{"colou?r", []string{"color", "colour"}, []string{"colouur", "colr"}},
}

func TestMatchNFAandDFA(t *testing.T) {
	for _, tc := range matchCases {
		re, err := Compile(tc.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tc.pattern, err)
		}
		for _, in := range tc.ok {
			if !re.Match(in) {
				t.Errorf("NFA %q should match %q", tc.pattern, in)
			}
			if !re.MatchDFA(in) {
				t.Errorf("DFA %q should match %q", tc.pattern, in)
			}
		}
		for _, in := range tc.ng {
			if re.Match(in) {
				t.Errorf("NFA %q should NOT match %q", tc.pattern, in)
			}
			if re.MatchDFA(in) {
				t.Errorf("DFA %q should NOT match %q", tc.pattern, in)
			}
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, p := range []string{"(", "a(b", "*", "+abc", ")", "a)", "?x"} {
		if _, err := Parse(p); !errors.Is(err, ErrSyntax) {
			t.Errorf("Parse(%q) want ErrSyntax got %v", p, err)
		}
	}
}

func TestParseValidEdgeCases(t *testing.T) {
	// これらは正当(| の両側が空になりうる、** は Star の Star)。
	for _, p := range []string{"a|", "|a", "a**", "(a|)b"} {
		if _, err := Parse(p); err != nil {
			t.Errorf("Parse(%q) should be valid, got %v", p, err)
		}
	}
	// "a|" は a か 空 にマッチ。
	re, _ := Compile("a|")
	if !re.Match("a") || !re.Match("") || re.Match("b") {
		t.Fatal("a| のマッチが不正")
	}
}

func TestStarNestingIsLinear(t *testing.T) {
	// バックトラッキング型なら (a*)* は指数爆発する古典的な ReDoS パターン。
	// 状態集合を並行に進める本実装は、長い非マッチ入力でも即座に返る。
	re, err := Compile("(a*)*b")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	input := strings.Repeat("a", 200) + "c" // b で終わらない = 非マッチ
	if re.Match(input) {
		t.Fatal("非マッチのはず")
	}
	if re.MatchDFA(input) {
		t.Fatal("DFA も非マッチのはず")
	}
	if re.Match(strings.Repeat("a", 200)+"b") != true {
		t.Fatal("b で終わればマッチのはず")
	}
}

func TestDFAReducesStates(t *testing.T) {
	re, err := Compile("(a|b)*abb")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	nfaN := re.nfa.NumStates()
	dfa := ToDFA(re.nfa)
	if nfaN == 0 || dfa.NumStates() == 0 {
		t.Fatal("状態数が 0")
	}
	// DFA は ε 遷移を畳み込むので、この例では NFA より状態が少ない。
	if dfa.NumStates() >= nfaN {
		t.Errorf("この例では DFA(%d) < NFA(%d) を期待", dfa.NumStates(), nfaN)
	}
}

func TestCompileErrorPropagates(t *testing.T) {
	if _, err := Compile("(a"); !errors.Is(err, ErrSyntax) {
		t.Fatalf("Compile 不正パターン want ErrSyntax got %v", err)
	}
}
