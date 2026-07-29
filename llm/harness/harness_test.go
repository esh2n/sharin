package harness

import "testing"

// 題材は「バグを1つ直す」。道具は4つ。
//
// 直す前の test は落ち、直したあとの test は通る、という筋書きにしてある。
func fixTools() (map[string]Tool, *bool) {
	fixed := false
	return map[string]Tool{
		"search": func(string) string { return "見つけた: calc.go:42" },
		"read":   func(string) string { return "return a - b // ここが違う" },
		"edit": func(string) string {
			fixed = true
			return "書き換えた"
		},
		"test": func(string) string {
			if fixed {
				return "PASS"
			}
			return "FAIL"
		},
	}, &fixed
}

// planner は決めた順に道具を使い、使い切ったら答える台本。
//
// 訊かれた回数で進める。グラフでは一部の手しかモデルに訊かないので、
// 観測の数で進めると数が合わなくなる。
type planner struct {
	plan  []Action
	calls int
}

func (p *planner) Decide([]Step) Action {
	i := p.calls
	p.calls++
	if i < len(p.plan) {
		return p.plan[i]
	}
	return Action{Answer: "直した"}
}

// stubborn は test が落ちても同じ手を繰り返す台本(よくある詰まり方)。
type stubborn struct{}

func (stubborn) Decide(seen []Step) Action {
	if len(seen) == 0 {
		return Action{Tool: "test"}
	}
	return Action{Tool: "test"} // 同じことを繰り返す
}

// この章の中心その1。1回きりでは、途中で分かったことを次に使えない。
func TestOnceCannotUseWhatItLearns(t *testing.T) {
	// 道具を使いたいと言うモデル。
	want := &planner{plan: []Action{{Tool: "search"}}}
	r := Once(want)
	if r.OK {
		t.Fatal("道具を渡していないのに成功した")
	}
	if r.ModelCalls != 1 {
		t.Fatalf("呼び出しは1回のはず: %d", r.ModelCalls)
	}

	// 自分の中だけで答えるモデルなら「成功」はする。だが確かめていない。
	guess := &planner{}
	if g := Once(guess); !g.OK || g.ToolCalls != 0 {
		t.Fatalf("道具を使わずに答えるはず: %+v", g)
	}
}

// ループなら、観測を次の判断に渡せるので解ける。ただし毎回モデルに訊く。
func TestLoopSolvesItButAsksEveryTime(t *testing.T) {
	tools, fixed := fixTools()
	m := &planner{plan: []Action{
		{Tool: "search"}, {Tool: "read"}, {Tool: "edit"}, {Tool: "test"},
	}}
	r := Loop(m, tools, LoopConfig{MaxSteps: 10})

	if !r.OK || !*fixed {
		t.Fatalf("直っていない: %+v", r)
	}
	if r.ToolCalls != 4 {
		t.Fatalf("道具は4回のはず: %d", r.ToolCalls)
	}
	// 4手ぶんと、最後の「終わり」で5回訊いている。
	if r.ModelCalls != 5 {
		t.Fatalf("モデル呼び出し: %d", r.ModelCalls)
	}
	for _, s := range r.Steps {
		if !s.ByModel {
			t.Fatal("ループでは毎手モデルが選んでいるはず")
		}
	}
}

// この章の中心その2。ループは自分では止まれない。上限が要る。
func TestLoopNeedsALimit(t *testing.T) {
	tools, _ := fixTools()
	r := Loop(stubborn{}, tools, LoopConfig{MaxSteps: 6})

	if r.OK {
		t.Fatal("直っていないのに成功した")
	}
	if r.Reason != "上限に達した" {
		t.Fatalf("止まった理由: %q", r.Reason)
	}
	if r.ToolCalls != 6 {
		t.Fatalf("上限まで回るはず: %d", r.ToolCalls)
	}
	// 同じことを繰り返しているだけで、観測は1つも変わっていない。
	for _, s := range r.Steps {
		if s.Obs != "FAIL" {
			t.Fatalf("観測が変わっている: %q", s.Obs)
		}
	}
}

// fixGraph は同じ仕事を、経路を先に描いた形で組む。
func fixGraph() *Graph {
	return &Graph{
		Start:     "search",
		MaxVisits: 4,
		Nodes: map[string]Node{
			"search": {Name: "search", Tool: "search", Next: "read"},
			"read":   {Name: "read", Tool: "read", Next: "edit"},
			// 直し方だけはモデルに決めさせる。
			"edit": {Name: "edit", Decide: true, Next: "test"},
			"test": {
				Name: "test", Tool: "test",
				Check: func(obs string) bool { return obs == "PASS" },
				Retry: "edit", // 落ちたら edit からやり直す。最初からではない
			},
		},
	}
}

