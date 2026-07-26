package bytecode

import (
	"strings"
	"testing"

	"github.com/esh2n/sharin/foundations/lang"
)

// --- code.go: 符号化と逆アセンブル ---

func TestMakeEncodesOpcodeAndOperand(t *testing.T) {
	ins := Make(OpConstant, 65534)
	want := []byte{byte(OpConstant), 255, 254} // 65534 = 0xFFFE(ビッグエンディアン)
	if len(ins) != len(want) {
		t.Fatalf("length: want %d got %d", len(want), len(ins))
	}
	for i, b := range want {
		if ins[i] != b {
			t.Fatalf("byte %d: want %d got %d", i, b, ins[i])
		}
	}
	// オペランドを取らない命令は 1 バイト。
	if got := Make(OpAdd); len(got) != 1 || got[0] != byte(OpAdd) {
		t.Fatalf("OpAdd encode: got %v", got)
	}
	// 未知オペコードは空。
	if got := Make(Opcode(250), 1); len(got) != 0 {
		t.Fatalf("unknown opcode encode: want empty, got %v", got)
	}
}

func TestReadUint16RoundTrips(t *testing.T) {
	ins := Make(OpJump, 40000)
	if got := ReadUint16(ins[1:]); got != 40000 {
		t.Fatalf("ReadUint16: want 40000 got %d", got)
	}
}

