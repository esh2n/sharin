package wasm

import (
	"strings"
	"testing"
)

// --- 小さな .wasm エンコーダ(テスト用)。実バイナリを組んで実パーサに食わせる ---

func cat(parts ...[]byte) []byte {
	var o []byte
	for _, p := range parts {
		o = append(o, p...)
	}
	return o
}

// encU は符号なし LEB128。
func encU(n uint32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			out = append(out, b|0x80)
		} else {
			out = append(out, b)
			return out
		}
	}
}

// encS は符号つき LEB128。
func encS(n int32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7 // Go の int32 >> は算術シフト
		if (n == 0 && b&0x40 == 0) || (n == -1 && b&0x40 != 0) {
			out = append(out, b)
			return out
		}
		out = append(out, b|0x80)
	}
}

func valvec(n int) []byte {
	out := encU(uint32(n))
	for i := 0; i < n; i++ {
		out = append(out, i32Type)
	}
	return out
}
func vec(items [][]byte) []byte {
	out := encU(uint32(len(items)))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}
func sect(id byte, content []byte) []byte {
	return cat([]byte{id}, encU(uint32(len(content))), content)
}

// 命令エンコーダ。
func i32c(v int32) []byte   { return cat([]byte{opI32Const}, encS(v)) }
func lget(i uint32) []byte  { return cat([]byte{opLocalGet}, encU(i)) }
func lset(i uint32) []byte  { return cat([]byte{opLocalSet}, encU(i)) }
func ltee(i uint32) []byte  { return cat([]byte{opLocalTee}, encU(i)) }
func callf(i uint32) []byte { return cat([]byte{opCall}, encU(i)) }
func br(i uint32) []byte    { return cat([]byte{opBr}, encU(i)) }
func brif(i uint32) []byte  { return cat([]byte{opBrIf}, encU(i)) }

var (
	end   = []byte{opEnd}
	block = []byte{opBlock, blockVoid}
	loop  = []byte{opLoop, blockVoid}
	ifv   = []byte{opIf, blockVoid}
	ret   = []byte{opReturn}
)

type fn struct {
	params, results, locals int
	body                    []byte
	export                  string
}

func buildModule(fns []fn) []byte {
	var types, funcs, exports, codes [][]byte
	for i, f := range fns {
		types = append(types, cat([]byte{funcTypeTag}, valvec(f.params), valvec(f.results)))
		funcs = append(funcs, encU(uint32(i)))
		if f.export != "" {
			exports = append(exports, cat(encU(uint32(len(f.export))), []byte(f.export), []byte{exportFunc}, encU(uint32(i))))
		}
		var localsDecl []byte
		if f.locals > 0 {
			localsDecl = cat(encU(1), encU(uint32(f.locals)), []byte{i32Type})
		} else {
			localsDecl = encU(0)
		}
		inner := cat(localsDecl, f.body)
		codes = append(codes, cat(encU(uint32(len(inner))), inner))
	}
	return cat(
		[]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
		sect(secType, vec(types)),
		sect(secFunc, vec(funcs)),
		sect(secExport, vec(exports)),
		sect(secCode, vec(codes)),
	)
}

func mustRun(t *testing.T, bin []byte, fn string, args ...int32) int32 {
	t.Helper()
	got, err := Run(bin, fn, args...)
	if err != nil {
		t.Fatalf("Run(%s, %v): %v", fn, args, err)
	}
	return got
}

// --- テスト本体 ---

func TestAddFunction(t *testing.T) {
	// (a, b) => a + b。local.get + i32.add + end。
	bin := buildModule([]fn{{params: 2, results: 1, body: cat(lget(0), lget(1), []byte{opI32Add}, end), export: "add"}})
	if got := mustRun(t, bin, "add", 2, 3); got != 5 {
		t.Fatalf("add(2,3)=%d, want 5", got)
	}
	if got := mustRun(t, bin, "add", -4, 10); got != 6 {
		t.Fatalf("add(-4,10)=%d, want 6", got)
	}
}

func TestMaxWithIfReturn(t *testing.T) {
	// (a,b) => a>b ? a : b。else 無しの if + return + フォールスルー。
	body := cat(lget(0), lget(1), []byte{opI32GtS}, ifv, lget(0), ret, end, lget(1), end)
	bin := buildModule([]fn{{params: 2, results: 1, body: body, export: "max"}})
	if got := mustRun(t, bin, "max", 3, 7); got != 7 {
		t.Fatalf("max(3,7)=%d want 7", got)
	}
	if got := mustRun(t, bin, "max", 9, 2); got != 9 {
		t.Fatalf("max(9,2)=%d want 9", got)
	}
}

