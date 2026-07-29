package lang

import (
	"fmt"
	"strconv"
)

// Pratt 構文解析: トークンごとに「前置(nud)」と「中置(led)」の関数を登録し、
// 演算子の優先順位(precedence)で結合を決める。再帰下降 + 優先順位表 の合わせ技で、
// a + b * c を a + (b * c) と正しく組める。

// #region prec
// 優先順位。下ほど強く結びつく。
const (
	_ int = iota
	LOWEST
	EQUALS      // == !=
	LESSGREATER // < >
	SUM         // + -
	PRODUCT     // * /
	PREFIX      // -x !x
	CALL        // fn(x)
)

// 中置演算子トークン → 優先順位。
var precedences = map[TokenType]int{
	EQ: EQUALS, NOTEQ: EQUALS,
	LT: LESSGREATER, GT: LESSGREATER,
	PLUS: SUM, MINUS: SUM,
	SLASH: PRODUCT, ASTERISK: PRODUCT,
	LPAREN: CALL, // 関数呼び出しの "(" も中置扱い
}

// #endregion prec

type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression
)

type Parser struct {
	l      *Lexer
	errors []string

	curToken  Token
	peekToken Token

	prefixFns map[TokenType]prefixParseFn
	infixFns  map[TokenType]infixParseFn
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}

	p.prefixFns = map[TokenType]prefixParseFn{}
	p.registerPrefix(IDENT, p.parseIdentifier)
	p.registerPrefix(INT, p.parseIntegerLiteral)
	p.registerPrefix(BANG, p.parsePrefixExpression)
	p.registerPrefix(MINUS, p.parsePrefixExpression)
	p.registerPrefix(TRUE, p.parseBoolean)
	p.registerPrefix(FALSE, p.parseBoolean)
	p.registerPrefix(LPAREN, p.parseGroupedExpression)
	p.registerPrefix(IF, p.parseIfExpression)
	p.registerPrefix(FUNCTION, p.parseFunctionLiteral)

	p.infixFns = map[TokenType]infixParseFn{}
	for _, t := range []TokenType{PLUS, MINUS, SLASH, ASTERISK, EQ, NOTEQ, LT, GT} {
		p.registerInfix(t, p.parseInfixExpression)
	}
	p.registerInfix(LPAREN, p.parseCallExpression)

	p.nextToken() // curToken と peekToken を埋める
	p.nextToken()
	return p
}

func (p *Parser) Errors() []string { return p.errors }

func (p *Parser) registerPrefix(t TokenType, fn prefixParseFn) { p.prefixFns[t] = fn }
func (p *Parser) registerInfix(t TokenType, fn infixParseFn)   { p.infixFns[t] = fn }

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// #region program
// ParseProgram はトークン列を Program(文の並び)にする。
func (p *Parser) ParseProgram() *Program {
	program := &Program{Statements: []Statement{}}
	for p.curToken.Type != EOF {
		if stmt := p.parseStatement(); stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}
	return program
}

func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case LET:
		return p.parseLetStatement()
	case RETURN:
		return p.parseReturnStatement()
	default:
		return p.parseExpressionStatement() // それ以外は式文
	}
}

// #endregion program

