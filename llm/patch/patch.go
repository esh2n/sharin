// Package patch は画像をトークン列に変える入口(パッチ化)を最小構成で実装する。
//
// gemini-lineage 編で見たとおり、Transformer は入力の種類を区別せず、
// トークンの列でありさえすれば処理できる。画像をトークンにする標準の方法が
// パッチ化で、画像を格子状の小さなパッチ(例 16×16 ピクセル)に切り、各パッチを
// 1 つのトークンとして線形射影する(ViT の方式)。テキストの BPE に対応する、
// 画像側のトークナイザにあたる。パッチが細かいほどトークンが増え、それが
// マルチモーダルの計算コストを決める。
package patch

import "errors"

// #region patchify

// Patchify は画像(行×列の 2 次元)を size×size の正方パッチに切り、
// 各パッチを行優先で平坦化したベクトルの列にする。
// 画像の縦横が size で割り切れる前提。
func Patchify(img [][]float64, size int) ([][]float64, error) {
	if len(img) == 0 || len(img[0]) == 0 {
		return nil, errors.New("patch: empty image")
	}
	h, w := len(img), len(img[0])
	if size <= 0 || h%size != 0 || w%size != 0 {
		return nil, errors.New("patch: image dimensions must be divisible by patch size")
	}
	var patches [][]float64
	for pr := 0; pr < h; pr += size {
		for pc := 0; pc < w; pc += size {
			patch := make([]float64, 0, size*size)
			for r := pr; r < pr+size; r++ {
				for c := pc; c < pc+size; c++ {
					patch = append(patch, img[r][c])
				}
			}
			patches = append(patches, patch)
		}
	}
	return patches, nil
}

// NumPatches は sizeXsize パッチで縦横 dim の画像を切ったときのパッチ数。
// 224×224 を 16×16 で切ると (224/16)² = 196。トークン数の見積もりに使う。
func NumPatches(dim, size int) int {
	n := dim / size
	return n * n
}

// #endregion patchify

// #region project

// Projector は平坦化したパッチを dModel 次元のトークンへ線形射影し、
// 位置埋め込みを足す。テキストの埋め込み表に対応する画像側の入口。
type Projector struct {
	weight [][]float64 // (patchDim × dModel)
	dModel int
	seed   uint64
}

// NewProjector は patchDim 次元のパッチを dModel 次元へ写す射影を作る。
// 重みは決定的な擬似乱数。
func NewProjector(patchDim, dModel int, seed uint64) *Projector {
	w := make([][]float64, patchDim)
	s := seed*6364136223846793005 + 1442695040888963407
	for i := range w {
		w[i] = make([]float64, dModel)
		for j := range w[i] {
			s = s*6364136223846793005 + 1442695040888963407
			w[i][j] = float64(s>>40)/float64(1<<24) - 0.5
		}
	}
	return &Projector{weight: w, dModel: dModel, seed: seed}
}

// ToTokens は各パッチを線形射影し、位置埋め込みを足したトークン列を返す。
// 位置埋め込みにより、同じ内容のパッチでも位置が違えば別トークンになる。
func (p *Projector) ToTokens(patches [][]float64) [][]float64 {
	tokens := make([][]float64, len(patches))
	for i, patch := range patches {
		tok := make([]float64, p.dModel)
		for j := 0; j < p.dModel; j++ {
			sum := 0.0
			for k := range patch {
				sum += patch[k] * p.weight[k][j]
			}
			tok[j] = sum + p.positionEmb(i, j)
		}
		tokens[i] = tok
	}
	return tokens
}

// positionEmb は位置 i の次元 j の位置埋め込み(決定的な小さな値)。
func (p *Projector) positionEmb(i, j int) float64 {
	s := (uint64(i+1)*911 + uint64(j+1)*2749 + p.seed) * 6364136223846793005
	return float64(s>>40)/float64(1<<24)*0.2 - 0.1
}

// #endregion project