func TestSumToWithLoop(t *testing.T) {
	// sum_to(n) = n + (n-1) + ... + 1。block/loop/br/br_if で構造化ループ。
	// local 0 = n(引数), local 1 = acc。
	body := cat(
		i32c(0), lset(1), // acc = 0
		block,                              //           ラベル1(脱出先)
		loop,                               //           ラベル0(繰り返し)
		lget(0), []byte{opI32Eqz}, brif(1), // n==0 なら block を抜ける
		lget(1), lget(0), []byte{opI32Add}, lset(1), // acc += n
		lget(0), i32c(1), []byte{opI32Sub}, lset(0), // n -= 1
		br(0),   //           loop の先頭へ戻る
		end,     //           loop 終わり
		end,     //           block 終わり
		lget(1), // return acc
		end,
	)
	bin := buildModule([]fn{{params: 1, results: 1, locals: 1, body: body, export: "sum_to"}})
	if got := mustRun(t, bin, "sum_to", 5); got != 15 {
		t.Fatalf("sum_to(5)=%d want 15", got)
	}
	if got := mustRun(t, bin, "sum_to", 100); got != 5050 {
		t.Fatalf("sum_to(100)=%d want 5050", got)
	}
	if got := mustRun(t, bin, "sum_to", 0); got != 0 {
		t.Fatalf("sum_to(0)=%d want 0", got)
	}
}

func TestFactorialRecursive(t *testing.T) {
	// fact(n) = n==0 ? 1 : n*fact(n-1)。再帰呼び出し(call が自分自身)。
	body := cat(
		lget(0), []byte{opI32Eqz}, ifv, i32c(1), ret, end, // if n==0 return 1
		lget(0), lget(0), i32c(1), []byte{opI32Sub}, callf(0), []byte{opI32Mul}, // n * fact(n-1)
		end,
	)
	bin := buildModule([]fn{{params: 1, results: 1, body: body, export: "fact"}})
	if got := mustRun(t, bin, "fact", 5); got != 120 {
		t.Fatalf("fact(5)=%d want 120", got)
	}
	if got := mustRun(t, bin, "fact", 0); got != 1 {
		t.Fatalf("fact(0)=%d want 1", got)
	}
	if got := mustRun(t, bin, "fact", 6); got != 720 {
		t.Fatalf("fact(6)=%d want 720", got)
	}
}

func TestCrossFunctionCall(t *testing.T) {
	// compute() = add(3, 4)。別関数の call とインデックス解決。
	add := fn{params: 2, results: 1, body: cat(lget(0), lget(1), []byte{opI32Add}, end), export: "add"}
	compute := fn{params: 0, results: 1, body: cat(i32c(3), i32c(4), callf(0), end), export: "compute"}
	bin := buildModule([]fn{add, compute})
	if got := mustRun(t, bin, "compute"); got != 7 {
		t.Fatalf("compute()=%d want 7", got)
	}
}

func TestConstLEB128Encoding(t *testing.T) {
	// 複数バイト LEB128 と符号拡張を通す(200 は2バイト、-200 も)。
	pos := buildModule([]fn{{results: 1, body: cat(i32c(200), end), export: "c"}})
	if got := mustRun(t, pos, "c"); got != 200 {
		t.Fatalf("const 200=%d", got)
	}
	neg := buildModule([]fn{{results: 1, body: cat(i32c(-200), end), export: "c"}})
	if got := mustRun(t, neg, "c"); got != -200 {
		t.Fatalf("const -200=%d", got)
	}
}

func TestMiscOpsTeeDropRem(t *testing.T) {
	// (a) => (a%3)!=0。local.tee / drop / i32.rem_s / i32.ne を通す。
	body := cat(
		lget(0), i32c(3), []byte{opI32RemS}, ltee(1), []byte{opDrop},
		lget(1), i32c(0), []byte{opI32Ne},
		end,
	)
	bin := buildModule([]fn{{params: 1, results: 1, locals: 1, body: body, export: "m"}})
	if got := mustRun(t, bin, "m", 5); got != 1 {
		t.Fatalf("m(5)=%d want 1", got)
	}
	if got := mustRun(t, bin, "m", 6); got != 0 {
		t.Fatalf("m(6)=%d want 0", got)
	}
}

func TestNopExecutes(t *testing.T) {
	body := cat([]byte{opNop}, i32c(42), []byte{opNop}, end)
	bin := buildModule([]fn{{results: 1, body: body, export: "f"}})
	if got := mustRun(t, bin, "f"); got != 42 {
		t.Fatalf("nop f()=%d want 42", got)
	}
}

