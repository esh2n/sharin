package attention

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/tensor"
)

const eps = 1e-4

// attention の出力は入力と同じ形(系列長 × 次元)。
func TestSelfAttentionShape(t *testing.T) {
	x := tensor.FromRows([][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	})
	h := NewHead(4, 4) // dModel=4, dHead=4
	out := h.Forward(x, false)
	if out.Rows != 3 || out.Cols != 4 {
		t.Errorf("出力の形 = (%d,%d), want (3,4)", out.Rows, out.Cols)
	}
}

// attention の重み(各行)は確率分布(合計1)。
func TestAttentionWeightsAreProbabilities(t *testing.T) {
	x := tensor.FromRows([][]float32{{1, 2, 3, 4}, {5, 6, 7, 8}, {1, 1, 1, 1}})
	h := NewHead(4, 4)
	_, weights := h.forwardWithWeights(x, false)
	for r := 0; r < weights.Rows; r++ {
		var sum float32
		for c := 0; c < weights.Cols; c++ {
			sum += weights.At(r, c)
		}
		if math.Abs(float64(sum-1)) > eps {
			t.Errorf("重み行 %d の合計 = %v, want 1", r, sum)
		}
	}
}

// 因果マスク: GPT はトークン i がトークン j>i(未来)に注目してはいけない。
// マスクありなら、上三角(未来)の重みが 0 になる。
func TestCausalMaskBlocksFuture(t *testing.T) {
	x := tensor.FromRows([][]float32{{1, 0, 0, 0}, {0, 1, 0, 0}, {0, 0, 1, 0}})
	h := NewHead(4, 4)
	_, weights := h.forwardWithWeights(x, true) // causal=true

	for i := 0; i < weights.Rows; i++ {
		for j := i + 1; j < weights.Cols; j++ {
			if weights.At(i, j) > eps {
				t.Errorf("トークン %d が未来の %d に注目している(重み %v)", i, j, weights.At(i, j))
			}
		}
	}
	// 各行はマスク後も合計1(過去+自分だけで再正規化される)。
	for i := 0; i < weights.Rows; i++ {
		var sum float32
		for j := 0; j <= i; j++ {
			sum += weights.At(i, j)
		}
		if math.Abs(float64(sum-1)) > eps {
			t.Errorf("マスク後の行 %d の合計 = %v, want 1", i, sum)
		}
	}
}

// 最初のトークンは自分にしか注目できない(過去がないので重み1.0)。
func TestFirstTokenAttendsSelf(t *testing.T) {
	x := tensor.FromRows([][]float32{{1, 2, 3, 4}, {4, 3, 2, 1}})
	h := NewHead(4, 4)
	_, weights := h.forwardWithWeights(x, true)
	if math.Abs(float64(weights.At(0, 0)-1)) > eps {
		t.Errorf("トークン0の自己注目 = %v, want 1", weights.At(0, 0))
	}
}

func TestScaledBySqrtDHead(t *testing.T) {
	// スケーリング係数 1/sqrt(dHead) が効いていること。
	// 恒等重み(Wq=Wk=I)を注入して、スコアが素の内積/sqrt(d) になるか確認する。
	h := NewHeadIdentity(2)
	x := tensor.FromRows([][]float32{{3, 4}, {0, 0}})
	scores := h.rawScores(x)
	// score[0][0] = (3*3+4*4)/sqrt(2) = 25/1.41421 ≈ 17.6777
	want := float32(25.0 / math.Sqrt2)
	if math.Abs(float64(scores.At(0, 0)-want)) > 1e-2 {
		t.Errorf("scaled score = %v, want %v", scores.At(0, 0), want)
	}
}

