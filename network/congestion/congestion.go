// Package congestion は TCP の輻輳制御を最小構成で実装する。
//
// ネットワークの帯域は共有資源だ。送り手が一斉に全力で送れば経路が詰まり、
// パケットが溢れて捨てられ、みなが再送してさらに詰まる(輻輳崩壊)。かといって
// 誰も全体の空き帯域を知らない。TCP は各接続が自分で探る。輻輳ウィンドウ
// (cwnd)を、まずスロースタートで指数的に増やして手早く空きを探り、閾値
// (ssthresh)からは輻輳回避に切り替えて 1 往復に 1 ずつ慎重に増やす。パケットが
// 落ちたら「詰まった」合図とみなし、ウィンドウを半分に切る。この「線形に増やし、
// 詰まったら掛け算で減らす(AIMD)」が要で、中央の調整役なしに、各接続が帯域を
// 効率よく使い、しかも競合する接続どうしが公平な取り分へ収束する。
package congestion

// #region controller

// State は輻輳制御の局面。
type State int

const (
	SlowStart           State = iota // 指数的に増やして空きを素早く探る
	CongestionAvoidance              // 1 往復に 1 ずつ慎重に増やす
)

func (s State) String() string {
	if s == SlowStart {
		return "slow-start"
	}
	return "congestion-avoidance"
}

// Controller は 1 接続の輻輳ウィンドウを管理する。単位は MSS(最大セグメント長)。
type Controller struct {
	Cwnd     float64 // 輻輳ウィンドウ(一度に送ってよい量)
	Ssthresh float64 // スロースタートから輻輳回避へ移る閾値
	State    State
}

// New は cwnd=1 のスロースタートで始める。
func New(ssthresh float64) *Controller {
	if ssthresh < 1 {
		ssthresh = 1
	}
	return &Controller{Cwnd: 1, Ssthresh: ssthresh, State: SlowStart}
}

// OnRoundACKed は 1 往復ぶんの ACK が返ったときの増やし方。
//   - スロースタート: cwnd を倍にする(各パケットの ACK で +1 = 1 往復で倍)
//   - 輻輳回避: cwnd を 1 だけ増やす(加算増加)
func (c *Controller) OnRoundACKed() {
	if c.State == SlowStart {
		c.Cwnd *= 2
		if c.Cwnd >= c.Ssthresh {
			c.Cwnd = c.Ssthresh
			c.State = CongestionAvoidance
		}
	} else {
		c.Cwnd += 1 // 加算増加(additive increase)
	}
}

// OnLoss は重複 ACK による損失(fast retransmit)。ssthresh を今の半分にし、
// cwnd もそこまで落として輻輳回避を続ける(乗算減少)。
func (c *Controller) OnLoss() {
	c.Ssthresh = c.Cwnd / 2 // 乗算減少(multiplicative decrease)
	if c.Ssthresh < 1 {
		c.Ssthresh = 1
	}
	c.Cwnd = c.Ssthresh
	c.State = CongestionAvoidance
}

// OnTimeout はタイムアウト(より深刻な損失)。cwnd を 1 に戻し、スロースタートから
// やり直す。ssthresh は半分に。
func (c *Controller) OnTimeout() {
	c.Ssthresh = c.Cwnd / 2
	if c.Ssthresh < 1 {
		c.Ssthresh = 1
	}
	c.Cwnd = 1
	c.State = SlowStart
}

// #endregion controller

// #region sawtooth

// Simulate は容量 capacity の経路で rounds 往復ぶん送ったときの cwnd の推移を返す。
// cwnd が容量を超えると損失が起き(OnLoss)、半分に切られる。増やしては切られる
// のこぎり波(sawtooth)になり、cwnd が容量の周りを探り続ける。
func Simulate(ssthresh, capacity float64, rounds int) []float64 {
	c := New(ssthresh)
	history := make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		if c.Cwnd > capacity {
			c.OnLoss() // 経路が溢れた
		} else {
			c.OnRoundACKed()
		}
		history = append(history, c.Cwnd)
	}
	return history
}

// #endregion sawtooth

// #region fairness

// SimulateFairness は容量 capacity を 2 接続で分け合うときの各ウィンドウの推移を返す。
// 両者とも毎往復 1 ずつ増やし(加算増加)、合計が容量を超えたら両者とも半分に切る
// (乗算減少)。加算増加は差を保ち、乗算減少は差を半分にするので、取り分は
// 等しい方へ収束する。これが AIMD が公平をもたらす仕組み。
func SimulateFairness(capacity, a, b float64, rounds int) (histA, histB []float64) {
	histA = make([]float64, 0, rounds)
	histB = make([]float64, 0, rounds)
	for i := 0; i < rounds; i++ {
		a += 1 // 加算増加(両者同じだけ増える)
		b += 1
		if a+b > capacity {
			a /= 2 // 乗算減少(両者とも半分)
			b /= 2
		}
		histA = append(histA, a)
		histB = append(histB, b)
	}
	return histA, histB
}

// #endregion fairness
