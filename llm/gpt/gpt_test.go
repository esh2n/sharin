package gpt

import (
	"testing"
)

func newTestModel() *Model {
	return New(Config{
		VocabSize: 16,
		DModel:    8,
		NHeads:    2,
		NLayers:   2,
		DFF:       16,
		MaxSeq:    8,
		Seed:      1,
	})
}

// forward pass はトークン列を受けて「各位置での次トークンの logits」を返す。
// 形は (系列長, 語彙数)。
func TestForwardShape(t *testing.T) {
	m := newTestModel()
	logits := m.Forward([]int{1, 2, 3})
	if logits.Rows != 3 || logits.Cols != 16 {
		t.Errorf("logits の形 = (%d,%d), want (3,16)", logits.Rows, logits.Cols)
	}
}

// 決定的: 同じ入力・同じ重みなら同じ logits(推論は再現可能)。
func TestForwardDeterministic(t *testing.T) {
	m := newTestModel()
	a := m.Forward([]int{1, 2, 3})
	b := m.Forward([]int{1, 2, 3})
	for i := range a.Data {
		if a.Data[i] != b.Data[i] {
			t.Fatal("同じ入力で logits が変わった(非決定的)")
		}
	}
}

// Generate は greedy に次トークンを1つずつ足していく(自己回帰生成)。
func TestGenerateLength(t *testing.T) {
	m := newTestModel()
	out := m.Generate([]int{1}, 5)
	if len(out) != 6 { // 元の1 + 生成5
		t.Errorf("生成後の長さ = %d, want 6", len(out))
	}
	if out[0] != 1 {
		t.Error("プロンプトが保持されていない")
	}
	for _, tok := range out {
		if tok < 0 || tok >= 16 {
			t.Errorf("語彙範囲外のトークン: %d", tok)
		}
	}
}

// 因果性: 位置 i の logits は、それより後ろのトークンに依存しない。
// 末尾トークンを変えても、それ以前の位置の logits は変わらないはず。
func TestCausality(t *testing.T) {
	m := newTestModel()
	a := m.Forward([]int{1, 2, 3, 4})
	b := m.Forward([]int{1, 2, 3, 9}) // 末尾だけ変える

	// 位置 0,1,2 の logits は一致するべき(末尾の変更が過去に漏れない)。
	for pos := 0; pos < 3; pos++ {
		for c := 0; c < a.Cols; c++ {
			if abs(a.At(pos, c)-b.At(pos, c)) > 1e-4 {
				t.Fatalf("位置 %d の logits が末尾変更で動いた(因果性違反)", pos)
			}
		}
	}
}

func TestConfigValidation(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("DModel が NHeads で割り切れない設定は panic すべき")
		}
	}()
	New(Config{VocabSize: 16, DModel: 7, NHeads: 2, NLayers: 1, DFF: 8, MaxSeq: 8})
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
