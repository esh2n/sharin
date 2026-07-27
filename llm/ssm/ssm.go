// Package ssm は状態空間モデル(state space model)を最小構成で実装する。
//
// attention は全トークンが全トークンを見るので計算量が系列長の二乗になる
// (transformer 編)。SSM は別の方針を取る。1 個の状態ベクトルを系列に沿って
// 更新していく線形漸化式で、1 ステップの計算は定数、全体で系列長に線形になる。
// RNN に似るが、更新が線形なので並列スキャンで高速化でき、学習も回せる。
// Mamba の選択的 SSM は、この状態への「取り込み量」を入力ごとに変える工夫で、
// どの情報を覚えてどれを捨てるかをトークンごとに選べるようにした。
package ssm

// #region ssm

// SSM は 1 次元の線形状態空間モデル。
//
//	h_t = A·h_{t-1} + B·x_t   (状態の更新)
//	y_t = C·h_t               (状態からの出力)
//
// A が減衰率で、|A|<1 なら過去の影響が指数的に薄れ、A=1 なら状態が保持される。
type SSM struct {
	A, B, C float64
}

// Scan は入力列を先頭から流し、各時刻の出力を返す。
// 1 ステップの計算は定数で、全体は系列長に線形。
func (m *SSM) Scan(x []float64) []float64 {
	y := make([]float64, len(x))
	h := 0.0
	for t, xt := range x {
		h = m.A*h + m.B*xt
		y[t] = m.C * h
	}
	return y
}

// ScanCounted は Scan に加え、状態更新の回数(= 系列長)を返す。
// attention の全ペア計算(AttentionOps)と比べて線形であることを示すための計測点。
func (m *SSM) ScanCounted(x []float64) ([]float64, int) {
	return m.Scan(x), len(x)
}

// AttentionOps は素の attention が長さ n で行う全ペアスコア計算の回数 n²。
// SSM の線形コストと対比するための参照値。
func AttentionOps(n int) int { return n * n }

// #endregion ssm

// #region selective

// Selective は選択的 SSM(Mamba の核)。入力ごとにゲート(取り込み量)が変わる。
// ゲートが 0 に近いトークンは状態にほとんど影響せず素通りし、1 に近いトークンは
// 強く取り込まれる。これで「関係ある情報だけ状態に残す」を系列上で選べる。
type Selective struct {
	decay float64 // 状態の基本減衰率(A に相当)
}

// NewSelective は基本減衰率 decay の選択的 SSM を作る。
func NewSelective(decay float64) *Selective {
	return &Selective{decay: decay}
}

// ScanSelective は入力列 x を、対応するゲート列 gate に従って流す。
// gate[t] は [0,1] にクランプされ、その時刻の入力の取り込み量になる:
//
//	h_t = decay·h_{t-1} + gate_t · x_t
//
// gate=1 は通常の取り込み、gate=0 は入力遮断(状態は減衰のみ)。
func (m *Selective) ScanSelective(x, gate []float64) []float64 {
	y := make([]float64, len(x))
	h := 0.0
	for t := range x {
		g := clamp01(gate[t])
		h = m.decay*h + g*0.1*x[t]
		y[t] = h
	}
	return y
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// #endregion selective
