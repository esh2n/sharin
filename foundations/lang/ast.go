package lang

import (
	"strconv"
	"strings"
)

// AST(抽象構文木)のノード群。構文解析の出力であり、評価器の入力。
// Node は共通インタフェース。Statement(文)と Expression(式)に分かれる。
// この言語では if も fn も「値を生む式」なのが特徴(式指向)。

// #region ast
type Node interface {
	String() string // デバッグ・テスト用に元のソースへ近い形を復元する
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

// Program は木の根。文の並び。
type Program struct {
	Statements []Statement
}

func (p *Program) String() string {
	var out strings.Builder
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}
// #endregion ast

// --- 文(Statement) ---

// let <name> = <value>;
type LetStatement struct {
	Name  *Identifier
	Value Expression
}

func (*LetStatement) statementNode()   {}
func (ls *LetStatement) String() string { return "let " + ls.Name.String() + " = " + exprStr(ls.Value) + ";" }

// return <value>;
type ReturnStatement struct {
	ReturnValue Expression
}

func (*ReturnStatement) statementNode()    {}
func (rs *ReturnStatement) String() string { return "return " + exprStr(rs.ReturnValue) + ";" }

// 式だけの文(例: `x + 1;`)。REPL で式を書けるように。
type ExpressionStatement struct {
	Expression Expression
}

func (*ExpressionStatement) statementNode()    {}
func (es *ExpressionStatement) String() string { return exprStr(es.Expression) }

// { ... } のブロック。if や fn の本体。
type BlockStatement struct {
	Statements []Statement
}

func (*BlockStatement) statementNode() {}
func (bs *BlockStatement) String() string {
	var out strings.Builder
	for _, s := range bs.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// --- 式(Expression) ---

type Identifier struct{ Value string }

func (*Identifier) expressionNode()   {}
func (i *Identifier) String() string { return i.Value }

type IntegerLiteral struct{ Value int64 }

func (*IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) String() string {
	return strconv.FormatInt(il.Value, 10)
}

type Boolean struct{ Value bool }

func (*Boolean) expressionNode() {}
func (b *Boolean) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// 前置演算: !x, -x
type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (*PrefixExpression) expressionNode()    {}
func (pe *PrefixExpression) String() string { return "(" + pe.Operator + exprStr(pe.Right) + ")" }

// 中置演算: a + b, a == b。括弧を付けて優先順位が見えるようにする。
type InfixExpression struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (*InfixExpression) expressionNode() {}
func (ie *InfixExpression) String() string {
	return "(" + exprStr(ie.Left) + " " + ie.Operator + " " + exprStr(ie.Right) + ")"
}

// if (<cond>) { <then> } else { <else> } — これも式(値を返す)
type IfExpression struct {
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement // 無ければ nil
}

func (*IfExpression) expressionNode() {}
func (ie *IfExpression) String() string {
	out := "if" + exprStr(ie.Condition) + " " + ie.Consequence.String()
	if ie.Alternative != nil {
		out += "else " + ie.Alternative.String()
	}
	return out
}

// fn(<params>) { <body> } — 関数リテラル(第一級)
type FunctionLiteral struct {
	Parameters []*Identifier
	Body       *BlockStatement
}

func (*FunctionLiteral) expressionNode() {}
func (fl *FunctionLiteral) String() string {
	params := make([]string, len(fl.Parameters))
	for i, p := range fl.Parameters {
		params[i] = p.String()
	}
	return "fn(" + strings.Join(params, ", ") + ") " + fl.Body.String()
}

// <fn>(<args>) — 呼び出し
type CallExpression struct {
	Function  Expression // Identifier か FunctionLiteral
	Arguments []Expression
}

func (*CallExpression) expressionNode() {}
func (ce *CallExpression) String() string {
	args := make([]string, len(ce.Arguments))
	for i, a := range ce.Arguments {
		args[i] = exprStr(a)
	}
	return exprStr(ce.Function) + "(" + strings.Join(args, ", ") + ")"
}

// exprStr は nil 安全に式を文字列化する(構文エラー時に nil が混じるため)。
func exprStr(e Expression) string {
	if e == nil {
		return ""
	}
	return e.String()
}
