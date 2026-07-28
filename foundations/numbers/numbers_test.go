package numbers

import "testing"

const W8 = Width(8)

func roundtrip(t *testing.T, kind Kind, w Width) {
	t.Helper()
	min, max := Range(w, kind)
	for v := min; v <= max; v++ {
		bits := Encode(v, w, kind)
		if got := Decode(bits, w, kind); got != v {
			t.Fatalf("%v: %d を書いて %d が読めた(bits=%b)", kind, v, got, bits)
		}
	}
}

// どの表し方でも、範囲の中なら書いて読めば元に戻る。
func TestRoundTrip(t *testing.T) {
	for _, k := range []Kind{Twos, Ones, SignMag} {
		roundtrip(t, k, 8)
		roundtrip(t, k, 5)
	}
}

// この章の中心その1。2の補数だけが 0 を1通りで表す。
func TestOnlyTwosHasASingleZero(t *testing.T) {
	if got := Zeros(W8, Twos); len(got) != 1 {
		t.Fatalf("2の補数の 0 が %d 通りある", len(got))
	}
	for _, k := range []Kind{Ones, SignMag} {
		zs := Zeros(W8, k)
		if len(zs) != 2 {
			t.Fatalf("%v の 0 が %d 通り", k, len(zs))
		}
		// ビットは違うのに、どちらも 0 として読める。
		if Decode(zs[0], W8, k) != 0 || Decode(zs[1], W8, k) != 0 {
			t.Fatalf("%v: 2つとも 0 にならない", k)
		}
		if zs[0] == zs[1] {
			t.Fatalf("%v: 同じビットになっている", k)
		}
	}
}

// 0 が1通りしかないぶん、2の補数は範囲が非対称になる。
func TestTwosRangeIsAsymmetric(t *testing.T) {
	min, max := Range(W8, Twos)
	if min != -128 || max != 127 {
		t.Fatalf("2の補数の範囲が違う: %d..%d", min, max)
	}
	if -min == max {
		t.Fatal("対称になっている")
	}
	for _, k := range []Kind{Ones, SignMag} {
		lo, hi := Range(W8, k)
		if lo != -127 || hi != 127 {
			t.Fatalf("%v の範囲が違う: %d..%d", k, lo, hi)
		}
	}
}

// この章の中心その2。減算が加算と同じ回路になる。
func TestSubtractionIsAddition(t *testing.T) {
	cases := []struct{ a, b int64 }{{10, 3}, {3, 10}, {-5, 7}, {-5, -7}, {0, 1}, {127, 1}}
	for _, c := range cases {
		a := Encode(c.a, W8, Twos)
		b := Encode(c.b, W8, Twos)

		viaSub := Decode(Sub(a, b, W8).Bits, W8, Twos)
		viaAdd := Decode(Add(a, Neg(b, W8), W8).Bits, W8, Twos)
		if viaSub != viaAdd {
			t.Fatalf("%d - %d: 引き算と「補数を足す」が違う: %d vs %d", c.a, c.b, viaSub, viaAdd)
		}
		if viaSub != c.a-c.b {
			t.Fatalf("%d - %d = %d のはずが %d", c.a, c.b, c.a-c.b, viaSub)
		}
	}
}

// この章の中心その3。いちばん小さい数は符号を反転できない。
func TestNegatingTheMinimumFails(t *testing.T) {
	min, _ := Range(W8, Twos)
	bits := Encode(min, W8, Twos)

	if got := Neg(bits, W8); got != bits {
		t.Fatalf("最小値の反転が自分自身にならない: %b → %b", bits, got)
	}
	if Decode(Neg(bits, W8), W8, Twos) != min {
		t.Fatal("反転しても最小値のまま、になっていない")
	}
	// つまり、この値だけ絶対値が取れない。
	if v := Decode(Neg(bits, W8), W8, Twos); v >= 0 {
		t.Fatalf("絶対値が取れてしまった: %d", v)
	}
	// 他の値は普通に反転できる。
	for _, v := range []int64{1, 42, 127, -1, -127} {
		if got := Decode(Neg(Encode(v, W8, Twos), W8), W8, Twos); got != -v {
			t.Fatalf("%d の反転が %d", v, got)
		}
	}
}

