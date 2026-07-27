package lora

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/tensor"
)

func input(rows, cols int, seed float64) *tensor.Tensor {
	x := tensor.New(rows, cols)
	v := seed
	for i := range x.Data {
		v = math.Mod(v*7.13+0.37, 1.0)
		x.Data[i] = float32(v*2 - 1)
	}
	return x
}

func maxAbsDiff(a, b *tensor.Tensor) float32 {
	m := float32(0)
	for i := range a.Data {
		if d := float32(math.Abs(float64(a.Data[i] - b.Data[i]))); d > m {
			m = d
		}
	}
	return m
}

func TestNewValidates(t *testing.T) {
	base := tensor.New(8, 16)
	if _, err := New(base, 0, 1); err == nil {
		t.Fatal("rank 0 should be rejected")
	}
	if _, err := New(base, 9, 1); err == nil {
		t.Fatal("rank > min(dim) should be rejected")
	}
	if _, err := New(base, 4, 1); err != nil {
		t.Fatalf("valid rank rejected: %v", err)
	}
}

func TestInitIsIdentityToBase(t *testing.T) {
	// 初期化時は B=0 なので、LoRA の寄与は 0。出力は base だけと一致する。
	// これが「学習開始時にモデルの挙動を変えない」という LoRA の要件。
	base := input(8, 16, 0.3)
	l, _ := New(base, 4, 2.0)
	x := input(5, 8, 0.7)
	got := l.Forward(x)
	want := tensor.MatMul(x, base)
	if d := maxAbsDiff(got, want); d > 1e-6 {
		t.Fatalf("init LoRA should equal base, diff=%g", d)
	}
}

func TestParamCount(t *testing.T) {
	// LoRA のパラメータは 2 枚の小行列 A(d×r)+B(r×k) だけ。
	// base 全体(d×k)に比べて r が小さいほど激減する。
	base := tensor.New(4096, 4096)
	l, _ := New(base, 8, 1)
	full := 4096 * 4096
	want := 4096*8 + 8*4096 // A + B
	if l.TrainableParams() != want {
		t.Fatalf("trainable = %d, want %d", l.TrainableParams(), want)
	}
	// 学習対象は全体の 0.5% 未満。
	if float64(l.TrainableParams())/float64(full) > 0.005 {
		t.Fatalf("LoRA should train <0.5%%, got %f", float64(l.TrainableParams())/float64(full))
	}
	if BaseParams(base) != full {
		t.Fatalf("base params = %d", BaseParams(base))
	}
}

func TestUpdateChangesOutput(t *testing.T) {
	// B に非ゼロを入れると出力が base から動く(= 学習が効く経路がある)。
	base := input(8, 16, 0.3)
	l, _ := New(base, 4, 2.0)
	x := input(5, 8, 0.7)
	before := l.Forward(x)
	l.SetB(input(4, 16, 0.9))
	after := l.Forward(x)
	if maxAbsDiff(before, after) < 1e-3 {
		t.Fatal("nonzero B should change output")
	}
}

func TestScalingByAlpha(t *testing.T) {
	// LoRA の寄与は alpha/rank でスケールされる。
	// alpha を 2 倍にすると、base からのズレも 2 倍になる。
	base := input(8, 16, 0.3)
	x := input(5, 8, 0.7)
	bmat := input(4, 16, 0.9)

	l1, _ := New(base, 4, 2.0)
	l1.SetB(bmat)
	l2, _ := New(base, 4, 4.0)
	l2.SetB(bmat)

	baseOut := tensor.MatMul(x, base)
	d1 := maxAbsDiff(l1.Forward(x), baseOut)
	d2 := maxAbsDiff(l2.Forward(x), baseOut)
	if math.Abs(float64(d2/d1)-2.0) > 1e-4 {
		t.Fatalf("doubling alpha should double delta, ratio=%g", d2/d1)
	}
}

func TestMergeEqualsForward(t *testing.T) {
	// 学習後、A·B を base に足し込んで 1 枚の行列にしても、出力は変わらない。
	// これが「推論時は追加コストゼロ」の根拠。
	base := input(8, 16, 0.3)
	l, _ := New(base, 4, 2.0)
	l.SetB(input(4, 16, 0.9))
	x := input(5, 8, 0.7)

	forward := l.Forward(x)
	merged := l.Merge()
	mergedOut := tensor.MatMul(x, merged)
	if d := maxAbsDiff(forward, mergedOut); d > 1e-5 {
		t.Fatalf("merged should equal forward, diff=%g", d)
	}
	// merge 後の行列サイズは base と同じ(追加パラメータなし)。
	if merged.Rows != base.Rows || merged.Cols != base.Cols {
		t.Fatalf("merged shape = (%d,%d)", merged.Rows, merged.Cols)
	}
}
