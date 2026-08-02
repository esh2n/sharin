// Package sqlinject は、外から来た値を SQL に混ぜる3通りのやり方を実装して、
// 何がどこまで守れるかを測る。
//
// 注入が起きるというのは、要するに**値のはずのものが構文になった**ということだ。
// `' OR '1'='1` を名前の欄に入れると、それは1つの文字列ではなく、
// 引用符・キーワード・比較演算子の並びとして読まれる。
//
// だから測り方も1つに決まる。**組み立てた問い合わせを字句に切って、
// 外から来た値が構文に何トークン寄与したかを数える**。
// 1トークン(値そのもの)なら注入は起きていない。2つ以上なら起きている。
//
// [ミニSQL](mini-sql)の字句解析器は最小構成で文字列も演算子も持たないので、
// ここでは注入を見るのに足りるぶんだけを別に持つ。
//
// 実時間も乱数も使わない。同じ入力なら何度でも同じ結果になる。
package sqlinject

import (
	"strings"
)

// #region token

// Kind はトークンの種類。
type Kind int

const (
	// Word は識別子かキーワード。
	Word Kind = iota
	// Num は数値。
	Num
	// Str は引用符で囲まれた文字列。全体で1つ。
	Str
	// Op は演算子や区切り。
	Op
	// Comment はコメント。ここから行末は読まれない。
	Comment
)

// Token は1つのトークン。From は元の文字列での開始位置。
type Token struct {
	Kind Kind
	Text string
	From int
}

var keywords = map[string]bool{
	"select": true, "from": true, "where": true, "or": true, "and": true,
	"insert": true, "into": true, "values": true, "drop": true, "table": true,
	"order": true, "by": true, "union": true, "delete": true, "update": true,
}

// IsKeyword は、その語が構文を動かすキーワードか。
func IsKeyword(t Token) bool { return t.Kind == Word && keywords[strings.ToLower(t.Text)] }

// Lex は問い合わせをトークンに切る。
//
// 文字列は開き引用符から閉じ引用符までで**1つ**にする。ここが要点で、
// 引用符が閉じられてしまうと、その先は文字列ではなく構文として切られる。
func Lex(q string) []Token {
	var out []Token
	r := []rune(q)
	i := 0
	for i < len(r) {
		c := r[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '-' && i+1 < len(r) && r[i+1] == '-':
			j := i
			for j < len(r) && r[j] != '\n' {
				j++
			}
			out = append(out, Token{Comment, string(r[i:j]), i})
			i = j
		case c == '\'':
			j := i + 1
			for j < len(r) {
				// '' は文字列の中の引用符1つ。ここでは閉じない。
				if r[j] == '\'' && j+1 < len(r) && r[j+1] == '\'' {
					j += 2
					continue
				}
				if r[j] == '\'' {
					j++
					break
				}
				j++
			}
			out = append(out, Token{Str, string(r[i:j]), i})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(r) && r[j] >= '0' && r[j] <= '9' {
				j++
			}
			out = append(out, Token{Num, string(r[i:j]), i})
			i = j
		case isWordRune(c):
			j := i
			for j < len(r) && isWordRune(r[j]) {
				j++
			}
			out = append(out, Token{Word, string(r[i:j]), i})
			i = j
		default:
			out = append(out, Token{Op, string(c), i})
			i++
		}
	}
	return out
}