// この章の中心その3。経路を先に描けるところは、モデルに訊かなくてよい。
func TestGraphAsksTheModelOnlyWhereItMatters(t *testing.T) {
	tools, fixed := fixTools()
	m := &planner{plan: []Action{{Tool: "edit"}}}
	r := fixGraph().Run(m, tools)

	if !r.OK || !*fixed {
		t.Fatalf("直っていない: %+v", r)
	}
	if r.ToolCalls != 4 {
		t.Fatalf("道具は4回のはず: %d", r.ToolCalls)
	}
	// 訊いたのは edit の節だけ。ループの5回に対して1回で済んでいる。
	if r.ModelCalls != 1 {
		t.Fatalf("モデル呼び出し: %d", r.ModelCalls)
	}
	byModel := 0
	for _, s := range r.Steps {
		if s.ByModel {
			byModel++
		}
	}
	if byModel != 1 {
		t.Fatalf("モデルが選んだ手: %d", byModel)
	}
}

// 失敗したとき、グラフは節1つぶんだけ戻る。
func TestGraphRetriesOnlyTheFailedNode(t *testing.T) {
	// 1回目の edit は直せず、2回目で直る筋書き。
	attempts := 0
	tools := map[string]Tool{
		"search": func(string) string { return "見つけた" },
		"read":   func(string) string { return "読んだ" },
		"edit": func(string) string {
			attempts++
			return "書き換えた"
		},
		"test": func(string) string {
			if attempts >= 2 {
				return "PASS"
			}
			return "FAIL"
		},
	}
	// やり直しでもう一度訊かれるので、edit を2つ用意しておく。
	m := &planner{plan: []Action{{Tool: "edit"}, {Tool: "edit"}}}
	r := fixGraph().Run(m, tools)

	if !r.OK {
		t.Fatalf("やり直せていない: %+v", r)
	}
	// 訊いたのは edit の節に入った2回だけ。
	if r.ModelCalls != 2 {
		t.Fatalf("モデル呼び出し: %d", r.ModelCalls)
	}
	// search と read は1回ずつ。やり直したのは edit と test だけ。
	count := map[string]int{}
	for _, s := range r.Steps {
		count[s.Tool]++
	}
	if count["search"] != 1 || count["read"] != 1 {
		t.Fatalf("最初からやり直している: %v", count)
	}
	if count["edit"] != 2 || count["test"] != 2 {
		t.Fatalf("やり直しの回数: %v", count)
	}
	// 全部で6手。ループで最初からやり直すと8手になる。
	if r.ToolCalls != 6 {
		t.Fatalf("総手数: %d", r.ToolCalls)
	}
}

// やり直しも止まらなくなりうるので、こちらにも上限が要る。
func TestGraphNeedsALimitToo(t *testing.T) {
	tools := map[string]Tool{
		"search": func(string) string { return "見つけた" },
		"read":   func(string) string { return "読んだ" },
		"edit":   func(string) string { return "書き換えた" },
		"test":   func(string) string { return "FAIL" }, // 何度やっても直らない
	}
	m := &planner{plan: []Action{{Tool: "edit"}, {Tool: "edit"}, {Tool: "edit"}, {Tool: "edit"}, {Tool: "edit"}}}
	r := fixGraph().Run(m, tools)

	if r.OK {
		t.Fatal("直っていないのに成功した")
	}
	if r.Reason != "同じ節を回りすぎた: edit" {
		t.Fatalf("止まった理由: %q", r.Reason)
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	tools, _ := fixTools()

	bad := func() *planner { return &planner{plan: []Action{{Tool: "compile"}}} }

	// 無い道具を使おうとしたら、そこで止まる。
	if r := Loop(bad(), tools, LoopConfig{MaxSteps: 5}); r.OK || r.Reason == "" {
		t.Fatalf("止まっていない: %+v", r)
	}
	g := &Graph{Start: "x", Nodes: map[string]Node{"x": {Name: "x", Tool: "compile"}}}
	if r := g.Run(bad(), tools); r.OK {
		t.Fatal("無い道具で進んだ")
	}
	// 無い節を指していたら、そこで止まる。
	g2 := &Graph{Start: "nowhere", Nodes: map[string]Node{}}
	if r := g2.Run(bad(), tools); r.OK || r.Reason == "" {
		t.Fatalf("止まっていない: %+v", r)
	}
	// 失敗しても戻り先が無ければ、そこで終わる。
	g3 := &Graph{Start: "t", Nodes: map[string]Node{
		"t": {Name: "t", Tool: "test", Check: func(o string) bool { return o == "PASS" }},
	}}
	if r := g3.Run(bad(), tools); r.OK || r.Reason != "t が失敗した" {
		t.Fatalf("理由: %+v", r)
	}
	// 節の中でモデルが「終わり」と言ったら、そこで返る。
	done := &planner{}
	g4 := &Graph{Start: "d", Nodes: map[string]Node{"d": {Name: "d", Decide: true}}}
	if r := g4.Run(done, tools); !r.OK || r.Answer != "直した" {
		t.Fatalf("終われていない: %+v", r)
	}
	// 上限を置かないループも、モデルが終わりと言えば止まる。
	if r := Loop(&planner{}, tools, LoopConfig{}); !r.OK {
		t.Fatalf("止まらない: %+v", r)
	}
	if lastObs(nil) != "" {
		t.Fatal("空の記録で落ちた")
	}
}
