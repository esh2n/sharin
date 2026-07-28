package quant

import "math"

// #region outlier

// Mixed は外れ値だけを元の精度で残し、残りを整数にした形。
type Mixed struct {
	// Base は外れ値を抜いた残りの量子化結果。抜いた位置には 0 が入っている。
	Base *Quantized
	// Outlier は添字から元の値へ。ここだけ fp16 のまま持つ。
	Outlier map[int]float64
}

// QuantizeWithOutliers は絶対値が threshold を超える要素を分けてから量子化する。
//
// LLM の活性には、ごく一部だけ桁違いに大きい成分が出る。全体で1つの scale を
// 決めると、その数個に合わせて格子が粗くなり、残りの大多数の分解能が潰れる。
// 桁が違うので、per-channel や区切りを細かくするだけでは追いつかない。
//
// 外れ値を配列から抜いて元の精度のまま持ち、残りだけを量子化すると、
// 格子は残りのレンジに合う。LLM.int8() がしているのはこれになる。
func QuantizeWithOutliers(x []float64, bits int, threshold float64) *Mixed {
	rest := make([]float64, len(x))
	out := map[int]float64{}
	for i, v := range x {
		if math.Abs(v) > threshold {
			out[i] = v
			continue // 抜いた位置は 0 のまま。scale の決定に加わらない
		}
		rest[i] = v
	}
	return &Mixed{Base: QuantizeSymmetric(rest, bits), Outlier: out}
}

// Dequantize は整数の側を戻し、外れ値の位置を元の値で上書きする。
func (m *Mixed) Dequantize() []float64 {
	out := m.Base.Dequantize()
	for i, v := range m.Outlier {
		out[i] = v
	}
	return out
}

// OutlierRatio は外れ値の割合。ここが小さいから成り立つ手になる。
func (m *Mixed) OutlierRatio() float64 {
	if len(m.Base.Codes) == 0 {
		return 0
	}
	return float64(len(m.Outlier)) / float64(len(m.Base.Codes))
}

// #endregion outlier

// #region salient

// Scaled は列ごとの倍率で引き伸ばしてから量子化した重み。
type Scaled struct {
	Q *Quantized
	S []float64
}

// QuantizeScaled は列ごとの倍率 s で重みを引き伸ばしてから量子化する。
//
// 丸めの誤差は格子間隔の半分で、どの列でも同じだけ出る。だが出力への効きは
// 列ごとに違う。活性が大きい列の誤差は、そのぶん増幅されて出力に届く。
//
// そこで、効きの大きい列だけ重みを s 倍しておき、活性を 1/s 倍しておく。
// 掛けて割るので積は変わらないが、引き伸ばした列は格子の目盛りを s 倍
// 多く使うので、その列の丸め誤差は実質 1/s になる。AWQ の考え方になる。
func QuantizeScaled(w, s []float64, bits int) *Scaled {
	up := make([]float64, len(w))
	for i, v := range w {
		up[i] = v * s[i]
	}
	return &Scaled{Q: QuantizeSymmetric(up, bits), S: append([]float64(nil), s...)}
}

// Dequantize は復元してから倍率で割り戻す。
func (s *Scaled) Dequantize() []float64 {
	out := s.Q.Dequantize()
	for i := range out {
		out[i] /= s.S[i]
	}
	return out
}

// #endregion salient