func isWordRune(c rune) bool {
	return c == '_' || c == '*' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// #endregion token

// #region build

// Mode は値の混ぜ方。
type Mode int

const (
	// Concat は文字列連結。値がそのまま問い合わせの一部になる。
	Concat Mode = iota
	// QuoteEscape は連結する前に ' を '' にする。
	QuoteEscape
	// Placeholder は値を問い合わせに入れず、別の経路で渡す。
	Placeholder
)

func (m Mode) String() string {
	switch m {
	case QuoteEscape:
		return "引用符を二重にする"
	case Placeholder:
		return "プレースホルダ"
	default:
		return "文字列連結"
	}
}

// Modes は測る対象。
func Modes() []Mode { return []Mode{Concat, QuoteEscape, Placeholder} }

// Slot は値を埋める場所。埋める先によって、使える守りが変わる。
type Slot int

const (
	// Quoted は引用符で囲まれた値。WHERE name = 'ここ'
	Quoted Slot = iota
	// Bare は引用符の無い値。WHERE id = ここ
	Bare
	// Ident は識別子。ORDER BY ここ
	Ident
)

func (s Slot) String() string {
	switch s {
	case Bare:
		return "引用符なしの値"
	case Ident:
		return "識別子(列名など)"
	default:
		return "引用符ありの値"
	}
}

// Slots は測る対象。
func Slots() []Slot { return []Slot{Quoted, Bare, Ident} }

// Query は組み立てた結果。
type Query struct {
	// Text は実際にデータベースへ渡す文字列。
	Text string
	// Params は別の経路で渡す値。Text には現れない。
	Params []string
	// span は Text の中で、外から来た値が占めた範囲。
	span [2]int
}

// Build は、その混ぜ方でその場所へ値を埋めた問い合わせを返す。
//
// プレースホルダだけは、値を Text に入れない。ここが他の2つと根本的に違う。
func Build(m Mode, s Slot, v string) Query {
	if m == Placeholder && s != Ident {
		// 値は文字列に入らない。引用符も要らない。値の種類はデータベース側が
		// 別経路で受け取るので、問い合わせの形は入力に依らず一定になる。
		return Query{Text: bindFrame(s), Params: []string{v}}
	}
	head, tail := frame(s)
	body := v
	if m == QuoteEscape {
		body = strings.ReplaceAll(v, "'", "''")
	}
	return Query{Text: head + body + tail, span: [2]int{len(head), len(head) + len(body)}}
}

// bindFrame は、値を別経路で渡すときの形。引用符が消えるのが要点になる。
func bindFrame(s Slot) string {
	if s == Bare {
		return "SELECT * FROM users WHERE id = ?"
	}
	return "SELECT * FROM users WHERE name = ?"
}

// frame は、その場所の前後の決まった部分。
func frame(s Slot) (head, tail string) {
	switch s {
	case Bare:
		return "SELECT * FROM users WHERE id = ", ""
	case Ident:
		return "SELECT * FROM users ORDER BY ", ""
	default:
		return "SELECT * FROM users WHERE name = '", "'"
	}
}

// #endregion build

// #region measure

// FromValue は、外から来た値が構文に寄与したトークンを返す。
//
// 値そのものが1つのトークンに収まっていれば、それは値として扱われている。
// 2つ以上になっていたら、値の一部が構文として読まれたということになる。
func (q Query) FromValue() []Token {
	if q.span == [2]int{} {
		return nil // 値は Text に入っていない
	}
	var out []Token
	for _, t := range Lex(q.Text) {
		to := t.From + len([]rune(t.Text))
		// 重なっていれば、そのトークンには値が関わっている。
		// 普通の値は、囲みの引用符を含む文字列トークン 1 つに収まる。
		if t.From < q.span[1] && to > q.span[0] {
			out = append(out, t)
		}
	}
	return out
}

// Injected は注入が起きたか。
//
// 判定は素朴で、値の寄与が2トークン以上か、キーワードかコメントを含むか。
func (q Query) Injected() bool {
	ts := q.FromValue()
	if len(ts) > 1 {
		return true
	}
	for _, t := range ts {
		if IsKeyword(t) || t.Kind == Comment {
			return true
		}
	}
	return false
}

// Attack は攻撃に使う値と、それが刺さる場所。
type Attack struct {
	Name  string
	Value string
	Slots []Slot
}

// Attacks は測るときの攻撃。
func Attacks() []Attack {
	return []Attack{
		{"条件を常に真にする", `' OR '1'='1`, []Slot{Quoted}},
		{"文を打ち切って足す", `'; DROP TABLE users;--`, []Slot{Quoted}},
		{"引用符が要らない", `1 OR 1=1`, []Slot{Bare}},
		{"列名に紛れ込ませる", `name; DROP TABLE users;--`, []Slot{Ident}},
	}
}

// Hits は、その攻撃がその場所を狙っているか。
func Hits(a Attack, s Slot) bool {
	for _, x := range a.Slots {
		if x == s {
			return true
		}
	}
	return false
}

// Score は、その混ぜ方がその場所で攻撃を何件止めたか。
func Score(m Mode, s Slot) (stopped, total int) {
	for _, a := range Attacks() {
		if !Hits(a, s) {
			continue
		}
		total++
		if !Build(m, s, a.Value).Injected() {
			stopped++
		}
	}
	return
}

// Usable は、その場所でその混ぜ方が使えるか。
//
// **プレースホルダは識別子には使えない**。列名やテーブル名は値ではなく
// 構文の一部なので、実行する前に決まっていなければならない。
// ここは変換や委譲で守るところではなく、許可制にするところになる。
func Usable(m Mode, s Slot) bool { return !(m == Placeholder && s == Ident) }

// AllowIdent は、識別子を許可制で選び直す。
func AllowIdent(allow []string, v string) string {
	for _, a := range allow {
		if a == v {
			return a
		}
	}
	return allow[0] // 知らない名前は既定へ倒す
}

// #endregion measure
