// Package wasm は WebAssembly の最小サブセットを Go で実装する——**実際の .wasm
// バイナリをパースし、スタックマシンで実行する**。ランタイム内部編のパーツ。
//
// bytecode 編で作った自前バイトコード VM の「実物・標準化版」が WASM だ。違いは3つ:
//   - 実在するバイナリ形式(マジック \0asm + セクション + LEB128 可変長整数)をパースする
//   - 値は型付き(ここでは i32 に限定)で、スタックマシンで受け渡す
//   - 制御フローが**構造化**されている——任意のアドレスへ goto するのではなく、
//     block / loop / if という入れ子ブロックと、その「深さ」への br(脱出/継続)だけ。
//     これが WASM を検証可能(安全)にしている核心
//
// 4 段で構成する:
//   - leb.go    LEB128(可変長整数)の読み取り。バイナリ形式の土台
//   - module.go 型・関数・モジュールの表現とオペコード定義
//   - parser.go .wasm バイト列 → Module(セクションを読み、関数本体を命令列へ)
//   - interp.go スタックマシン + 構造化制御フローの実行
package wasm

import (
	"errors"
	"fmt"
)

// #region leb

// reader は .wasm バイト列を先頭から順に読むカーソル。
type reader struct {
	b   []byte
	pos int
}

func newReader(b []byte) *reader { return &reader{b: b} }

func (r *reader) eof() bool { return r.pos >= len(r.b) }

// readByte は 1 バイト読み進める。
func (r *reader) readByte() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, errors.New("wasm: 予期しない終端")
	}
	b := r.b[r.pos]
	r.pos++
	return b, nil
}

// readBytes は n バイトぶんの部分列を返す。
func (r *reader) readBytes(n int) ([]byte, error) {
	if r.pos+n > len(r.b) {
		return nil, errors.New("wasm: バイト不足")
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

// uleb は符号なし LEB128 を読む。7 ビットずつ、最上位ビットが「続く」印。
// セクション長・インデックス・要素数など、WASM の可変長整数はほぼこれ。
func (r *reader) uleb() (uint64, error) {
	var result uint64
	var shift uint
	for {
		b, err := r.readByte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, errors.New("wasm: LEB128 が長すぎる")
		}
	}
	return result, nil
}

// u32 は符号なし LEB128 を uint32 として読む。
func (r *reader) u32() (uint32, error) {
	v, err := r.uleb()
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

// sleb は符号つき LEB128 を読む。最後のバイトの符号ビットで負数へ拡張する。
// i32.const の即値などはこちら。
func (r *reader) sleb() (int64, error) {
	var result int64
	var shift uint
	var b byte
	for {
		bb, err := r.readByte()
		if err != nil {
			return 0, err
		}
		b = bb
		result |= int64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
		if shift >= 64 {
			return 0, errors.New("wasm: LEB128 が長すぎる")
		}
	}
	// 符号拡張: 読み切ったビット幅の外側を、符号ビットで埋める。
	if shift < 64 && b&0x40 != 0 {
		result |= -1 << shift
	}
	return result, nil
}

// s32 は符号つき LEB128 を int32 として読む。
func (r *reader) s32() (int32, error) {
	v, err := r.sleb()
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

// expect は指定バイト列が続くことを確かめて読み飛ばす(マジック等の検査)。
func (r *reader) expect(want []byte, what string) error {
	got, err := r.readBytes(len(want))
	if err != nil {
		return err
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("wasm: %s が不正", what)
		}
	}
	return nil
}

// #endregion leb
