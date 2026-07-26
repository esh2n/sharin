package bytecode

import (
	"fmt"

	"github.com/esh2n/sharin/foundations/lang"
)

// #region vm

const (
	stackSize   = 2048  // スタックの深さ上限
	globalsSize = 65536 // グローバル変数の本数上限(OpSetGlobal の 2 バイトぶん)
)

// 真偽値と null は毎回作らず、共有インスタンスを積む(値の同一性も単純になる)。
var (
	True  = &lang.BooleanObj{Value: true}
	False = &lang.BooleanObj{Value: false}
	Null  = &lang.Null{}
)

// VM はスタックマシン。レジスタを持たず、オペランドはすべてスタックに積んで
// やり取りする。命令を 1 つずつ取り出して解釈する(fetch-decode-execute)。
type VM struct {
	constants    []lang.Object
	instructions Instructions
	stack        []lang.Object
	sp           int // 次に積む位置。先頭要素は stack[sp-1]
	globals      []lang.Object
}

// NewVM はコンパイル結果から VM を作る。
func NewVM(bc *Bytecode) *VM {
	return &VM{
		constants:    bc.Constants,
		instructions: bc.Instructions,
		stack:        make([]lang.Object, stackSize),
		sp:           0,
		globals:      make([]lang.Object, globalsSize),
	}
}

// Run は命令列を先頭から回す。ip(命令ポインタ)を進めながら、オペコードごとに
// スタックを操作する——これがバイトコード実行の全体。木を歩く代わりに、平らな
// バイト列を一直線に舐めていく(ジャンプ命令だけが ip を飛ばす)。
func (vm *VM) Run() error {
	for ip := 0; ip < len(vm.instructions); ip++ {
		op := Opcode(vm.instructions[ip])

		switch op {
		case OpConstant:
			idx := ReadUint16(vm.instructions[ip+1:])
			ip += 2
			if err := vm.push(vm.constants[idx]); err != nil {
				return err
			}

		case OpPop:
			vm.pop()

		case OpTrue:
			if err := vm.push(True); err != nil {
				return err
			}
		case OpFalse:
			if err := vm.push(False); err != nil {
				return err
			}
		case OpNull:
			if err := vm.push(Null); err != nil {
				return err
			}

		case OpAdd, OpSub, OpMul, OpDiv:
			if err := vm.binaryOp(op); err != nil {
				return err
			}

		case OpEqual, OpNotEqual, OpGreater:
			if err := vm.comparison(op); err != nil {
				return err
			}

		case OpMinus:
			operand := vm.pop()
			i, ok := operand.(*lang.Integer)
			if !ok {
				return fmt.Errorf("- は整数にしか使えない: %s", operand.Type())
			}
			if err := vm.push(&lang.Integer{Value: -i.Value}); err != nil {
				return err
			}

		case OpBang:
			operand := vm.pop()
			if err := vm.push(boolObj(!isTruthy(operand))); err != nil {
				return err
			}

		case OpJump:
			// 無条件ジャンプ。飛び先はオペランド。ip は直後に ++ されるので -1 する。
			pos := int(ReadUint16(vm.instructions[ip+1:]))
			ip = pos - 1

		case OpJumpNotTruthy:
			pos := int(ReadUint16(vm.instructions[ip+1:]))
			ip += 2
			cond := vm.pop()
			if !isTruthy(cond) {
				ip = pos - 1 // 偽なら飛ぶ。真ならそのまま then へ進む
			}

		case OpSetGlobal:
			idx := ReadUint16(vm.instructions[ip+1:])
			ip += 2
			vm.globals[idx] = vm.pop()

		case OpGetGlobal:
			idx := ReadUint16(vm.instructions[ip+1:])
			ip += 2
			if err := vm.push(vm.globals[idx]); err != nil {
				return err
			}

		default:
			return fmt.Errorf("未知のオペコード: %d", op)
		}
	}
	return nil
}

