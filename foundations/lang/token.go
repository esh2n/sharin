// Package lang は小さなプログラミング言語をフルスクラッチする。
//
// ソース文字列を字句解析(lexer)でトークン列にし、Pratt 構文解析(parser)で AST にし、
// ツリーウォーク評価器(eval)で実行する。整数・真偽・let束縛・if式・第一級関数・
// クロージャまでを扱う。Thorsten Ball の Monkey 言語のコアを日本語コメントで再構成したもの。
package lang

// TokenType はトークンの種類。文字列にしておくとデバッグ表示が楽。
type TokenType string

// Token は1つの字句単位。種類と、元の文字列(Literal)を持つ。
type Token struct {
	Type    TokenType
	Literal string
}

const (
	ILLEGAL = "ILLEGAL" // 未知の文字
	EOF     = "EOF"     // 入力の終わり

	IDENT = "IDENT" // 識別子: x, add, ...
	INT   = "INT"   // 整数: 123

	ASSIGN   = "="
	PLUS     = "+"
	MINUS    = "-"
	BANG     = "!"
	ASTERISK = "*"
	SLASH    = "/"
	LT       = "<"
	GT       = ">"
	EQ       = "=="
	NOTEQ    = "!="

	COMMA     = ","
	SEMICOLON = ";"
	LPAREN    = "("
	RPAREN    = ")"
	LBRACE    = "{"
	RBRACE    = "}"

	FUNCTION = "FUNCTION"
	LET      = "LET"
	TRUE     = "TRUE"
	FALSE    = "FALSE"
	IF       = "IF"
	ELSE     = "ELSE"
	RETURN   = "RETURN"
)

// keywords は予約語の対応表。識別子を読んだ後にここを引いて種類を確定する。
var keywords = map[string]TokenType{
	"fn":     FUNCTION,
	"let":    LET,
	"true":   TRUE,
	"false":  FALSE,
	"if":     IF,
	"else":   ELSE,
	"return": RETURN,
}

// lookupIdent は識別子が予約語ならその種類を、そうでなければ IDENT を返す。
func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