// はみ出しの判定は2つある。同じビット列でも、符号つきと符号なしで壊れ方が違う。
func TestCarryAndOverflowAreDifferent(t *testing.T) {
	// 200 + 100(符号なし)。繰り上がりは出るが、符号つきとしては壊れていない。
	r := Add(200, 100, W8)
	if !r.Carry {
		t.Fatal("符号なしの繰り上がりが出ていない")
	}
	if r.Overflow {
		t.Fatal("符号つきとしては壊れていないはず")
	}

	// 100 + 100(符号つき)。繰り上がりは出ないが、符号が壊れる。
	r = Add(Encode(100, W8, Twos), Encode(100, W8, Twos), W8)
	if r.Carry {
		t.Fatal("繰り上がりが出ている")
	}
	if !r.Overflow {
		t.Fatal("符号つきのはみ出しが検出されていない")
	}
	if got := Decode(r.Bits, W8, Twos); got >= 0 {
		t.Fatalf("正どうしを足して正のまま: %d", got)
	}

	// 負どうしでも起きる。
	r = Add(Encode(-100, W8, Twos), Encode(-100, W8, Twos), W8)
	if !r.Overflow {
		t.Fatal("負どうしのはみ出しが検出されていない")
	}
	// 符号が違うものどうしは、決して壊れない。
	for _, c := range []struct{ a, b int64 }{{127, -1}, {-128, 127}, {1, -1}} {
		if Add(Encode(c.a, W8, Twos), Encode(c.b, W8, Twos), W8).Overflow {
			t.Fatalf("%d + %d で壊れた", c.a, c.b)
		}
	}
	// 0 を引くと借りが出ない。
	if !Sub(Encode(5, W8, Twos), 0, W8).Carry {
		t.Fatal("0 を引いて借りが出たことになっている")
	}
}

// 幅を広げるとき、符号を保つかどうかで結果が変わる。
func TestSignExtendVersusZeroExtend(t *testing.T) {
	bits := Encode(-1, W8, Twos) // 0b11111111

	se := SignExtend(bits, W8, 16)
	if Decode(se, 16, Twos) != -1 {
		t.Fatalf("符号を保って広げられていない: %d", Decode(se, 16, Twos))
	}
	ze := ZeroExtend(bits, W8, 16)
	if Decode(ze, 16, Twos) != 255 {
		t.Fatalf("0 で埋めた結果が違う: %d", Decode(ze, 16, Twos))
	}
	if se == ze {
		t.Fatal("同じビット列を広げて同じ結果になった")
	}
	// 正の数なら、どちらで広げても同じ。
	pos := Encode(42, W8, Twos)
	if SignExtend(pos, W8, 16) != ZeroExtend(pos, W8, 16) {
		t.Fatal("正の数で結果が分かれた")
	}
}

// 右にずらす操作も2つある。負の数で結果が分かれる。
func TestArithmeticVersusLogicalShift(t *testing.T) {
	bits := Encode(-8, W8, Twos)

	arith := ShiftRight(bits, 1, W8, true)
	if got := Decode(arith, W8, Twos); got != -4 {
		t.Fatalf("算術シフトで -8 >> 1 が %d", got)
	}
	logical := ShiftRight(bits, 1, W8, false)
	if got := Decode(logical, W8, Twos); got == -4 {
		t.Fatal("論理シフトで符号が保たれてしまった")
	}
	if got := logical; got != 124 {
		t.Fatalf("論理シフトの結果が違う: %d", got)
	}
	// 正の数なら同じ。
	p := Encode(8, W8, Twos)
	if ShiftRight(p, 1, W8, true) != ShiftRight(p, 1, W8, false) {
		t.Fatal("正の数で結果が分かれた")
	}
}

// バイト順は取り決め。書いた順と違う順で読むと壊れる。
func TestEndiannessMustMatch(t *testing.T) {
	const v = uint64(0x12345678)

	le := PutLittle(v, 32)
	be := PutBig(v, 32)
	if le[0] != 0x78 || le[3] != 0x12 {
		t.Fatalf("下位から並んでいない: %x", le)
	}
	if be[0] != 0x12 || be[3] != 0x78 {
		t.Fatalf("上位から並んでいない: %x", be)
	}

	if GetLittle(le) != v || GetBig(be) != v {
		t.Fatal("同じ順で読めば戻るはず")
	}
	if GetBig(le) == v {
		t.Fatal("違う順で読んでも壊れなかった")
	}
	if GetBig(le) != 0x78563412 {
		t.Fatalf("入れ替わり方が想定と違う: %x", GetBig(le))
	}
}

// 下位から並べる順の取り柄。途中で止めても意味が壊れない。
func TestLittleEndianCanBeReadNarrow(t *testing.T) {
	const v = uint64(0x12345678)
	le := PutLittle(v, 32)

	// 先頭の1バイトだけ読むと、下位8ビットになる。
	if GetLittle(le[:1]) != 0x78 {
		t.Fatalf("先頭1バイトが下位になっていない: %x", GetLittle(le[:1]))
	}
	// 先頭の2バイトなら下位16ビット。
	if GetLittle(le[:2]) != 0x5678 {
		t.Fatalf("先頭2バイトが下位16ビットになっていない: %x", GetLittle(le[:2]))
	}

	// 上位から並べる順では、先頭を読んでも上位が出る。
	be := PutBig(v, 32)
	if GetBig(be[:1]) == 0x78 {
		t.Fatal("上位から並べたのに下位が読めた")
	}
	if GetBig(be[:1]) != 0x12 {
		t.Fatalf("先頭が上位になっていない: %x", GetBig(be[:1]))
	}
}

// 表し方の名前。
func TestKindNames(t *testing.T) {
	if Twos.String() == "" || Ones.String() == "" || SignMag.String() == "" {
		t.Fatal("名前が空")
	}
	if Twos.String() == Ones.String() {
		t.Fatal("名前が同じ")
	}
}
