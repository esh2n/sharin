// Package quota は Kubernetes の ResourceQuota と LimitRange を最小構成で
// 実装する。
//
// スケジューラは Pod ごとに「置けるか」を見た。だが1つずつ見ているかぎり、
// 誰かが小さな Pod を1000個作ってクラスタを埋め尽くすことは止められない。
// 1つ1つは正しく、合計だけが問題になる。
//
// ResourceQuota は、その合計に上限を置く。namespace ごとに「この区画で
// 使ってよい CPU は合計これだけ」と宣言する。個々の判断でなく、総量の判断に
// なっている。
//
// もう1つ、要求を書かない Pod の扱いという問題がある。要求が 0 の Pod は
// 総量にも 0 として数えられるので、いくつ作っても上限に当たらない。だが
// 実際には資源を使う。ここで LimitRange が効く。要求を書いていない Pod に
// 既定値を入れてから数える。書き忘れが総量の勘定を素通りしないようにする。
//
// 順序に注意が要る。既定値を入れてから数えるのであって、数えてから入れるの
// ではない。この順序が逆だと、書き忘れた Pod が 0 として数えられ、上限は
// 意味を失う。admission の章で見た「書き換えが先、検証が後」と同じ形が、
// ここにも出ている。
package quota

import "sort"

// #region model

// Resources は CPU(ミリコア)とメモリ(MiB)の量。
type Resources struct {
	CPU int
	Mem int
}

func (r Resources) add(o Resources) Resources {
	return Resources{CPU: r.CPU + o.CPU, Mem: r.Mem + o.Mem}
}

func (r Resources) sub(o Resources) Resources {
	return Resources{CPU: r.CPU - o.CPU, Mem: r.Mem - o.Mem}
}

// fitsIn は自分が cap に収まるかを返す。
func (r Resources) fitsIn(cap Resources) bool {
	return r.CPU <= cap.CPU && r.Mem <= cap.Mem
}

// IsZero は何も要求していないかを返す。
func (r Resources) IsZero() bool { return r.CPU == 0 && r.Mem == 0 }

// LimitRange は、要求を書いていない Pod に入れる既定値と、
// 1つあたりに許す上限を持つ。
type LimitRange struct {
	// Default は要求が書かれていないときに入れる値。
	Default Resources
	// Max は1つあたりの上限。これを超える要求は、総量に余裕があっても通さない。
	Max Resources
}

// ResourceQuota は namespace ごとの総量の上限。
type ResourceQuota struct {
	// Hard は合計の上限。
	Hard Resources
	// MaxPods は個数の上限(0 なら無制限)。
	MaxPods int
}

// Pod は要求を持つ1つの実体。
type Pod struct {
	Name string
	Req  Resources
	// Defaulted は既定値が入れられたかを示す(観測用)。
	Defaulted bool
}

// #endregion model

// #region namespace

// Namespace は区画。ここに属する Pod の総量が上限に照らされる。
type Namespace struct {
	Name  string
	quota ResourceQuota
	limit LimitRange

	pods []*Pod
	used Resources

	Admitted int
	Rejected int
	Log      []string
}

// New は上限と既定値を持つ区画を作る。
func New(name string, q ResourceQuota, l LimitRange) *Namespace {
	return &Namespace{Name: name, quota: q, limit: l}
}

// Used は今使っている合計を返す。
func (n *Namespace) Used() Resources { return n.used }

// Free は残りを返す。
func (n *Namespace) Free() Resources { return n.quota.Hard.sub(n.used) }

// Pods は Pod を名前順に返す。
func (n *Namespace) Pods() []*Pod {
	out := append([]*Pod(nil), n.pods...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Quota は上限を返す。
func (n *Namespace) Quota() ResourceQuota { return n.quota }

// #endregion namespace

// #region admit

// Result は1回の受け入れ判定の結果。
type Result struct {
	Admitted  bool
	Pod       *Pod
	Defaulted bool
	Reason    string
}

// Admit は Pod を受け入れるかを判定する。
//
// 順序が肝になる。まず既定値を入れ、次に1つあたりの上限を見て、最後に
// 総量を見る。既定値を先に入れるので、要求を書き忘れた Pod も総量に
// 正しく数えられる。逆順だと、書き忘れが 0 として通り、上限が意味を失う。
func (n *Namespace) Admit(name string, req Resources) Result {
	p := &Pod{Name: name, Req: req}

	// ① 要求が書かれていなければ、既定値を入れる。
	if p.Req.IsZero() && !n.limit.Default.IsZero() {
		p.Req = n.limit.Default
		p.Defaulted = true
		n.logf(name + " に既定値を入れた(" + res(p.Req) + ")")
	}

	// ② 1つあたりの上限。総量に余裕があっても、大きすぎるものは通さない。
	if !n.limit.Max.IsZero() && !p.Req.fitsIn(n.limit.Max) {
		n.Rejected++
		n.logf(name + " を拒否(1つあたりの上限 " + res(n.limit.Max) + " を超える)")
		return Result{Pod: p, Defaulted: p.Defaulted,
			Reason: "1つあたりの上限を超える要求"}
	}

	// ③ 個数の上限。
	if n.quota.MaxPods > 0 && len(n.pods) >= n.quota.MaxPods {
		n.Rejected++
		n.logf(name + " を拒否(個数の上限 " + itoa(n.quota.MaxPods) + " に達している)")
		return Result{Pod: p, Defaulted: p.Defaulted, Reason: "個数の上限に達している"}
	}

	// ④ 総量の上限。
	after := n.used.add(p.Req)
	if !after.fitsIn(n.quota.Hard) {
		n.Rejected++
		n.logf(name + " を拒否(総量が上限 " + res(n.quota.Hard) + " を超える)")
		return Result{Pod: p, Defaulted: p.Defaulted, Reason: "総量の上限を超える"}
	}

	n.pods = append(n.pods, p)
	n.used = after
	n.Admitted++
	return Result{Admitted: true, Pod: p, Defaulted: p.Defaulted,
		Reason: "受け入れた(合計 " + res(n.used) + " / " + res(n.quota.Hard) + ")"}
}

// Remove は Pod を消し、使っていた分を返す。
func (n *Namespace) Remove(name string) bool {
	for i, p := range n.pods {
		if p.Name != name {
			continue
		}
		n.used = n.used.sub(p.Req)
		n.pods = append(n.pods[:i], n.pods[i+1:]...)
		n.logf(name + " を削除(合計 " + res(n.used) + ")")
		return true
	}
	return false
}

// #endregion admit

func res(r Resources) string { return itoa(r.CPU) + "m/" + itoa(r.Mem) + "Mi" }

func (n *Namespace) logf(msg string) { n.Log = append(n.Log, msg) }

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