func TestInstructionsDisassemble(t *testing.T) {
	concat := Instructions{}
	concat = append(concat, Make(OpConstant, 1)...)
	concat = append(concat, Make(OpConstant, 2)...)
	concat = append(concat, Make(OpAdd)...)
	concat = append(concat, Make(OpPop)...)
	want := "0000 OpConstant 1\n0003 OpConstant 2\n0006 OpAdd\n0007 OpPop\n"
	if got := concat.String(); got != want {
		t.Fatalf("disassemble:\n got %q\nwant %q", got, want)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, err := Lookup(250); err == nil {
		t.Fatalf("Lookup(unknown): want error")
	}
}

func TestDisassembleUnknownByte(t *testing.T) {
	// 未知バイトが混じっても String はパニックせず ERROR 行を出して進む。
	out := Instructions{250}.String()
	if !strings.Contains(out, "ERROR") {
		t.Fatalf("want ERROR line, got %q", out)
	}
}

// --- 実行(コンパイル + VM)。値まで通す ---

func runInt(t *testing.T, src string, want int64) {
	t.Helper()
	got, err := Run(src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	i, ok := got.(*lang.Integer)
	if !ok {
		t.Fatalf("Run(%q): want Integer, got %T (%s)", src, got, got.Inspect())
	}
	if i.Value != want {
		t.Fatalf("Run(%q) = %d, want %d", src, i.Value, want)
	}
}

func runBool(t *testing.T, src string, want bool) {
	t.Helper()
	got, err := Run(src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	b, ok := got.(*lang.BooleanObj)
	if !ok {
		t.Fatalf("Run(%q): want Boolean, got %T", src, got)
	}
	if b.Value != want {
		t.Fatalf("Run(%q) = %v, want %v", src, b.Value, want)
	}
}

func TestArithmetic(t *testing.T) {
	runInt(t, "1 + 2", 3)
	runInt(t, "5 - 3", 2)
	runInt(t, "4 * 5", 20)
	runInt(t, "20 / 4", 5)
	runInt(t, "2 * 3 + 4", 10)   // 優先順位: (2*3)+4
	runInt(t, "2 + 3 * 4", 14)   // 2+(3*4)
	runInt(t, "(2 + 3) * 4", 20) // 括弧
	runInt(t, "-5", -5)
	runInt(t, "-(3 + 4)", -7)
}

func TestBooleansAndComparison(t *testing.T) {
	runBool(t, "true", true)
	runBool(t, "false", false)
	runBool(t, "1 < 2", true)   // < は > へ読み替えられる
	runBool(t, "2 < 1", false)
	runBool(t, "1 > 2", false)
	runBool(t, "1 == 1", true)
	runBool(t, "1 != 1", false)
	runBool(t, "true == true", true)
	runBool(t, "true != false", true)
	runBool(t, "!true", false)
	runBool(t, "!false", true)
	runBool(t, "!!true", true)
	runBool(t, "(1 < 2) == true", true)
}

func TestIfExpression(t *testing.T) {
	runInt(t, "if (true) { 10 }", 10)
	runInt(t, "if (true) { 10 } else { 20 }", 10)
	runInt(t, "if (false) { 10 } else { 20 }", 20)
	runInt(t, "if (1 < 2) { 10 } else { 20 }", 10)
	runInt(t, "if (1 > 2) { 30 } else { 40 }", 40)
	// else 無しで条件が偽 → 値は null。
	got, err := Run("if (false) { 10 }")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := got.(*lang.Null); !ok {
		t.Fatalf("falsy if without else: want Null, got %T", got)
	}
}

func TestGlobalLetBindings(t *testing.T) {
	runInt(t, "let x = 5; x", 5)
	runInt(t, "let x = 5; let y = 10; x + y", 15)
	runInt(t, "let x = 5; let y = x * 2; y", 10)
	runInt(t, "let x = 1; let x = x + 9; x", 10) // 再代入は同じ番号に上書き
}

func TestConditionalWithGlobals(t *testing.T) {
	runInt(t, "let x = 3; if (x > 2) { x * 10 } else { 0 }", 30)
}

// --- エラー系 ---

func TestRuntimeAndCompileErrors(t *testing.T) {
	cases := []struct {
		src  string
		frag string
	}{
		{"5 / 0", "ゼロ除算"},
		{"foo", "未定義の変数"},
		{"fn(x) { x }", "関数を扱わない"},
		{"foo()", "関数を扱わない"},
		{"let x = 1 +", "構文エラー"},
		{"-true", "整数にしか使えない"},
		{"true + false", "整数どうし"},
		{"true > false", "真偽値には使えない"},
	}
	for _, c := range cases {
		_, err := Run(c.src)
		if err == nil {
			t.Fatalf("Run(%q): want error containing %q, got nil", c.src, c.frag)
		}
		if !strings.Contains(err.Error(), c.frag) {
			t.Fatalf("Run(%q): want error containing %q, got %q", c.src, c.frag, err.Error())
		}
	}
}

func TestReturnFromCompilerIsRejected(t *testing.T) {
	// return 文もフレームが要るので範囲外(明示的に弾く)。
	_, err := Run("return 5;")
	if err == nil || !strings.Contains(err.Error(), "関数を扱わない") {
		t.Fatalf("return should be rejected, got %v", err)
	}
}

// --- コンパイル成果物そのものの検査(命令列の形) ---

func TestCompileEmitsExpectedInstructions(t *testing.T) {
	p := lang.NewParser(lang.NewLexer("1 + 2"))
	comp := New()
	if err := comp.Compile(p.ParseProgram()); err != nil {
		t.Fatalf("compile: %v", err)
	}
	bc := comp.Bytecode()
	want := "0000 OpConstant 0\n0003 OpConstant 1\n0006 OpAdd\n0007 OpPop\n"
	if got := bc.Instructions.String(); got != want {
		t.Fatalf("instructions:\n got %q\nwant %q", got, want)
	}
	if len(bc.Constants) != 2 {
		t.Fatalf("constants: want 2 got %d", len(bc.Constants))
	}
}

func TestIfCompilesToJumps(t *testing.T) {
	// if は OpJumpNotTruthy / OpJump に化ける——制御フローがジャンプになる証拠。
	p := lang.NewParser(lang.NewLexer("if (true) { 10 } else { 20 }"))
	comp := New()
	if err := comp.Compile(p.ParseProgram()); err != nil {
		t.Fatalf("compile: %v", err)
	}
	dis := comp.Bytecode().Instructions.String()
	if !strings.Contains(dis, "OpJumpNotTruthy") || !strings.Contains(dis, "OpJump ") {
		t.Fatalf("if should compile to jumps, got:\n%s", dis)
	}
}

// --- VM の細かな観察 ---

func TestStackTopAndLastPopped(t *testing.T) {
	p := lang.NewParser(lang.NewLexer("1; 2; 3"))
	comp := New()
	_ = comp.Compile(p.ParseProgram())
	vm := NewVM(comp.Bytecode())
	if vm.StackTop() != nil {
		t.Fatalf("StackTop before run: want nil")
	}
	if err := vm.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	last, ok := vm.LastPopped().(*lang.Integer)
	if !ok || last.Value != 3 {
		t.Fatalf("LastPopped: want 3, got %v", vm.LastPopped())
	}
}

func TestStackOverflowGuard(t *testing.T) {
	// 定数を stackSize + 1 回積むだけの命令列を手で作り、あふれを起こす。
	ins := Instructions{}
	for i := 0; i < stackSize+1; i++ {
		ins = append(ins, Make(OpConstant, 0)...)
	}
	vm := NewVM(&Bytecode{Instructions: ins, Constants: []lang.Object{&lang.Integer{Value: 1}}})
	if err := vm.Run(); err == nil || !strings.Contains(err.Error(), "あふれ") {
		t.Fatalf("want stack overflow, got %v", err)
	}
}

func TestUnknownOpcodeAtRuntime(t *testing.T) {
	vm := NewVM(&Bytecode{Instructions: Instructions{250}, Constants: nil})
	if err := vm.Run(); err == nil || !strings.Contains(err.Error(), "未知のオペコード") {
		t.Fatalf("want unknown opcode error, got %v", err)
	}
}

func TestStackTopReturnsValueWhenNonEmpty(t *testing.T) {
	// OpPop を付けない命令列を手で組むと、実行後もスタック先頭に値が残る。
	vm := NewVM(&Bytecode{
		Instructions: Make(OpConstant, 0),
		Constants:    []lang.Object{&lang.Integer{Value: 42}},
	})
	if err := vm.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	top, ok := vm.StackTop().(*lang.Integer)
	if !ok || top.Value != 42 {
		t.Fatalf("StackTop: want 42, got %v", vm.StackTop())
	}
}

func TestIsTruthyIntegerAndNull(t *testing.T) {
	// 非真偽値(整数)は真として扱う。
	runInt(t, "if (5) { 10 } else { 20 }", 10)
	// null は偽。null に ! を付けると true(isTruthy(Null)=false の反転)。
	runBool(t, "!(if (false) { 1 })", true)
}

func TestIfBranchEndingInLetDoesNotCrash(t *testing.T) {
	// 本体が let で終わる if は値を残さない(退化)。パニックせず通ることだけ確かめる。
	if _, err := Run("if (true) { let x = 5 } else { 0 }"); err != nil {
		t.Fatalf("degenerate if: %v", err)
	}
}

func TestCompileIfPropagatesErrors(t *testing.T) {
	// if の 条件 / then / else それぞれのコンパイルエラーが伝播する。
	for _, src := range []string{
		"if (foo) { 1 }",           // 条件で未定義
		"if (true) { foo }",        // then で未定義
		"if (true) { 1 } else { bar }", // else で未定義
	} {
		if _, err := Run(src); err == nil || !strings.Contains(err.Error(), "未定義の変数") {
			t.Fatalf("Run(%q): want undefined-var error, got %v", src, err)
		}
	}
}

func TestFmtInstructionErrorBranches(t *testing.T) {
	// オペランド数が定義と食い違う → ERROR 文字列(パニックしない)。
	mismatch := fmtInstruction(&Definition{Name: "X", OperandWidths: []int{2}}, []int{})
	if !strings.Contains(mismatch, "ERROR") {
		t.Fatalf("operand-count mismatch: want ERROR, got %q", mismatch)
	}
	// 2 オペランド以上は未対応 → ERROR。
	tooMany := fmtInstruction(&Definition{Name: "Y", OperandWidths: []int{2, 2}}, []int{1, 2})
	if !strings.Contains(tooMany, "ERROR") {
		t.Fatalf("2-operand: want ERROR, got %q", tooMany)
	}
}
