package bytecode

import (
	"fmt"

	"github.com/esh2n/sharin/foundations/lang"
)

// #region compiler

// Bytecode はコンパイルの成果物: 平らな命令列と、そこから参照される定数プール。
// 整数リテラルなどの値は命令に埋め込まず、定数プールに置いて番号で指す。
type Bytecode struct {
	Instructions Instructions
	Constants    []lang.Object
}

// emitted は直近に出力した命令(オペコードと、その命令列内の位置)。
// if の後始末で「末尾の OpPop を消す」「飛び先を後から埋める」のに使う。
type emitted struct {
	Opcode   Opcode
	Position int
}

// Compiler は lang の AST を一度だけ歩いて、バイトコードに落とす。
// シンボルテーブルで「変数名 → グローバル番号」を管理する。
type Compiler struct {
	instructions Instructions
	constants    []lang.Object
	symbols      map[string]int // 変数名 → グローバル番号
	last         emitted        // 直近に出した命令
	prev         emitted        // その 1 つ前(OpPop 除去で末尾を戻すため)
}

// New は空のコンパイラを作る。
func New() *Compiler {
	return &Compiler{symbols: map[string]int{}}
}

// Compile は AST ノードを再帰的にコンパイルする。木を一度歩き、対応する命令を
// 命令列の末尾に足していく。これが「木 → 平らな命令列」への変換の全体だ。
func (c *Compiler) Compile(node lang.Node) error {
	switch node := node.(type) {
	case *lang.Program:
		for _, s := range node.Statements {
			if err := c.Compile(s); err != nil {
				return err
			}
		}

	case *lang.ExpressionStatement:
		// 式だけの文。値を評価したら、その結果は捨てる(次の文のためにスタックを空に保つ)。
		if err := c.Compile(node.Expression); err != nil {
			return err
		}
		c.emit(OpPop)

	case *lang.BlockStatement:
		for _, s := range node.Statements {
			if err := c.Compile(s); err != nil {
				return err
			}
		}

	case *lang.InfixExpression:
		// a < b は「b > a」に読み替えて OpGreater だけで賄う——オペコードを1つ減らす
		// コンパイラの定番テク。左右を逆順にコンパイルするだけでよい。
		if node.Operator == "<" {
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			c.emit(OpGreater)
			return nil
		}
		if err := c.Compile(node.Left); err != nil {
			return err
		}
		if err := c.Compile(node.Right); err != nil {
			return err
		}
		switch node.Operator {
		case "+":
			c.emit(OpAdd)
		case "-":
			c.emit(OpSub)
		case "*":
			c.emit(OpMul)
		case "/":
			c.emit(OpDiv)
		case "==":
			c.emit(OpEqual)
		case "!=":
			c.emit(OpNotEqual)
		case ">":
			c.emit(OpGreater)
		default:
			return fmt.Errorf("未知の演算子: %s", node.Operator)
		}

	case *lang.PrefixExpression:
		if err := c.Compile(node.Right); err != nil {
			return err
		}
		switch node.Operator {
		case "-":
			c.emit(OpMinus)
		case "!":
			c.emit(OpBang)
		default:
			return fmt.Errorf("未知の前置演算子: %s", node.Operator)
		}

	case *lang.IntegerLiteral:
		// 値は命令に埋めず、定数プールに登録して「その番号」を OpConstant で指す。
		c.emit(OpConstant, c.addConstant(&lang.Integer{Value: node.Value}))

	case *lang.Boolean:
		if node.Value {
			c.emit(OpTrue)
		} else {
			c.emit(OpFalse)
		}

	case *lang.IfExpression:
		if err := c.compileIf(node); err != nil {
			return err
		}

	case *lang.LetStatement:
		if err := c.Compile(node.Value); err != nil {
			return err
		}
		idx := c.define(node.Name.Value)
		c.emit(OpSetGlobal, idx)

	case *lang.Identifier:
		idx, ok := c.symbols[node.Value]
		if !ok {
			return fmt.Errorf("未定義の変数: %s", node.Value)
		}
		c.emit(OpGetGlobal, idx)

	case *lang.FunctionLiteral, *lang.CallExpression, *lang.ReturnStatement:
		// 関数/呼び出し/return は、フレームとコール規約が要る。本章の範囲外。
		return fmt.Errorf("この章のコンパイラは関数を扱わない(式・let・if まで)")

	default:
		return fmt.Errorf("コンパイル未対応のノード: %T", node)
	}
	return nil
}

// compileIf は if 式をコンパイルする。制御フローが**条件ジャンプ**に化ける、
// バイトコードの肝。飛び先はコンパイル時にはまだ分からないので、仮の値で出して
// おき、行き先が確定したら後から書き換える(back-patching)。
func (c *Compiler) compileIf(node *lang.IfExpression) error {
	if err := c.Compile(node.Condition); err != nil {
		return err
	}
	// 条件が偽なら「then を飛び越す」。飛び先は未確定なので仮に 9999。
	jumpNotTruthy := c.emit(OpJumpNotTruthy, 9999)

	if err := c.Compile(node.Consequence); err != nil {
		return err
	}
	// ブロックの式文が付けた OpPop を消す——if 式は値を残したいから。
	c.removeLastPopIfAny()

	// then を実行し終えたら else を飛び越す。ここも仮の飛び先。
	jumpOverElse := c.emit(OpJump, 9999)

	// ここが「条件が偽のときの行き先」。確定したので JumpNotTruthy を書き換える。
	c.changeOperand(jumpNotTruthy, len(c.instructions))

	if node.Alternative == nil {
		c.emit(OpNull) // else が無ければ、偽のとき null を値にする
	} else {
		if err := c.Compile(node.Alternative); err != nil {
			return err
		}
		c.removeLastPopIfAny()
	}
	// if 全体の直後。then 実行後の Jump をここへ向ける。
	c.changeOperand(jumpOverElse, len(c.instructions))
	return nil
}

// Bytecode はコンパイル結果を返す。
func (c *Compiler) Bytecode() *Bytecode {
	return &Bytecode{Instructions: c.instructions, Constants: c.constants}
}

// --- 低レベルの出力補助 ---

// emit は 1 命令を符号化して命令列の末尾に足し、その位置を返す。
func (c *Compiler) emit(op Opcode, operands ...int) int {
	ins := Make(op, operands...)
	pos := len(c.instructions)
	c.instructions = append(c.instructions, ins...)
	c.prev = c.last
	c.last = emitted{Opcode: op, Position: pos}
	return pos
}

// addConstant は定数プールに値を登録し、その番号を返す。
func (c *Compiler) addConstant(obj lang.Object) int {
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

// define は変数名に次のグローバル番号を割り当てる(let の左辺)。
func (c *Compiler) define(name string) int {
	if idx, ok := c.symbols[name]; ok {
		return idx // 再代入は同じ番号に上書き
	}
	idx := len(c.symbols)
	c.symbols[name] = idx
	return idx
}

// removeLastPopIfAny は直近が OpPop なら命令列末尾から取り除く。
func (c *Compiler) removeLastPopIfAny() {
	if c.last.Opcode != OpPop {
		return
	}
	c.instructions = c.instructions[:c.last.Position]
	c.last = c.prev
}

// changeOperand は既に出した命令のオペランドを差し替える(back-patching)。
// 同じオペコードで作り直して、その場に上書きする。
func (c *Compiler) changeOperand(opPos int, operand int) {
	op := Opcode(c.instructions[opPos])
	newIns := Make(op, operand)
	copy(c.instructions[opPos:], newIns)
}

// #endregion compiler
