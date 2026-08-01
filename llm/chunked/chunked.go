// Package chunked は、attention と線形 attention が 1 つのつまみで繋がることを
// 最小構成で実装する。
//
// [attention](attention) は全トークン対のスコアを取るので、計算量が系列長の二乗になった。
// [Mamba / SSM](ssm) は状態を 1 個持って流すので線形になった。この 2 つは別方式に見えるが、
// 実際には連続的に繋がっている。繋いでいるのは「何トークンずつまとめて処理するか」
// というチャンクの大きさ 1 つだけになる。
//
// 出発点は結合則だ。softmax を外すと (q kᵀ) v と q (kᵀ v) が同じ値になり、
// 後者は K と V を d×d の状態に畳める。系列長に依存しない固定サイズの状態が、
// ここで手に入る。
//
// チャンクに切ると、その両端の間を取れる。チャンクの中は全対のスコアを取り(attention の側)、
// チャンクをまたぐところは状態で運ぶ(線形の側)。チャンクの大きさを系列長にすれば
// 全体が 1 つのチャンクになって通常の attention に戻り、1 にすれば線形 attention になる。
//
// 行列は整数で持つ。丸めが入らないので、結合則の一致を厳密に確かめられる。
package chunked

// #region matrix

// Mat は行優先の整数行列。丸めを入れないために整数で持つ。
type Mat struct {
	Rows, Cols int
	Data       []int
}

// New は rows×cols の行列を作る。
func New(rows, cols int, data ...int) Mat {
	m := Mat{Rows: rows, Cols: cols, Data: make([]int, rows*cols)}
	copy(m.Data, data)
	return m
}

// At は (r, c) の値。
func (m Mat) At(r, c int) int { return m.Data[r*m.Cols+c] }

// Mul は行列積。形が合わなければ panic する。
func Mul(a, b Mat) Mat {
	if a.Cols != b.Rows {
		panic("形が合わない")
	}
	out := New(a.Rows, b.Cols)
	for i := 0; i < a.Rows; i++ {
		for p := 0; p < a.Cols; p++ {
			av := a.At(i, p)
			if av == 0 {
				continue
			}
			for j := 0; j < b.Cols; j++ {
				out.Data[i*out.Cols+j] += av * b.At(p, j)
			}
		}
	}
	return out
}

// T は転置。
func (m Mat) T() Mat {
	out := New(m.Cols, m.Rows)
	for i := 0; i < m.Rows; i++ {
		for j := 0; j < m.Cols; j++ {
			out.Data[j*out.Cols+i] = m.At(i, j)
		}
	}
	return out
}

// Equal は形と全要素が一致するか。
func Equal(a, b Mat) bool {
	if a.Rows != b.Rows || a.Cols != b.Cols {
		return false
	}
	for i := range a.Data {
		if a.Data[i] != b.Data[i] {
			return false
		}
	}
	return true
}

// #endregion matrix

// #region assoc

// ScoresFirst は (q kᵀ) v の順で計算する。attention の順序。
//
// 先に全対のスコア(L×L)を作るので、そこで系列長の二乗が現れる。
func ScoresFirst(q, k, v Mat) Mat {
	scores := Mul(q, k.T()) // L×L ← ここが二乗
	return Mul(scores, v)
}

// StateFirst は q (kᵀ v) の順で計算する。線形 attention の順序。
//
// 先に kᵀv(d×d)を作るので、系列長に依存しない固定サイズの中間になる。
func StateFirst(q, k, v Mat) Mat {
	state := Mul(k.T(), v) // d×d ← 系列長が出てこない
	return Mul(q, state)
}

// State は K と V から畳んだ状態を返す。大きさは d×d で、系列長に依らない。
func State(k, v Mat) Mat { return Mul(k.T(), v) }

// #endregion assoc

// #region causal

// CausalScoresFirst は因果マスクつきで (q kᵀ) v を計算する。attention の側の基準。
//
// 全対のスコアを作ってから上三角を落とすので、L×L の面積をいったん確保する。
func CausalScoresFirst(q, k, v Mat) Mat {
	scores := Mul(q, k.T())
	tril(&scores)
	return Mul(scores, v)
}

