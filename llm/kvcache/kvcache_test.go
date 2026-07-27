package kvcache

import (
	"reflect"
	"testing"
)

func TestCacheAppend(t *testing.T) {
	c := &Cache{}
	if c.Len() != 0 {
		t.Fatal("new cache should be empty")
	}
	c.Append(KV{K: []float64{1}, V: []float64{2}})
	c.Append(KV{K: []float64{3}, V: []float64{4}})
	if c.Len() != 2 {
		t.Fatalf("len = %d", c.Len())
	}
}

func TestGenerateWithAndWithoutCacheAgree(t *testing.T) {
	// 同じモデル・同じプロンプトなら、キャッシュの有無で生成列は 1 トークンも変わらない。
	m := NewModel(16, 8)
	prompt := []int{3, 1, 4}
	a, _ := m.GenerateNoCache(prompt, 10)
	b, _ := m.GenerateWithCache(prompt, 10)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("outputs differ:\nnocache %v\ncache   %v", a, b)
	}
	if len(a) != len(prompt)+10 {
		t.Fatalf("length = %d", len(a))
	}
}

func TestCacheReducesProjectionsToLinear(t *testing.T) {
	// K/V 射影の回数: キャッシュ無しは毎ステップ全位置を作り直すので二次、
	// キャッシュ有りは新トークンぶんだけなので線形。
	m := NewModel(16, 8)
	prompt := []int{3, 1, 4}
	n := 20
	_, opsNo := m.GenerateNoCache(prompt, n)
	_, opsWith := m.GenerateWithCache(prompt, n)

	// キャッシュ有り: 各位置の K/V を一度だけ作る。プロンプト 3 + 生成 19
	// (最後の生成トークンは文脈として使われないので射影不要)。
	if opsWith != len(prompt)+n-1 {
		t.Fatalf("cached ops = %d, want %d", opsWith, len(prompt)+n-1)
	}
	// キャッシュ無し: ステップ k で文脈(プロンプト + 生成済み k-1)を全部作り直す。
	want := 0
	for step := 1; step <= n; step++ {
		want += len(prompt) + step - 1
	}
	if opsNo != want {
		t.Fatalf("nocache ops = %d, want %d", opsNo, want)
	}
	if opsNo <= opsWith*5 {
		t.Fatalf("quadratic vs linear gap too small: %d vs %d", opsNo, opsWith)
	}
}

// --- speculative decoding ---

// 決定的な「本命モデル」: 直近 2 トークンから次を決める。
func target(prefix []int) int {
	last := prefix[len(prefix)-1]
	prev := 0
	if len(prefix) >= 2 {
		prev = prefix[len(prefix)-2]
	}
	return (last*3 + prev + 1) % 32
}

func TestSpeculativeMatchesTargetExactly(t *testing.T) {
	prompt := []int{5, 9}
	want := GenerateGreedy(target, prompt, 24)

	drafts := map[string]func([]int) int{
		"perfect": target,                                              // 常に一致
		"half":    func(p []int) int { return target(p) &^ 1 },         // 偶数化: 半分ほど一致
		"wrong":   func(p []int) int { return (p[len(p)-1] + 7) % 32 }, // ほぼ不一致
	}
	for name, draft := range drafts {
		got, _ := Speculative(target, draft, prompt, 24, 4)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s draft: output differs\ngot  %v\nwant %v", name, got, want)
		}
	}
}

func TestSpeculativeSavesTargetPasses(t *testing.T) {
	prompt := []int{5, 9}
	n := 24

	// 完全一致のドラフトなら、1 パスで γ+1 トークン進む。
	_, passesPerfect := Speculative(target, target, prompt, n, 4)
	if passesPerfect >= n {
		t.Fatalf("perfect draft should need far fewer passes than tokens: %d", passesPerfect)
	}
	// 常に外すドラフトでも、1 パスにつき最低 1 トークンは進む(本命の訂正)。
	wrong := func(p []int) int { return (p[len(p)-1] + 7) % 32 }
	_, passesWrong := Speculative(target, wrong, prompt, n, 4)
	if passesWrong > n {
		t.Fatalf("even a bad draft must not need more passes than tokens: %d", passesWrong)
	}
	if passesPerfect >= passesWrong {
		t.Fatalf("better draft should save passes: perfect=%d wrong=%d", passesPerfect, passesWrong)
	}
}

func TestSpeculativeGammaValidation(t *testing.T) {
	if _, p := Speculative(target, target, []int{1}, 5, 0); p != 0 {
		t.Fatal("gamma<1 should generate nothing")
	}
}
