// Package gc は、mark-sweep 方式のガベージコレクタの核を Go でモデル化する。
// ランタイム内部編の1つ目のパーツ。
//
// GC が答える問いはただ1つ——「このオブジェクトはもう要らないか?」。判定基準は
// 到達可能性(reachability)だ。プログラムが今スタックやグローバルから辿れる
// オブジェクトだけが生きており、辿れなくなったものはゴミ(garbage)として回収できる。
// ここでは実メモリや unsafe を使わず、ヒープをオブジェクトの有向グラフとして
// 決定的にモデル化し、tricolor マーキングでその到達集合を求める。
package gc

// #region object

// color は tricolor(3色)マーキングの色。mark-sweep や並行 GC の土台となる考え方。
//
//	white = まだ到達していない(回収候補)
//	gray  = 到達したが、参照先(子)をまだ走査していない
//	black = 到達し、子も走査し終えた
type color int

const (
	white color = iota
	gray
	black
)

// String は色の表示名を返す(トレースやデモ用)。
func (c color) String() string {
	switch c {
	case white:
		return "white"
	case gray:
		return "gray"
	case black:
		return "black"
	}
	return "?"
}

// Object はヒープ上の1オブジェクト。refs は「このオブジェクトが指している」他
// オブジェクトの ID(構造体のフィールドに入ったポインタに相当)。col は GC 中の
// マーキング色で、平常時は white。
type Object struct {
	ID   int
	Name string
	refs []int
	col  color
}

// Refs はこのオブジェクトが参照する ID の一覧を返す(内部スライスは渡さない)。
func (o *Object) Refs() []int {
	out := make([]int, len(o.refs))
	copy(out, o.refs)
	return out
}

// #endregion object