func TestBinaryOpsTable(t *testing.T) {
	// (a,b) => a <op> b を全二項演算ぶん組んで実行し、binI32 の全分岐を通す。
	cases := []struct {
		op   byte
		a, b int32
		want int32
	}{
		{opI32Add, 7, 5, 12},
		{opI32Sub, 7, 5, 2},
		{opI32Mul, 7, 5, 35},
		{opI32DivS, 20, 6, 3},
		{opI32RemS, 20, 6, 2},
		{opI32Eq, 5, 5, 1},
		{opI32Ne, 5, 6, 1},
		{opI32LtS, 3, 4, 1},
		{opI32GtS, 4, 3, 1},
		{opI32LeS, 4, 4, 1},
		{opI32GeS, 4, 5, 0},
	}
	for _, c := range cases {
		bin := buildModule([]fn{{params: 2, results: 1, body: cat(lget(0), lget(1), []byte{c.op}, end), export: "op"}})
		if got := mustRun(t, bin, "op", c.a, c.b); got != c.want {
			t.Fatalf("op 0x%x (%d,%d)=%d want %d", c.op, c.a, c.b, got, c.want)
		}
	}
}

// --- エラー系 ---

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte{0, 0, 0, 0}); err == nil || !strings.Contains(err.Error(), "マジック") {
		t.Fatalf("bad magic: %v", err)
	}
	badVer := cat([]byte{0x00, 0x61, 0x73, 0x6d, 0x02, 0, 0, 0})
	if _, err := Parse(badVer); err == nil || !strings.Contains(err.Error(), "バージョン") {
		t.Fatalf("bad version: %v", err)
	}
	// 未対応オペコード(0xFE)を本体に混ぜる。
	bin := buildModule([]fn{{results: 1, body: cat([]byte{0xFE}, i32c(1), end), export: "f"}})
	if _, err := Parse(bin); err == nil || !strings.Contains(err.Error(), "未対応のオペコード") {
		t.Fatalf("unsupported opcode: %v", err)
	}
}

func TestRuntimeErrors(t *testing.T) {
	// ゼロ除算。
	divz := buildModule([]fn{{params: 1, results: 1, body: cat(lget(0), i32c(0), []byte{opI32DivS}, end), export: "d"}})
	if _, err := Run(divz, "d", 10); err == nil || !strings.Contains(err.Error(), "ゼロ除算") {
		t.Fatalf("div by zero: %v", err)
	}
	// unreachable。
	unr := buildModule([]fn{{results: 1, body: cat([]byte{opUnreachable}, end), export: "u"}})
	if _, err := Run(unr, "u"); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("unreachable: %v", err)
	}
	// 存在しない export。
	add := buildModule([]fn{{params: 2, results: 1, body: cat(lget(0), lget(1), []byte{opI32Add}, end), export: "add"}})
	if _, err := Run(add, "nope"); err == nil || !strings.Contains(err.Error(), "export") {
		t.Fatalf("missing export: %v", err)
	}
	// 引数の数が違う。
	if _, err := Run(add, "add", 1); err == nil || !strings.Contains(err.Error(), "引数の数") {
		t.Fatalf("arg count: %v", err)
	}
}

func TestRemZeroError(t *testing.T) {
	remz := buildModule([]fn{{params: 1, results: 1, body: cat(lget(0), i32c(0), []byte{opI32RemS}, end), export: "r"}})
	if _, err := Run(remz, "r", 10); err == nil || !strings.Contains(err.Error(), "ゼロ剰余") {
		t.Fatalf("rem by zero: %v", err)
	}
}

func TestInfiniteRecursionGuard(t *testing.T) {
	// f() = f() を延々。深さガードで止まる。
	bin := buildModule([]fn{{results: 1, body: cat(callf(0), end), export: "f"}})
	if _, err := Run(bin, "f"); err == nil || !strings.Contains(err.Error(), "深すぎる") {
		t.Fatalf("recursion guard: %v", err)
	}
}

func TestAbsWithIfElse(t *testing.T) {
	// abs(n) = n<0 ? -n : n。if/else(else 節あり)を通す。
	els := []byte{opElse}
	body := cat(
		lget(0), i32c(0), []byte{opI32LtS}, ifv,
		i32c(0), lget(0), []byte{opI32Sub}, ret, // then: -n
		els,
		lget(0), ret, // else: n
		end,          // end if
		lget(0), end, // (到達しない)関数末尾
	)
	bin := buildModule([]fn{{params: 1, results: 1, body: body, export: "abs"}})
	if got := mustRun(t, bin, "abs", -5); got != 5 {
		t.Fatalf("abs(-5)=%d want 5", got)
	}
	if got := mustRun(t, bin, "abs", 8); got != 8 {
		t.Fatalf("abs(8)=%d want 8", got)
	}
}

func TestMissingResultError(t *testing.T) {
	// results=1 なのに空スタックで関数を終える → 戻り値が足りない。
	bin := buildModule([]fn{{results: 1, body: end, export: "f"}})
	if _, err := Run(bin, "f"); err == nil || !strings.Contains(err.Error(), "戻り値が足りない") {
		t.Fatalf("missing result: %v", err)
	}
}

