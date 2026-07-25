// Package sampling は LLM の出力層で行われるサンプリングの最小実装。
//
// モデルが出すのは「次のトークンの確からしさ」を表す logits(実数の列)であり、
// 生成とは logits を確率分布に変換して1トークン抽選する行為の繰り返しにすぎない。
// このパッケージはその変換(softmax)、分布の変形(temperature)、
// 尻尾の切り落とし(top-k / top-p / min-p)、抽選(sample)を個別の関数として提供する。
package sampling

import "errors"

import "math"

// #region softmax
// Softmax は logits を確率分布に変換する。
// exp の overflow を防ぐため、全要素から最大値を引いてから計算する(数値安定化)。
// 定数を引いても exp の比は変わらないので、結果の分布は同じ。
func Softmax(logits []float64) ([]float64, error) {
	if len(logits) == 0 {
		return nil, errors.New("sampling: logits must not be empty")
	}
	maxLogit := math.Inf(-1)
	for _, l := range logits {
		maxLogit = math.Max(maxLogit, l)
	}

	exps := make([]float64, len(logits))
	sum := 0.0
	for i, l := range logits {
		exps[i] = math.Exp(l - maxLogit)
		sum += exps[i]
	}

	probs := make([]float64, len(logits))
	for i, e := range exps {
		probs[i] = e / sum
	}
	return probs, nil
}

// #endregion softmax

// #region greedy
// Greedy は最も logit の大きいトークンを選ぶ。temperature → 0 の極限と同じ。
// 常に同じ入力から同じ出力が出る(決定的)。
func Greedy(logits []float64) (int, error) {
	if len(logits) == 0 {
		return 0, errors.New("sampling: logits must not be empty")
	}
	best := 0
	for i, l := range logits {
		if l > logits[best] {
			best = i
		}
	}
	return best, nil
}

// #endregion greedy

// #region sample
// Sample は確率分布から1トークン抽選する(inverse CDF法)。
// rng は [0,1) の乱数を返す関数。累積確率が乱数を超えた位置のトークンを返す。
// 浮動小数点誤差で累積が1に届かない場合に備え、最後のトークンにフォールバックする。
func Sample(probs []float64, rng func() float64) (int, error) {
	if len(probs) == 0 {
		return 0, errors.New("sampling: probs must not be empty")
	}
	if rng == nil {
		return 0, errors.New("sampling: rng must not be nil")
	}

	r := rng()
	cum := 0.0
	for i, p := range probs {
		cum += p
		if r < cum {
			return i, nil
		}
	}
	return len(probs) - 1, nil
}

// #endregion sample
