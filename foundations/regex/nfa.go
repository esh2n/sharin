package regex

// nfa.go は AST を NFA(非決定性有限オートマトン)に変換する Thompson 構成と、
// その NFA を状態集合として動かすシミュレーションを実装する。
//
// NFA は「同じ入力に対して複数の状態に同時にいられる」オートマトン。ε(空)遷移で
// 入力を消費せず状態を移れるのが特徴だ。Thompson 構成は、正規表現の各演算子に対応する
// 小さな NFA 断片(fragment)を組み合わせて、木全体の NFA を機械的に作る。
//
// シミュレーションは「今いる状態の集合」を丸ごと持ち、入力を 1 文字読むごとに全状態を
// 並行に進める。バックトラッキングしないので、悪意ある入力でも入力長に比例した時間で済む。

// edge は NFA の遷移。eps は ε 遷移(入力を消費しない)、any は任意 1 文字(.)、
// それ以外は ch のリテラル 1 文字にマッチする。
type edge struct {
	to  int
	eps bool
	any bool
	ch  byte
}

// NFA は開始状態と受理状態、状態ごとの遷移表を持つ。Thompson 構成は「開始 1 つ・受理 1 つ」の
// 断片を組み合わせるので、完成した NFA も開始と受理が 1 つずつになる。
type NFA struct {
	start  int
	accept int
	trans  [][]edge // trans[state] = その状態から出る遷移
}

// #region build

type builder struct {
	trans [][]edge
}

func (b *builder) newState() int {
	b.trans = append(b.trans, nil)
	return len(b.trans) - 1
}
func (b *builder) link(from int, e edge) { b.trans[from] = append(b.trans[from], e) }

// frag は構成途中の NFA 断片(開始と受理が 1 つずつ)。
type frag struct{ start, accept int }

// Build は AST を Thompson 構成で NFA にする。各ノード型に対応する断片を作り、
// ε 遷移で繋いでいく。ここが「正規表現 → オートマトン」の心臓部。
func Build(n Node) *NFA {
	b := &builder{}
	f := b.build(n)
	return &NFA{start: f.start, accept: f.accept, trans: b.trans}
}

func (b *builder) build(n Node) frag {
	switch v := n.(type) {
	case Lit: // s --ch--> a
		s, a := b.newState(), b.newState()
		b.link(s, edge{to: a, ch: v.Ch})
		return frag{s, a}
	case Any: // s --.--> a
		s, a := b.newState(), b.newState()
		b.link(s, edge{to: a, any: true})
		return frag{s, a}
	case Empty: // s --ε--> a
		s, a := b.newState(), b.newState()
		b.link(s, edge{to: a, eps: true})
		return frag{s, a}
	case Concat: // L の受理 --ε--> R の開始
		l, r := b.build(v.L), b.build(v.R)
		b.link(l.accept, edge{to: r.start, eps: true})
		return frag{l.start, r.accept}
	case Alt: // 新開始から両方へ ε、両受理から新受理へ ε
		s, a := b.newState(), b.newState()
		l, r := b.build(v.L), b.build(v.R)
		b.link(s, edge{to: l.start, eps: true})
		b.link(s, edge{to: r.start, eps: true})
		b.link(l.accept, edge{to: a, eps: true})
		b.link(r.accept, edge{to: a, eps: true})
		return frag{s, a}
	case Star: // 0 回以上: 入口で本体を飛ばせ、本体後は戻れる
		s, a := b.newState(), b.newState()
		x := b.build(v.X)
		b.link(s, edge{to: x.start, eps: true})
		b.link(s, edge{to: a, eps: true})              // 0 回(本体を飛ばす)
		b.link(x.accept, edge{to: x.start, eps: true}) // 繰り返す
		b.link(x.accept, edge{to: a, eps: true})       // 抜ける
		return frag{s, a}
	case Plus: // 1 回以上: 必ず本体を通り、その後は繰り返しか脱出
		x := b.build(v.X)
		a := b.newState()
		b.link(x.accept, edge{to: x.start, eps: true})
		b.link(x.accept, edge{to: a, eps: true})
		return frag{x.start, a}
	case Quest: // 0 か 1 回: 本体を通るか飛ばすか
		s, a := b.newState(), b.newState()
		x := b.build(v.X)
		b.link(s, edge{to: x.start, eps: true})
		b.link(s, edge{to: a, eps: true})
		b.link(x.accept, edge{to: a, eps: true})
		return frag{s, a}
	default:
		panic("regex: 未知の AST ノード")
	}
}

// #endregion build

// #region sim

// epsClosure は与えた状態集合から ε 遷移だけで到達できる状態を全て加えた集合を返す。
// 「入力を消費せずに今いられる全状態」を求める操作で、シミュレーションの各ステップで使う。
func (n *NFA) epsClosure(states map[int]bool) map[int]bool {
	stack := make([]int, 0, len(states))
	closure := make(map[int]bool, len(states))
	for s := range states {
		closure[s] = true
		stack = append(stack, s)
	}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, e := range n.trans[s] {
			if e.eps && !closure[e.to] {
				closure[e.to] = true
				stack = append(stack, e.to)
			}
		}
	}
	return closure
}

// Match は入力全体がこの NFA にマッチするかを返す。今いる状態集合を ε 閉包で広げ、
// 入力を 1 文字読むごとに「その文字で進める遷移」を全状態について集め、また ε 閉包する。
// バックトラッキングは無い——集合を並行に進めるだけなので、時間は入力長に比例する。
func (n *NFA) Match(input string) bool {
	cur := n.epsClosure(map[int]bool{n.start: true})
	for i := 0; i < len(input); i++ {
		c := input[i]
		next := make(map[int]bool)
		for s := range cur {
			for _, e := range n.trans[s] {
				if e.eps {
					continue
				}
				if e.any || e.ch == c {
					next[e.to] = true
				}
			}
		}
		cur = n.epsClosure(next)
		if len(cur) == 0 {
			return false // どの状態にもいられない = 詰み
		}
	}
	return cur[n.accept]
}

// NumStates は NFA の状態数(表示・検査用)。
func (n *NFA) NumStates() int { return len(n.trans) }

// Regexp はコンパイル済みの正規表現。NFA を持ち、必要になったら DFA も作る。
type Regexp struct {
	nfa *NFA
	dfa *DFA
}

// Compile は正規表現をパースし、Thompson 構成で NFA にする。
func Compile(pattern string) (*Regexp, error) {
	ast, err := Parse(pattern)
	if err != nil {
		return nil, err
	}
	return &Regexp{nfa: Build(ast)}, nil
}

// Match は NFA シミュレーションでマッチ判定する。
func (r *Regexp) Match(input string) bool { return r.nfa.Match(input) }

// MatchDFA は NFA を DFA に変換してからマッチ判定する(初回のみ変換)。
func (r *Regexp) MatchDFA(input string) bool {
	if r.dfa == nil {
		r.dfa = ToDFA(r.nfa)
	}
	return r.dfa.Match(input)
}

// #endregion sim
