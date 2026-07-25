package sampling

import (
	"errors"
	"math"
	"sort"
)

// #region temperature
// ApplyTemperature は logits を t で割る。softmax の直前に掛かる唯一のスカラー操作で、
// t < 1 は logits の差を拡大して分布を尖らせ(確定的寄り)、
// t > 1 は差を縮小して分布を平らにする(多様性寄り)。
// t = 0 は0除算になるため受け付けない。決定的にしたければ Greedy を使う。
func ApplyTemperature(logits []float64, t float64) ([]float64, error) {
	if t <= 0 {
		return nil, errors.New("sampling: temperature must be positive (use Greedy for t=0)")
	}
	out := make([]float64, len(logits))
	for i, l := range logits {
		out[i] = l / t
	}
	return out, nil
}

// #endregion temperature

// filterByKeep は keep[i] が false のトークンの logit を -Inf にした新しいスライスを返す。
// -Inf は softmax を通すと確率0になるので「候補から外す」ことと等価。
func filterByKeep(logits []float64, keep []bool) []float64 {
	out := make([]float64, len(logits))
	for i, l := range logits {
		if keep[i] {
			out[i] = l
		} else {
			out[i] = math.Inf(-1)
		}
	}
	return out
}

// #region topk
// FilterTopK は logit の大きい上位 k 個だけを残し、他を -Inf にする。
func FilterTopK(logits []float64, k int) ([]float64, error) {
	if k <= 0 {
		return nil, errors.New("sampling: k must be positive")
	}
	if k >= len(logits) {
		return append([]float64(nil), logits...), nil
	}

	order := argsortDesc(logits)
	keep := make([]bool, len(logits))
	for _, idx := range order[:k] {
		keep[idx] = true
	}
	return filterByKeep(logits, keep), nil
}

// #endregion topk

// #region topp
// FilterTopP(nucleus sampling)は確率の大きい順に足していき、
// 累積が p 以上になる最小の集合だけを残す。
// top-k と違い「残る個数」が分布の形に応じて変わるのが特徴。
func FilterTopP(logits []float64, p float64) ([]float64, error) {
	if p <= 0 || p > 1 {
		return nil, errors.New("sampling: p must be in (0, 1]")
	}
	probs, err := Softmax(logits)
	if err != nil {
		return nil, err
	}

	order := argsortDesc(probs)
	keep := make([]bool, len(logits))
	cum := 0.0
	for _, idx := range order {
		keep[idx] = true // 累積が p を超える境界のトークンまでは含める
		cum += probs[idx]
		if cum >= p {
			break
		}
	}
	return filterByKeep(logits, keep), nil
}

// #endregion topp

// #region minp
// FilterMinP は「最大確率の minP 倍」を閾値として、それ未満のトークンを切る。
// 分布が尖っているときは厳しく、平らなときは緩く切れる相対的なフィルタで、
// top-p の「累積」ではなく「個々の確率」で判定するのが違い。
func FilterMinP(logits []float64, minP float64) ([]float64, error) {
	if minP < 0 || minP > 1 {
		return nil, errors.New("sampling: minP must be in [0, 1]")
	}
	probs, err := Softmax(logits)
	if err != nil {
		return nil, err
	}

	maxProb := 0.0
	for _, p := range probs {
		maxProb = math.Max(maxProb, p)
	}
	threshold := minP * maxProb

	keep := make([]bool, len(logits))
	for i, p := range probs {
		keep[i] = p >= threshold
	}
	return filterByKeep(logits, keep), nil
}

// #endregion minp

// argsortDesc は values を降順に並べたときのインデックス列を返す。values は変更しない。
func argsortDesc(values []float64) []int {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return values[order[a]] > values[order[b]]
	})
	return order
}
