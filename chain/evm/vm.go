package evm

import (
	"errors"
	"fmt"
	"hash/fnv"
)

// #region vm

// 実行が中断する理由。out-of-gas と revert は「gas は消費するが状態は巻き戻す」で共通。
var (
	ErrOutOfGas       = errors.New("evm: gas が尽きた")
	ErrStackUnderflow = errors.New("evm: スタックが空なのに pop しようとした")
	ErrInvalidJump    = errors.New("evm: JUMPDEST でない位置へ飛ぼうとした")
	ErrInvalidOp      = errors.New("evm: 未知の命令")
	ErrRevert         = errors.New("evm: REVERT で実行が中止された")
)

// addrWord はアドレスを 1 ワード(uint64)に畳む。CALLER が積む値・比較に使う
// (本物の EVM は 160bit をそのまま 256bit ワードに載せる。ここは簡略化)。
func addrWord(a Address) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(a))
	return h.Sum64()
}

// vm は 1 回の実行の内部状態。スタックマシンなので、命令はスタック上位を読み書きする。
type vm struct {
	code   []byte
	stack  []uint64
	pc     int
	gas    uint64 // 残り gas
	self   *Account
	caller Address
	value  uint64
	jumpOK map[int]bool // 有効な JUMPDEST 位置(PUSH データ内を除外して事前計算)
}

// jumpdests は PUSH のオペレータ・バイトを飛ばしつつ、JUMPDEST の位置を集める。
// PUSH1 の次の 1 バイトはデータなので、そこにたまたま 0x5b があっても飛び先にしない。
func analyzeJumpdests(code []byte) map[int]bool {
	dests := map[int]bool{}
	for i := 0; i < len(code); i++ {
		op := Op(code[i])
		if op == OpJUMPDEST {
			dests[i] = true
		}
		if op == OpPUSH1 {
			i++ // 次の 1 バイトはデータ。読み飛ばす
		}
	}
	return dests
}

func (m *vm) push(v uint64) { m.stack = append(m.stack, v) }

func (m *vm) pop() (uint64, error) {
	if len(m.stack) == 0 {
		return 0, ErrStackUnderflow
	}
	v := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	return v, nil
}

// useGas は命令ぶんの gas を引く。足りなければ out-of-gas。
func (m *vm) useGas(cost uint64) error {
	if m.gas < cost {
		m.gas = 0
		return ErrOutOfGas
	}
	m.gas -= cost
	return nil
}

// run は code を fetch-decode-execute で回す。STOP/RETURN で正常終了、
// REVERT/エラーで異常終了する。異常時も gas は消費済みのまま返る。
func (m *vm) run() error {
	for m.pc < len(m.code) {
		op := Op(m.code[m.pc])
		if _, known := opTable[op]; !known {
			return fmt.Errorf("%w: 0x%02x @%d", ErrInvalidOp, byte(op), m.pc)
		}
		if err := m.useGas(gasCost(op)); err != nil {
			return err
		}
		m.pc++

		switch op {
		case OpSTOP, OpRETURN:
			return nil
		case OpREVERT:
			return ErrRevert
		case OpPUSH1:
			if m.pc >= len(m.code) {
				return fmt.Errorf("%w: PUSH1 のデータが無い", ErrInvalidOp)
			}
			m.push(uint64(m.code[m.pc]))
			m.pc++
		case OpPOP:
			if _, err := m.pop(); err != nil {
				return err
			}
		case OpADD, OpMUL, OpSUB, OpLT, OpGT, OpEQ:
			if err := m.binOp(op); err != nil {
				return err
			}
		case OpISZERO:
			a, err := m.pop()
			if err != nil {
				return err
			}
			m.push(b2i(a == 0))
		case OpDUP1:
			if len(m.stack) == 0 {
				return ErrStackUnderflow
			}
			m.push(m.stack[len(m.stack)-1])
		case OpSWAP1:
			if len(m.stack) < 2 {
				return ErrStackUnderflow
			}
			n := len(m.stack)
			m.stack[n-1], m.stack[n-2] = m.stack[n-2], m.stack[n-1]
		case OpCALLER:
			m.push(addrWord(m.caller))
		case OpCALLVALUE:
			m.push(m.value)
		case OpSLOAD:
			slot, err := m.pop()
			if err != nil {
				return err
			}
			m.push(m.self.Storage[slot])
		case OpSSTORE:
			slot, err := m.pop()
			if err != nil {
				return err
			}
			val, err := m.pop()
			if err != nil {
				return err
			}
			m.self.Storage[slot] = val // 状態の書き換え。失敗すれば呼び出し側が巻き戻す
		case OpJUMP:
			dest, err := m.pop()
			if err != nil {
				return err
			}
			if !m.jumpOK[int(dest)] {
				return fmt.Errorf("%w: @%d", ErrInvalidJump, dest)
			}
			m.pc = int(dest)
		case OpJUMPI:
			dest, err := m.pop()
			if err != nil {
				return err
			}
			cond, err := m.pop()
			if err != nil {
				return err
			}
			if cond != 0 {
				if !m.jumpOK[int(dest)] {
					return fmt.Errorf("%w: @%d", ErrInvalidJump, dest)
				}
				m.pc = int(dest)
			}
		case OpJUMPDEST:
			// マーカー。何もしない
		}
	}
	return nil // コード末尾に達したら STOP と同じ扱い
}

func (m *vm) binOp(op Op) error {
	a, err := m.pop()
	if err != nil {
		return err
	}
	b, err := m.pop()
	if err != nil {
		return err
	}
	switch op {
	case OpADD:
		m.push(a + b)
	case OpMUL:
		m.push(a * b)
	case OpSUB:
		m.push(a - b)
	case OpLT:
		m.push(b2i(a < b))
	case OpGT:
		m.push(b2i(a > b))
	case OpEQ:
		m.push(b2i(a == b))
	}
	return nil
}

func b2i(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// #endregion vm
