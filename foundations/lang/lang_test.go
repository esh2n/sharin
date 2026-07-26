package lang

import "testing"

func TestLexer(t *testing.T) {
	input := `let five = 5;
let add = fn(x, y) { x + y };
!-/*5;
5 < 10 > 5;
if (5 == 5) { true } else { false != true }`

	want := []struct {
		typ TokenType
		lit string
	}{
		{LET, "let"}, {IDENT, "five"}, {ASSIGN, "="}, {INT, "5"}, {SEMICOLON, ";"},
		{LET, "let"}, {IDENT, "add"}, {ASSIGN, "="}, {FUNCTION, "fn"}, {LPAREN, "("},
		{IDENT, "x"}, {COMMA, ","}, {IDENT, "y"}, {RPAREN, ")"}, {LBRACE, "{"},
		{IDENT, "x"}, {PLUS, "+"}, {IDENT, "y"}, {RBRACE, "}"}, {SEMICOLON, ";"},
		{BANG, "!"}, {MINUS, "-"}, {SLASH, "/"}, {ASTERISK, "*"}, {INT, "5"}, {SEMICOLON, ";"},
		{INT, "5"}, {LT, "<"}, {INT, "10"}, {GT, ">"}, {INT, "5"}, {SEMICOLON, ";"},
		{IF, "if"}, {LPAREN, "("}, {INT, "5"}, {EQ, "=="}, {INT, "5"}, {RPAREN, ")"},
		{LBRACE, "{"}, {TRUE, "true"}, {RBRACE, "}"}, {ELSE, "else"}, {LBRACE, "{"},
		{FALSE, "false"}, {NOTEQ, "!="}, {TRUE, "true"}, {RBRACE, "}"}, {EOF, ""},
	}

	l := NewLexer(input)
	for i, w := range want {
		tok := l.NextToken()
		if tok.Type != w.typ || tok.Literal != w.lit {
			t.Fatalf("token[%d] = {%s %q}, want {%s %q}", i, tok.Type, tok.Literal, w.typ, w.lit)
		}
	}
}

func TestIllegalToken(t *testing.T) {
	l := NewLexer("@")
	if tok := l.NextToken(); tok.Type != ILLEGAL {
		t.Fatalf("want ILLEGAL, got %s", tok.Type)
	}
}

// 構文解析: 優先順位が正しく括弧に反映されるかを String() で確認する。
func TestOperatorPrecedence(t *testing.T) {
	cases := []struct{ input, want string }{
		{"-a * b", "((-a) * b)"},
		{"!-a", "(!(-a))"},
		{"a + b + c", "((a + b) + c)"},
		{"a + b * c", "(a + (b * c))"},
		{"a + b / c", "(a + (b / c))"},
		{"3 + 4 * 5 == 3 * 1 + 4 * 5", "((3 + (4 * 5)) == ((3 * 1) + (4 * 5)))"},
		{"1 < 2 == true", "((1 < 2) == true)"},
		{"(1 + 2) * 3", "((1 + 2) * 3)"},
		{"2 / (5 + 5)", "(2 / (5 + 5))"},
		{"add(a, b * c)", "add(a, (b * c))"},
		{"a + add(b, c) + d", "((a + add(b, c)) + d)"},
	}
	for _, c := range cases {
		p := NewParser(NewLexer(c.input))
		program := p.ParseProgram()
		if errs := p.Errors(); len(errs) > 0 {
			t.Fatalf("%q: parse errors: %v", c.input, errs)
		}
		if got := program.String(); got != c.want {
			t.Errorf("%q → %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseLetAndReturn(t *testing.T) {
	p := NewParser(NewLexer("let x = 5; return x;"))
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("errors: %v", p.Errors())
	}
	if len(program.Statements) != 2 {
		t.Fatalf("statements = %d, want 2", len(program.Statements))
	}
	if _, ok := program.Statements[0].(*LetStatement); !ok {
		t.Errorf("stmt[0] は LetStatement でない: %T", program.Statements[0])
	}
	if _, ok := program.Statements[1].(*ReturnStatement); !ok {
		t.Errorf("stmt[1] は ReturnStatement でない: %T", program.Statements[1])
	}
}

func TestParseErrors(t *testing.T) {
	p := NewParser(NewLexer("let = 5;"))
	p.ParseProgram()
	if len(p.Errors()) == 0 {
		t.Fatal("構文エラーが検出されるべき")
	}
}

// --- 評価 ---

func testInt(t *testing.T, input string, want int64) {
	t.Helper()
	obj := Run(input)
	i, ok := obj.(*Integer)
	if !ok {
		t.Fatalf("%q → %T(%s), want Integer", input, obj, obj.Inspect())
	}
	if i.Value != want {
		t.Errorf("%q → %d, want %d", input, i.Value, want)
	}
}

func TestEvalArithmetic(t *testing.T) {
	testInt(t, "5", 5)
	testInt(t, "-5", -5)
	testInt(t, "2 + 3 * 4", 14)
	testInt(t, "(2 + 3) * 4", 20)
	testInt(t, "50 / 2 * 2 + 10", 60)
	testInt(t, "-50 + 100 + -50", 0)
}

func TestEvalBoolean(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"true", true},
		{"1 < 2", true},
		{"1 > 2", false},
		{"1 == 1", true},
		{"1 != 1", false},
		{"true == true", true},
		{"true != false", true},
		{"!true", false},
		{"!!true", true},
		{"!5", false}, // 5 は truthy → !5 は false
	}
	for _, c := range cases {
		obj := Run(c.input)
		b, ok := obj.(*BooleanObj)
		if !ok {
			t.Fatalf("%q → %T, want BooleanObj", c.input, obj)
		}
		if b.Value != c.want {
			t.Errorf("%q → %v, want %v", c.input, b.Value, c.want)
		}
	}
}

