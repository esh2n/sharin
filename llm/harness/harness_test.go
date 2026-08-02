package harness

import (
	"strings"
	"testing"
)

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

func (p *planner) Decide(Window) Action {
	i := p.calls
	p.calls++
	if i < len(p.plan) {
		return p.plan[i]
	}
	return Action{Answer: "直した"}
}

// stubborn は test が落ちても同じ手を繰り返す台本(よくある詰まり方)。
type stubborn struct{}

func (stubborn) Decide(Window) Action { return Action{Tool: "test"} }

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
	r := Loop(m, tools, LoopConfig{MaxCalls: 10})

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
	t.Logf("ループ: 道具 %d 手 / モデル %d 回 / 渡した文字 %d",
		r.ToolCalls, r.ModelCalls, r.InputChars)
}

// この章の中心その2。ループは自分では止まれない。上限が要る。
func TestLoopNeedsALimit(t *testing.T) {
	tools, _ := fixTools()
	r := Loop(stubborn{}, tools, LoopConfig{MaxCalls: 6})

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

// ------------------------------------------------------------------
// ここからコンテキストの層。題材は「呼び出し元を全部見てから直す」。
// ------------------------------------------------------------------

// 8 本のファイルを順に読む。当たりは 2 本目で、直すにはその中身が要る。
var files = []string{
	"calc.go", "tax.go", "fee.go", "cart.go",
	"item.go", "user.go", "order.go", "view.go",
}

const hit = "tax.go"

func traceTools() map[string]Tool {
	return map[string]Tool{
		"search": func(string) string { return "呼び出し元 8 件: calc.go ほか" },
		"read": func(f string) string {
			if f == hit {
				return f + " 24 行目 rate + price。掛けるのが正しい。呼び出し 3 箇所"
			}
			return f + " 異常なし。呼び出し 3 箇所 / 24 行 / 直近の変更なし"
		},
		"edit": func(f string) string { return f + " を書き換えた" },
		"test": func(string) string { return "PASS" },
	}
}

// tracer は「窓に見えているもの」だけで動く台本。人と同じで、
// 見えていなければ、もう一度やる。
//
// 決まりは1つ。呼び出し元を全部見てから直す。
type tracer struct {
	writeMemo bool // 当たりを見つけたとき、窓の外に書き残すか
}

func (t *tracer) Decide(w Window) Action {
	if !w.Did("search", "") {
		return Action{Tool: "search"}
	}
	// 直したあとは確かめて終わる。
	if w.Did("edit", hit) {
		if !w.Did("test", "") {
			return Action{Tool: "test"}
		}
		return Action{Answer: "直した"}
	}
	// 当たりの中身が今まさに窓にあるなら、要点だけ窓の外へ書き出しておく。
	if _, ok := w.Obs("read", hit); ok && t.writeMemo && !w.Recall(hit) {
		return Action{Note: hit + " は 24 行目、足すのでなく掛ける"}
	}
	// まだ読んでいないファイルがあれば読む。
	for _, f := range files {
		if !w.Did("read", f) {
			return Action{Tool: "read", Arg: f}
		}
	}
	// 直すには当たりの中身が要る。読めなければ読み直すしかない。
	if _, ok := w.Obs("read", hit); !ok && !w.Recall(hit) {
		return Action{Tool: "read", Arg: hit}
	}
	return Action{Tool: "edit", Arg: hit}
}

// この章の中心その3。窓に収まる件数は記録より少ない。溢れる側は古い側になる。
func TestWindowHoldsFewerStepsThanTheRecordHas(t *testing.T) {
	// まず、落とさずに走らせて記録を作る。
	full := Loop(&tracer{}, traceTools(), LoopConfig{MaxCalls: 40})
	if !full.OK {
		t.Fatalf("落とさなければ終わるはず: %+v", full)
	}
	all := full.Steps
	t.Logf("記録は %d 手 / %d 文字", len(all), totalSize(all))
	if len(all) != 11 {
		t.Fatalf("手数: %d", len(all))
	}

	t.Logf("%-8s %8s %8s %10s %10s", "窓", "収まる手", "占めた", "落ちた手", "消えた観測")
	prev := -1
	for _, b := range []Budget{100, 200, 400, 800} {
		w := Recent(all, nil, b)
		t.Logf("%-8d %8d %8d %10d %10d", b, len(w.Steps), w.Size, w.LostSteps, w.LostChars)
		// 窓を広げれば収まる手数は増える。減ることはない。
		if len(w.Steps) < prev {
			t.Fatalf("窓を広げたのに減った: %d", len(w.Steps))
		}
		prev = len(w.Steps)
		// 収まると言っている以上、超えていてはいけない。
		if w.Over != 0 {
			t.Fatalf("窓 %d を %d 超えている", b, w.Over)
		}
	}

	// 全部残すと入らない。ここが実物では拒否か、黙って消えるかになる。
	keep := KeepAll(all, nil, 200)
	t.Logf("全部残す: %d 文字。窓 200 を %d 超える", keep.Size, keep.Over)
	if keep.Over <= 0 {
		t.Fatal("超えていない")
	}
	// 落ちるのは古い側。当たりは 3 手目なので、窓 200 では読めない。
	w := Recent(all, nil, 200)
	if _, ok := w.Obs("read", hit); ok {
		t.Fatal("当たりが窓に残っている")
	}
}

// この章の中心その4。同じ台本でも、何を残すかで終わったり終わらなかったりする。
func TestWhatYouKeepDecidesWhetherItFinishes(t *testing.T) {
	const budget = 200
	const limit = 40

	type run struct {
		name string
		cfg  LoopConfig
		memo bool
	}
	runs := []run{
		{"全部残す", LoopConfig{MaxCalls: limit, Budget: budget, Curate: KeepAll}, false},
		{"直近だけ", LoopConfig{MaxCalls: limit, Budget: budget, Curate: Recent}, false},
		{"畳む", LoopConfig{MaxCalls: limit, Budget: budget, Curate: Fold}, false},
		{"畳む+覚え書き", LoopConfig{MaxCalls: limit, Budget: budget, Curate: Fold}, true},
	}

	got := map[string]Result{}
	t.Logf("窓 %d 文字 / 上限 %d 回", budget, limit)
	t.Logf("%-14s %6s %8s %10s %10s %8s", "残し方", "手数", "モデル", "渡した文字", "窓を超えた", "終わった")
	for _, rn := range runs {
		r := Loop(&tracer{writeMemo: rn.memo}, traceTools(), rn.cfg)
		over := rn.cfg.Curate(r.Steps, r.Memo, budget).Over
		t.Logf("%-14s %6d %8d %10d %10d %8v",
			rn.name, r.ToolCalls, r.ModelCalls, r.InputChars, over, r.OK)
		got[rn.name] = r
	}

	// 全部残すと終わりはするが、窓には入っていない。
	all := got["全部残す"]
	if !all.OK {
		t.Fatalf("全部残せば終わるはず: %+v", all)
	}
	if KeepAll(all.Steps, all.Memo, budget).Over <= 0 {
		t.Fatal("窓に収まってしまっている。題材が小さすぎる")
	}

	// 直近だけ残すと、読んだこと自体が窓から消えるので同じ手を繰り返す。
	rec := got["直近だけ"]
	if rec.OK {
		t.Fatal("直近だけ残して終わってしまった")
	}
	if rec.Reason != "上限に達した" {
		t.Fatalf("止まった理由: %q", rec.Reason)
	}
	reads := map[string]int{}
	for _, s := range rec.Steps {
		if s.Tool == "read" {
			reads[s.Arg]++
		}
	}
	t.Logf("直近だけ: 同じファイルを %d 回まで読み直した", maxOf(reads))
	if maxOf(reads) < 2 {
		t.Fatal("読み直しが起きていない")
	}

	// 畳むと「何をやったか」は残るので、読み直すのは当たりの1本だけになる。
	fold := got["畳む"]
	if !fold.OK {
		t.Fatalf("畳めば終わるはず: %+v", fold)
	}
	if fold.ToolCalls != all.ToolCalls+1 {
		t.Fatalf("読み直しは1本のはず: %d と %d", fold.ToolCalls, all.ToolCalls)
	}

	// 覚え書きに1行残しておけば、その読み直しも要らない。
	memo := got["畳む+覚え書き"]
	if !memo.OK || memo.ToolCalls != all.ToolCalls {
		t.Fatalf("覚え書きがあれば読み直さないはず: %+v", memo)
	}
	if len(memo.Memo) != 1 {
		t.Fatalf("覚え書きの行数: %v", memo.Memo)
	}
	// 落とした観測は数百文字、残した覚え書きは数十文字。
	lost := Fold(memo.Steps, memo.Memo, budget).LostChars
	kept := memoSize(memo.Memo)
	t.Logf("捨てた観測 %d 文字 / 残した覚え書き %d 文字", lost, kept)
	if lost < kept*5 {
		t.Fatalf("差が出ていない: %d と %d", lost, kept)
	}
}

// 畳んだ行から読み取れるのは「やったこと」だけ。「分かったこと」は読めない。
func TestFoldKeepsTheActionAndDropsTheObservation(t *testing.T) {
	all := Loop(&tracer{}, traceTools(), LoopConfig{MaxCalls: 40}).Steps

	fold := Fold(all, nil, 120)
	rec := Recent(all, nil, 120)

	// 畳んだ側は、当たりを読んだことを知っている。
	if !fold.Did("read", hit) {
		t.Fatal("畳んだのに読んだことが消えた")
	}
	// だが中身は読めない。
	if _, ok := fold.Obs("read", hit); ok {
		t.Fatal("畳んだのに観測が残っている")
	}
	// 直近だけの側は、読んだことすら知らない。
	if rec.Did("read", hit) {
		t.Fatal("落としたのに残っている")
	}
	t.Logf("窓 120  畳む: 原文 %d 手 + 畳んだ %d 行 = %d 文字",
		len(fold.Steps), len(fold.Folded), fold.Size)
	t.Logf("窓 120  直近: 原文 %d 手 = %d 文字", len(rec.Steps), rec.Size)

	// 畳んだ行のぶんだけ、原文で残せる手は減る。ただだとは思わないほうがいい。
	if len(fold.Steps) > len(rec.Steps) {
		t.Fatal("畳んだのに原文が増えた")
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

// #region decide

// visitOrder は fixGraph をたどる順。
var visitOrder = []string{"search", "read", "edit", "test"}

// decideGraph は fixGraph と同じ経路のまま、どの節で訊くかだけを変える。
//
// 経路も道具の手数も変わらないので、比べられるのは「どこで訊くか」の効果だけになる。
func decideGraph(ask ...string) (*Graph, []Action) {
	set := map[string]bool{}
	for _, a := range ask {
		set[a] = true
	}
	g := fixGraph()
	for name, n := range g.Nodes {
		if set[name] {
			n.Decide, n.Tool = true, "" // 何を使うかはモデルが返す
		} else {
			n.Decide, n.Tool = false, name // 節の名前がそのまま道具の名前
		}
		g.Nodes[name] = n
	}
	var plan []Action
	for _, name := range visitOrder {
		if set[name] {
			plan = append(plan, Action{Tool: name})
		}
	}
	return g, plan
}

// #endregion decide

func runDecide(t *testing.T, ask ...string) Result {
	t.Helper()
	g, plan := decideGraph(ask...)
	tools, fixed := fixTools()
	r := g.Run(&planner{plan: plan}, tools)
	if !r.OK || !*fixed {
		t.Fatalf("%v で直っていない: %+v", ask, r)
	}
	if r.ToolCalls != 4 {
		t.Fatalf("%v で道具の手数が変わった: %d", ask, r.ToolCalls)
	}
	return r
}

// 訊く節を増やすと、呼び出しは比例して増える。だが経路は選べないままになる。
func TestMoreDecidingNodesCostMoreWithoutDecidingMore(t *testing.T) {
	// 比べる相手のループ。同じ 4 手を毎回訊いて進める。
	lt, _ := fixTools()
	loop := Loop(&planner{plan: []Action{
		{Tool: "search"}, {Tool: "read"}, {Tool: "edit"}, {Tool: "test"},
	}}, lt, LoopConfig{MaxCalls: 10})

	t.Logf("%-22s %6s %8s %12s %12s", "形", "道具", "モデル", "渡した文字", "経路を選べる")
	t.Logf("%-22s %6d %8d %12d %12s", "ループ(毎手訊く)", loop.ToolCalls, loop.ModelCalls, loop.InputChars, "はい")

	// edit を起点に、後ろから足していく。
	sets := [][]string{
		{"edit"},
		{"edit", "test"},
		{"read", "edit", "test"},
		{"search", "read", "edit", "test"},
	}
	var calls, chars []int
	for _, s := range sets {
		r := runDecide(t, s...)
		calls = append(calls, r.ModelCalls)
		chars = append(chars, r.InputChars)
		t.Logf("%-22s %6d %8d %12d %12s",
			"グラフ 訊く節"+string(rune('0'+len(s)))+"つ", r.ToolCalls, r.ModelCalls, r.InputChars, "いいえ")
	}

	// 呼び出しは訊く節の数に正比例する。
	for i, c := range calls {
		if c != i+1 {
			t.Fatalf("訊く節 %d で呼び出しが %d", i+1, c)
		}
	}
	// 全部の節で訊いても、まだループより安い。ループは最後の「終わり」を
	// いちばん記録が長い状態で訊くので、そこが余分に効く。
	t.Logf("全部訊く %d 回 %d 文字 / ループ %d 回 %d 文字",
		calls[3], chars[3], loop.ModelCalls, loop.InputChars)
	if calls[3] >= loop.ModelCalls || chars[3] >= loop.InputChars {
		t.Fatalf("ループを超えた: %d/%d と %d/%d",
			calls[3], chars[3], loop.ModelCalls, loop.InputChars)
	}
	// それでいて、決められることは 1 つも増えていない。
	// 経路は 4 通りとも同じで、モデルが選べるのは各節で使う道具だけになる。
	if chars[0] >= chars[3] {
		t.Fatal("増やしたのに渡す量が減った")
	}
}

// 同じ 1 つでも、後ろの節で訊くほど高くつく。
func TestWhereYouAskDecidesWhatItCosts(t *testing.T) {
	t.Logf("%-10s %8s %12s", "訊く節", "モデル", "渡した文字")
	var chars []int
	for _, name := range visitOrder {
		r := runDecide(t, name)
		chars = append(chars, r.InputChars)
		t.Logf("%-10s %8d %12d", name, r.ModelCalls, r.InputChars)
	}

	// どこで訊いても呼び出しは 1 回。だが渡す量は単調に増える。
	for i := 1; i < len(chars); i++ {
		if chars[i] <= chars[i-1] {
			t.Fatalf("後ろの節のほうが安い: %v", chars)
		}
	}
	// 最初の節では記録がまだ空なので、渡すものが無い。
	if chars[0] != 0 {
		t.Fatalf("最初の節で渡した文字: %d", chars[0])
	}
	t.Logf("最後の節で訊くのは、2 番目の節で訊くより %d 文字ぶん重い", chars[3]-chars[1])
}

// この章の中心その5。経路を先に描けるところは、モデルに訊かなくてよい。
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

	// 訊く回数だけでなく、渡す量も減る。値段で効くのはこちらになる。
	tools2, _ := fixTools()
	lp := Loop(&planner{plan: []Action{
		{Tool: "search"}, {Tool: "read"}, {Tool: "edit"}, {Tool: "test"},
	}}, tools2, LoopConfig{MaxCalls: 10})
	t.Logf("渡した文字  ループ %d / グラフ %d", lp.InputChars, r.InputChars)
	if r.InputChars*3 > lp.InputChars {
		t.Fatalf("差が出ていない: %d と %d", lp.InputChars, r.InputChars)
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
	if r := Loop(bad(), tools, LoopConfig{MaxCalls: 5}); r.OK || r.Reason == "" {
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
	// グラフの節でも覚え書きは書ける。書いたら同じ節をもう一度。
	memoM := &planner{plan: []Action{{Note: "ここが怪しい"}, {Tool: "edit"}}}
	g5 := &Graph{Start: "d", MaxVisits: 4, Nodes: map[string]Node{"d": {Name: "d", Decide: true}}}
	if r := g5.Run(memoM, tools); !r.OK || len(r.Memo) != 1 {
		t.Fatalf("覚え書きが残っていない: %+v", r)
	}
	// 上限を置かないループも、モデルが終わりと言えば止まる。
	if r := Loop(&planner{}, tools, LoopConfig{}); !r.OK {
		t.Fatalf("止まらない: %+v", r)
	}
	// 同じ行は二度書かない。
	if m := note(note(nil, "あ"), "あ"); len(m) != 1 {
		t.Fatalf("重複した: %v", m)
	}
	// 窓が空でも落ちない。
	empty := Fold(nil, nil, 10)
	if empty.Size != 0 || empty.Did("read", "x") || empty.Recall("x") {
		t.Fatalf("空の窓: %+v", empty)
	}
	if _, ok := empty.Obs("read", "x"); ok {
		t.Fatal("空の窓から観測が読めた")
	}
	// 覚え書きだけで窓を超えることもある。落とすものが無くても超える。
	big := Recent(nil, []string{strings.Repeat("あ", 60)}, 50)
	if big.Over != 10 || big.LostSteps != 0 {
		t.Fatalf("覚え書きが窓を超えたとき: %+v", big)
	}
	// 窓を指定しなければ何も落とさない。
	if w := Recent([]Step{{Tool: "a", Obs: "xxxx"}}, nil, 0); w.LostSteps != 0 {
		t.Fatalf("窓なしで落とした: %+v", w)
	}
	// 引数のない手は道具名だけの行に畳まれる。
	if k := foldKey(Step{Tool: "test"}); k != "test" {
		t.Fatalf("畳んだ行: %q", k)
	}
	if lastObs(nil) != "" {
		t.Fatal("空の記録で落ちた")
	}
	if strings.Contains(foldKey(Step{Tool: "read", Arg: "a.go", Obs: "中身"}), "中身") {
		t.Fatal("畳んだ行に観測が残っている")
	}
}

func maxOf(m map[string]int) int {
	n := 0
	for _, v := range m {
		if v > n {
			n = v
		}
	}
	return n
}
