package wasm

import (
	"errors"
	"fmt"
)

// #region interp

const maxCallDepth = 1000 // 再帰の暴走から Go スタックを守る

// Interp は Module を実行するインタプリタ。関数呼び出しは Go の再帰で表す。
type Interp struct {
	mod   *Module
	depth int
}

// NewInterp はモジュールに対するインタプリタを作る。
func NewInterp(m *Module) *Interp { return &Interp{mod: m} }

// Invoke は export された関数を名前で呼ぶ。引数・戻り値は i32。
func (in *Interp) Invoke(name string, args ...int32) ([]int32, error) {
	fi, ok := in.mod.Exports[name]
	if !ok {
		return nil, fmt.Errorf("wasm: export %q が無い", name)
	}
	return in.execFunc(fi, args)
}

// ctrl は制御スタックの1エントリ。target は br でこのラベルを狙ったときの飛び先
// (block/if は end の直後 = 脱出、loop は本体先頭 = 継続)、end は対応する end の位置。
type ctrl struct {
	op     byte
	target int
	end    int
}

// execFunc は 1 つの関数を、値スタックと制御スタックで実行する。ここが WASM の
// **構造化制御フロー**の肝——任意番地への goto は無く、br は「今いるブロックの
// N 個外側」を狙う。だから飛び先は常に静的に決まり、検証(安全性の担保)ができる。
func (in *Interp) execFunc(fi int, args []int32) ([]int32, error) {
	if fi < 0 || fi >= len(in.mod.Funcs) {
		return nil, fmt.Errorf("wasm: 関数 %d が無い", fi)
	}
	in.depth++
	if in.depth > maxCallDepth {
		in.depth--
		return nil, errors.New("wasm: 呼び出しが深すぎる(無限再帰?)")
	}
	defer func() { in.depth-- }()

	fn := &in.mod.Funcs[fi]
	ft := in.mod.FuncType(fi)
	if len(args) != ft.Params {
		return nil, fmt.Errorf("wasm: 引数の数が違う(%d 個, 期待 %d)", len(args), ft.Params)
	}
	locals := make([]int32, ft.Params+fn.Locals)
	copy(locals, args)

	var stack []int32
	var control []ctrl
	push := func(v int32) { stack = append(stack, v) }
	pop := func() (int32, error) {
		if len(stack) == 0 {
			return 0, errors.New("wasm: スタックアンダーフロー")
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v, nil
	}
	b2i := func(b bool) int32 {
		if b {
			return 1
		}
		return 0
	}

	body := fn.Body
	for ip := 0; ip < len(body); {
		ins := body[ip]
		switch ins.Op {
		case opNop:
			ip++
		case opUnreachable:
			return nil, errors.New("wasm: unreachable に到達")
		case opI32Const:
			push(int32(ins.Imm))
			ip++
		case opDrop:
			if _, err := pop(); err != nil {
				return nil, err
			}
			ip++
		case opLocalGet:
			if int(ins.Imm) >= len(locals) {
				return nil, errors.New("wasm: local 範囲外")
			}
			push(locals[ins.Imm])
			ip++
		case opLocalSet:
			v, err := pop()
			if err != nil {
				return nil, err
			}
			locals[ins.Imm] = v
			ip++
		case opLocalTee:
			v, err := pop()
			if err != nil {
				return nil, err
			}
			locals[ins.Imm] = v
			push(v)
			ip++
		case opI32Add, opI32Sub, opI32Mul, opI32DivS, opI32RemS,
			opI32Eq, opI32Ne, opI32LtS, opI32GtS, opI32LeS, opI32GeS:
			b, err := pop()
			if err != nil {
				return nil, err
			}
			a, err := pop()
			if err != nil {
				return nil, err
			}
			res, err := binI32(ins.Op, a, b)
			if err != nil {
				return nil, err
			}
			push(res)
			ip++
		case opI32Eqz:
			a, err := pop()
			if err != nil {
				return nil, err
			}
			push(b2i(a == 0))
			ip++
		case opCall:
			res, err := in.doCall(int(ins.Imm), &stack)
			if err != nil {
				return nil, err
			}
			_ = res
			ip++

		// --- 構造化制御フロー ---
		case opBlock:
			control = append(control, ctrl{op: opBlock, target: ins.End + 1, end: ins.End})
			ip++
		case opLoop:
			control = append(control, ctrl{op: opLoop, target: ip + 1, end: ins.End})
			ip++
		case opIf:
			cond, err := pop()
			if err != nil {
				return nil, err
			}
			control = append(control, ctrl{op: opIf, target: ins.End + 1, end: ins.End})
			if cond != 0 {
				ip++ // then 節へ
			} else if ins.Else != ins.End {
				ip = ins.Else + 1 // else 節へ
			} else {
				ip = ins.End // else 無し → end へ(フレームは end が畳む)
			}
		case opElse:
			// then 節を実行し終えた → else 節を飛ばして end へ。
			ip = control[len(control)-1].end
		case opEnd:
			if len(control) > 0 {
				control = control[:len(control)-1] // ブロックから普通に抜ける
			}
			ip++
		case opBr:
			ip, control = doBranch(int(ins.Imm), control)
		case opBrIf:
			cond, err := pop()
			if err != nil {
				return nil, err
			}
			if cond != 0 {
				ip, control = doBranch(int(ins.Imm), control)
			} else {
				ip++
			}
		case opReturn:
			return takeResults(stack, ft.Results)
		default:
			return nil, fmt.Errorf("wasm: 実行時に未対応のオペコード 0x%x", ins.Op)
		}
	}
	return takeResults(stack, ft.Results)
}

// #region branch

// doBranch は br <label> の飛び先と、畳んだあとの制御スタックを返す。
// label は「今いるブロックの何個外側か」。loop を狙ったら本体先頭へ戻る(継続)、
// block/if を狙ったら end の先へ抜ける(脱出)——同じ br が、狙う種類で意味を変える。
func doBranch(label int, control []ctrl) (int, []ctrl) {
	idx := len(control) - 1 - label
	target := control[idx]
	if target.op == opLoop {
		return target.target, control[:idx+1] // loop フレームは残す(次の周回のため)
	}
	return target.target, control[:idx] // block/if フレームは畳んで抜ける
}

// #endregion branch

// doCall は call を処理する。呼び先の引数を値スタックから取り、結果を積み直す。
func (in *Interp) doCall(fi int, stack *[]int32) ([]int32, error) {
	if fi < 0 || fi >= len(in.mod.Funcs) {
		return nil, fmt.Errorf("wasm: call 先 %d が無い", fi)
	}
	nargs := in.mod.FuncType(fi).Params
	if len(*stack) < nargs {
		return nil, errors.New("wasm: call の引数が足りない")
	}
	args := make([]int32, nargs)
	copy(args, (*stack)[len(*stack)-nargs:])
	*stack = (*stack)[:len(*stack)-nargs]
	res, err := in.execFunc(fi, args)
	if err != nil {
		return nil, err
	}
	*stack = append(*stack, res...)
	return res, nil
}

// binI32 は2項の i32 演算。除算・剰余のゼロ除算はエラー。
func binI32(op byte, a, b int32) (int32, error) {
	switch op {
	case opI32Add:
		return a + b, nil
	case opI32Sub:
		return a - b, nil
	case opI32Mul:
		return a * b, nil
	case opI32DivS:
		if b == 0 {
			return 0, errors.New("wasm: ゼロ除算")
		}
		return a / b, nil
	case opI32RemS:
		if b == 0 {
			return 0, errors.New("wasm: ゼロ剰余")
		}
		return a % b, nil
	case opI32Eq:
		return boolI32(a == b), nil
	case opI32Ne:
		return boolI32(a != b), nil
	case opI32LtS:
		return boolI32(a < b), nil
	case opI32GtS:
		return boolI32(a > b), nil
	case opI32LeS:
		return boolI32(a <= b), nil
	case opI32GeS:
		return boolI32(a >= b), nil
	}
	return 0, fmt.Errorf("wasm: 二項演算ではない 0x%x", op)
}

func boolI32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// takeResults は値スタックの先頭 n 個を戻り値として返す。
func takeResults(stack []int32, n int) ([]int32, error) {
	if n == 0 {
		return nil, nil
	}
	if len(stack) < n {
		return nil, errors.New("wasm: 戻り値が足りない")
	}
	out := make([]int32, n)
	copy(out, stack[len(stack)-n:])
	return out, nil
}

// Run は .wasm バイト列をパースし、export 関数を呼んで単一の i32 を返す簡便入口。
func Run(bin []byte, fn string, args ...int32) (int32, error) {
	m, err := Parse(bin)
	if err != nil {
		return 0, err
	}
	res, err := NewInterp(m).Invoke(fn, args...)
	if err != nil {
		return 0, err
	}
	if len(res) != 1 {
		return 0, fmt.Errorf("wasm: 戻り値が1個でない(%d 個)", len(res))
	}
	return res[0], nil
}

// #endregion interp
