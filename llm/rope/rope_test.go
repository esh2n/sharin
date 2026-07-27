package rope

import (
	"math"
	"testing"
)

// 決定的な擬似ベクトル(テスト用)。
func vec(dim int, seed float64) []float64 {
	out := make([]float64, dim)
	x := seed
	for i := range out {
		x = math.Mod(x*7.13+0.37, 1.0)
		out[i] = x*2 - 1
	}
	return out
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func norm(a []float64) float64 { return math.Sqrt(dot(a, a)) }

func TestNewRejectsOddDim(t *testing.T) {
	if _, err := New(7); err == nil {
		t.Fatal("odd dim should be rejected")
	}
	if _, err := New(0); err == nil {
		t.Fatal("zero dim should be rejected")
	}
}

func TestApplyPosZeroIsIdentity(t *testing.T) {
	r, _ := New(8)
	x := vec(8, 0.3)
	got := r.Apply(x, 0)
	for i := range x {
		if math.Abs(got[i]-x[i]) > 1e-12 {
			t.Fatalf("pos 0 should be identity: %v vs %v", got, x)
		}
	}
}

func TestApplyPreservesNorm(t *testing.T) {
	// 回転は直交変換なので、どの位置でもベクトルの長さを変えない。
	r, _ := New(16)
	x := vec(16, 0.7)
	for _, pos := range []int{1, 5, 100, 10000} {
		if got, want := norm(r.Apply(x, pos)), norm(x); math.Abs(got-want) > 1e-9 {
			t.Fatalf("norm changed at pos %d: %f vs %f", pos, got, want)
		}
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	r, _ := New(4)
	x := vec(4, 0.5)
	orig := append([]float64(nil), x...)
	r.Apply(x, 3)
	for i := range x {
		if x[i] != orig[i] {
			t.Fatal("Apply must not mutate its input")
		}
	}
}

func TestScoreDependsOnlyOnOffset(t *testing.T) {
	// RoPE の核心: 回転後の内積は相対位置 (m-n) だけで決まる。
	// (q を位置 m、k を位置 n に置いた内積) と (両方を s だけずらした内積) が一致する。
	r, _ := New(32)
	q := vec(32, 0.11)
	k := vec(32, 0.83)
	base := dot(r.Apply(q, 9), r.Apply(k, 5)) // offset 4
	for _, shift := range []int{1, 10, 500} {
		got := dot(r.Apply(q, 9+shift), r.Apply(k, 5+shift))
		if math.Abs(got-base) > 1e-9 {
			t.Fatalf("score changed under shift %d: %f vs %f", shift, got, base)
		}
	}
	// 逆に offset が変われば内積も変わる(自明な恒等でないことの確認)。
	other := dot(r.Apply(q, 9), r.Apply(k, 8)) // offset 1
	if math.Abs(other-base) < 1e-9 {
		t.Fatalf("different offsets should give different scores: %f", base)
	}
}

func TestInterpolationCompressesPositions(t *testing.T) {
	// 位置補間: 位置を factor で割って学習済みレンジに押し込む。
	// factor=4 なら位置 8 は位置 2 と同じ回転になる。
	r, _ := New(8)
	x := vec(8, 0.42)
	a := r.ApplyInterpolated(x, 8, 4)
	b := r.Apply(x, 2)
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			t.Fatalf("interpolated pos 8 (factor 4) should equal pos 2: %v vs %v", a, b)
		}
	}
	// factor 1 は通常の Apply と同じ。
	c := r.ApplyInterpolated(x, 5, 1)
	d := r.Apply(x, 5)
	for i := range c {
		if math.Abs(c[i]-d[i]) > 1e-12 {
			t.Fatal("factor 1 should equal Apply")
		}
	}
}

func TestFrequencySpectrum(t *testing.T) {
	// 先頭ペアほど高周波(1 位置で大きく回る)、末尾ペアほど低周波(ほぼ回らない)。
	r, _ := New(64)
	if !(r.freqs[0] > r.freqs[len(r.freqs)-1]) {
		t.Fatalf("freqs should decrease: %v", r.freqs)
	}
	if math.Abs(r.freqs[0]-1.0) > 1e-12 {
		t.Fatalf("first freq should be 1.0, got %f", r.freqs[0])
	}
}

func TestSinusoidal(t *testing.T) {
	pe := Sinusoidal(4, 6)
	if len(pe) != 4 || len(pe[0]) != 6 {
		t.Fatalf("shape = %dx%d", len(pe), len(pe[0]))
	}
	// 位置 0 は sin(0)=0, cos(0)=1 の交互列。
	for i := 0; i < 6; i += 2 {
		if pe[0][i] != 0 || pe[0][i+1] != 1 {
			t.Fatalf("pos 0 row = %v", pe[0])
		}
	}
	// 値域は [-1, 1]。異なる位置は異なるパターン。
	same := true
	for p := 0; p < 4; p++ {
		for i := 0; i < 6; i++ {
			if pe[p][i] < -1 || pe[p][i] > 1 {
				t.Fatalf("out of range: %f", pe[p][i])
			}
			if pe[p][i] != pe[0][i] {
				same = false
			}
		}
	}
	if same {
		t.Fatal("all positions identical")
	}
}
