package tracesampling

import (
	"testing"
	"time"
)

// seqRng は決められた値を順に返す擬似乱数。使い切ったら最後の値を返し続ける。
func seqRng(values ...float64) func() float64 {
	i := 0
	return func() float64 {
		v := values[min(i, len(values)-1)]
		i++
		return v
	}
}

func TestHeadSampler(t *testing.T) {
	errTrace := Trace{ID: 1, Duration: 100 * time.Millisecond, Err: true}
	okTrace := Trace{ID: 2, Duration: 100 * time.Millisecond, Err: false}

	t.Run("判定は乱数とレートだけで決まる", func(t *testing.T) {
		s, err := NewHeadSampler(0.1, seqRng(0.05, 0.5))
		if err != nil {
			t.Fatal(err)
		}
		if !s.Keep(okTrace) {
			t.Error("rng=0.05 < rate=0.1 なので残すべき")
		}
		if s.Keep(okTrace) {
			t.Error("rng=0.5 >= rate=0.1 なので落とすべき")
		}
	})

	t.Run("エラートレースであっても関係なく落とす(headの本質)", func(t *testing.T) {
		s, err := NewHeadSampler(0.1, seqRng(0.9))
		if err != nil {
			t.Fatal(err)
		}
		if s.Keep(errTrace) {
			t.Error("head は開始時点で決めるので、後からエラーになるトレースも救えない")
		}
	})

	if _, err := NewHeadSampler(-0.1, nil); err == nil {
		t.Error("rate<0 はエラーになるべき")
	}
	if _, err := NewHeadSampler(1.1, nil); err == nil {
		t.Error("rate>1 はエラーになるべき")
	}
}

func TestTailSampler(t *testing.T) {
	s, err := NewTailSampler(500*time.Millisecond, 0.01, seqRng(0.9))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		trace Trace
		want  bool
	}{
		{"エラートレースは必ず残す", Trace{Err: true, Duration: 10 * time.Millisecond}, true},
		{"遅いトレースは必ず残す", Trace{Err: false, Duration: 800 * time.Millisecond}, true},
		{"普通のトレースはベースレートで落ちる(rng=0.9 >= 0.01)", Trace{Err: false, Duration: 100 * time.Millisecond}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Keep(tt.trace); got != tt.want {
				t.Errorf("Keep = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("普通のトレースもベースレートでは残る", func(t *testing.T) {
		s, _ := NewTailSampler(500*time.Millisecond, 0.5, seqRng(0.3))
		if !s.Keep(Trace{Duration: 100 * time.Millisecond}) {
			t.Error("rng=0.3 < baseRate=0.5 なので残すべき")
		}
	})

	if _, err := NewTailSampler(0, 0.01, nil); err == nil {
		t.Error("slowThreshold<=0 はエラーになるべき")
	}
	if _, err := NewTailSampler(time.Second, 1.5, nil); err == nil {
		t.Error("baseRate>1 はエラーになるべき")
	}
}

func TestEvaluate(t *testing.T) {
	// エラー2件を含む10件のトレース。
	traces := make([]Trace, 10)
	for i := range traces {
		traces[i] = Trace{ID: i, Duration: 100 * time.Millisecond, Err: i < 2}
	}

	t.Run("tail はエラー捕捉率100%", func(t *testing.T) {
		s, _ := NewTailSampler(time.Second, 0, seqRng(0.9)) // baseRate 0: エラー以外は全部落とす
		sum := Evaluate(traces, s)
		if sum.Total != 10 || sum.Errors != 2 {
			t.Fatalf("集計がおかしい: %+v", sum)
		}
		if sum.Kept != 2 || sum.ErrorsKept != 2 {
			t.Errorf("エラー2件だけが残るべき: %+v", sum)
		}
		if sum.ErrorCaptureRate() != 1.0 {
			t.Errorf("捕捉率 = %v, want 1.0", sum.ErrorCaptureRate())
		}
		if sum.KeepRatio() != 0.2 {
			t.Errorf("保存率 = %v, want 0.2", sum.KeepRatio())
		}
	})

	t.Run("head は10%サンプリングだとエラーもおよそ10%しか残らない", func(t *testing.T) {
		// 乱数を「エラー1件目だけ残ってエラー2件目は落ちる」列に固定する。
		s, _ := NewHeadSampler(0.1, seqRng(0.05, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9, 0.9))
		sum := Evaluate(traces, s)
		if sum.ErrorsKept != 1 {
			t.Errorf("ErrorsKept = %d, want 1", sum.ErrorsKept)
		}
		if sum.ErrorCaptureRate() != 0.5 {
			t.Errorf("捕捉率 = %v, want 0.5", sum.ErrorCaptureRate())
		}
	})

	t.Run("トレース0件でも0除算しない", func(t *testing.T) {
		s, _ := NewHeadSampler(0.1, seqRng(0.5))
		sum := Evaluate(nil, s)
		if sum.KeepRatio() != 0 || sum.ErrorCaptureRate() != 0 {
			t.Errorf("空入力は0を返すべき: %+v", sum)
		}
	})
}
