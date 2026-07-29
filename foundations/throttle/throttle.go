// Package throttle は CPU スロットリング(cgroup v2 の cpu.max)の最小実装。
//
// メモリの上限は「超えたら断る」で済む。CPU は断れない。時間は流れ続けるので、
// 上限を超えた相手にできることは「次の期間まで走らせない」だけになる。
// これがスロットリングで、メモリの上限とは効き方がまるで違う。
//
// cpu.max は "quota period" の2つの数で書く。100000 のうち 50000 なら
// 「100ms ごとに 50ms ぶんだけ CPU を使ってよい」。期間の頭で使える量が戻り、
// 使い残しは(既定では)消える。つまり固定窓で、貯められない。
//
// ここでは実時間も乱数も使わない。tick を1つずつ進めるので、何回やっても同じ結果になる。
package throttle

// #region limit

// Limit は cpu.max。1期間 Period tick のうち、Quota tick ぶん CPU を使ってよい。
type Limit struct {
	Quota  int
	Period int
	// Burst は使い残しを次の期間へ繰り越せる上限(cgroup v2 の cpu.max.burst)。
	// 0 なら繰り越さない。これが既定で、期間の頭で使い残しは消える。
	Burst int
}

// Task は仕事。Arrive の時刻に現れて、Need tick ぶんの CPU を使うと終わる。
type Task struct {
	Name   string
	Arrive int
	Need   int
}

// Result は1つの仕事の結果。
type Result struct {
	Name   string
	Arrive int
	Done   int
	// Latency は現れてから終わるまでの時間。Need より長ければ、その差は待たされた時間になる。
	Latency int
	// Throttled は CPU を使いたかったのに、枠を使い切っていて止められた tick の数。
	Throttled int
}

// Period は1期間ぶんの内訳。
type Period struct {
	Start int
	// Used はこの期間に使った CPU tick。
	Used int
	// Stalled は「走りたい仕事が居たのに枠が無かった」壁時計の tick 数。
	Stalled int
	// Carried はこの期間の頭に繰り越されてきた量。
	Carried int
	// Exhausted は枠を使い切った時刻。使い切らなかったら -1。
	// 期間の頭からの距離が短いほど、残りを長く止まって過ごすことになる。
	Exhausted int
}

// #endregion limit

// #region run

// Run は決定的に模擬する。ncpu は同時に走らせられる本数。
//
// 1 tick ごとに、走れる仕事へ最大 ncpu 本まで CPU を配る。ただし
// その期間に残っている枠を超えては配らない。超えたぶんが止められた時間になる。
func Run(limit Limit, ncpu int, tasks []Task) ([]Result, []Period) {
	res := make([]Result, len(tasks))
	left := make([]int, len(tasks))
	for i, t := range tasks {
		res[i] = Result{Name: t.Name, Arrive: t.Arrive, Done: -1}
		left[i] = t.Need
	}

	var periods []Period
	remaining := 0 // この期間に残っている枠
	done := 0

	for t := 0; done < len(tasks); t++ {
		if t%limit.Period == 0 {
			// 期間の頭。使える量が戻る。
			// 直前の期間の使い残しは Burst までしか持ち込めない。既定の 0 なら消える。
			carry := 0
			if len(periods) > 0 {
				carry = min(remaining, limit.Burst)
			}
			remaining = limit.Quota + carry
			periods = append(periods, Period{Start: t, Carried: carry, Exhausted: -1})
		}
		p := &periods[len(periods)-1]

		ran := 0
		stalledHere := false
		for i := range tasks {
			if left[i] == 0 || tasks[i].Arrive > t {
				continue
			}
			if ran >= ncpu {
				break // CPU の本数が足りない。これは制限ではなく並列度の話
			}
			if remaining == 0 {
				// 枠を使い切っている。走りたいのに走れない。
				res[i].Throttled++
				stalledHere = true
				continue
			}
			remaining--
			p.Used++
			if remaining == 0 && p.Exhausted < 0 {
				p.Exhausted = t + 1
			}
			ran++
			left[i]--
			if left[i] == 0 {
				res[i].Done = t + 1
				res[i].Latency = t + 1 - tasks[i].Arrive
				done++
			}
		}
		if stalledHere {
			p.Stalled++
		}
	}
	return res, periods
}

// #endregion run

// #region stats

// TotalThrottled は止められた tick を全部足す。
func TotalThrottled(res []Result) int {
	n := 0
	for _, r := range res {
		n += r.Throttled
	}
	return n
}

// MaxLatency はいちばん待たされた仕事の所要時間を返す。
func MaxLatency(res []Result) int {
	m := 0
	for _, r := range res {
		if r.Latency > m {
			m = r.Latency
		}
	}
	return m
}

// StallRatio は「枠が無くて止まっていた期間の割合」を返す。
//
// 最後の期間は途中で終わるので、割合が実態からずれる。数えるのは
// 最後より前の、まるまる1期間ぶんだけにする。
func StallRatio(limit Limit, periods []Period) float64 {
	if len(periods) < 2 {
		return 0
	}
	stalled, total := 0, 0
	for _, p := range periods[:len(periods)-1] {
		stalled += p.Stalled
		total += limit.Period
	}
	return float64(stalled) / float64(total)
}

// #endregion stats
