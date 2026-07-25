// Package minisql は SQL の極小サブセット(INSERT / SELECT)を、
// これまで作った btreewal ストレージの上に載せた「ミニ実データベース」。db 編の完結編。
//
// SQL の実行は3段のパイプライン: 文字列 → [lexer] トークン列 → [parser] AST → [engine] 実行。
// このファイルは第1段、lexer(字句解析器)。文字の並びを意味の最小単位(トークン)に切り分ける。
package minisql

import (
	"fmt"
	"strings"
	"unicode"
)

// Kind はトークンの種類。
type Kind int

const (
	TokEOF Kind = iota
	TokKeyword
	TokIdent
	TokNumber
	TokStar
	TokComma
	TokLParen
	TokRParen
	TokEq
)

// Token は1つのトークン。
type Token struct {
	Kind Kind
	Text string
}

var keywords = map[string]bool{
	"INSERT": true, "INTO": true, "VALUES": true,
	"SELECT": true, "FROM": true, "WHERE": true,
}

// Lex は入力文字列をトークン列に切り分ける(末尾に TokEOF を付ける)。
func Lex(input string) ([]Token, error) {
	var toks []Token
	runes := []rune(input)
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '*':
			toks = append(toks, Token{TokStar, "*"})
			i++
		case c == ',':
			toks = append(toks, Token{TokComma, ","})
			i++
		case c == '(':
			toks = append(toks, Token{TokLParen, "("})
			i++
		case c == ')':
			toks = append(toks, Token{TokRParen, ")"})
			i++
		case c == '=':
			toks = append(toks, Token{TokEq, "="})
			i++
		case unicode.IsDigit(c):
			start := i
			for i < len(runes) && unicode.IsDigit(runes[i]) {
				i++
			}
			toks = append(toks, Token{TokNumber, string(runes[start:i])})
		case unicode.IsLetter(c) || c == '_':
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			word := string(runes[start:i])
			// キーワードは大小無視。識別子はそのまま。
			if keywords[strings.ToUpper(word)] {
				toks = append(toks, Token{TokKeyword, strings.ToUpper(word)})
			} else {
				toks = append(toks, Token{TokIdent, word})
			}
		default:
			return nil, fmt.Errorf("minisql: unexpected character %q", c)
		}
	}
	toks = append(toks, Token{Kind: TokEOF})
	return toks, nil
}
