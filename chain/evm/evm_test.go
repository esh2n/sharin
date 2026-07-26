package evm

import (
	"errors"
	"testing"
)

// PUSH1 2, PUSH1 3, ADD → スタックに 5。gas は 3+3+3=9。
func TestArithmeticAndGas(t *testing.T) {
	code := []byte{
		byte(OpPUSH1), 0x02,
		byte(OpPUSH1), 0x03,
		byte(OpADD),
		byte(OpSTOP),
	}
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if !r.Success {
		t.Fatalf("call failed: %v", r.Err)
	}
	if len(r.Stack) != 1 || r.Stack[0] != 5 {
		t.Fatalf("stack = %v, want [5]", r.Stack)
	}
	if r.GasUsed != 9 {
		t.Fatalf("gas used = %d, want 9", r.GasUsed)
	}
}

// counter: storage[0] を +1 する。2 回呼ぶと 2 になる(状態が永続する)。
func counterCode() []byte {
	return []byte{
		byte(OpPUSH1), 0x00, // slot
		byte(OpSLOAD),       // storage[0]
		byte(OpPUSH1), 0x01, // +1
		byte(OpADD),
		byte(OpPUSH1), 0x00, // slot(SSTORE は slot を先に pop)
		byte(OpSSTORE),
		byte(OpSTOP),
	}
}

func TestCounterPersistsState(t *testing.T) {
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", counterCode())

	for i := 1; i <= 3; i++ {
		r := e.Call("alice", addr, 0, 1000)
		if !r.Success {
			t.Fatalf("call %d failed: %v", i, r.Err)
		}
		if got := st.Storage(addr, 0); got != uint64(i) {
			t.Fatalf("after call %d storage[0] = %d, want %d", i, got, i)
		}
	}
	// gas: PUSH1 3 + SLOAD 50 + PUSH1 3 + ADD 3 + PUSH1 3 + SSTORE 100 = 162
	r := e.Call("alice", addr, 0, 1000)
	if r.GasUsed != 162 {
		t.Fatalf("gas = %d, want 162", r.GasUsed)
	}
}

func TestValueTransferToEOA(t *testing.T) {
	st := NewState()
	st.GetOrCreate("alice").Balance = 100
	e := NewEVM(st)
	r := e.Call("alice", "bob", 30, 1000)
	if !r.Success {
		t.Fatalf("transfer failed: %v", r.Err)
	}
	if st.Balance("alice") != 70 || st.Balance("bob") != 30 {
		t.Fatalf("balances: alice=%d bob=%d, want 70/30", st.Balance("alice"), st.Balance("bob"))
	}
}

func TestInsufficientBalanceReverts(t *testing.T) {
	st := NewState()
	st.GetOrCreate("alice").Balance = 10
	e := NewEVM(st)
	r := e.Call("alice", "bob", 30, 1000)
	if r.Success || !r.Reverted {
		t.Fatalf("expected revert, got %+v", r)
	}
	if !errors.Is(r.Err, ErrInsufficientBalance) {
		t.Fatalf("want ErrInsufficientBalance, got %v", r.Err)
	}
	// 残高は動いていない。
	if st.Balance("alice") != 10 || st.Balance("bob") != 0 {
		t.Fatalf("balances changed on revert: alice=%d bob=%d", st.Balance("alice"), st.Balance("bob"))
	}
}

// out-of-gas: 状態は巻き戻るが gas は全部消費される(=gasLimit)。
func TestOutOfGasRevertsButConsumesGas(t *testing.T) {
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", counterCode())
	// SLOAD(50) に届かない gas しか与えない。
	r := e.Call("alice", addr, 0, 40)
	if !r.Reverted {
		t.Fatalf("expected revert on OOG, got %+v", r)
	}
	if !errors.Is(r.Err, ErrOutOfGas) {
		t.Fatalf("want ErrOutOfGas, got %v", r.Err)
	}
	if r.GasUsed != 40 {
		t.Fatalf("OOG should consume all gas: used=%d want 40", r.GasUsed)
	}
	if st.Storage(addr, 0) != 0 {
		t.Fatalf("storage changed on OOG revert: %d", st.Storage(addr, 0))
	}
}

// REVERT 命令: 直前に書いたストレージも巻き戻るが、gas は消費される。
func TestExplicitRevertRollsBackState(t *testing.T) {
	// storage[0]=42 を書いてから REVERT する。
	code := []byte{
		byte(OpPUSH1), 0x2a, // value 42
		byte(OpPUSH1), 0x00, // slot 0
		byte(OpSSTORE),
		byte(OpREVERT),
	}
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if !r.Reverted || !errors.Is(r.Err, ErrRevert) {
		t.Fatalf("expected ErrRevert, got %+v", r)
	}
	if st.Storage(addr, 0) != 0 {
		t.Fatalf("SSTORE before REVERT should roll back, got %d", st.Storage(addr, 0))
	}
	if r.GasUsed == 0 {
		t.Fatalf("REVERT should still consume gas, used=%d", r.GasUsed)
	}
}

