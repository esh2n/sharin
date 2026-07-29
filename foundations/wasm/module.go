package wasm

// #region module

// オペコード(WASM の実際のバイト値)。本実装が解釈する最小サブセット。
const (
	opUnreachable = 0x00
	opNop         = 0x01
	opBlock       = 0x02 // block <blocktype> ... end
	opLoop        = 0x03 // loop  <blocktype> ... end
	opIf          = 0x04 // if <blocktype> ... (else ...) end
	opElse        = 0x05
	opEnd         = 0x0b
	opBr          = 0x0c // br <labelidx>: labelidx 個外側のブロックへ脱出/継続
	opBrIf        = 0x0d // br_if <labelidx>: スタック先頭が真なら br
	opReturn      = 0x0f
	opCall        = 0x10 // call <funcidx>
	opDrop        = 0x1a
	opLocalGet    = 0x20 // local.get <localidx>
	opLocalSet    = 0x21
	opLocalTee    = 0x22 // set しつつ値をスタックに残す
	opI32Const    = 0x41 // i32.const <s32>
	opI32Eqz      = 0x45
	opI32Eq       = 0x46
	opI32Ne       = 0x47
	opI32LtS      = 0x48
	opI32GtS      = 0x4a
	opI32LeS      = 0x4c
	opI32GeS      = 0x4e
	opI32Add      = 0x6a
	opI32Sub      = 0x6b
	opI32Mul      = 0x6c
	opI32DivS     = 0x6d
	opI32RemS     = 0x6f
)

// セクション ID。
const (
	secType   = 1
	secFunc   = 3
	secExport = 7
	secCode   = 10
)

const (
	i32Type     = 0x7f // i32 の値型タグ
	funcTypeTag = 0x60 // 関数型の先頭タグ
	blockVoid   = 0x40 // 結果なしブロック型
	exportFunc  = 0x00 // export の種類: 関数
)

// FuncType は関数シグネチャ。本実装は i32 のみ(型は個数だけ意味を持つ)。
type FuncType struct {
	Params  int // 引数の個数
	Results int // 戻り値の個数(0 か 1)
}

// Instr はデコード済みの 1 命令。Op はオペコード、Imm は即値(定数/インデックス)。
// 構造化命令(block/loop/if)は、対応する else / end の命令インデックスを持ち、
// 実行時のジャンプ先計算をコンパイル時に済ませておく(検証プリパスで埋める)。
type Instr struct {
	Op   byte
	Imm  int64 // i32.const の値、または local/br/call のインデックス
	Else int   // if の else の命令インデックス(無ければ end と同じ)
	End  int   // block/loop/if に対応する end の命令インデックス
}

// Func は 1 つの関数: 型・ローカル変数の本数(引数を除く)・デコード済み命令列。
type Func struct {
	Type   int // Module.Types のインデックス
	Locals int // 引数以外のローカル変数の本数(初期値 0)
	Body   []Instr
}

// Module はパース済みの WASM モジュール。
type Module struct {
	Types   []FuncType
	Funcs   []Func
	Exports map[string]int // export 名 → funcidx
}

// FuncType はある関数の型を返す。
func (m *Module) FuncType(fi int) FuncType { return m.Types[m.Funcs[fi].Type] }

// #endregion module
