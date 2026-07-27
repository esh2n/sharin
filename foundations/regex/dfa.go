package regex

import (
	"sort"
	"strconv"
	"strings"
)

// dfa.go は NFA を DFA(決定性有限オートマトン)に変換する部分集合構成
// (subset / powerset construction)と、DFA でのマッチを実装する。
//
// NFA は同時に複数状態にいられるが、DFA は「今どの状態にいるか」が常に 1 つに決まる。
// 変換の要は、NFA の「状態集合」を DFA の「1 つの状態」と見なすことだ。ε 遷移も消える
// (構成時に閉包へ畳み込む)。結果、マッチは入力 1 文字あたり表引き 1 回で済み、NFA
// シミュレーションのような集合操作が要らない。代わりに状態数が最悪で指数的に増えうる——
// 時間(実行)とメモリ(状態表)のトレードオフになる。

// #region dfa

// DFA は決定的な遷移表。alphabet はパターンに現れるリテラル文字の集合。
// lit[state][c] はその文字での遷移、any[state] は alphabet 外の任意文字での遷移(. 用)。
type DFA struct {
	start    int
	accept   map[int]bool
	lit      []map[byte]int // state -> (文字 -> 次状態)
	any      []int          // state -> 次状態(alphabet 外の文字。無ければ -1)
	alphabet map[byte]bool
}

// ToDFA は NFA を部分集合構成で DFA に変換する。
//
// 開始は NFA 開始の ε 閉包。各 DFA 状態(= NFA 状態の集合)について、alphabet の各文字と
// 「alphabet 外(any)」で遷移先の集合を求め、未知の集合なら新しい DFA 状態にする。
// NFA 状態集合に受理状態が含まれれば、その DFA 状態は受理。
func ToDFA(n *NFA) *DFA {
	alphabet := collectAlphabet(n)
	d := &DFA{accept: map[int]bool{}, alphabet: alphabet}

	ids := map[string]int{} // 状態集合のキー -> DFA 状態 id
	var sets []map[int]bool

	intern := func(set map[int]bool) int {
		key := setKey(set)
		if id, ok := ids[key]; ok {
			return id
		}
		id := len(sets)
		ids[key] = id
		sets = append(sets, set)
		d.lit = append(d.lit, map[byte]int{})
		d.any = append(d.any, -1)
		if set[n.accept] {
			d.accept[id] = true
		}
		return id
	}

	d.start = intern(n.epsClosure(map[int]bool{n.start: true}))

	for work := 0; work < len(sets); work++ {
		set := sets[work]
		// alphabet の各リテラル文字での遷移。any 辺も含めて動く。
		for c := range alphabet {
			if next := n.epsClosure(move(n, set, c, false)); len(next) > 0 {
				d.lit[work][c] = intern(next)
			}
		}
		// alphabet 外の文字での遷移(. の any 辺のみ)。
		if next := n.epsClosure(move(n, set, 0, true)); len(next) > 0 {
			d.any[work] = intern(next)
		}
	}
	return d
}

// move は状態集合から、文字 c で進める先の集合を返す。
// anyOnly が true なら any 辺(.)だけをたどる(alphabet 外の文字の遷移)。
// false なら「c に一致するリテラル辺」と「any 辺」の両方をたどる。
func move(n *NFA, set map[int]bool, c byte, anyOnly bool) map[int]bool {
	out := map[int]bool{}
	for s := range set {
		for _, e := range n.trans[s] {
			if e.eps {
				continue
			}
			if anyOnly {
				if e.any {
					out[e.to] = true
				}
			} else if e.any || e.ch == c {
				out[e.to] = true
			}
		}
	}
	return out
}

// collectAlphabet はパターンに現れるリテラル文字を集める(DFA の入力記号集合)。
func collectAlphabet(n *NFA) map[byte]bool {
	al := map[byte]bool{}
	for _, edges := range n.trans {
		for _, e := range edges {
			if !e.eps && !e.any {
				al[e.ch] = true
			}
		}
	}
	return al
}

// Match は入力全体が DFA にマッチするかを返す。1 文字ごとに表引き 1 回。
// alphabet 内の文字は lit で、外の文字は any で遷移する。行き先が無ければ即不一致。
func (d *DFA) Match(input string) bool {
	st := d.start
	for i := 0; i < len(input); i++ {
		c := input[i]
		var next int
		if d.alphabet[c] {
			n, ok := d.lit[st][c]
			if !ok {
				return false
			}
			next = n
		} else {
			if d.any[st] < 0 {
				return false
			}
			next = d.any[st]
		}
		st = next
	}
	return d.accept[st]
}

// NumStates は DFA の状態数(NFA との比較・表示用)。
func (d *DFA) NumStates() int { return len(d.lit) }

// setKey は状態集合を決定的な文字列キーにする(同じ集合を同じ DFA 状態に対応づけるため)。
func setKey(set map[int]bool) string {
	xs := make([]int, 0, len(set))
	for s := range set {
		xs = append(xs, s)
	}
	sort.Ints(xs)
	parts := make([]string, len(xs))
	for i, s := range xs {
		parts[i] = strconv.Itoa(s)
	}
	return strings.Join(parts, ",")
}

// #endregion dfa
