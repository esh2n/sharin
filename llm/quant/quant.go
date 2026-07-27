// Package quant は重みの量子化を最小構成で実装する。
//
// 学習済みの重みは fp32(4 byte)で持つと巨大になる。70B パラメータなら
// それだけで 280GB。量子化は各重みを少ないビット(int8 なら 1 byte、int4 なら
// 0.5 byte)の整数に写して、メモリを 1/4〜1/8 に縮める。仕組みは単純で、
// 値の範囲を整数の格子に写す scale を決め、round で最寄りの格子点に丸めるだけ。
// 丸め誤差と引き換えにメモリを買う。どこまで刻むか(ビット数)、どの単位で
// scale を決めるか(テンソル全体か行ごとか)が設計の勘所になる。
package quant

import "math"

// #region symmetric

// Quantized は量子化した 1 次元データ。Codes は整数コード、Scale は復元係数、
// Zero は非対称量子化のゼロ点(対称なら 0)。
type Quantized struct {
	Codes []int
	Scale float64
	Zero  int
}

// QuantizeSymmetric は対称量子化。max|x| を整数の最大コードに合わせ、
// 0 を厳密にコード 0 へ写す。重み(0 対称に分布しがち)に向く。
func QuantizeSymmetric(x []float64, bits int) *Quantized {
	qmax := (1 << (bits - 1)) - 1 // 例: 8bit → 127
	maxAbs := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > maxAbs {
			maxAbs = a
		}
	}
	scale := maxAbs / float64(qmax)
	if scale == 0 {
		scale = 1 // 全ゼロ入力のフォールバック(0除算回避)
	}
	codes := make([]int, len(x))
	for i, v := range x {
		codes[i] = clampInt(int(math.Round(v/scale)), -qmax, qmax)
	}
	return &Quantized{Codes: codes, Scale: scale, Zero: 0}
}

// #endregion symmetric

// #region asymmetric

// QuantizeAsymmetric は非対称量子化。[min, max] を [0, 2^bits-1] に写す。
// 片側に偏った分布(活性値など、0 以上に固まる)で全コード域を使え、
// 対称より誤差が小さくなる。
func QuantizeAsymmetric(x []float64, bits int) *Quantized {
	qmax := (1 << bits) - 1 // 例: 8bit → 255
	lo, hi := x[0], x[0]
	for _, v := range x {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	scale := (hi - lo) / float64(qmax)
	if scale == 0 {
		scale = 1
	}
	zero := int(math.Round(-lo / scale)) // 実数 0 に対応するコード
	codes := make([]int, len(x))
	for i, v := range x {
		codes[i] = clampInt(int(math.Round(v/scale))+zero, 0, qmax)
	}
	return &Quantized{Codes: codes, Scale: scale, Zero: zero}
}

// Dequantize は整数コードを実数へ戻す。x' = (code - zero) · scale。
func (q *Quantized) Dequantize() []float64 {
	out := make([]float64, len(q.Codes))
	for i, c := range q.Codes {
		out[i] = float64(c-q.Zero) * q.Scale
	}
	return out
}

// #endregion asymmetric

// #region perchannel

// QuantizedMatrix は行ごとに独立の scale を持つ量子化行列(per-channel)。
type QuantizedMatrix struct {
	rows       [][]int
	perChannel []float64 // 行ごとの scale(per-tensor なら全行同一)
}

// QuantizeMatrixPerTensor は行列全体で 1 つの scale を使う(per-tensor)。
// スケールがまちまちな行があると、大きい行に引っ張られて小さい行の分解能が潰れる。
func QuantizeMatrixPerTensor(rows [][]float64, bits int) *QuantizedMatrix {
	qmax := (1 << (bits - 1)) - 1
	maxAbs := 0.0
	for _, r := range rows {
		for _, v := range r {
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}
	}
	scale := maxAbs / float64(qmax)
	if scale == 0 {
		scale = 1
	}
	m := &QuantizedMatrix{}
	for _, r := range rows {
		codes := make([]int, len(r))
		for j, v := range r {
			codes[j] = clampInt(int(math.Round(v/scale)), -qmax, qmax)
		}
		m.rows = append(m.rows, codes)
		m.perChannel = append(m.perChannel, scale)
	}
	return m
}

// QuantizeMatrixPerChannel は行ごとに scale を決める(per-channel)。
// 各行が自分のレンジをフルに使えるので、行間でスケールが違っても精度が保たれる。
// 実物の重み量子化はほぼこの単位(出力チャネルごと)で行う。
func QuantizeMatrixPerChannel(rows [][]float64, bits int) *QuantizedMatrix {
	qmax := (1 << (bits - 1)) - 1
	m := &QuantizedMatrix{}
	for _, r := range rows {
		maxAbs := 0.0
		for _, v := range r {
			if a := math.Abs(v); a > maxAbs {
				maxAbs = a
			}
		}
		scale := maxAbs / float64(qmax)
		if scale == 0 {
			scale = 1
		}
		codes := make([]int, len(r))
		for j, v := range r {
			codes[j] = clampInt(int(math.Round(v/scale)), -qmax, qmax)
		}
		m.rows = append(m.rows, codes)
		m.perChannel = append(m.perChannel, scale)
	}
	return m
}

// DequantizeRow は指定行を実数へ戻す。
func (m *QuantizedMatrix) DequantizeRow(r int) []float64 {
	out := make([]float64, len(m.rows[r]))
	for j, c := range m.rows[r] {
		out[j] = float64(c) * m.perChannel[r]
	}
	return out
}

// #endregion perchannel

// #region memory

// MemoryBytes は count 個の値を bits ビットで持つときのバイト数。
// fp32=4byte、int8=1byte、int4=0.5byte。
func MemoryBytes(count, bits int) int {
	return count * bits / 8
}

// CompressionRatio は fp32(32bit)に対する圧縮率。
func CompressionRatio(bits int) float64 {
	return 32.0 / float64(bits)
}

// #endregion memory

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
