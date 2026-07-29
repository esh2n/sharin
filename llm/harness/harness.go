// Package harness は、言語モデルに仕事をさせる枠組みを3通り作って比べる。
//
// 呼び方は場所によって違うが、おおよそこの順で語彙が増えてきた。
//
//	プロンプト   1回入れて1回返す
//	ハーネス     道具を渡し、観測を返し、また訊く
//	ループ       止まるまでそれを繰り返す
//	グラフ       通ってよい道を先に描いておく
//
// 順に強くなっていく話に見えるが、そうではない。2026年に起きたのは
// むしろ逆向きの揺り戻しで、経路を先に描けない仕事(調べもの等)では
// グラフに焼き付けるより、ループに任せたほうがうまくいくと分かってきた。
//
// 分かれ目は1つだけになる。**経路を先に描けるかどうか**。
// 描けるところはコードで固定したほうが安く速く読みやすい。
// 描けないところはモデルに決めさせるしかない。
//
// ここでは言語モデルの代わりに台本を置く。実時間も乱数も使わないので、
// 何回やっても同じ手数、同じ結果になる。
package harness

// #region model

// Action はモデルが返す1手。道具を使うか、答えて終わるか。
type Action struct {
	Tool   string
	Arg    string
	Answer string // 空でなければ、ここで終わる
}

// Model は言語モデルの立ち位置。今までの観測を見て、次の1手を返す。
type Model interface {
	Decide(seen []Step) Action
}

// Step は1手の記録。
type Step struct {
	Tool string
	Arg  string
	Obs  string
	// ByModel はこの手をモデルが選んだかどうか。
	// 経路が決まっているところは、選ばせる必要が無い。
	ByModel bool
}

// Tool は道具。引数を受け取って観測を返す。
type Tool func(arg string) string

// Result は仕事の結果。
type Result struct {
	Answer string
	OK     bool
	Steps  []Step
	// ModelCalls はモデルを呼んだ回数。ここが費用と遅さになる。
	ModelCalls int
	// ToolCalls は道具を使った回数。
	ToolCalls int
	// Reason は終わった理由。
	Reason string
}

// #endregion model

// #region once

// Once は1回だけ訊いて、返ってきたものをそのまま答えにする。
//
// 道具を渡していないので、モデルは自分の中にあるものだけで答える。
// 途中で分かったことを次に使う、ということが起きようがない。
func Once(m Model) Result {
	a := m.Decide(nil)
	r := Result{ModelCalls: 1, Answer: a.Answer, Reason: "1回で打ち切り"}
	r.OK = a.Answer != "" && a.Tool == ""
	if a.Tool != "" {
		// 道具を使いたいと言われても、渡していないので応じられない。
		r.Answer = ""
		r.Reason = "道具を使いたいと言われたが、渡していない"
	}
	return r
}

// #endregion once

// #region loop

// LoopConfig はループの止め方。
type LoopConfig struct {
	// MaxSteps は手数の上限。ここが無いと止まらないことがある。
	MaxSteps int
}

// Loop は「答えるか、道具を使うか」をモデルに訊き続ける。
//
// 道具の結果を観測として次の判断に渡すので、途中で分かったことを使える。
// 代わりに、毎回モデルに訊くので手数ぶんの費用がかかる。
// そして自分では止まれないことがあるので、上限が要る。
func Loop(m Model, tools map[string]Tool, cfg LoopConfig) Result {
	var r Result
	for {
		if cfg.MaxSteps > 0 && len(r.Steps) >= cfg.MaxSteps {
			r.Reason = "上限に達した"
			return r
		}
		a := m.Decide(r.Steps)
		r.ModelCalls++

		if a.Answer != "" {
			r.Answer = a.Answer
			r.OK = true
			r.Reason = "モデルが終わりだと言った"
			return r
		}
		t, ok := tools[a.Tool]
		if !ok {
			r.Reason = "無い道具を使おうとした: " + a.Tool
			return r
		}
		r.Steps = append(r.Steps, Step{Tool: a.Tool, Arg: a.Arg, Obs: t(a.Arg), ByModel: true})
		r.ToolCalls++
	}
}

// #endregion loop

// #region graph

// Node はグラフの1つの節。
//
// Decide が false なら、この節ではモデルに訊かない。何をするかは先に決まっている。
type Node struct {
	Name string
	// Tool と Arg は、モデルに訊かずに実行する内容。
	Tool string
	Arg  string
	// Decide が true のとき、この節だけモデルに次の1手を訊く。
	Decide bool
	// Check は観測を見て、この節が成功したかを返す。nil なら常に成功。
	Check func(obs string) bool
	// Retry は失敗したときに戻る先の節名。空なら戻らずに終わる。
	Retry string
	// Next は次の節名。空なら終わり。
	Next string
}

// Graph は節と辺。通ってよい道が先に描いてある。
type Graph struct {
	Start string
	Nodes map[string]Node
	// MaxVisits は同じ節へ入れる回数の上限。やり直しが止まらなくなるのを防ぐ。
	MaxVisits int
}

// Run はグラフをたどる。
//
// 経路が決まっている節ではモデルを呼ばない。ここが費用の差になる。
// 節が失敗したら Retry の節から やり直す。ループのように最初からではない。
func (g *Graph) Run(m Model, tools map[string]Tool) Result {
	var r Result
	visits := map[string]int{}
	name := g.Start

	for name != "" {
		n, ok := g.Nodes[name]
		if !ok {
			r.Reason = "無い節へ行こうとした: " + name
			return r
		}
		visits[name]++
		if g.MaxVisits > 0 && visits[name] > g.MaxVisits {
			r.Reason = "同じ節を回りすぎた: " + name
			return r
		}

		tool, arg, byModel := n.Tool, n.Arg, false
		if n.Decide {
			a := m.Decide(r.Steps)
			r.ModelCalls++
			if a.Answer != "" {
				r.Answer, r.OK, r.Reason = a.Answer, true, "モデルが終わりだと言った"
				return r
			}
			tool, arg, byModel = a.Tool, a.Arg, true
		}

		t, ok := tools[tool]
		if !ok {
			r.Reason = "無い道具を使おうとした: " + tool
			return r
		}
		obs := t(arg)
		r.Steps = append(r.Steps, Step{Tool: tool, Arg: arg, Obs: obs, ByModel: byModel})
		r.ToolCalls++

		if n.Check != nil && !n.Check(obs) {
			if n.Retry == "" {
				r.Reason = n.Name + " が失敗した"
				return r
			}
			name = n.Retry // 失敗した節の手前へ戻る。最初からではない
			continue
		}
		name = n.Next
	}

	r.OK = true
	r.Answer = lastObs(r.Steps)
	r.Reason = "最後の節まで来た"
	return r
}

// #endregion graph

func lastObs(steps []Step) string {
	if len(steps) == 0 {
		return ""
	}
	return steps[len(steps)-1].Obs
}
