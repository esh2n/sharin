package patch

import "testing"

// 4×4 の 1 チャネル画像(値 = 行*10+列)。
func image4x4() [][]float64 {
	img := make([][]float64, 4)
	for r := 0; r < 4; r++ {
		img[r] = make([]float64, 4)
		for c := 0; c < 4; c++ {
			img[r][c] = float64(r*10 + c)
		}
	}
	return img
}

func TestPatchifyGrid(t *testing.T) {
	// 4×4 を 2×2 パッチに切ると、2×2 = 4 パッチ、各パッチ 4 要素。
	patches, err := Patchify(image4x4(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(patches) != 4 {
		t.Fatalf("num patches = %d, want 4", len(patches))
	}
	for i, p := range patches {
		if len(p) != 4 {
			t.Fatalf("patch %d has %d values, want 4", i, len(p))
		}
	}
	// 左上パッチ = 行0,1 × 列0,1 = [0,1,10,11](行優先で平坦化)。
	want := []float64{0, 1, 10, 11}
	for i := range want {
		if patches[0][i] != want[i] {
			t.Fatalf("top-left patch = %v, want %v", patches[0], want)
		}
	}
	// 右下パッチ = 行2,3 × 列2,3 = [22,23,32,33]。
	wantBR := []float64{22, 23, 32, 33}
	for i := range wantBR {
		if patches[3][i] != wantBR[i] {
			t.Fatalf("bottom-right patch = %v, want %v", patches[3], wantBR)
		}
	}
}

func TestPatchifyValidates(t *testing.T) {
	if _, err := Patchify(image4x4(), 3); err == nil {
		t.Fatal("non-divisible patch size should error")
	}
	if _, err := Patchify(image4x4(), 0); err == nil {
		t.Fatal("zero patch size should error")
	}
	if _, err := Patchify(nil, 2); err == nil {
		t.Fatal("empty image should error")
	}
}

func TestNumPatches(t *testing.T) {
	// 224×224 を 16×16 パッチに切ると (224/16)^2 = 196 パッチ。ViT の定番。
	if got := NumPatches(224, 16); got != 196 {
		t.Fatalf("num patches = %d, want 196", got)
	}
	if got := NumPatches(32, 8); got != 16 {
		t.Fatalf("num patches = %d, want 16", got)
	}
}

func TestProjectToTokens(t *testing.T) {
	// 各パッチを線形射影で dModel 次元のトークンに写す。
	// パッチ数 × dModel の形になり、位置埋め込みが足される。
	patches, _ := Patchify(image4x4(), 2)
	proj := NewProjector(4, 8, 42) // patchDim=4 → dModel=8
	tokens := proj.ToTokens(patches)
	if len(tokens) != 4 {
		t.Fatalf("token count = %d", len(tokens))
	}
	if len(tokens[0]) != 8 {
		t.Fatalf("token dim = %d, want 8", len(tokens[0]))
	}
	// 位置埋め込みで、同じパッチ内容でも位置が違えばトークンが変わる。
	same := [][]float64{{1, 1, 1, 1}, {1, 1, 1, 1}}
	toks := proj.ToTokens(same)
	allEqual := true
	for i := range toks[0] {
		if toks[0][i] != toks[1][i] {
			allEqual = false
		}
	}
	if allEqual {
		t.Fatal("position embedding should distinguish identical patches")
	}
}

func TestProjectorDeterministic(t *testing.T) {
	patches, _ := Patchify(image4x4(), 2)
	a := NewProjector(4, 8, 7).ToTokens(patches)
	b := NewProjector(4, 8, 7).ToTokens(patches)
	for i := range a {
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatal("same seed should give identical tokens")
			}
		}
	}
}

func TestTokenBudget(t *testing.T) {
	// マルチモーダルの費用 = トークン数。パッチが細かいほどトークンが増える。
	// 画像1枚(224², patch16)=196トークン。動画は フレーム数 × それ。
	img := NumPatches(224, 16)
	if img != 196 {
		t.Fatalf("image tokens = %d", img)
	}
	// 30fps × 60秒 の動画をフレームごとにパッチ化すると膨大になる。
	video := img * 30 * 60
	if video != 196*1800 {
		t.Fatalf("video tokens = %d", video)
	}
	// 小さいパッチ(8×8)は4倍のトークン = 4倍の計算。
	if NumPatches(224, 8) != img*4 {
		t.Fatalf("smaller patches should quadruple tokens")
	}
}
