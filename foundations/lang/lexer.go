package lang

// Lexer はソース文字列を1文字ずつ舐めてトークンに切り出す。
// position(今読んだ位置)と readPosition(次に読む位置)を持ち、ch に現在の文字を置く。

// #region lexer
type Lexer struct {
	input        string
	position     int  // 現在の文字の位置
	readPosition int  // 次に読む位置(先読み用)
	ch           byte // 現在検査中の文字
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar() // ch に最初の文字を入れる
	return l
}

// readChar は1文字進む。末尾を超えたら 0(NUL)で「終わり」を表す。
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
}

// peekChar は進まずに次の文字だけ覗く。== や != の2文字トークンの判定に要る。
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// NextToken は次の1トークンを返す。空白を飛ばし、現在の文字で分岐する。
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	var tok Token
	switch l.ch {
	case '=':
		if l.peekChar() == '=' { // "==" は2文字まとめて1トークン
			l.readChar()
			tok = Token{Type: EQ, Literal: "=="}
		} else {
			tok = newToken(ASSIGN, l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: NOTEQ, Literal: "!="}
		} else {
			tok = newToken(BANG, l.ch)
		}
	case '+':
		tok = newToken(PLUS, l.ch)
	case '-':
		tok = newToken(MINUS, l.ch)
	case '*':
		tok = newToken(ASTERISK, l.ch)
	case '/':
		tok = newToken(SLASH, l.ch)
	case '<':
		tok = newToken(LT, l.ch)
	case '>':
		tok = newToken(GT, l.ch)
	case ',':
		tok = newToken(COMMA, l.ch)
	case ';':
		tok = newToken(SEMICOLON, l.ch)
	case '(':
		tok = newToken(LPAREN, l.ch)
	case ')':
		tok = newToken(RPAREN, l.ch)
	case '{':
		tok = newToken(LBRACE, l.ch)
	case '}':
		tok = newToken(RBRACE, l.ch)
	case 0:
		tok = Token{Type: EOF, Literal: ""}
	default:
		// 記号でなければ、識別子(文字始まり)か数値(数字始まり)のかたまりを読む
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			return tok // readIdentifier が既に次へ進めているので即返す
		} else if isDigit(l.ch) {
			tok.Literal = l.readNumber()
			tok.Type = INT
			return tok
		}
		tok = newToken(ILLEGAL, l.ch)
	}

	l.readChar()
	return tok
}

// #endregion lexer

// readIdentifier は識別子を読む。先頭は英字(呼ぶ側が保証)、2文字目以降は英字か数字。
// これで add2 のような名前が add + 2 に割れず1トークンになる。
func (l *Lexer) readIdentifier() string {
	start := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.position]
}

func (l *Lexer) readNumber() string {
	start := l.position
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.position]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
func newToken(t TokenType, ch byte) Token {
	return Token{Type: t, Literal: string(ch)}
}