func TestMalformedTruncations(t *testing.T) {
	// 正常モジュールを 1 バイトずつ切り詰めても、パニックせずエラーで返ること。
	// 途中終端に対する多数の防御分岐(readByte/readBytes/uleb 等)を通す。
	full := buildModule([]fn{{params: 1, results: 1, locals: 1, body: cat(
		i32c(0), lset(1), block, loop, lget(0), []byte{opI32Eqz}, brif(1),
		lget(1), lget(0), []byte{opI32Add}, lset(1),
		lget(0), i32c(1), []byte{opI32Sub}, lset(0), br(0), end, end, lget(1), end,
	), export: "sum_to"}})
	for i := 1; i < len(full); i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Parse panicked on truncation len=%d: %v", i, r)
				}
			}()
			_, _ = Parse(full[:i]) // エラーでも成功でもよい。パニックしないことが条件
		}()
	}
}

func TestTypeAndLocalValidation(t *testing.T) {
	header := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	// i64(0x7e)の引数型 → i32 以外は未対応。
	badVal := cat(header, sect(secType, vec([][]byte{cat([]byte{funcTypeTag}, encU(1), []byte{0x7e}, encU(0))})))
	if _, err := Parse(badVal); err == nil || !strings.Contains(err.Error(), "i32 以外") {
		t.Fatalf("bad valtype: %v", err)
	}
	// 関数型タグが 0x60 でない。
	badTag := cat(header, sect(secType, vec([][]byte{cat([]byte{0x61}, encU(0), encU(0))})))
	if _, err := Parse(badTag); err == nil || !strings.Contains(err.Error(), "関数型タグ") {
		t.Fatalf("bad tag: %v", err)
	}
	// i64 のローカル。
	localsBad := cat(encU(1), encU(1), []byte{0x7e})
	inner := cat(localsBad, i32c(0), end)
	badLocal := cat(header,
		sect(secType, vec([][]byte{cat([]byte{funcTypeTag}, encU(0), valvec(1))})),
		sect(secFunc, vec([][]byte{encU(0)})),
		sect(secCode, vec([][]byte{cat(encU(uint32(len(inner))), inner)})),
	)
	if _, err := Parse(badLocal); err == nil || !strings.Contains(err.Error(), "i32 以外のローカル") {
		t.Fatalf("bad local: %v", err)
	}
	// コード数が関数数を超える。
	tooMany := cat(header,
		sect(secType, vec([][]byte{cat([]byte{funcTypeTag}, encU(0), valvec(1))})),
		sect(secFunc, vec([][]byte{encU(0)})),
		sect(secCode, vec([][]byte{
			cat(encU(uint32(len(cat(encU(0), i32c(1), end)))), cat(encU(0), i32c(1), end)),
			cat(encU(uint32(len(cat(encU(0), i32c(1), end)))), cat(encU(0), i32c(1), end)),
		})),
	)
	if _, err := Parse(tooMany); err == nil || !strings.Contains(err.Error(), "コード数") {
		t.Fatalf("code>func: %v", err)
	}
}

func TestExecBoundsErrors(t *testing.T) {
	// local 範囲外。
	lo := buildModule([]fn{{results: 1, body: cat(lget(5), end), export: "f"}})
	if _, err := Run(lo, "f"); err == nil || !strings.Contains(err.Error(), "local 範囲外") {
		t.Fatalf("local oob: %v", err)
	}
	// call 先が範囲外。
	co := buildModule([]fn{{results: 1, body: cat(callf(9), end), export: "f"}})
	if _, err := Run(co, "f"); err == nil || !strings.Contains(err.Error(), "call 先") {
		t.Fatalf("call oob: %v", err)
	}
	// call の引数不足(add は2引数だが空スタックから呼ぶ)。
	add := fn{params: 2, results: 1, body: cat(lget(0), lget(1), []byte{opI32Add}, end), export: "add"}
	g := fn{results: 1, body: cat(callf(0), end), export: "g"}
	if _, err := Run(buildModule([]fn{add, g}), "g"); err == nil || !strings.Contains(err.Error(), "引数が足りない") {
		t.Fatalf("call underflow: %v", err)
	}
}

func TestInvokeMultiAndVoid(t *testing.T) {
	// 戻り値なし関数(results=0)。Invoke は空スライスを返す。
	m, err := Parse(buildModule([]fn{{params: 1, results: 0, body: cat(lget(0), []byte{opDrop}, end), export: "v"}}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res, err := NewInterp(m).Invoke("v", 9)
	if err != nil {
		t.Fatalf("invoke void: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("void result: want empty, got %v", res)
	}
	// Run は単一 i32 を期待するので void はエラー。
	if _, err := Run(buildModule([]fn{{params: 1, results: 0, body: cat(lget(0), []byte{opDrop}, end), export: "v"}}), "v", 1); err == nil {
		t.Fatalf("Run on void should error")
	}
}
