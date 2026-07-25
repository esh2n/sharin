package sampling

import (
	"math"
	"testing"
)

func TestApplyTemperature(t *testing.T) {
	logits := []float64{2, 1, 0}

	t.Run("t=1は恒等変換", func(t *testing.T) {
		got, err := ApplyTemperature(logits, 1)
		if err != nil || !almostEqual(got, logits) {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("t<1は差を拡大し分布を尖らせる", func(t *testing.T) {
		cold, _ := ApplyTemperature(logits, 0.5)
		pCold, _ := Softmax(cold)
		pBase, _ := Softmax(logits)
		if pCold[0] <= pBase[0] {
			t.Errorf("低温で先頭の確率が上がるべき: %v vs %v", pCold[0], pBase[0])
		}
	})

	t.Run("t>1は分布を一様に近づける", func(t *testing.T) {
		hot, _ := ApplyTemperature(logits, 10)
		pHot, _ := Softmax(hot)
		pBase, _ := Softmax(logits)
		if pHot[0] >= pBase[0] {
			t.Errorf("高温で先頭の確率が下がるべき: %v vs %v", pHot[0], pBase[0])
		}
	})

	t.Run("入力を破壊しない", func(t *testing.T) {
		before := append([]float64(nil), logits...)
		if _, err := ApplyTemperature(logits, 0.5); err != nil {
			t.Fatal(err)
		}
		if !almostEqual(logits, before) {
			t.Error("入力logitsが書き換えられている")
		}
	})

	if _, err := ApplyTemperature(logits, 0); err == nil {
		t.Error("t=0はエラーになるべき(greedyを使う)")
	}
}

func TestFilterTopK(t *testing.T) {
	logits := []float64{3, 1, 2, 0}

	got, err := FilterTopK(logits, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{3, math.Inf(-1), 2, math.Inf(-1)}
	if !almostEqual(got, want) {
		t.Errorf("FilterTopK(k=2) = %v, want %v", got, want)
	}

	t.Run("kが語彙数以上なら何も切らない", func(t *testing.T) {
		got, _ := FilterTopK(logits, 10)
		if !almostEqual(got, logits) {
			t.Errorf("got %v", got)
		}
	})

	if _, err := FilterTopK(logits, 0); err == nil {
		t.Error("k=0はエラーになるべき")
	}
}

func TestFilterTopP(t *testing.T) {
	// softmax([log4, log3, log2, log1]) = [0.4, 0.3, 0.2, 0.1]
	logits := []float64{math.Log(4), math.Log(3), math.Log(2), math.Log(1)}

	tests := []struct {
		name string
		p    float64
		keep []bool
	}{
		{"p=1.0は全部残す", 1.0, []bool{true, true, true, true}},
		{"p=0.7はちょうど累積0.7で切る", 0.7, []bool{true, true, false, false}},
		{"p=0.65は累積が0.65を超えるまで含める", 0.65, []bool{true, true, false, false}},
		{"p=0.3は最上位1つでも超えるが必ず1つは残す", 0.3, []bool{true, false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterTopP(logits, tt.p)
			if err != nil {
				t.Fatal(err)
			}
			for i, keep := range tt.keep {
				gotKeep := !math.IsInf(got[i], -1)
				if gotKeep != keep {
					t.Errorf("token %d: keep=%v, want %v (got=%v)", i, gotKeep, keep, got)
				}
			}
		})
	}

	if _, err := FilterTopP(logits, 0); err == nil {
		t.Error("p=0はエラーになるべき")
	}
	if _, err := FilterTopP(logits, 1.1); err == nil {
		t.Error("p>1はエラーになるべき")
	}
}

func TestFilterMinP(t *testing.T) {
	// probs = [0.4, 0.3, 0.2, 0.1], maxProb = 0.4
	logits := []float64{math.Log(4), math.Log(3), math.Log(2), math.Log(1)}

	// minP=0.6 → 閾値 0.24 → 0.4, 0.3 が残る
	got, err := FilterMinP(logits, 0.6)
	if err != nil {
		t.Fatal(err)
	}
	wantKeep := []bool{true, true, false, false}
	for i, keep := range wantKeep {
		gotKeep := !math.IsInf(got[i], -1)
		if gotKeep != keep {
			t.Errorf("token %d: keep=%v, want %v", i, gotKeep, keep)
		}
	}

	if _, err := FilterMinP(logits, -0.1); err == nil {
		t.Error("minP<0はエラーになるべき")
	}
	if _, err := FilterMinP(logits, 1.1); err == nil {
		t.Error("minP>1はエラーになるべき")
	}
}

// 全フィルタ共通: フィルタ後に Softmax を通すと確率の合計は1に戻る(renormalize)。
func TestFilterThenSoftmaxSumsToOne(t *testing.T) {
	logits := []float64{3, 1, 2, 0, -1}
	filters := map[string]func() ([]float64, error){
		"topk": func() ([]float64, error) { return FilterTopK(logits, 2) },
		"topp": func() ([]float64, error) { return FilterTopP(logits, 0.5) },
		"minp": func() ([]float64, error) { return FilterMinP(logits, 0.3) },
	}
	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			filtered, err := f()
			if err != nil {
				t.Fatal(err)
			}
			probs, err := Softmax(filtered)
			if err != nil {
				t.Fatal(err)
			}
			sum := 0.0
			for _, p := range probs {
				sum += p
			}
			if math.Abs(sum-1) > eps {
				t.Errorf("確率の合計 = %v, want 1", sum)
			}
		})
	}
}
