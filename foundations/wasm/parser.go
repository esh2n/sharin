package wasm

import "fmt"

// #region parser

// Parse は .wasm バイト列を Module へデコードする。マジックとバージョンを確かめ、
// セクションを順に読む。知らないセクションは長さぶん読み飛ばす(前方互換)。
func Parse(bin []byte) (*Module, error) {
	r := newReader(bin)
	if err := r.expect([]byte{0x00, 0x61, 0x73, 0x6d}, "マジック(\\0asm)"); err != nil {
		return nil, err
	}
	if err := r.expect([]byte{0x01, 0x00, 0x00, 0x00}, "バージョン"); err != nil {
		return nil, err
	}

	m := &Module{Exports: map[string]int{}}
	for !r.eof() {
		id, err := r.readByte()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		body, err := r.readBytes(int(size))
		if err != nil {
			return nil, err
		}
		sr := newReader(body)
		switch id {
		case secType:
			if err := parseTypes(sr, m); err != nil {
				return nil, err
			}
		case secFunc:
			if err := parseFuncs(sr, m); err != nil {
				return nil, err
			}
		case secExport:
			if err := parseExports(sr, m); err != nil {
				return nil, err
			}
		case secCode:
			if err := parseCode(sr, m); err != nil {
				return nil, err
			}
		default:
			// 未対応セクション(memory/global/import 等)は読み飛ばす。
		}
	}
	return m, nil
}

// parseTypes は型セクション: 各関数シグネチャ(引数型ベクタ + 戻り型ベクタ)。
func parseTypes(r *reader, m *Module) error {
	n, err := r.u32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		tag, err := r.readByte()
		if err != nil {
			return err
		}
		if tag != funcTypeTag {
			return fmt.Errorf("wasm: 関数型タグが不正 (0x%x)", tag)
		}
		params, err := readValTypes(r)
		if err != nil {
			return err
		}
		results, err := readValTypes(r)
		if err != nil {
			return err
		}
		m.Types = append(m.Types, FuncType{Params: params, Results: results})
	}
	return nil
}

// readValTypes は値型ベクタを読み、その本数を返す(本実装は i32 のみ受理)。
func readValTypes(r *reader) (int, error) {
	n, err := r.u32()
	if err != nil {
		return 0, err
	}
	for i := uint32(0); i < n; i++ {
		vt, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if vt != i32Type {
			return 0, fmt.Errorf("wasm: i32 以外の値型は未対応 (0x%x)", vt)
		}
	}
	return int(n), nil
}

// parseFuncs は関数セクション: 各関数の型インデックス。
func parseFuncs(r *reader, m *Module) error {
	n, err := r.u32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		ti, err := r.u32()
		if err != nil {
			return err
		}
		m.Funcs = append(m.Funcs, Func{Type: int(ti)})
	}
	return nil
}

// parseExports は export セクション: 名前 → (種類, インデックス)。関数だけ拾う。
func parseExports(r *reader, m *Module) error {
	n, err := r.u32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		nameLen, err := r.u32()
		if err != nil {
			return err
		}
		nameB, err := r.readBytes(int(nameLen))
		if err != nil {
			return err
		}
		kind, err := r.readByte()
		if err != nil {
			return err
		}
		idx, err := r.u32()
		if err != nil {
			return err
		}
		if kind == exportFunc {
			m.Exports[string(nameB)] = int(idx)
		}
	}
	return nil
}

// parseCode はコードセクション: 各関数のローカル宣言と本体命令列。
func parseCode(r *reader, m *Module) error {
	n, err := r.u32()
	if err != nil {
		return err
	}
	for i := uint32(0); i < n; i++ {
		bodySize, err := r.u32()
		if err != nil {
			return err
		}
		bodyBytes, err := r.readBytes(int(bodySize))
		if err != nil {
			return err
		}
		br := newReader(bodyBytes)
		locals, err := readLocals(br)
		if err != nil {
			return err
		}
		body, err := decodeBody(br)
		if err != nil {
			return err
		}
		if int(i) >= len(m.Funcs) {
			return fmt.Errorf("wasm: コード数が関数数を超えた")
		}
		m.Funcs[i].Locals = locals
		m.Funcs[i].Body = body
	}
	return nil
}

// readLocals はローカル宣言(同じ型が何本、を並べたもの)を読み、総本数を返す。
func readLocals(r *reader) (int, error) {
	groups, err := r.u32()
	if err != nil {
		return 0, err
	}
	total := 0
	for i := uint32(0); i < groups; i++ {
		count, err := r.u32()
		if err != nil {
			return 0, err
		}
		vt, err := r.readByte()
		if err != nil {
			return 0, err
		}
		if vt != i32Type {
			return 0, fmt.Errorf("wasm: i32 以外のローカルは未対応 (0x%x)", vt)
		}
		total += int(count)
	}
	return total, nil
}

// decodeBody は本体を命令列にデコードし、block/loop/if を対応する end / else に
// 結びつける(実行時のジャンプ先をコンパイル時に確定させる)。
func decodeBody(r *reader) ([]Instr, error) {
	var instrs []Instr
	for !r.eof() {
		op, err := r.readByte()
		if err != nil {
			return nil, err
		}
		ins := Instr{Op: op}
		switch op {
		case opBlock, opLoop, opIf:
			if _, err := r.readByte(); err != nil { // blocktype(void/i32 を許容し無視)
				return nil, err
			}
			if op == opIf {
				ins.Else = -1 // else 未検出の印
			}
		case opBr, opBrIf, opCall, opLocalGet, opLocalSet, opLocalTee:
			v, err := r.u32()
			if err != nil {
				return nil, err
			}
			ins.Imm = int64(v)
		case opI32Const:
			v, err := r.s32()
			if err != nil {
				return nil, err
			}
			ins.Imm = int64(v)
		case opEnd, opElse, opReturn, opDrop, opNop, opUnreachable,
			opI32Eqz, opI32Eq, opI32Ne, opI32LtS, opI32GtS, opI32LeS, opI32GeS,
			opI32Add, opI32Sub, opI32Mul, opI32DivS, opI32RemS:
			// 即値なし
		default:
			return nil, fmt.Errorf("wasm: 未対応のオペコード 0x%x", op)
		}
		instrs = append(instrs, ins)
	}
	matchBlocks(instrs)
	return instrs, nil
}

// #region control

// matchBlocks は入れ子ブロックの開始命令に、対応する else / end のインデックスを埋める。
// これで実行時の br の飛び先が静的に決まる——WASM が実行前に検証できる理由の一端。
func matchBlocks(instrs []Instr) {
	var stack []int
	for i := range instrs {
		switch instrs[i].Op {
		case opBlock, opLoop, opIf:
			stack = append(stack, i)
		case opElse:
			if len(stack) > 0 {
				instrs[stack[len(stack)-1]].Else = i
			}
		case opEnd:
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				instrs[top].End = i
				if instrs[top].Op == opIf && instrs[top].Else == -1 {
					instrs[top].Else = i // else 無き if は、偽なら end へ
				}
			}
		}
	}
}

// #endregion control

// #endregion parser
