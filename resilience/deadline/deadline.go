// Package deadline は、呼び出しが何段も連なるときの締め切りの扱いを扱う。
// 段ごとに勝手に再送すると、いちばん下への呼び出し回数は掛け算で増える。
// 締め切りを下へ渡すかどうかで、増え方と無駄仕事の量がどう変わるかを測る。
package deadline

// Policy は締め切りの渡し方。
type Policy int

const (
	// None は締め切りを置かない。段ごとに決めた回数だけ試し切る。
	None Policy = iota
	// Each は段ごとに同じ長さの締め切りを別々に持つ。
	// 下の段は自分が呼ばれた時刻から測り直すので、上より遅くまで働く。
	Each
	// Pass は入口の締め切りをそのまま下へ渡す。全段が同じ時刻で止まる。
	Pass
)

// Chain は呼び出しの連なりの形。
// #region chain
type Chain struct {
	Hops   int    // 段数。入口を 1 段目とし、いちばん下が末端になる
	Tries  int    // 1 段が下へ試す回数
	Cost   int    // 末端が 1 回の呼び出しに使う時間
	Budget int    // 締め切りまでの時間
	Fails  int    // 末端が最初の何回を失敗するか
	Policy Policy // 締め切りの渡し方
}

// #endregion chain

// Result は 1 回走らせた結果。
type Result struct {
	Leaf    int  // 末端が呼ばれた回数
	Total   int  // 全段あわせた呼び出し回数
	Elapsed int  // 経過した時間
	Wasted  int  // 入口が諦めたあとも末端が使い続けた時間
	OK      bool // 入口から見て成功したか
}

type sim struct {
	c    Chain
	now  int
	left int // 末端があと何回失敗するか
	res  Result
}

// call は depth 段目の呼び出し。until はこの段が守る締め切りの時刻。
// 0 は締め切り無しを表す。
func (s *sim) call(depth, until int) bool {
	if depth == s.c.Hops-1 {
		return s.leaf()
	}
	for range s.c.Tries {
		// 次の 1 回を始める前に、締め切りに間に合うかを見る。
		// 間に合わないなら、始めずに諦める。
		if until > 0 && s.now+s.c.Cost > until {
			return false
		}
		s.res.Total++
		if s.call(depth+1, s.childUntil(until)) {
			return true
		}
	}
	return false
}

// childUntil は下の段へ渡す締め切りを決める。ここが方針の違いそのものになる。
// #region child
func (s *sim) childUntil(until int) int {
	switch s.c.Policy {
	case Pass:
		return until // 入口の時刻をそのまま渡す
	case Each:
		return s.now + s.c.Budget // 自分が呼ばれた時刻から測り直す
	default:
		return 0 // 締め切り無し
	}
}

// #endregion child

// leaf は末端の 1 回。時間を使い、決めた回数までは失敗する。
func (s *sim) leaf() bool {
	s.res.Leaf++
	s.res.Total++
	// 入口が既に諦めている時刻に始まった仕事は、誰も待っていない
	if s.c.Budget > 0 && s.c.Policy != None && s.now >= s.c.Budget {
		s.res.Wasted += s.c.Cost
	}
	s.now += s.c.Cost
	if s.left > 0 {
		s.left--
		return false
	}
	return true
}

// Run は連なりを 1 回走らせる。
// #region run
func Run(c Chain) Result {
	if c.Hops < 1 || c.Tries < 1 {
		return Result{}
	}
	s := &sim{c: c, left: c.Fails}
	until := 0
	if c.Policy != None {
		until = c.Budget
	}
	s.res.OK = s.call(0, until)
	s.res.Elapsed = s.now
	return s.res
}

// #endregion run
