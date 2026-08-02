// Package harness は、言語モデルに仕事をさせる枠組みを層ごとに作って比べる。
//
// 呼び方は場所によって違うが、おおよそこの順で語彙が増えてきた。層はそれぞれ
// 「1回ぶん」と数える単位が違う。
//
//	プロンプト   1入力          1回入れて1回返す
//	コンテキスト  窓に残るもの     次に訊くとき、何を見せるか
//	ハーネス     1パス          道具を渡し、観測を返し、また訊く
//	ループ       実行全体        止まるまでそれを繰り返す
//	グラフ       ジョブ全体      通ってよい道を先に描いておく
//
// 順に強くなっていく話に見えるが、そうではない。2026年に起きたのはむしろ
// 逆向きの揺り戻しで、経路を先に描けない仕事(調べもの等)ではグラフに
// 焼き付けるより、ループに任せたほうがうまくいくと分かってきた。
//
// 分かれ目は1つだけになる。**経路を先に描けるかどうか**。
// 描けるところはコードで固定したほうが安く速く読みやすい。
// 描けないところはモデルに決めさせるしかない。
//
// そして、どの層を選んでもコンテキストの層は避けて通れない。窓は有限なので、
// 回すほど古い側から押し出される。何を落とし、何を畳み、何を窓の外へ
// 書き出すかで、同じ台本でも終わったり終わらなかったりする。
//
// ここでは言語モデルの代わりに台本を置く。実時間も乱数も使わないので、
// 何回やっても同じ手数、同じ結果になる。
package harness

import (
	"strings"
	"unicode/utf8"
)

// #region model

// Action はモデルが返す1手。道具を使うか、覚え書きを書くか、答えて終わるか。
type Action struct {
	Tool   string
	Arg    string
	Note   string // 空でなければ、窓の外の覚え書きに1行足す
	Answer string // 空でなければ、ここで終わる
}

