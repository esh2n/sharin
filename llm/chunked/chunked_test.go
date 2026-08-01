package chunked

import "testing"

// 4 トークン、次元 2 の小さな例。整数なので丸めが入らない。
func fixture() (q, k, v Mat) {
	q = New(4, 2, 1, 2, 0, 1, 3, 1, 2, 2)
	k = New(4, 2, 2, 1, 1, 3, 0, 2, 1, 1)
	v = New(4, 2, 1, 0, 0, 1, 2, 1, 1, 2)
	return
}

// この章の中心その1。softmax が無ければ、掛ける順を変えても同じ値になる。
func TestOrderDoesNotChangeTheResult(t *testing.T) {
	q, k, v := fixture()

	a := ScoresFirst(q, k, v) // (q kᵀ) v ← attention の順
	b := StateFirst(q, k, v)  // q (kᵀ v) ← 線形の順

	t.Logf("(q kᵀ) v = %v", a.Data)
	t.Logf("q (kᵀ v) = %v", b.Data)
	if !Equal(a, b) {
		t.Fatalf("結合則が成り立っていない: %v vs %v", a.Data, b.Data)
	}

	// 途中に現れるものの大きさが違う。ここが分かれ目になる。
	scores := Mul(q, k.T())
	state := State(k, v)
	t.Logf("途中の大きさ  スコア %d×%d(系列長の二乗) / 状態 %d×%d(次元だけ)",
		scores.Rows, scores.Cols, state.Rows, state.Cols)
	if scores.Rows != 4 || scores.Cols != 4 {
		t.Fatalf("スコアの形が違う: %d×%d", scores.Rows, scores.Cols)
	}
	if state.Rows != 2 || state.Cols != 2 {
		t.Fatalf("状態の形が違う: %d×%d", state.Rows, state.Cols)
	}
}

// 状態の大きさは系列長に依らない。KV キャッシュは比例して伸びる。
func TestStateDoesNotGrowWithLength(t *testing.T) {
	const d = 64
	t.Logf("%-10s %14s %14s", "系列長", "状態(d×d)", "KVキャッシュ")
	for _, n := range []int{128, 1024, 8192, 65536} {
		t.Logf("%-10d %14d %14d", n, StateSize(d), CacheSize(n, d))
	}
	// 系列長を 512 倍にしても状態は変わらない。
	if StateSize(d) != StateSize(d) || StateSize(d) != 4096 {
		t.Fatal("状態が系列長に依存している")
	}
	// KV キャッシュは比例する。
	if CacheSize(8192, d) != CacheSize(128, d)*64 {
		t.Fatal("キャッシュが比例していない")
	}
	// 短いうちは状態のほうが大きい。長くなると逆転する。
	if !(CacheSize(16, d) < StateSize(d) && CacheSize(1024, d) > StateSize(d)) {
		t.Fatal("逆転が起きていない")
	}
}

// この章の中心その2。チャンクの大きさを両端に振ると、既知の 2 つに一致する。
func TestChunkSizeInterpolatesBetweenTheTwo(t *testing.T) {
	q, k, v := fixture()

	full := CausalScoresFirst(q, k, v)
	linear := CausalStateFirst(q, k, v)

	t.Logf("因果 attention   %v", full.Data)
	t.Logf("因果 線形        %v", linear.Data)
	if !Equal(full, linear) {
		t.Fatalf("両端が一致しない: %v vs %v", full.Data, linear.Data)
	}

	// C = 系列長 → 全体が 1 チャンク → 通常の attention と同じ
	if got := Chunked(q, k, v, 4); !Equal(got, full) {
		t.Fatalf("C=L で attention に一致しない: %v vs %v", got.Data, full.Data)
	}
	// C = 1 → チャンクの中に自分しかいない → 線形と同じ
	if got := Chunked(q, k, v, 1); !Equal(got, linear) {
		t.Fatalf("C=1 で線形に一致しない: %v vs %v", got.Data, linear.Data)
	}
	// 間の値でも、結果は同じ。変わるのは計算の配分だけになる。
	for _, c := range []int{1, 2, 3, 4} {
		got := Chunked(q, k, v, c)
		t.Logf("C=%d → %v", c, got.Data)
		if !Equal(got, full) {
			t.Fatalf("C=%d で値が変わった: %v", c, got.Data)
		}
	}
}