// binaryOp は算術2項演算。右→左の順に pop し(スタックなので後入れが右)、
// 結果を積む。整数以外はエラー。
func (vm *VM) binaryOp(op Opcode) error {
	right := vm.pop()
	left := vm.pop()
	l, lok := left.(*lang.Integer)
	r, rok := right.(*lang.Integer)
	if !lok || !rok {
		return fmt.Errorf("算術は整数どうしだけ: %s %s", left.Type(), right.Type())
	}
	var res int64
	switch op {
	case OpAdd:
		res = l.Value + r.Value
	case OpSub:
		res = l.Value - r.Value
	case OpMul:
		res = l.Value * r.Value
	case OpDiv:
		if r.Value == 0 {
			return fmt.Errorf("ゼロ除算")
		}
		res = l.Value / r.Value
	}
	return vm.push(&lang.Integer{Value: res})
}

// comparison は比較演算。整数どうしなら大小/等値、真偽値どうしなら等値のみ。
func (vm *VM) comparison(op Opcode) error {
	right := vm.pop()
	left := vm.pop()
	l, lok := left.(*lang.Integer)
	r, rok := right.(*lang.Integer)
	if lok && rok {
		switch op {
		case OpEqual:
			return vm.push(boolObj(l.Value == r.Value))
		case OpNotEqual:
			return vm.push(boolObj(l.Value != r.Value))
		case OpGreater:
			return vm.push(boolObj(l.Value > r.Value))
		}
	}
	// 整数でない(= 真偽値)の比較は等値のみ。共有インスタンスなのでポインタ一致で足りる。
	switch op {
	case OpEqual:
		return vm.push(boolObj(left == right))
	case OpNotEqual:
		return vm.push(boolObj(left != right))
	default:
		return fmt.Errorf("その比較は真偽値には使えない: %s %s", left.Type(), right.Type())
	}
}

func (vm *VM) push(o lang.Object) error {
	if vm.sp >= stackSize {
		return fmt.Errorf("スタックあふれ")
	}
	vm.stack[vm.sp] = o
	vm.sp++
	return nil
}

func (vm *VM) pop() lang.Object {
	if vm.sp == 0 {
		return Null // 値を残さない式(例: 本体が let だけの if)でも壊れないように
	}
	o := vm.stack[vm.sp-1]
	vm.sp--
	return o
}

// StackTop は現在のスタック先頭を返す(空なら nil)。
func (vm *VM) StackTop() lang.Object {
	if vm.sp == 0 {
		return nil
	}
	return vm.stack[vm.sp-1]
}

// LastPopped は直前に pop された値(sp が指す位置に残っている)を返す。
// プログラム末尾の OpPop で捨てられた「最後の式の値」= 実行結果を取り出すのに使う。
func (vm *VM) LastPopped() lang.Object { return vm.stack[vm.sp] }

// boolObj は Go の bool を共有 True/False に写す。
func boolObj(b bool) *lang.BooleanObj {
	if b {
		return True
	}
	return False
}

// isTruthy は VM 上の真偽判定。false と null が偽、それ以外は真。
func isTruthy(o lang.Object) bool {
	switch o := o.(type) {
	case *lang.BooleanObj:
		return o.Value
	case *lang.Null:
		return false
	default:
		return true
	}
}

// Run はソース文字列を字句解析→構文解析(lang を再利用)→コンパイル→VM 実行し、
// 結果オブジェクトを返す。lang.Run と同じ入口を、評価器の代わりにコンパイラ+VM で。
func Run(input string) (lang.Object, error) {
	p := lang.NewParser(lang.NewLexer(input))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("構文エラー: %s", errs[0])
	}
	comp := New()
	if err := comp.Compile(program); err != nil {
		return nil, err
	}
	machine := NewVM(comp.Bytecode())
	if err := machine.Run(); err != nil {
		return nil, err
	}
	return machine.LastPopped(), nil
}

// #endregion vm