// Model は言語モデルの立ち位置。窓に見えているものだけを見て、次の1手を返す。
//
// 引数が Window であって全履歴でないところが要点になる。モデルは
// 「起きたこと」ではなく「窓に残ったこと」に対して判断する。
type Model interface {
	Decide(w Window) Action
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
	// Memo は窓の外に書き出した覚え書き。
	Memo []string
	// ModelCalls はモデルを呼んだ回数。
	ModelCalls int
	// InputChars はモデルに渡した窓の大きさの合計。
	// 実物ではここがトークン数、つまり値段になる。呼び出し回数より実態に近い。
	InputChars int
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
	a := m.Decide(Window{})
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

// #region window

// Budget は窓に入る大きさ。ここでは文字数で数える。
type Budget int

// Size は、この手が窓で占める大きさ。
func (s Step) Size() int { return utf8.RuneCountInString(s.Tool + s.Arg + s.Obs) }

// Window は、これまでの記録のうち窓へ実際に入れたもの。
//
// 記録が伸びても窓は伸びないので、この構造体が毎回作り直される。
// どのフィールドに何が残るかが、そのまま「モデルに何が見えるか」になる。
type Window struct {
	// Memo は窓の外に書き出した覚え書き。短いので落とさない。
	Memo []string
	// Folded は畳んだ手の「道具と引数」。何をやったかは残り、観測は消える。
	Folded []string
	// Steps は原文のまま入れた手。観測まで読める。
	Steps []Step
	// Size は窓が実際に占めた大きさ。
	Size int
	// Over は Budget を超えた大きさ。0 でなければ、この窓は入らない。
	Over int
	// LostSteps は原文が窓から消えた手の数。
	LostSteps int
	// LostChars は窓から消えた観測の文字数。
	LostChars int
}

// Did は、その道具をその引数で使った記録が窓から読み取れるか。
// 原文で残っていても、畳んだ行に名前だけ残っていても読み取れる。
func (w Window) Did(tool, arg string) bool {
	for _, s := range w.Steps {
		if s.Tool == tool && s.Arg == arg {
			return true
		}
	}
	key := foldKey(Step{Tool: tool, Arg: arg})
	for _, f := range w.Folded {
		if f == key {
			return true
		}
	}
	return false
}

// Obs はその手の観測を返す。畳んだ行からは読めないので、そこが失われる。
func (w Window) Obs(tool, arg string) (string, bool) {
	for _, s := range w.Steps {
		if s.Tool == tool && s.Arg == arg {
			return s.Obs, true
		}
	}
	return "", false
}

// Recall は覚え書きにその語を含む行があるか。
func (w Window) Recall(word string) bool {
	for _, m := range w.Memo {
		if strings.Contains(m, word) {
			return true
		}
	}
	return false
}

// Curator は、これまでの記録と覚え書きから、窓に入れるものを決める。
type Curator func(all []Step, memo []string, b Budget) Window

// #endregion window

// #region recent

// KeepAll は何も落とさない。窓に収まるかどうかも見ない。
//
// 記録は手数に比例して伸びるので、放っておけばいつか入らなくなる。
// Over が 0 でなくなった時点で、実物なら拒否されるか、黙って古い側が消える。
func KeepAll(all []Step, memo []string, b Budget) Window {
	w := Window{Memo: memo, Steps: all, Size: memoSize(memo) + totalSize(all)}
	if b > 0 && w.Size > int(b) {
		w.Over = w.Size - int(b)
	}
	return w
}

// Recent は新しいほうから、入るだけ原文で詰める。溢れた古い側は消える。
//
// いちばん素直な削り方だが、消えるのは古い側だと決まっている。
// そして調べものでは、当たりを引いたのはたいてい古い側になる。
func Recent(all []Step, memo []string, b Budget) Window {
	return curate(all, memo, b, false)
}

// curate は新しいほうから詰められるだけ詰める。
// fold が true なら、詰めきれなかったぶんを畳んだ行として残す。
func curate(all []Step, memo []string, b Budget, fold bool) Window {
	base := memoSize(memo)
	keep := 0
	for keep < len(all) {
		cut := len(all) - keep - 1
		size := base + totalSize(all[cut:])
		if fold {
			size += foldedSize(all[:cut])
		}
		if b > 0 && size > int(b) {
			break
		}
		keep++
	}

	dropped := all[:len(all)-keep]
	w := Window{Memo: memo, Steps: all[len(all)-keep:]}
	for _, s := range dropped {
		w.LostSteps++
		w.LostChars += utf8.RuneCountInString(s.Obs)
		if fold {
			w.Folded = append(w.Folded, foldKey(s))
		}
	}
	w.Size = base + totalSize(w.Steps)
	if fold {
		w.Size += foldedSize(dropped)
	}
	if b > 0 && w.Size > int(b) {
		w.Over = w.Size - int(b)
	}
	return w
}

func totalSize(steps []Step) int {
	n := 0
	for _, s := range steps {
		n += s.Size()
	}
	return n
}

// #endregion recent

// #region fold

// Fold は溢れたぶんを「道具と引数」だけの行に畳む。
//
// 何をやったかは残り、何が分かったかは消える。だから同じ手を繰り返すことは
// 無くなるが、観測の中身が要る仕事なら、その手だけやり直すことになる。
func Fold(all []Step, memo []string, b Budget) Window {
	return curate(all, memo, b, true)
}

// foldKey は畳んだ1行。観測は入れない。
func foldKey(s Step) string {
	if s.Arg == "" {
		return s.Tool
	}
	return s.Tool + " " + s.Arg
}

func foldedSize(steps []Step) int {
	n := 0
	for _, s := range steps {
		n += utf8.RuneCountInString(foldKey(s))
	}
	return n
}

// #endregion fold

// #region memo

// note は覚え書きに1行足す。同じ行は二度書かない。
//
// 覚え書きは窓の外に置く。観測を落としても、ここに書いた行は残る。
// 観測が数百文字あっても、そこから取り出した1行は数十文字で済む。
func note(memo []string, line string) []string {
	for _, m := range memo {
		if m == line {
			return memo
		}
	}
	return append(memo, line)
}

// memoSize は覚え書きが窓で占める大きさ。
func memoSize(memo []string) int {
	n := 0
	for _, m := range memo {
		n += utf8.RuneCountInString(m)
	}
	return n
}

// #endregion memo

// #region loop

// LoopConfig はループの止め方と、窓の作り方。
type LoopConfig struct {
	// MaxCalls はモデルを呼ぶ回数の上限。ここが無いと止まらないことがある。
	MaxCalls int
	// Budget は窓の大きさ。0 なら窓を無限とみなす。
	Budget Budget
	// Curate は窓に何を残すかを決める。nil なら KeepAll。
	Curate Curator
}

// Loop は「答えるか、道具を使うか」をモデルに訊き続ける。
//
// 道具の結果を観測として次の判断に渡すので、途中で分かったことを使える。
// 代わりに、毎回モデルに訊くので手数ぶんの費用がかかる。
// そして自分では止まれないことがあるので、上限が要る。
//
// 渡すのは記録そのものではなく、Curate が作った窓になる。
// 窓に残らなかったものは、モデルにとっては起きなかったことと変わらない。
func Loop(m Model, tools map[string]Tool, cfg LoopConfig) Result {
	cur := cfg.Curate
	if cur == nil {
		cur = KeepAll
	}
	var r Result
	for {
		if cfg.MaxCalls > 0 && r.ModelCalls >= cfg.MaxCalls {
			r.Reason = "上限に達した"
			return r
		}
		w := cur(r.Steps, r.Memo, cfg.Budget)
		a := m.Decide(w)
		r.ModelCalls++
		r.InputChars += w.Size

		if a.Note != "" {
			r.Memo = note(r.Memo, a.Note)
			continue
		}
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
	// Curate は決める節で窓を作る。nil なら KeepAll。
	Curate Curator
	// Budget は窓の大きさ。0 なら窓を無限とみなす。
	Budget Budget
}

// Run はグラフをたどる。
//
// 経路が決まっている節ではモデルを呼ばない。ここが費用の差になる。
// 節が失敗したら Retry の節から やり直す。ループのように最初からではない。
func (g *Graph) Run(m Model, tools map[string]Tool) Result {
	cur := g.Curate
	if cur == nil {
		cur = KeepAll
	}
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
			w := cur(r.Steps, r.Memo, g.Budget)
			a := m.Decide(w)
			r.ModelCalls++
			r.InputChars += w.Size
			if a.Note != "" {
				r.Memo = note(r.Memo, a.Note)
				continue // 同じ節をもう一度。訪問回数の上限が効く
			}
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

// #region fanout

// Fanout は同じ仕事を何人に配ったかの結果。
type Fanout struct {
	// Workers は配った人数。
	Workers int
	// Calls はモデルを呼んだ合計回数。
	Calls int
	// InputChars は渡した窓の合計。人数ぶん重ねて払う。
	InputChars int
	// Wall は壁時計。並列なので、いちばん遅い1人で決まる。
	Wall int
	// Serial は直列にやったときの壁時計。比べる相手になる。
	Serial int
	// Choices はモデルが選べたことの数。
	Choices int
	// WorkChars は子が生み出した記録の合計。親はこれを読まない。
	WorkChars int
}

// Spread は同じ仕事を n 人へ配って、費用と時間と選べることを数える。
//
// 子は真っ新な窓から始まるので、モデルも道具も毎回作り直す。
// 前置き(何をしてほしいか)も**人数ぶん渡し直す**ことになる。
// ここが費用の効くところで、人数に正比例して増える。
//
// 一方で選べることは増えない。n 本の答えが出るだけで、
// 子は互いを知らないので、混ぜて 1 本にはできないからだ。
func Spread(n int, brief []Step, newModel func() Model, newTools func() map[string]Tool, cfg LoopConfig) Fanout {
	if n < 1 {
		n = 1
	}
	f := Fanout{Workers: n, Choices: 1}
	for i := 0; i < n; i++ {
		r := Loop(newModel(), newTools(), cfg)
		f.Calls += r.ModelCalls
		f.InputChars += r.InputChars + totalSize(brief)
		f.WorkChars += totalSize(r.Steps)
		if r.ToolCalls > f.Wall {
			f.Wall = r.ToolCalls // いちばん遅い1人
		}
		f.Serial += r.ToolCalls
	}
	return f
}

// Integrate は配った結果を統合する側の負担。
//
// 親が受け取るのは要約だけなので、読む量は人数ぶんの要約で済む。
// 代わりに**現物を見ていない**。ここが安さの正体になる。
func Integrate(f Fanout, summary int) (read int, sawWork bool) {
	return f.Workers * summary, false
}

// #endregion fanout
