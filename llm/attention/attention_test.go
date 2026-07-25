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
