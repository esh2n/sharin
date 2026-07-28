package quant

// #region matmul

// DotCodes は整数コードのまま内積を取る。
//
//	Σ (a_i·sa)(b_i·sb) = sa·sb · Σ a_i b_i
//
// scale は総和の外に括り出せる。だから中は整数の積和だけで済む。
// 量子化がメモリだけでなく計算まで速くする理由がここにある。
//
// 桁あふれの心配も勘定できる。int8 どうしの積は最大 127×127 = 16129 で、
// 4096 項を足しても 6.6×10⁷。int32 の上限 2.1×10⁹ には遠い。
func DotCodes(a, b *Quantized) int {
	acc := 0
	for i := range a.Codes {
		acc += (a.Codes[i] - a.Zero) * (b.Codes[i] - b.Zero)
	}
	return acc
}

// Dot は括り出した scale を最後に1回だけ掛ける。
//
// 復元してから掛けると、要素ごとに浮動小数の積が要る。括り出すと、
// 浮動小数の積は最後の1回だけになる。
func Dot(a, b *Quantized) float64 {
	return float64(DotCodes(a, b)) * a.Scale * b.Scale
}

// #endregion matmul