// value 送金額で分岐: value>0 なら 42 を保存、value==0 なら REVERT。
func requireValueCode() []byte {
	return []byte{
		byte(OpCALLVALUE),   // [value]
		byte(OpPUSH1), 0x08, // dest 8
		byte(OpJUMPI),       // value!=0 なら 8 へ
		byte(OpPUSH1), 0x00, // value==0 path
		byte(OpPOP),
		byte(OpREVERT),
		byte(OpJUMPDEST), // pc 8
		byte(OpPUSH1), 0x2a,
		byte(OpPUSH1), 0x00,
		byte(OpSSTORE),
		byte(OpSTOP),
	}
}

func TestConditionalJumpAndRevert(t *testing.T) {
	st := NewState()
	st.GetOrCreate("alice").Balance = 100
	e := NewEVM(st)
	addr := e.Deploy("alice", requireValueCode())

	// value=0 → REVERT。残高も戻る。
	r := e.Call("alice", addr, 0, 1000)
	if !r.Reverted {
		t.Fatalf("value=0 should revert, got %+v", r)
	}
	if st.Storage(addr, 0) != 0 {
		t.Fatalf("storage should be untouched, got %d", st.Storage(addr, 0))
	}
	if st.Balance("alice") != 100 {
		t.Fatalf("balance should roll back to 100, got %d", st.Balance("alice"))
	}

	// value=5 → 42 を保存。残高が移る。
	r = e.Call("alice", addr, 5, 1000)
	if !r.Success {
		t.Fatalf("value=5 should succeed: %v", r.Err)
	}
	if st.Storage(addr, 0) != 42 {
		t.Fatalf("storage[0] = %d, want 42", st.Storage(addr, 0))
	}
	if st.Balance(addr) != 5 {
		t.Fatalf("contract balance = %d, want 5", st.Balance(addr))
	}
}

func TestInvalidJumpFails(t *testing.T) {
	code := []byte{
		byte(OpPUSH1), 0x03, // dest 3 = PUSH1 data, not a JUMPDEST
		byte(OpJUMP),
	}
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if !errors.Is(r.Err, ErrInvalidJump) {
		t.Fatalf("want ErrInvalidJump, got %v", r.Err)
	}
}

func TestUnknownOpcodeFails(t *testing.T) {
	code := []byte{0xef} // 未定義
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if !errors.Is(r.Err, ErrInvalidOp) {
		t.Fatalf("want ErrInvalidOp, got %v", r.Err)
	}
}

func TestStackUnderflow(t *testing.T) {
	code := []byte{byte(OpADD)} // 空スタックで ADD
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if !errors.Is(r.Err, ErrStackUnderflow) {
		t.Fatalf("want ErrStackUnderflow, got %v", r.Err)
	}
}

func TestDupSwapAndCompare(t *testing.T) {
	// PUSH1 7, DUP1, EQ → 7==7 → 1
	code := []byte{
		byte(OpPUSH1), 0x07,
		byte(OpDUP1),
		byte(OpEQ),
		byte(OpSTOP),
	}
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if len(r.Stack) != 1 || r.Stack[0] != 1 {
		t.Fatalf("stack = %v, want [1]", r.Stack)
	}
}

func TestCallerIsPushed(t *testing.T) {
	code := []byte{byte(OpCALLER), byte(OpSTOP)}
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	r := e.Call("alice", addr, 0, 1000)
	if len(r.Stack) != 1 || r.Stack[0] != addrWord("alice") {
		t.Fatalf("CALLER = %v, want %d", r.Stack, addrWord("alice"))
	}
}

func TestDisassemble(t *testing.T) {
	lines := Disassemble(counterCode())
	if lines[0] != "0: PUSH1 0x00" {
		t.Fatalf("line0 = %q", lines[0])
	}
	if lines[len(lines)-1] != "9: STOP" {
		t.Fatalf("last line = %q", lines[len(lines)-1])
	}
}

func run(t *testing.T, code []byte) Result {
	t.Helper()
	st := NewState()
	e := NewEVM(st)
	addr := e.Deploy("alice", code)
	return e.Call("alice", addr, 0, 10000)
}

