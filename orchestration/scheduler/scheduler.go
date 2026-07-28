// Package scheduler は Kubernetes のスケジューラを最小構成で実装する。
//
// 前章の調整ループは「Pod を 3 個作れ」までを決めた。だが作られた Pod は
// まだどこにも置かれていない。Pending のまま宙に浮いている。それをどの
// ノードに載せるかを決めるのがスケジューラだ。
//
// 決め方は 2 段階に分かれている。まず filter で、置けないノードを落とす。
// CPU やメモリの空きが足りない、汚れ(taint)を許容していない、といった
// 条件を満たさないノードは候補から外れる。ここは可否の判定であり、
// 通るか落ちるかしかない。次に score で、残った候補に点をつけて順位を
// つける。どこにでも置けるとき、どこが「より良い」かを決める段だ。
// 点の付け方を変えるだけで、負荷を散らす配置にも、詰め込む配置にも
// なる。可否と優劣を分けることが、この設計の肝になっている。
package scheduler

import "sort"

// #region types

// Resources は CPU(ミリコア)とメモリ(MiB)の量を表す。
type Resources struct {
	CPU int
	Mem int
}

// add は 2 つの量を足した新しい値を返す(元は変えない)。
func (r Resources) add(o Resources) Resources {
	return Resources{CPU: r.CPU + o.CPU, Mem: r.Mem + o.Mem}
}

// fitsIn は自分が空き容量 free の中に収まるかを返す。
func (r Resources) fitsIn(free Resources) bool {
	return r.CPU <= free.CPU && r.Mem <= free.Mem
}

// Taint はノードに付ける「汚れ」。これを許容する Pod しか置けない。
type Taint struct {
	Key   string
	Value string
}

// Node は Pod を載せる 1 台のマシン。
type Node struct {
	Name   string
	Cap    Resources // 総容量
	used   Resources // すでに載っている Pod の要求の合計
	taints []Taint
	pods   []string
}

// NewNode は容量 capacity のノードを作る。
func NewNode(name string, capacity Resources) *Node {
	return &Node{Name: name, Cap: capacity}
}

// Taint はノードに汚れを付け、そのノード自身を返す(組み立て用)。
func (n *Node) Taint(key, value string) *Node {
	n.taints = append(n.taints, Taint{Key: key, Value: value})
	return n
}

// Used は現在の使用量を返す。
func (n *Node) Used() Resources { return n.used }

// Free は空き容量を返す。
func (n *Node) Free() Resources {
	return Resources{CPU: n.Cap.CPU - n.used.CPU, Mem: n.Cap.Mem - n.used.Mem}
}

// Pods は載っている Pod 名を配置順に返す。
func (n *Node) Pods() []string { return append([]string(nil), n.pods...) }

// Pod は置きたいワークロード。Req が「これだけ要る」という要求(request)。
type Pod struct {
	Name        string
	Req         Resources
	Tolerations []Taint // 許容する汚れ
}

// tolerates は Pod が汚れ t を許容するかを返す。
func (p Pod) tolerates(t Taint) bool {
	for _, tol := range p.Tolerations {
		if tol == t {
			return true
		}
	}
	return false
}

// #endregion types

// #region filter

// Verdict は 1 つのノードに対する filter の判定。落ちた理由も残す。
type Verdict struct {
	Node string
	Fits bool
	Why  string // 落ちた理由(通ったときは空)
}

// Filter は predicates を順に当てて、Pod を置けるノードだけを残す。
// ここでの判定は可否だけで、優劣はつけない。落ちたノードには理由を残す。
// 実物の Kubernetes も、なぜ Pending のままなのかをこの理由で説明する。
func Filter(p Pod, nodes []*Node) (feasible []*Node, verdicts []Verdict) {
	for _, n := range nodes {
		switch {
		case !p.Req.fitsIn(n.Free()):
			// 空きが足りない。要求(request)は予約であり、実測ではない。
			verdicts = append(verdicts, Verdict{Node: n.Name, Why: "空き不足(" + res(n.Free()) + " < " + res(p.Req) + ")"})
		case untolerated(p, n) != "":
			verdicts = append(verdicts, Verdict{Node: n.Name, Why: "汚れ " + untolerated(p, n) + " を許容していない"})
		default:
			feasible = append(feasible, n)
			verdicts = append(verdicts, Verdict{Node: n.Name, Fits: true})
		}
	}
	return feasible, verdicts
}

