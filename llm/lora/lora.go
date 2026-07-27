// Package lora は LoRA(low-rank adaptation)を最小構成で実装する。
//
// 大規模モデルを丸ごと微調整するには、全パラメータ分の勾配とオプティマイザ状態を
// 持つ必要があり、メモリが巨大になる。LoRA は元の重み行列 W を凍結したまま、
// その隣に小さな低ランク行列の積 A·B を足す。学習するのは A と B だけで、
// これは W 全体のごく一部(数百分の一)にすぎない。更新が低ランクで足りるのは、
// 微調整で必要な重みの変化が本質的に低次元に収まる、という観察に基づく。
// 学習後は A·B を W に足し込めば 1 枚の行列に戻り、推論時の追加コストは無い。
package lora

import (
	"errors"

	"github.com/esh2n/sharin/llm/tensor"
)

// #region lora

// LoRA は凍結した base 行列 (d×k) に、低ランク補正 A(d×r)·B(r×k) を足す層。
//
//	y = x·W + (alpha/r)·x·A·B     W は凍結、A・B だけ学習
//
// A は小さな乱数、B は 0 で初期化するので、学習開始時の補正は 0
// (= モデルの挙動を変えない)。
type LoRA struct {
	base  *tensor.Tensor // 凍結された元の重み (d×k)
	a     *tensor.Tensor // (d×r)
	b     *tensor.Tensor // (r×k)
	rank  int
	scale float32 // alpha/rank
}

// New は base の隣に rank 次元の LoRA を作る。alpha は補正の強さ。
// A は決定的な擬似乱数、B は 0 初期化(初期補正ゼロ)。
func New(base *tensor.Tensor, rank int, alpha float32) (*LoRA, error) {
	d, k := base.Rows, base.Cols
	if rank < 1 {
		return nil, errors.New("lora: rank must be >= 1")
	}
	if rank > d || rank > k {
		return nil, errors.New("lora: rank must not exceed matrix dimensions")
	}
	return &LoRA{
		base:  base,
		a:     randMatrix(d, rank, 42),
		b:     tensor.New(rank, k), // ゼロ初期化
		rank:  rank,
		scale: alpha / float32(rank),
	}, nil
}

// Forward は y = x·base + scale·(x·A·B)。base は変えず補正だけ足す。
func (l *LoRA) Forward(x *tensor.Tensor) *tensor.Tensor {
	out := tensor.MatMul(x, l.base)
	delta := tensor.MatMul(tensor.MatMul(x, l.a), l.b) // (x·A)·B
	for i := range out.Data {
		out.Data[i] += l.scale * delta.Data[i]
	}
	return out
}

// SetB は学習相当の更新をテストするため B を差し替える(通常は勾配で更新される)。
func (l *LoRA) SetB(b *tensor.Tensor) { l.b = b }

// #endregion lora

// #region merge

// Merge は base に scale·(A·B) を足し込んだ 1 枚の行列を返す。
// 学習後にこれを使えば、推論時は通常の行列 1 枚と同じで追加コストが無い。
func (l *LoRA) Merge() *tensor.Tensor {
	ab := tensor.MatMul(l.a, l.b) // (d×k)
	merged := tensor.New(l.base.Rows, l.base.Cols)
	for i := range merged.Data {
		merged.Data[i] = l.base.Data[i] + l.scale*ab.Data[i]
	}
	return merged
}

// #endregion merge

// #region params

// TrainableParams は学習対象(A + B)のパラメータ数。base は含まない。
func (l *LoRA) TrainableParams() int {
	return l.a.Rows*l.a.Cols + l.b.Rows*l.b.Cols
}

// BaseParams は凍結された base のパラメータ数(参考: フル微調整で学習する量)。
func BaseParams(base *tensor.Tensor) int { return base.Rows * base.Cols }

// #endregion params

// randMatrix は seed から決定的に小さな値を敷いた行列を作る。
func randMatrix(rows, cols int, seed uint64) *tensor.Tensor {
	out := tensor.New(rows, cols)
	s := seed*6364136223846793005 + 1442695040888963407
	for i := range out.Data {
		s = s*6364136223846793005 + 1442695040888963407
		out.Data[i] = (float32(s>>40)/float32(1<<24) - 0.5) * 0.02
	}
	return out
}