func TestArithmeticOps(t *testing.T) {
	tests := []struct {
		name string
		code []byte
		want uint64
	}{
		// 二項命令は EVM に倣い「先頭(top) OP 2 番目(second)」。
		// PUSH1 X, PUSH1 Y だと top=Y, second=X なので結果は Y OP X。
		{"SUB", []byte{byte(OpPUSH1), 3, byte(OpPUSH1), 10, byte(OpSUB), byte(OpSTOP)}, 7},    // 10-3
		{"MUL", []byte{byte(OpPUSH1), 6, byte(OpPUSH1), 7, byte(OpMUL), byte(OpSTOP)}, 42},    // 7*6
		{"LT_true", []byte{byte(OpPUSH1), 5, byte(OpPUSH1), 2, byte(OpLT), byte(OpSTOP)}, 1},  // 2<5
		{"GT_false", []byte{byte(OpPUSH1), 5, byte(OpPUSH1), 2, byte(OpGT), byte(OpSTOP)}, 0}, // 2>5
		{"ISZERO_true", []byte{byte(OpPUSH1), 0, byte(OpISZERO), byte(OpSTOP)}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, tc.code)
			if !r.Success || len(r.Stack) != 1 || r.Stack[0] != tc.want {
				t.Fatalf("%s: stack=%v want [%d] (err=%v)", tc.name, r.Stack, tc.want, r.Err)
			}
		})
	}
}

func TestSwap1(t *testing.T) {
	// PUSH1 10, PUSH1 3 → top=3, second=10。SWAP1 で top=10, second=3。
	// SUB(top-second)=10-3=7。SWAP しなければ 3-10 で桁あふれるので、入れ替えが効いた証拠。
	code := []byte{byte(OpPUSH1), 10, byte(OpPUSH1), 3, byte(OpSWAP1), byte(OpSUB), byte(OpSTOP)}
	r := run(t, code)
	if len(r.Stack) != 1 || r.Stack[0] != 7 {
		t.Fatalf("SWAP1: stack=%v want [7]", r.Stack)
	}
}

// 有効な JUMP: 前半をスキップして JUMPDEST 以降だけ実行する。
func TestValidJump(t *testing.T) {
	code := []byte{
		byte(OpPUSH1), 0x05, // dest 5
		byte(OpJUMP),     // pc0-2
		byte(OpREVERT),   // pc3: 飛び越される(踏んだら revert)
		byte(OpJUMPDEST), // pc4? -- 位置を合わせる
	}
	// 正確な位置合わせ: pc0 PUSH1,1 data,2 JUMP,3 REVERT,4 JUMPDEST。dest=4。
	code[1] = 0x04
	code = append(code, byte(OpPUSH1), 0x63, byte(OpSTOP)) // 99 を積んで STOP
	r := run(t, code)
	if !r.Success || len(r.Stack) != 1 || r.Stack[0] != 99 {
		t.Fatalf("valid jump: %+v", r)
	}
}

func TestPush1MissingData(t *testing.T) {
	r := run(t, []byte{byte(OpPUSH1)}) // データが無い
	if !errors.Is(r.Err, ErrInvalidOp) {
		t.Fatalf("want ErrInvalidOp, got %v", r.Err)
	}
}

func TestSwapUnderflow(t *testing.T) {
	r := run(t, []byte{byte(OpPUSH1), 1, byte(OpSWAP1)})
	if !errors.Is(r.Err, ErrStackUnderflow) {
		t.Fatalf("want ErrStackUnderflow, got %v", r.Err)
	}
	r = run(t, []byte{byte(OpDUP1)})
	if !errors.Is(r.Err, ErrStackUnderflow) {
		t.Fatalf("DUP1 underflow: got %v", r.Err)
	}
	r = run(t, []byte{byte(OpPOP)})
	if !errors.Is(r.Err, ErrStackUnderflow) {
		t.Fatalf("POP underflow: got %v", r.Err)
	}
}

func TestStateAccessors(t *testing.T) {
	st := NewState()
	if st.Balance("nobody") != 0 || st.Storage("nobody", 0) != 0 {
		t.Fatalf("empty accessors should be 0")
	}
	if st.get("nobody") != nil {
		t.Fatalf("get on missing should be nil")
	}
}

func TestMiscHelpers(t *testing.T) {
	if Op(0xff).Name() != "?" {
		t.Fatalf("unknown op name should be ?")
	}
	a := &Account{Balance: 5, Storage: map[uint64]uint64{1: 2}}
	if a.IsContract() {
		t.Fatalf("no code = not contract")
	}
	a.Code = []byte{0x00}
	if !a.IsContract() {
		t.Fatalf("with code = contract")
	}
}
