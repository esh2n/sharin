package bloomfilter

import (
	"fmt"
	"testing"
)

func TestNoFalseNegatives(t *testing.T) {
	// bloom filter の絶対的な保証: 入れたものは必ず「あるかも」と答える。
	// 「ない」と答えたら、それは確実にない(偽陰性は起きない)。
	bf := New(1000, 0.01)
	added := make([]string, 500)
	for i := range added {
		added[i] = fmt.Sprintf("key-%d", i)
		bf.Add(added[i])
	}
	for _, k := range added {
		if !bf.MayContain(k) {
			t.Fatalf("入れた %q に対して MayContain が false を返した(偽陰性はあってはならない)", k)
		}
	}
}

func TestDefinitelyAbsent(t *testing.T) {
	bf := New(1000, 0.01)
	bf.Add("apple")
	bf.Add("banana")
	// 入れていないキーの大半は false のはず(たまに偽陽性で true はありうる)。
	falseCount := 0
	total := 1000
	for i := 0; i < total; i++ {
		if !bf.MayContain(fmt.Sprintf("absent-%d", i)) {
			falseCount++
		}
	}
	if falseCount == 0 {
		t.Error("入れていないキーが全部 true になっている(フィルタが機能していない)")
	}
}

func TestFalsePositiveRateNearTarget(t *testing.T) {
	target := 0.01
	n := 10000
	bf := New(n, target)
	for i := 0; i < n; i++ {
		bf.Add(fmt.Sprintf("in-%d", i))
	}

	falsePos := 0
	trials := 100000
	for i := 0; i < trials; i++ {
		if bf.MayContain(fmt.Sprintf("out-%d", i)) {
			falsePos++
		}
	}
	rate := float64(falsePos) / float64(trials)
	// 実測偽陽性率が目標の3倍以内に収まっていればパラメータ計算は妥当。
	if rate > target*3 {
		t.Errorf("偽陽性率 = %.4f, 目標 %.4f に対して高すぎる", rate, target)
	}
}

func TestParamsSanity(t *testing.T) {
	bf := New(1000, 0.01)
	// 目安: n=1000, p=0.01 なら m≈9585bit, k≈7。桁が合っていることを確認。
	if bf.bits() < 8000 || bf.bits() > 12000 {
		t.Errorf("ビット数 = %d, 想定(約9600)から外れている", bf.bits())
	}
	if bf.hashes() < 5 || bf.hashes() > 9 {
		t.Errorf("ハッシュ関数の数 = %d, 想定(約7)から外れている", bf.hashes())
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		n int
		p float64
	}{
		{0, 0.01},
		{100, 0},
		{100, 1},
		{100, -0.1},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%d, %v) は panic すべき", tc.n, tc.p)
				}
			}()
			New(tc.n, tc.p)
		}()
	}
}