// この章の中心その3。C を動かすと計算量が固定部と比例部に分かれる。
func TestFlopsSplitIntoFixedAndGrowing(t *testing.T) {
	const L, D = 1024, 64
	t.Logf("系列長 %d / 次元 %d", L, D)
	t.Logf("%-8s %14s %14s %14s %12s", "C", "状態(固定)", "スコア(比例)", "合計", "中間の面積")

	var totals []int
	for _, c := range []int{1, 32, 64, 128, 256, 1024} {
		s := Shape{L: L, D: D, C: c}
		totals = append(totals, s.Flops())
		t.Logf("%-8d %14d %14d %14d %12d",
			c, s.StateFlops(), s.ScoreFlops(), s.Flops(), s.IntraMemory())
	}

	// 固定部は C を変えても動かない。
	a := Shape{L: L, D: D, C: 1}
	b := Shape{L: L, D: D, C: 1024}
	if a.StateFlops() != b.StateFlops() {
		t.Fatalf("固定部が動いている: %d %d", a.StateFlops(), b.StateFlops())
	}
	// 比例部は C に正比例する。
	if b.ScoreFlops() != a.ScoreFlops()*1024 {
		t.Fatalf("比例していない: %d %d", a.ScoreFlops(), b.ScoreFlops())
	}
	// 合計は C について単調増加。
	for i := 1; i < len(totals); i++ {
		if totals[i] <= totals[i-1] {
			t.Fatalf("単調でない: %v", totals)
		}
	}
	// 両端の比。C=L は C=1 の 16 倍を超える。
	ratio := b.Flops() / a.Flops()
	t.Logf("C=%d は C=1 の %d 倍", b.C, ratio)
	if ratio < 16 {
		t.Fatalf("差が出ていない: %d 倍", ratio)
	}
	// 端の判定。
	if !b.IsFullAttention() || !a.IsLinear() {
		t.Fatal("両端の判定が違う")
	}
}

// C=L のとき、比例部が系列長の二乗になる。
func TestFullAttentionIsQuadratic(t *testing.T) {
	const D = 64
	t.Logf("%-8s %16s %16s", "系列長", "C=L の合計", "C=64 の合計")
	var full, chunked []int
	for _, l := range []int{256, 512, 1024, 2048} {
		f := Shape{L: l, D: D, C: l}.Flops()
		c := Shape{L: l, D: D, C: 64}.Flops()
		full = append(full, f)
		chunked = append(chunked, c)
		t.Logf("%-8d %16d %16d", l, f, c)
	}
	// C=L は系列長を 2 倍にすると 4 倍に近づく(二乗の項が支配する)。
	last := full[len(full)-1] / full[len(full)-2]
	if last < 3 {
		t.Fatalf("二乗になっていない: %d 倍", last)
	}
	// C 固定なら 2 倍のまま(線形)。
	lin := chunked[len(chunked)-1] / chunked[len(chunked)-2]
	if lin != 2 {
		t.Fatalf("線形になっていない: %d 倍", lin)
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	q, k, v := fixture()
	// C が 0 以下でも 1 として扱う。
	if !Equal(Chunked(q, k, v, 0), CausalStateFirst(q, k, v)) {
		t.Fatal("C=0 の扱いが違う")
	}
	// C が系列長より大きくても全体 1 チャンク。
	if !Equal(Chunked(q, k, v, 99), CausalScoresFirst(q, k, v)) {
		t.Fatal("C>L の扱いが違う")
	}
	// 割り切れないチャンク。
	if !Equal(Chunked(q, k, v, 3), CausalScoresFirst(q, k, v)) {
		t.Fatal("割り切れないと値が変わる")
	}
	// 形が合わない積は止める。
	defer func() {
		if recover() == nil {
			t.Fatal("形が合わないのに通った")
		}
	}()
	Mul(New(2, 3), New(2, 3))
}
