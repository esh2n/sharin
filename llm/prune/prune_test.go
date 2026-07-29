package prune

import (
	"math"
	"testing"

	"github.com/esh2n/sharin/llm/gptq"
)

var (
	gptqHessian = gptq.Hessian
	gptqInverse = gptq.Inverse
)

// rnd は整数の線形合同法から [-1, 1) を作る。
type rnd struct{ s uint64 }

func newRnd(seed uint64) *rnd { return &rnd{s: seed | 1} }
func (r *rnd) next() float64 {
	r.s = r.s*6364136223846793005 + 1442695040888963407
	return float64(int64(r.s>>40)-(1<<23)) / (1 << 23)
}

// inputs は次元ごとに振れ幅の違う入力を作る。
//
// 偶数番の次元は大きく振れ、奇数番はほとんど動かない。
// share が大きいほど、次元どうしが一緒に動く。
func inputs(d, n int, share float64) [][]float64 {
	r := newRnd(7)
	x := make([][]float64, d)
	for i := range x {
		x[i] = make([]float64, n)
	}
	for k := 0; k < n; k++ {
		base := r.next()
		for i := 0; i < d; i++ {
			scale := 1.0
			if i%2 == 1 {
				scale = 0.05 // ほとんど動かない次元
			}
			x[i][k] = scale * (share*base + (1-share)*r.next())
		}
	}
	return x
}

// weights は、振れ幅の大きい次元ほど小さい重みが乗るように作る。
//
// 絶対値だけを見ると、消すべきでないものが小さく見える。
func weights(d int) []float64 {
	r := newRnd(31)
	w := make([]float64, d)
	for i := range w {
		w[i] = r.next()
		if i%2 == 0 {
			w[i] *= 0.1 // よく振れる次元には小さい重み
		}
	}
	return w
}

// この章の中心その1。同じ絶対値でも、消す損は同じではない。
func TestSaliencyIsNotMagnitude(t *testing.T) {
	d, n := 32, 256
	x := inputs(d, n, 0.5)
	w := weights(d)
	hinv := Plan(x, 0.01)

	s := Saliency(w, hinv)

	// よく振れる次元(偶数)は重みが小さいが、消す損は大きい。
	// ほとんど動かない次元(奇数)は重みが大きいが、消しても効かない。
	var smallW, bigW, smallL, bigL float64
	for i := 0; i < d; i++ {
		if i%2 == 0 {
			smallW += math.Abs(w[i])
			smallL += s[i]
			continue
		}
		bigW += math.Abs(w[i])
		bigL += s[i]
	}
	if !(smallW < bigW) {
		t.Fatalf("よく振れる側の重みが小さくない: %g vs %g", smallW, bigW)
	}
	if !(smallL > bigL) {
		t.Fatalf("よく振れる側の損が大きくない: %g vs %g", smallL, bigL)
	}

	// だから、選ばれる集合が食い違う。
	mag := ByMagnitude(w, d/2)
	sal := BySaliencyOnly(w, hinv, d/2)
	same := 0
	for i := range w {
		if (mag[i] == 0) == (sal[i] == 0) {
			same++
		}
	}
	if same == d {
		t.Fatal("同じものを選んでいる")
	}
}

// 入力の振れ幅が揃っていて相関が無ければ、効きで選ぶのは大きさで選ぶのと同じになる。
func TestWithFlatInputsSaliencyIsMagnitude(t *testing.T) {
	d, n := 16, 512
	r := newRnd(5)
	x := make([][]float64, d)
	for i := range x {
		x[i] = make([]float64, n)
		for k := range x[i] {
			x[i][k] = r.next()
		}
	}
	w := make([]float64, d)
	for i := range w {
		w[i] = r.next()
	}
	hinv := Plan(x, 0.01)

	s := Saliency(w, hinv)
	// 損の順と絶対値の順が一致する。
	for i := 0; i < d; i++ {
		for j := 0; j < d; j++ {
			if (math.Abs(w[i]) < math.Abs(w[j])) != (s[i] < s[j]) {
				t.Fatalf("順が食い違う: |w%d|=%g |w%d|=%g / L=%g,%g",
					i, math.Abs(w[i]), j, math.Abs(w[j]), s[i], s[j])
			}
		}
	}
}

// 対角だけに制限すると、OBS の見積もりは OBD の見積もりと一致する。
func TestDiagonalSaliencyIsOBD(t *testing.T) {
	d, n := 16, 256
	x := inputs(d, n, 0.5)
	w := weights(d)

	full := gptqHessian(x, 0.01)
	diag := make([][]float64, d)
	for i := range diag {
		diag[i] = make([]float64, d)
		diag[i][i] = full[i][i]
	}
	inv := gptqInverse(diag)

	got := Saliency(w, inv)
	for i := range w {
		want := w[i] * w[i] * full[i][i] / 2 // OBD の見積もり
		if math.Abs(got[i]-want) > 1e-9*math.Max(1, math.Abs(want)) {
			t.Fatalf("%d: %g vs %g", i, got[i], want)
		}
	}
}