func (p *Parser) parseLetStatement() Statement {
	stmt := &LetStatement{}
	if !p.expectPeek(IDENT) {
		return nil
	}
	stmt.Name = &Identifier{Value: p.curToken.Literal}
	if !p.expectPeek(ASSIGN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekToken.Type == SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseReturnStatement() Statement {
	stmt := &ReturnStatement{}
	p.nextToken()
	stmt.ReturnValue = p.parseExpression(LOWEST)
	if p.peekToken.Type == SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseExpressionStatement() Statement {
	stmt := &ExpressionStatement{Expression: p.parseExpression(LOWEST)}
	if p.peekToken.Type == SEMICOLON {
		p.nextToken()
	}
	return stmt
}

// #region pratt
// parseExpression が Pratt 解析の心臓。
//  1. 現在トークンの「前置関数」で左辺を作る
//  2. 次の中置演算子の優先順位が今の precedence より強い限り、左辺を食べて中置式にする
func (p *Parser) parseExpression(precedence int) Expression {
	prefix := p.prefixFns[p.curToken.Type]
	if prefix == nil {
		p.errors = append(p.errors, fmt.Sprintf("前置解析できないトークン: %s", p.curToken.Type))
		return nil
	}
	left := prefix()

	// 右の演算子の方が強く結びつくなら、左辺をその中置式に組み込む
	for p.peekToken.Type != SEMICOLON && precedence < p.peekPrecedence() {
		infix := p.infixFns[p.peekToken.Type]
		if infix == nil {
			return left
		}
		p.nextToken()
		left = infix(left)
	}
	return left
}

// #endregion pratt

func (p *Parser) parseIdentifier() Expression {
	return &Identifier{Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() Expression {
	v, err := strconv.ParseInt(p.curToken.Literal, 10, 64)
	if err != nil {
		p.errors = append(p.errors, fmt.Sprintf("整数として解析できない: %q", p.curToken.Literal))
		return nil
	}
	return &IntegerLiteral{Value: v}
}

func (p *Parser) parseBoolean() Expression {
	return &Boolean{Value: p.curToken.Type == TRUE}
}

func (p *Parser) parsePrefixExpression() Expression {
	expr := &PrefixExpression{Operator: p.curToken.Literal}
	p.nextToken()
	expr.Right = p.parseExpression(PREFIX)
	return expr
}

func (p *Parser) parseInfixExpression(left Expression) Expression {
	expr := &InfixExpression{Left: left, Operator: p.curToken.Literal}
	prec := p.curPrecedence()
	p.nextToken()
	expr.Right = p.parseExpression(prec)
	return expr
}

func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken()
	expr := p.parseExpression(LOWEST)
	if !p.expectPeek(RPAREN) {
		return nil
	}
	return expr
}

func (p *Parser) parseIfExpression() Expression {
	expr := &IfExpression{}
	if !p.expectPeek(LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Condition = p.parseExpression(LOWEST)
	if !p.expectPeek(RPAREN) {
		return nil
	}
	if !p.expectPeek(LBRACE) {
		return nil
	}
	expr.Consequence = p.parseBlockStatement()
	if p.peekToken.Type == ELSE {
		p.nextToken()
		if !p.expectPeek(LBRACE) {
			return nil
		}
		expr.Alternative = p.parseBlockStatement()
	}
	return expr
}

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{Statements: []Statement{}}
	p.nextToken()
	for p.curToken.Type != RBRACE && p.curToken.Type != EOF {
		if stmt := p.parseStatement(); stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}
	return block
}

func (p *Parser) parseFunctionLiteral() Expression {
	fn := &FunctionLiteral{}
	if !p.expectPeek(LPAREN) {
		return nil
	}
	fn.Parameters = p.parseFunctionParameters()
	if !p.expectPeek(LBRACE) {
		return nil
	}
	fn.Body = p.parseBlockStatement()
	return fn
}

func (p *Parser) parseFunctionParameters() []*Identifier {
	params := []*Identifier{}
	if p.peekToken.Type == RPAREN {
		p.nextToken()
		return params
	}
	p.nextToken()
	params = append(params, &Identifier{Value: p.curToken.Literal})
	for p.peekToken.Type == COMMA {
		p.nextToken()
		p.nextToken()
		params = append(params, &Identifier{Value: p.curToken.Literal})
	}
	if !p.expectPeek(RPAREN) {
		return nil
	}
	return params
}

func (p *Parser) parseCallExpression(fn Expression) Expression {
	call := &CallExpression{Function: fn}
	call.Arguments = p.parseCallArguments()
	return call
}

func (p *Parser) parseCallArguments() []Expression {
	args := []Expression{}
	if p.peekToken.Type == RPAREN {
		p.nextToken()
		return args
	}
	p.nextToken()
	args = append(args, p.parseExpression(LOWEST))
	for p.peekToken.Type == COMMA {
		p.nextToken()
		p.nextToken()
		args = append(args, p.parseExpression(LOWEST))
	}
	if !p.expectPeek(RPAREN) {
		return nil
	}
	return args
}

// expectPeek は次トークンが期待通りなら進んで true、違えばエラーを積んで false。
func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	p.errors = append(p.errors, fmt.Sprintf("%s を期待したが %s が来た", t, p.peekToken.Type))
	return false
}

func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}
func (p *Parser) curPrecedence() int {
	if prec, ok := precedences[p.curToken.Type]; ok {
		return prec
	}
	return LOWEST
}
