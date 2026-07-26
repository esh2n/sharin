package evm

// #region opcode

// Op は 1 バイトの命令。値は実際の EVM に寄せてある(PUSH1=0x60 など)。
type Op byte

const (
	OpSTOP Op = 0x00 // 実行を正常終了

	OpADD    Op = 0x01 // a b -> a+b
	OpMUL    Op = 0x02 // a b -> a*b
	OpSUB    Op = 0x03 // a b -> a-b
	OpLT     Op = 0x10 // a b -> (a<b)
	OpGT     Op = 0x11 // a b -> (a>b)
	OpEQ     Op = 0x14 // a b -> (a==b)
	OpISZERO Op = 0x15 // a   -> (a==0)

	OpCALLER    Op = 0x33 // 呼び出し元アドレスの識別子(msg.sender 相当)を積む
	OpCALLVALUE Op = 0x34 // 送られてきた value を積む

	OpPOP      Op = 0x50 // スタック先頭を捨てる
	OpSLOAD    Op = 0x54 // slot -> storage[slot]
	OpSSTORE   Op = 0x55 // slot value -> (storage[slot]=value)
	OpJUMP     Op = 0x56 // dest -> pc=dest(JUMPDEST でなければ失敗)
	OpJUMPI    Op = 0x57 // dest cond -> cond!=0 なら pc=dest
	OpJUMPDEST Op = 0x5b // 飛び先マーカー(何もしない)

	OpPUSH1 Op = 0x60 // 次の 1 バイトを値としてスタックに積む
	OpDUP1  Op = 0x80 // スタック先頭を複製
	OpSWAP1 Op = 0x90 // スタック上位 2 つを入れ替え

	OpRETURN Op = 0xf3 // 正常終了(値を返す)
	OpREVERT Op = 0xfd // 実行を中止し、状態変更を巻き戻す(gas は消費済み)
)

// opInfo は逆アセンブル表示と gas 単価。
type opInfo struct {
	name string
	gas  uint64
}

// gas 単価は EVM の桁感を簡略化したもの。SSTORE が飛び抜けて高いのは、
// 全ノードが永続的に保持するストレージへの書き込みだから——「状態を増やす」のは高い。
var opTable = map[Op]opInfo{
	OpSTOP:      {"STOP", 0},
	OpADD:       {"ADD", 3},
	OpMUL:       {"MUL", 5},
	OpSUB:       {"SUB", 3},
	OpLT:        {"LT", 3},
	OpGT:        {"GT", 3},
	OpEQ:        {"EQ", 3},
	OpISZERO:    {"ISZERO", 3},
	OpCALLER:    {"CALLER", 2},
	OpCALLVALUE: {"CALLVALUE", 2},
	OpPOP:       {"POP", 2},
	OpSLOAD:     {"SLOAD", 50},
	OpSSTORE:    {"SSTORE", 100},
	OpJUMP:      {"JUMP", 8},
	OpJUMPI:     {"JUMPI", 10},
	OpJUMPDEST:  {"JUMPDEST", 1},
	OpPUSH1:     {"PUSH1", 3},
	OpDUP1:      {"DUP1", 3},
	OpSWAP1:     {"SWAP1", 3},
	OpRETURN:    {"RETURN", 0},
	OpREVERT:    {"REVERT", 0},
}

// Name は命令の表示名(未知なら "?")。
func (op Op) Name() string {
	if info, ok := opTable[op]; ok {
		return info.name
	}
	return "?"
}

// gasCost は命令の gas 単価(未知命令は 0 を返し、実行側が invalid として扱う)。
func gasCost(op Op) uint64 { return opTable[op].gas }

// #endregion opcode
