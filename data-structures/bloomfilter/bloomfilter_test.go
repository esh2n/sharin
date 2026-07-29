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

// この章の中心その2。設計した精度が、実測とほぼ一致する。
func TestDesignedRateMatchesMeasured(t *testing.T) {
	const n = 10000
	for _, want := range []float64{0.1, 0.01, 0.001} {
		f := New(n, want)
		for i := 0; i < n; i++ {
			f.Add("key" + itoa(i))
		}
		// 入れていない 100000 件で試す。
		hits := 0
		const trials = 100000
		for i := 0; i < trials; i++ {
			if f.MayContain("miss" + itoa(i)) {
				hits++
			}
		}
		got := float64(hits) / trials
		if got > want*2 {
			t.Fatalf("設計 %.4f に対して実測 %.4f", want, got)
		}
		// ビットの詰まり具合から出した見込みとも合う。
		est := f.EstimatedRate()
		if est > want*3 || (got > 0 && est < got/3) {
			t.Fatalf("見込み %.4f が実測 %.4f と合わない(設計 %.4f)", est, got, want)
		}
	}
}

// この章の中心その3。入れすぎると効かなくなり、しかも黙って効かなくなる。
func TestOverfillingSilentlyBreaksIt(t *testing.T) {
	const n = 1000
	f := New(n, 0.01)

	rate := func() float64 {
		hits := 0
		const trials = 50000
		for i := 0; i < trials; i++ {
			if f.MayContain("miss" + itoa(i)) {
				hits++
			}
		}
		return float64(hits) / trials
	}

	for i := 0; i < n; i++ {
		f.Add("key" + itoa(i))
	}
	asDesigned := rate()
	if asDesigned > 0.02 {
		t.Fatalf("設計どおり入れて %.4f", asDesigned)
	}

	// 5倍まで入れる。エラーも警告も出ない。
	for i := n; i < 5*n; i++ {
		f.Add("key" + itoa(i))
	}
	overfilled := rate()
	if overfilled < 0.5 {
		t.Fatalf("5倍入れても %.4f しか悪化しない", overfilled)
	}
	// 入れた件数を数えれば分かるし、ビットの詰まり具合からも分かる。
	if f.Added() != 5*n {
		t.Fatalf("数えられていない: %d", f.Added())
	}
	if f.FillRatio() < 0.9 {
		t.Fatalf("詰まり具合 %.3f", f.FillRatio())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// 作れない設定は最初に止める。
func TestNewRejectsBadParameters(t *testing.T) {
	for _, c := range []struct {
		n int
		p float64
	}{{0, 0.01}, {-1, 0.01}, {100, 0}, {100, 1}, {100, 1.5}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("n=%d p=%g で止まらない", c.n, c.p)
				}
			}()
			New(c.n, c.p)
		}()
	}
	// 精度をゆるくすると、ハッシュは最低1本まで減る。
	if f := New(1, 0.9); f.Hashes() < 1 {
		t.Fatalf("ハッシュ数 %d", f.Hashes())
	}
}
