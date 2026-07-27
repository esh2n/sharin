// Package regex は正規表現エンジンを最小構成で自作する。
//
// 多くの言語の正規表現は「バックトラッキング」で動き、a(a|aa)*b のような式に悪意ある入力を
// 与えると指数時間で爆発する(ReDoS)。この章が作るのは、それとは別系統の——Ken Thompson が
// 1968 年に示した——オートマトンに基づくエンジンだ。正規表現をいったん NFA(非決定性有限
// オートマトン)に変換し、状態集合を並行に進めることで、バックトラッキング無しに入力長に
// 比例する時間でマッチする。
//
// 流れは 3 段で、lang 編の「字句 → 構文 → 評価」と同じ骨格を持つ:
//  1. パース: 正規表現の文字列を AST(構文木)にする(parser.go)
//  2. Thompson 構成: AST を ε 遷移付きの NFA にする(nfa.go)
//  3. シミュレーション / DFA 化: NFA を状態集合として動かす、あるいは DFA に変換する(nfa.go / dfa.go)
package regex

import (
	"errors"
	"fmt"
)

// ErrSyntax はパース失敗。
var ErrSyntax = errors.New("regex: 構文エラー")

// #region ast

// Node は正規表現の構文木のノード。
type Node interface{ isNode() }

// Lit は 1 文字リテラル。Any は任意 1 文字(.)。Empty は空(ε)。
type Lit struct{ Ch byte }
type Any struct{}
type Empty struct{}

// Concat は連接(L の後に R)。Alt は選択(L か R)。
type Concat struct{ L, R Node }
type Alt struct{ L, R Node }

// Star は 0 回以上、Plus は 1 回以上、Quest は 0 か 1 回の繰り返し。
type Star struct{ X Node }
type Plus struct{ X Node }
type Quest struct{ X Node }

func (Lit) isNode()    {}
func (Any) isNode()    {}
func (Empty) isNode()  {}
func (Concat) isNode() {}
func (Alt) isNode()    {}
func (Star) isNode()   {}
func (Plus) isNode()   {}
func (Quest) isNode()  {}

// #endregion ast

// #region parse

// Parse は正規表現の文字列を AST にする。再帰下降パーサで、優先順位は
// 選択(|)< 連接 < 繰り返し(* + ?)の順(繰り返しが最も強く結びつく)。
// 対応構文: リテラル・. ・グループ () ・選択 | ・繰り返し * + ?。
func Parse(pattern string) (Node, error) {
	p := &parser{src: pattern}
	n, err := p.alt()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("%w: 余分な文字 %q", ErrSyntax, p.src[p.pos:])
	}
	return n, nil
}

type parser struct {
	src string
	pos int
}

func (p *parser) peek() byte {
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

// alt は選択。concat を | で繋ぐ(最も弱い結合)。
func (p *parser) alt() (Node, error) {
	left, err := p.concat()
	if err != nil {
		return nil, err
	}
	for p.peek() == '|' {
		p.pos++
		right, err := p.concat()
		if err != nil {
			return nil, err
		}
		left = Alt{L: left, R: right}
	}
	return left, nil
}

// concat は連接。| ) か終端に当たるまで repeat を並べる。空なら Empty(ε)。
func (p *parser) concat() (Node, error) {
	var nodes []Node
	for {
		c := p.peek()
		if c == 0 || c == '|' || c == ')' {
			break
		}
		n, err := p.repeat()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return Empty{}, nil
	}
	left := nodes[0]
	for _, n := range nodes[1:] {
		left = Concat{L: left, R: n}
	}
	return left, nil
}

// repeat は後置の * + ? を atom に重ねる。
func (p *parser) repeat() (Node, error) {
	a, err := p.atom()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			a = Star{X: a}
		case '+':
			p.pos++
			a = Plus{X: a}
		case '?':
			p.pos++
			a = Quest{X: a}
		default:
			return a, nil
		}
	}
}

// atom はグループ () ・任意文字 . ・リテラル 1 文字。
func (p *parser) atom() (Node, error) {
	switch c := p.peek(); c {
	case 0, '|', ')':
		return nil, fmt.Errorf("%w: 式が必要な位置 %d", ErrSyntax, p.pos)
	case '(':
		p.pos++
		n, err := p.alt()
		if err != nil {
			return nil, err
		}
		if p.peek() != ')' {
			return nil, fmt.Errorf("%w: 閉じ括弧が無い", ErrSyntax)
		}
		p.pos++
		return n, nil
	case '.':
		p.pos++
		return Any{}, nil
	case '*', '+', '?':
		return nil, fmt.Errorf("%w: 繰り返しの対象が無い(位置 %d)", ErrSyntax, p.pos)
	default:
		p.pos++
		return Lit{Ch: c}, nil
	}
}

// #endregion parse
