package evm

import "fmt"

// #region asm

// Disassemble はバイトコードを人が読める行に開く。PUSH1 はデータ 1 バイトを添える。
// 各行は "pc: NAME [operand]" 形式で、飛び先(pc)を目で追えるようにしている。
func Disassemble(code []byte) []string {
	var out []string
	for i := 0; i < len(code); i++ {
		op := Op(code[i])
		if op == OpPUSH1 && i+1 < len(code) {
			out = append(out, fmt.Sprintf("%d: PUSH1 0x%02x", i, code[i+1]))
			i++
			continue
		}
		out = append(out, fmt.Sprintf("%d: %s", i, op.Name()))
	}
	return out
}

// #endregion asm