func TestEvalIf(t *testing.T) {
	testInt(t, "if (true) { 10 }", 10)
	testInt(t, "if (1 < 2) { 10 } else { 20 }", 10)
	testInt(t, "if (1 > 2) { 10 } else { 20 }", 20)
	// else が無く偽 → null
	if obj := Run("if (false) { 10 }"); obj.Type() != NULL_OBJ {
		t.Errorf("want NULL, got %s", obj.Inspect())
	}
}

func TestEvalLetAndReturn(t *testing.T) {
	testInt(t, "let a = 5; a;", 5)
	testInt(t, "let a = 5; let b = a; b;", 5)
	testInt(t, "let a = 5; let b = a * 2; a + b;", 15)
	testInt(t, "return 10; 9;", 10)
	testInt(t, "if (10 > 1) { if (10 > 1) { return 10; } return 1; }", 10) // 入れ子returnが外へ
}

func TestEvalFunctionsAndClosures(t *testing.T) {
	testInt(t, "let id = fn(x) { x }; id(5);", 5)
	testInt(t, "let double = fn(x) { x * 2 }; double(5);", 10)
	testInt(t, "let add = fn(a, b) { a + b }; add(3, add(4, 5));", 12)
	// クロージャ: 返された関数が外側の変数 x を覚えている
	testInt(t, "let adder = fn(x) { fn(y) { x + y } }; let add2 = adder(2); add2(10);", 12)
	// 高階関数
	testInt(t, "let apply = fn(f, v) { f(v) }; apply(fn(x){ x * x }, 5);", 25)
}

func TestEvalErrors(t *testing.T) {
	cases := []struct{ input, wantContains string }{
		{"5 + true", "型が違う"},
		{"foobar", "未定義の変数"},
		{"true + false", "未対応の演算"},
		{"10 / 0", "ゼロ除算"},
		{"let f = fn(x){x}; f(1, 2)", "引数の数が違う"},
		{"1(2)", "関数ではない"},
	}
	for _, c := range cases {
		obj := Run(c.input)
		e, ok := obj.(*Error)
		if !ok {
			t.Fatalf("%q → %T(%s), want Error", c.input, obj, obj.Inspect())
		}
		if !contains(e.Message, c.wantContains) {
			t.Errorf("%q → %q, want contains %q", c.input, e.Message, c.wantContains)
		}
	}
}

func TestRunSyntaxError(t *testing.T) {
	obj := Run("let = ;")
	if e, ok := obj.(*Error); !ok || !contains(e.Message, "構文エラー") {
		t.Fatalf("構文エラーが返るべき: %v", obj)
	}
}

// AST の String() が if / fn / call を復元できること(デバッグ表示の担保)。
func TestASTString(t *testing.T) {
	cases := []struct{ input, want string }{
		{"if (x < y) { x } else { y }", "if(x < y) xelse y"},
		{"fn(x, y) { x + y }", "fn(x, y) (x + y)"},
		{"add(1, 2 * 3)", "add(1, (2 * 3))"},
		{"return true;", "return true;"},
		{"-5", "(-5)"},
	}
	for _, c := range cases {
		p := NewParser(NewLexer(c.input))
		got := p.ParseProgram().String()
		if len(p.Errors()) > 0 {
			t.Fatalf("%q: %v", c.input, p.Errors())
		}
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.input, got, c.want)
		}
	}
}

// 各 Object の Inspect() 表示。
func TestObjectInspect(t *testing.T) {
	cases := []struct {
		obj  Object
		want string
	}{
		{&Integer{Value: 42}, "42"},
		{TRUE_OBJ, "true"},
		{FALSE_OBJ, "false"},
		{NULL_OBJ_, "null"},
		{&ReturnValue{Value: &Integer{Value: 7}}, "7"},
		{&Error{Message: "boom"}, "ERROR: boom"},
	}
	for _, c := range cases {
		if got := c.obj.Inspect(); got != c.want {
			t.Errorf("Inspect() = %q, want %q", got, c.want)
		}
	}
	// 関数の Inspect と Type
	fn := Run("fn(x) { x }")
	if fn.Type() != FUNCTION_OBJ {
		t.Fatalf("want FUNCTION, got %s", fn.Type())
	}
	if fn.Inspect() != "fn(x) { ... }" {
		t.Errorf("fn.Inspect() = %q", fn.Inspect())
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