// CausalStateFirst は状態を 1 トークンずつ育てながら読む。線形の側の基準。
//
// 位置 i では、そこまでの状態 S_i = Σ_{j≤i} kⱼᵀvⱼ を読む。持つのは d×d だけになる。
func CausalStateFirst(q, k, v Mat) Mat {
	l, d := q.Rows, v.Cols
	out := New(l, d)
	state := New(k.Cols, v.Cols)
	for i := 0; i < l; i++ {
		add(&state, Mul(rows(k, i, i+1).T(), rows(v, i, i+1)))
		o := Mul(rows(q, i, i+1), state)
		copy(out.Data[i*d:(i+1)*d], o.Data)
	}
	return out
}

// tril は上三角(自分より後ろを見る位置)を 0 にする。
func tril(m *Mat) {
	for i := 0; i < m.Rows; i++ {
		for j := i + 1; j < m.Cols; j++ {
			m.Data[i*m.Cols+j] = 0
		}
	}
}

// #endregion causal

// #region size

// StateSize は状態が持つ要素数。系列長に依らず d×d。
func StateSize(d int) int { return d * d }

// CacheSize は KV キャッシュが持つ要素数。K と V の 2 本ぶんで、系列長に比例する。
func CacheSize(n, d int) int { return 2 * n * d }

// #endregion size

// #region flops

// Shape は系列長 L、次元 d、チャンクの大きさ C。
type Shape struct {
	L, D, C int
}

// StateFlops は状態の読み書きにかかる積和の回数。
//
// チャンクごとに「状態へ書く」と「状態から読む」で 2 回、それぞれ C·d² かかる。
// チャンクの本数を掛けると C が消えて 2·L·d² になり、**チャンクの大きさに依らない**。
func (s Shape) StateFlops() int { return 2 * s.L * s.D * s.D }

// ScoreFlops はチャンク内の全対スコアにかかる積和の回数。
//
// チャンクごとに「スコアを作る」と「値を混ぜる」で 2 回、それぞれ C²·d かかる。
// チャンクの本数を掛けると 2·L·C·d になり、**チャンクの大きさに比例する**。
func (s Shape) ScoreFlops() int { return 2 * s.L * s.C * s.D }

// Flops は合計。固定部と、C に比例する部の和になる。
func (s Shape) Flops() int { return s.StateFlops() + s.ScoreFlops() }

// IntraMemory はチャンク内で持つスコア行列の要素数。C×C。
func (s Shape) IntraMemory() int { return s.C * s.C }

// IsFullAttention はチャンクが系列全体と同じ、つまり通常の attention か。
func (s Shape) IsFullAttention() bool { return s.C >= s.L }

// IsLinear はチャンクが 1、つまり線形 attention か。
func (s Shape) IsLinear() bool { return s.C == 1 }

// #endregion flops

// #region chunked

// Chunked はチャンクに切って計算する。
//
// チャンクの中は全対のスコア(上三角は落とす)、チャンクをまたぐところは状態で運ぶ。
// C を系列長にすれば CausalScoresFirst と、1 にすれば CausalStateFirst と同じ値になる。
// 途中の C でも結果は変わらず、変わるのは計算の配分だけになる。
func Chunked(q, k, v Mat, c int) Mat {
	if c <= 0 {
		c = 1
	}
	l, d := q.Rows, v.Cols
	out := New(l, d)
	state := New(k.Cols, v.Cols) // d×d。チャンクをまたいで持ち越す

	for start := 0; start < l; start += c {
		end := start + c
		if end > l {
			end = l
		}
		qc, kc, vc := rows(q, start, end), rows(k, start, end), rows(v, start, end)

		// チャンクをまたぐぶんは、持ち越した状態から読む。
		inter := Mul(qc, state)
		// チャンクの中は全対のスコアで混ぜる。自分より後ろは見ない。
		sc := Mul(qc, kc.T())
		tril(&sc)
		intra := Mul(sc, vc)

		for i := start; i < end; i++ {
			for j := 0; j < d; j++ {
				out.Data[i*d+j] = inter.At(i-start, j) + intra.At(i-start, j)
			}
		}
		// 状態を更新して次のチャンクへ持ち越す。
		add(&state, Mul(kc.T(), vc))
	}
	return out
}

func rows(m Mat, from, to int) Mat {
	out := New(to-from, m.Cols)
	copy(out.Data, m.Data[from*m.Cols:to*m.Cols])
	return out
}

func add(dst *Mat, src Mat) {
	for i := range dst.Data {
		dst.Data[i] += src.Data[i]
	}
}

// #endregion chunked
