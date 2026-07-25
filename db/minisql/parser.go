package minisql

import (
	"fmt"
	"strconv"
)

// #region ast
// Stmt は解析済みの文(AST)。INSERT か SELECT のどちらか。
type Stmt interface{ stmt() }

// InsertStmt は INSERT INTO <table> VALUES (<key>, <value>)。
type InsertStmt struct {
	Table string
	Key   uint64
	Value uint64
}

// SelectStmt は SELECT * FROM <table> [WHERE id = <key>]。
type SelectStmt struct {
	Table    string
	WhereKey *uint64 // nil なら全件
}

func (*InsertStmt) stmt() {}
func (*SelectStmt) stmt() {}

// #endregion ast

// #region parser
// parser はトークン列を1つずつ消費して AST を組み立てる再帰下降パーサ。
type parser struct {
	toks []Token
	pos  int
}

// Parse は SQL 文字列を解析して AST を返す。
func Parse(input string) (Stmt, error) {
	toks, err := Lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	stmt, err := p.parseStmt()
	if err != nil {
		return nil, err
	}
	if p.peek().Kind != TokEOF {
		return nil, fmt.Errorf("minisql: unexpected token %q after statement", p.peek().Text)
	}
	return stmt, nil
}

func (p *parser) peek() Token { return p.toks[p.pos] }

func (p *parser) next() Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// expect は次のトークンが期待した種類かを確かめて消費する。
func (p *parser) expect(kind Kind, what string) (Token, error) {
	t := p.peek()
	if t.Kind != kind {
		return t, fmt.Errorf("minisql: expected %s, got %q", what, t.Text)
	}
	return p.next(), nil
}

// expectKeyword は次が特定のキーワードかを確かめて消費する。
func (p *parser) expectKeyword(kw string) error {
	t := p.peek()
	if t.Kind != TokKeyword || t.Text != kw {
		return fmt.Errorf("minisql: expected %s, got %q", kw, t.Text)
	}
	p.next()
	return nil
}

func (p *parser) parseStmt() (Stmt, error) {
	t := p.peek()
	if t.Kind != TokKeyword {
		return nil, fmt.Errorf("minisql: expected a statement, got %q", t.Text)
	}
	switch t.Text {
	case "INSERT":
		return p.parseInsert()
	case "SELECT":
		return p.parseSelect()
	default:
		return nil, fmt.Errorf("minisql: unsupported statement %q", t.Text)
	}
}

func (p *parser) parseInsert() (Stmt, error) {
	p.next() // INSERT
	if err := p.expectKeyword("INTO"); err != nil {
		return nil, err
	}
	table, err := p.expect(TokIdent, "table name")
	if err != nil {
		return nil, err
	}
	if err := p.expectKeyword("VALUES"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokLParen, "'('"); err != nil {
		return nil, err
	}
	key, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokComma, "','"); err != nil {
		return nil, err
	}
	value, err := p.parseNumber()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokRParen, "')'"); err != nil {
		return nil, err
	}
	return &InsertStmt{Table: table.Text, Key: key, Value: value}, nil
}

func (p *parser) parseSelect() (Stmt, error) {
	p.next() // SELECT
	if _, err := p.expect(TokStar, "'*'"); err != nil {
		return nil, err
	}
	if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}
	table, err := p.expect(TokIdent, "table name")
	if err != nil {
		return nil, err
	}
	sel := &SelectStmt{Table: table.Text}

	if p.peek().Kind == TokKeyword && p.peek().Text == "WHERE" {
		p.next() // WHERE
		col, err := p.expect(TokIdent, "column name")
		if err != nil {
			return nil, err
		}
		if col.Text != "id" {
			return nil, fmt.Errorf("minisql: only WHERE id = ... is supported, got %q", col.Text)
		}
		if _, err := p.expect(TokEq, "'='"); err != nil {
			return nil, err
		}
		key, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		sel.WhereKey = &key
	}
	return sel, nil
}

func (p *parser) parseNumber() (uint64, error) {
	t, err := p.expect(TokNumber, "a number")
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(t.Text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("minisql: invalid number %q", t.Text)
	}
	return n, nil
}

// #endregion parser
