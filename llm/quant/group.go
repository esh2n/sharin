package quant

import "math"

// #region group

// Grouped は Size 要素ごとに独立の scale を持つ量子化データ。
type Grouped struct {
	Codes  []int
	Scales []float64
	Size   int
	bits   int
}

// QuantizeGroupwise は size 要素ごとに scale を決める。
//
// per-channel は行の端から端までで1つの scale なので、行の中に1つでも
// 大きい値があると、行ぜんぶの格子が粗くなる。区切りを細かくすれば、
// 大きい値の影響はその区切りの中だけで止まる。
//
// 実物の 4bit 量子化はほぼこの単位で、区切りは 64 か 128 要素が多い。
func QuantizeGroupwise(x []float64, bits, size int) *Grouped {
	qmax := (1 << (bits - 1)) - 1
	g := &Grouped{Codes: make([]int, len(x)), Size: size, bits: bits}
	for start := 0; start < len(x); start += size {
		end := start + size
		if end > len(x) {
			end = len(x)
		}
		maxAbs := 0.0
		for _, v := range x[start:end] {
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}
		scale := maxAbs / float64(qmax)
		if scale == 0 {
			scale = 1
		}
		for i := start; i < end; i++ {
			g.Codes[i] = clampInt(int(math.Round(x[i]/scale)), -qmax, qmax)
		}
		g.Scales = append(g.Scales, scale)
	}
	return g
}

// Dequantize は整数コードを実数へ戻す。区切りごとの scale を掛ける。
func (g *Grouped) Dequantize() []float64 {
	out := make([]float64, len(g.Codes))
	for i, c := range g.Codes {
		out[i] = float64(c) * g.Scales[i/g.Size]
	}
	return out
}

// BitsPerValue は1要素あたりの実効ビット数。
//
// 区切りごとに scale を fp16 で持つので、その分が上乗せになる。
// 4bit・64要素なら 4 + 16/64 = 4.25 ビット。細かくするほど精度は上がるが、
// 縮めたはずのメモリを scale が食い返す。
func (g *Grouped) BitsPerValue() float64 {
	return float64(g.bits) + 16.0/float64(g.Size)
}

// #endregion group
