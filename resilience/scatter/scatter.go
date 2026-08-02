// Package scatter は、同じ問い合わせを複数の相手へ配って答えを集める形を扱う。
// 全部揃うまで待つと、いちばん遅い1台が全体の時間を決める。
// その裾をどう切るかを、打ち切りと二重投げの2通りで測る。
package scatter

// 応答時間の作り。SlowEvery 回に 1 回だけ遅く、それ以外は速い。
// 実測ではなく、裾のある分布を決定的に作るための最小の模型になる。
const (
	Fast      = 10 // 速いときの下限
	FastWidth = 10 // 速いときの幅。Fast .. Fast+FastWidth-1
	Slow      = 200
	SlowEvery = 20
)

// lcg は整数の線形合同法。壁時計も乱数生成器も使わないので、
// 何度動かしても同じ結果になる。
func lcg(seed int) int {
	s := seed*6364136223846793005 + 1442695040888963407
	if s < 0 {
		s = -s
	}
	return s
}

// Took は node への attempt 本目が返るまでの時間を返す。
// 同じ台でも本数が違えば別の値になるので、投げ直しに意味が出る。
// #region took
func Took(node, attempt int) int {
	s := lcg(node*31 + attempt*7919)
	if s%SlowEvery == 0 {
		return Slow
	}
	return Fast + s%FastWidth
}

// #endregion took

// Result は 1 回の配りの結果。
type Result struct {
	Wait  int // 答えが揃うまでの時間
	Got   int // 集まった答えの数
	Sent  int // 投げた本数
	Slows int // 遅い応答に当たった台の数
}

// finishes は各台が返り終える時刻を並べる。
// hedgeAfter が 0 より大きいとき、その時刻までに返らない台へ 2 本目を投げる。
func finishes(n, hedgeAfter int) (done []int, sent, slows int) {
	done = make([]int, n)
	for i := range n {
		first := Took(i, 0)
		sent++
		if first >= Slow {
			slows++
		}
		if hedgeAfter > 0 && first > hedgeAfter {
			second := hedgeAfter + Took(i, 1)
			sent++
			if second < first {
				first = second
			}
		}
		done[i] = first
	}
	return done, sent, slows
}

// All は n 台へ配って、全部揃うまで待つ。
// 揃うのはいちばん遅い1台が返ったときなので、Wait は最大値になる。
// #region all
func All(n int) Result {
	done, sent, slows := finishes(n, 0)
	wait := 0
	for _, d := range done {
		if d > wait {
			wait = d
		}
	}
	return Result{Wait: wait, Got: n, Sent: sent, Slows: slows}
}

// #endregion all

// FirstK は n 台へ配って、k 台ぶん集まった時点で打ち切る。
// 残りの答えは捨てるので、Got は k で頭打ちになる。
// #region firstk
func FirstK(n, k int) Result {
	if k > n {
		k = n
	}
	done, sent, slows := finishes(n, 0)
	return Result{Wait: kth(done, k), Got: k, Sent: sent, Slows: slows}
}

// #endregion firstk

// Hedged は n 台へ配り、after を過ぎても返らない台へ 2 本目を投げる。
// 台ごとに早いほうを採るので、Sent は増えるが Wait は縮む。
// #region hedged
func Hedged(n, after int) Result {
	done, sent, slows := finishes(n, after)
	wait := 0
	for _, d := range done {
		if d > wait {
			wait = d
		}
	}
	return Result{Wait: wait, Got: n, Sent: sent, Slows: slows}
}

// #endregion hedged

// kth は k 番目に小さい値を返す。台数が小さいので素朴な選択で足りる。
func kth(xs []int, k int) int {
	c := make([]int, len(xs))
	copy(c, xs)
	for i := range k {
		min := i
		for j := i + 1; j < len(c); j++ {
			if c[j] < c[min] {
				min = j
			}
		}
		c[i], c[min] = c[min], c[i]
	}
	if k == 0 {
		return 0
	}
	return c[k-1]
}

// Tail は n 台のうち少なくとも 1 台が遅い応答に当たる割合を、
// 1 台あたりの割合 p(SlowEvery 分の 1)から計算する。
// 分数のままでは扱いにくいので、1万分率で返す。
// #region tail
func Tail(n int) int {
	const scale = 10000
	// (1 - 1/SlowEvery)^n を整数のまま計算する
	num, den := SlowEvery-1, SlowEvery
	allFastNum, allFastDen := 1, 1
	for range n {
		allFastNum *= num
		allFastDen *= den
		// 桁が溢れる前に約分の代わりに丸める
		for allFastDen > 1<<40 {
			allFastNum /= 1024
			allFastDen /= 1024
		}
	}
	return scale - allFastNum*scale/allFastDen
}

// #endregion tail
