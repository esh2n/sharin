// Package bytecode は、ツリーウォーク評価(foundations/lang)の次の段——
// ソースを**バイトコードにコンパイルし、スタックマシンで実行する**——を Go で
// モデル化する。ランタイム内部編のパーツ。
//
// lang 編のツリーウォーク評価は、実行のたびに AST(木)を再帰的にたどり直す。
// バイトコード方式は、木を一度**平らな命令列**に落としてから回す。命令の取得と
// 解釈(fetch-decode-dispatch)が単純なループになり、木を歩き回るより速い。
// これが CPython・Ruby(YARV)・Java(JVM)・WebAssembly といった実行系の基本形だ。
//
// 3 段で構成する:
//   - code.go     命令セット(オペコード)と、その符号化/逆アセンブル
//   - compiler.go lang の AST を平らな命令列 + 定数プールへコンパイルする
//   - vm.go       スタックマシン。命令を1つずつ取り出して実行する
//
// フロントエンド(字句・構文解析)は lang をそのまま再利用する——同じ言語を、
// 評価器の代わりにコンパイラ+VM で走らせる、という対比が主題だ。
package bytecode

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// #region code

// Opcode は 1 バイトの命令種別。VM はこれを見て何をするかを決める。
type Opcode byte

const (
	OpConstant      Opcode = iota // 定数プールの [operand] 番目をスタックに積む
	OpPop                         // スタック先頭を1つ捨てる(式文の後始末)
	OpTrue                        // true を積む
	OpFalse                       // false を積む
	OpNull                        // null を積む(else 無き if の値)
	OpAdd                         // 上位2つを pop して加算し、結果を積む
	OpSub                         // 減算
	OpMul                         // 乗算
	OpDiv                         // 除算
	OpMinus                       // 前置 -(符号反転)
	OpBang                        // 前置 !(真偽反転)
	OpEqual                       // ==
	OpNotEqual                    // !=
	OpGreater                     // >(< は compiler がオペランドを入れ替えて表現)
	OpJump                        // 無条件に [operand] へ飛ぶ
	OpJumpNotTruthy               // pop した値が偽なら [operand] へ飛ぶ
	OpSetGlobal                   // pop した値をグローバル [operand] に束縛(let)
	OpGetGlobal                   // グローバル [operand] を積む(識別子参照)
)

// Definition は1つのオペコードの、人間向けの名前とオペランドの幅(バイト数)。
// OperandWidths が空なら、そのオペコードはオペランドを取らない。
type Definition struct {
	Name          string
	OperandWidths []int
}

var definitions = map[Opcode]*Definition{
	OpConstant:      {"OpConstant", []int{2}}, // 2 バイトの定数インデックス
	OpPop:           {"OpPop", nil},
	OpTrue:          {"OpTrue", nil},
	OpFalse:         {"OpFalse", nil},
	OpNull:          {"OpNull", nil},
	OpAdd:           {"OpAdd", nil},
	OpSub:           {"OpSub", nil},
	OpMul:           {"OpMul", nil},
	OpDiv:           {"OpDiv", nil},
	OpMinus:         {"OpMinus", nil},
	OpBang:          {"OpBang", nil},
	OpEqual:         {"OpEqual", nil},
	OpNotEqual:      {"OpNotEqual", nil},
	OpGreater:       {"OpGreater", nil},
	OpJump:          {"OpJump", []int{2}}, // 2 バイトの飛び先(命令列内の位置)
	OpJumpNotTruthy: {"OpJumpNotTruthy", []int{2}},
	OpSetGlobal:     {"OpSetGlobal", []int{2}}, // 2 バイトのグローバル番号
	OpGetGlobal:     {"OpGetGlobal", []int{2}},
}

// Lookup はオペコードの定義を引く。未知なら error。
func Lookup(op byte) (*Definition, error) {
	def, ok := definitions[Opcode(op)]
	if !ok {
		return nil, fmt.Errorf("opcode %d undefined", op)
	}
	return def, nil
}

// Instructions はバイトコード(平らな命令列)。オペコードとオペランドが
// 隙間なく詰まった 1 本のバイト列だ。
type Instructions []byte

// Make は 1 命令を符号化する: オペコード 1 バイト + 各オペランドをその幅で
// ビッグエンディアン格納。これが「木の 1 ノード → 数バイトの命令」への変換。
func Make(op Opcode, operands ...int) Instructions {
	def, ok := definitions[op]
	if !ok {
		return Instructions{}
	}
	length := 1
	for _, w := range def.OperandWidths {
		length += w
	}
	ins := make(Instructions, length)
	ins[0] = byte(op)
	offset := 1
	for i, o := range operands {
		w := def.OperandWidths[i]
		switch w {
		case 2:
			binary.BigEndian.PutUint16(ins[offset:], uint16(o))
		}
		offset += w
	}
	return ins
}

// ReadUint16 は命令列の先頭 2 バイトを 16bit オペランドとして読む。
// VM が飛び先や定数インデックスを取り出すのに使う。
func ReadUint16(ins Instructions) uint16 {
	return binary.BigEndian.Uint16(ins)
}

// String は命令列を人間可読に逆アセンブルする。各行は "0000 OpConstant 0"
// の形で、先頭の 4 桁はその命令の命令列内オフセット(= 飛び先の単位)。
func (ins Instructions) String() string {
	var out bytes.Buffer
	i := 0
	for i < len(ins) {
		def, err := Lookup(ins[i])
		if err != nil {
			fmt.Fprintf(&out, "ERROR: %s\n", err)
			i++
			continue
		}
		operands, read := readOperands(def, ins[i+1:])
		fmt.Fprintf(&out, "%04d %s\n", i, fmtInstruction(def, operands))
		i += 1 + read
	}
	return out.String()
}

// readOperands は 1 命令ぶんのオペランドを、定義の幅に従って読み出す。
func readOperands(def *Definition, ins Instructions) ([]int, int) {
	operands := make([]int, len(def.OperandWidths))
	offset := 0
	for i, w := range def.OperandWidths {
		switch w {
		case 2:
			operands[i] = int(ReadUint16(ins[offset:]))
		}
		offset += w
	}
	return operands, offset
}

func fmtInstruction(def *Definition, operands []int) string {
	if len(operands) != len(def.OperandWidths) {
		return fmt.Sprintf("ERROR: operand len %d != defined %d", len(operands), len(def.OperandWidths))
	}
	switch len(operands) {
	case 0:
		return def.Name
	case 1:
		return fmt.Sprintf("%s %d", def.Name, operands[0])
	}
	return fmt.Sprintf("ERROR: unhandled operandCount for %s", def.Name)
}

// #endregion code