// untolerated は Pod が許容していない汚れを 1 つ返す(なければ空文字)。
func untolerated(p Pod, n *Node) string {
	for _, t := range n.taints {
		if !p.tolerates(t) {
			return t.Key + "=" + t.Value
		}
	}
	return ""
}

// #endregion filter

// #region score

// Strategy は候補ノードへの点の付け方。可否は変えず、優劣だけを変える。
type Strategy int

const (
	// Spread は空きの割合が大きいノードを高く評価する。負荷を散らす。
	Spread Strategy = iota
	// BinPack は置いた後の使用率が高いノードを高く評価する。詰め込む。
	BinPack
)

// Score は Pod を置いた場合のノードの点数(0..100)を返す。
// Spread は残る空きが大きいほど高く、BinPack は埋まるほど高い。
// 同じ filter を通ったノード同士の比較なので、どちらでも置けはする。
func Score(p Pod, n *Node, s Strategy) int {
	after := n.used.add(p.Req)
	cpu := pct(after.CPU, n.Cap.CPU) // 置いた後の使用率
	mem := pct(after.Mem, n.Cap.Mem)
	packed := (cpu + mem) / 2 // CPU とメモリの平均を取る
	if s == BinPack {
		return packed
	}
	return 100 - packed // Spread は使用率が低いほど高得点
}

// pct は a/b を百分率にする(b が 0 なら満杯とみなす)。
func pct(a, b int) int {
	if b <= 0 {
		return 100
	}
	return a * 100 / b
}

// #endregion score

// #region schedule

// NodeScore は 1 候補の点数(観測・説明用)。
type NodeScore struct {
	Node  string
	Score int
}

// Result は 1 回のスケジューリングの結果。選ばれたノードと、その過程。
type Result struct {
	Pod      string
	Node     string // 空なら置けなかった(Pending のまま)
	Verdicts []Verdict
	Scores   []NodeScore
}

// Scheduled は配置先が決まったかを返す。
func (r Result) Scheduled() bool { return r.Node != "" }

// Scheduler は Pod をノードに割り当てる。戦略は点の付け方だけを変える。
type Scheduler struct{ Strategy Strategy }

// New は戦略 s のスケジューラを作る。
func New(s Strategy) *Scheduler { return &Scheduler{Strategy: s} }

// Schedule は filter で候補を絞り、score で順位をつけ、最高点のノードに Pod を
// 束縛(bind)する。同点はノード名の辞書順で決めるので結果は決定的になる。
// 候補が 1 つも残らなければ配置しない。Pod は Pending のまま残る。
func (s *Scheduler) Schedule(p Pod, nodes []*Node) Result {
	r := Result{Pod: p.Name}

	// ① filter: 置けないノードを落とす。
	feasible, verdicts := Filter(p, nodes)
	r.Verdicts = verdicts
	if len(feasible) == 0 {
		return r // どこにも置けない。Pending のまま
	}

	// ② score: 残った候補に点をつける。
	for _, n := range feasible {
		r.Scores = append(r.Scores, NodeScore{Node: n.Name, Score: Score(p, n, s.Strategy)})
	}
	sort.SliceStable(r.Scores, func(i, j int) bool {
		if r.Scores[i].Score != r.Scores[j].Score {
			return r.Scores[i].Score > r.Scores[j].Score
		}
		return r.Scores[i].Node < r.Scores[j].Node // 同点は名前順(決定的)
	})

	// ③ bind: 最高点のノードに束縛し、要求のぶんだけ使用量を予約する。
	best := r.Scores[0].Node
	for _, n := range feasible {
		if n.Name == best {
			n.used = n.used.add(p.Req)
			n.pods = append(n.pods, p.Name)
			r.Node = n.Name
			break
		}
	}
	return r
}

// ScheduleAll は Pod を順に処理し、結果を返す。前の配置が次の判断に効く
// (1 つ置くたびに空きが減る)ので、順序が結果を変える。
func (s *Scheduler) ScheduleAll(pods []Pod, nodes []*Node) []Result {
	out := make([]Result, 0, len(pods))
	for _, p := range pods {
		out = append(out, s.Schedule(p, nodes))
	}
	return out
}

// #endregion schedule

// res は Resources を "CPU/Mem" の形の文字列にする(理由の説明用)。
func res(r Resources) string { return itoa(r.CPU) + "m/" + itoa(r.Mem) + "Mi" }

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