// この章の中心その2。選び方と補い方は対になっている。
//
// 補う前提の見積もりで選んでおいて補わないと、素朴な選び方より悪くなることがある。
func TestSaliencyWithoutCompensationCanBackfire(t *testing.T) {
	d, n := 64, 512
	w := weights(d)
	k := d / 2

	// 入力どうしがよく似て動く場合。
	x := inputs(d, n, 0.9)
	hinv := Plan(x, 0.01)
	mag := OutputError(w, ByMagnitude(w, k), x)
	sal := OutputError(w, BySaliencyOnly(w, hinv, k), x)
	obs := OutputError(w, BySaliency(w, hinv, k), x)

	// 補うところまでやれば、いちばん良い。
	if !(obs < mag) {
		t.Fatalf("補正まで入れても勝てない: %g vs %g", obs, mag)
	}
	// だが選び方だけ変えて補わないと、かえって悪くなる。
	if !(sal > mag) {
		t.Fatalf("裏目に出ていない: %g vs %g", sal, mag)
	}

	// 入力が似ていない場合は、選び方だけでも効く。
	flat := inputs(d, n, 0)
	fh := Plan(flat, 0.01)
	if !(OutputError(w, BySaliencyOnly(w, fh, k), flat) < OutputError(w, ByMagnitude(w, k), flat)) {
		t.Fatal("相関が無いときは選び方だけでも効くはず")
	}

	// どれも同じ数だけ消している。
	if Kept(ByMagnitude(w, k)) != d-k || Kept(BySaliency(w, hinv, k)) != d-k {
		t.Fatal("消した数が違う")
	}
}

// 補うところまで入れれば、どの疎さでも素朴な選び方に勝つ。
func TestCompensatedPruningAlwaysWins(t *testing.T) {
	d, n := 64, 512
	x := inputs(d, n, 0.7)
	w := weights(d)
	hinv := Plan(x, 0.01)

	for _, k := range []int{8, 16, 32, 48, 56} {
		mag := OutputError(w, ByMagnitude(w, k), x)
		obs := OutputError(w, BySaliency(w, hinv, k), x)
		if !(obs < mag) {
			t.Fatalf("%d 個消したときに負けた: %g vs %g", k, mag, obs)
		}
	}
}

// 補正の得は、入力の相関がそのまま決める。
func TestGainComesFromCorrelation(t *testing.T) {
	d, n := 64, 512
	w := weights(d)

	gain := func(share float64) float64 {
		x := inputs(d, n, share)
		hinv := Plan(x, 0.01)
		return OutputError(w, ByMagnitude(w, 32), x) / OutputError(w, BySaliency(w, hinv, 32), x)
	}
	g0, g3, g7, g9 := gain(0), gain(0.3), gain(0.7), gain(0.9)
	if !(g0 < g3 && g3 < g7 && g7 < g9) {
		t.Fatalf("相関が強いほど得が大きいはず: %g %g %g %g", g0, g3, g7, g9)
	}
}

// 全部消せば出力は 0 になる。端の振る舞い。
func TestPruneEverything(t *testing.T) {
	d, n := 8, 64
	x := inputs(d, n, 0.5)
	w := weights(d)
	hinv := Plan(x, 0.01)

	for _, got := range [][]float64{
		ByMagnitude(w, d),
		BySaliencyOnly(w, hinv, d),
		BySaliency(w, hinv, d),
	} {
		if Kept(got) != 0 {
			t.Fatalf("残っている: %v", got)
		}
	}
	// 数を超えて指定しても落ちない。
	if Kept(BySaliency(w, hinv, d+5)) != 0 {
		t.Fatal("超過指定で壊れた")
	}
	if Kept(ByMagnitude(w, d+5)) != 0 {
		t.Fatal("超過指定で壊れた")
	}
	// 1つも消さなければ元のまま。
	if Kept(BySaliency(w, hinv, 0)) != d {
		t.Fatal("消していないのに減った")
	}
}

// 補正は、消した重みを残りへ移す。だから残った重みは元とは変わる。
func TestCompensationMovesWeights(t *testing.T) {
	d, n := 32, 256
	x := inputs(d, n, 0.9)
	w := weights(d)
	hinv := Plan(x, 0.01)

	plain := BySaliencyOnly(w, hinv, 8)
	fixed := BySaliency(w, hinv, 8)

	moved := 0
	for i := range w {
		if plain[i] != 0 && plain[i] != fixed[i] {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("残った重みが動いていない")
	}
	// 消した側は両方とも 0 のまま。
	for i := range w {
		if plain[i] == 0 && fixed[i] != 0 {
			t.Fatalf("消したはずの %d が戻っている", i)
		}
	}
}

// 観測まわり。
func TestObservation(t *testing.T) {
	d, n := 8, 64
	x := inputs(d, n, 0.5)
	w := weights(d)
	hinv := Plan(x, 0.01)

	if len(Saliency(w, hinv)) != d {
		t.Fatal("形が違う")
	}
	if Kept(w) != d {
		t.Fatal("最初は全部生きている")
	}
	if OutputError(w, w, x) != 0 {
		t.Fatal("同じものでずれが出た")
	}
	// 対角が 0 以下の重みは選ばない(壊れた入力への備え)。
	broken := clone(hinv)
	for i := range broken {
		broken[i][i] = 0
	}
	if Kept(BySaliency(w, broken, 3)) != d {
		t.Fatal("選べないのに消した")
	}
	if got := smallest([]float64{3, 1, 2}, 10); len(got) != 3 {
		t.Fatal("超過指定で切り詰めていない")
	}
}