// この章の中心その1。attention は並びを見ない。だから位置情報を別に足す必要がある。
func TestAttentionIsBlindToOrder(t *testing.T) {
	x := tensor.FromRows([][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0, 0, 0, 1},
	})
	// 行を入れ替えた入力。
	swapped := tensor.FromRows([][]float32{
		{0, 0, 1, 0},
		{0, 1, 0, 0},
		{1, 0, 0, 0},
		{0, 0, 0, 1},
	})
	perm := []int{2, 1, 0, 3}

	h := NewHead(4, 4)
	a := h.Forward(x, false)
	b := h.Forward(swapped, false)

	// 入れ替えた順に、出力の行もそのまま入れ替わる。中身は1つも変わらない。
	for i := 0; i < a.Rows; i++ {
		for c := 0; c < a.Cols; c++ {
			if math.Abs(float64(a.At(perm[i], c)-b.At(i, c))) > eps {
				t.Fatalf("(%d,%d): %g vs %g", i, c, a.At(perm[i], c), b.At(i, c))
			}
		}
	}
}

// この章の中心その2。マスクがあると、後ろのトークンを変えても前の出力は動かない。
func TestFutureCannotLeakBackwards(t *testing.T) {
	base := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0, 0, 0, 1},
	}
	changed := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{9, 9, 9, 9}, // 最後のトークンだけを大きく変える
	}
	h := NewHead(4, 4)

	withMask := [2]*tensor.Tensor{
		h.Forward(tensor.FromRows(base), true),
		h.Forward(tensor.FromRows(changed), true),
	}
	// 前の3行は1つも動かない。
	for i := 0; i < 3; i++ {
		for c := 0; c < withMask[0].Cols; c++ {
			if math.Abs(float64(withMask[0].At(i, c)-withMask[1].At(i, c))) > eps {
				t.Fatalf("マスクがあるのに漏れた (%d,%d)", i, c)
			}
		}
	}

	// マスクを外すと動く。未来が見えている。
	no := [2]*tensor.Tensor{
		h.Forward(tensor.FromRows(base), false),
		h.Forward(tensor.FromRows(changed), false),
	}
	moved := false
	for i := 0; i < 3; i++ {
		for c := 0; c < no[0].Cols; c++ {
			if math.Abs(float64(no[0].At(i, c)-no[1].At(i, c))) > eps {
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("マスクを外しても前の行が動かない")
	}
}

// √d で割らないと、次元が大きいほど注目が1点に尖る。
func TestScalingKeepsAttentionFromSpiking(t *testing.T) {
	peak := func(d int, scaled bool) float32 {
		x := tensor.New(6, d)
		// 整数の線形合同法から作る。浮動小数の漸化式は不動点に落ちて
		// 全要素がほぼ同じ値になり、何を測っているのか分からなくなる。
		var s uint64 = 7
		for i := range x.Data {
			s = s*6364136223846793005 + 1442695040888963407
			x.Data[i] = float32(int64(s>>40)-(1<<23)) / (1 << 23)
		}
		h := NewHead(d, d)
		w := tensor.SoftmaxRows(h.Scores(x, scaled))
		var m float32
		for c := 0; c < w.Cols; c++ {
			if w.At(0, c) > m {
				m = w.At(0, c)
			}
		}
		return m
	}
	// 割らないと、次元を上げるほど1つの相手に集中していく。
	if !(peak(64, false) > peak(8, false)) {
		t.Fatalf("尖っていない: %g → %g", peak(8, false), peak(64, false))
	}
	// 割ると、そこまで尖らない。
	if !(peak(64, true) < peak(64, false)) {
		t.Fatalf("割っても尖ったまま: %g vs %g", peak(64, true), peak(64, false))
	}
}

// 注目の重みは各行が確率になっている。
func TestWeightsAreProbabilities(t *testing.T) {
	x := tensor.FromRows([][]float32{{1, 0}, {0, 1}, {1, 1}})
	h := NewHead(2, 2)
	w := h.Weights(x, true)
	for r := 0; r < w.Rows; r++ {
		var sum float32
		for c := 0; c < w.Cols; c++ {
			sum += w.At(r, c)
		}
		if math.Abs(float64(sum-1)) > eps {
			t.Fatalf("行 %d の合計が %g", r, sum)
		}
		// マスクがあるので、未来には 0 しか配られない。
		for c := r + 1; c < w.Cols; c++ {
			if w.At(r, c) != 0 {
				t.Fatalf("未来に %g 配っている", w.At(r, c))
			}
		}
	}
}
