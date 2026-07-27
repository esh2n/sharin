// Package bpe は BPE(byte-pair encoding)トークナイザを最小構成で実装する。
//
// LLM は文字列を直接は扱わず、トークン ID の列を受け取る。その切り方を
// データから学習するのが BPE で、「コーパスで最も頻出する隣接ペアを 1 つの
// トークンに併合する」を繰り返すだけの貪欲アルゴリズムでできている。
// ここでは基底シンボルを rune とし、空白を次の語に付けるチャンク分割
// (GPT-2 流)で、encode→decode が必ず元の文字列に戻ることを保証する。
package bpe

import "strings"

// #region types

// Pair は併合規則。学習順に適用される。
type Pair struct{ Left, Right string }

// Tokenizer は学習済みの併合規則と語彙を持つ。
// vocab[0] は未知シンボル用の <unk> に予約する。
type Tokenizer struct {
	merges []Pair
	vocab  []string
	ids    map[string]int
}

// #endregion types

// #region chunks

// chunks はテキストを「空白は次の語の先頭に付ける」単位に割る。
// "low lower" → ["low", " lower"]。どんな入力でも連結すれば元に戻るので、
// チャンク内だけで併合しても decode の忠実性が壊れない。
func chunks(text string) []string {
	var out []string
	var cur []rune
	for _, r := range text {
		if r == ' ' && len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// #endregion chunks

// #region train

// Train はコーパスから numMerges 回の併合規則を学習する。
// 各ステップで「最も頻出する隣接ペア」を選ぶ。同数のペアは辞書順の
// 小さい方を選び、実行のたびに同じ結果になる(決定的)。
func Train(corpus string, numMerges int) *Tokenizer {
	freq := map[string]int{}
	var order []string
	for _, c := range chunks(corpus) {
		if freq[c] == 0 {
			order = append(order, c)
		}
		freq[c]++
	}

	// 各チャンクをシンボル列(最初は rune 単位)に展開する。
	seqs := make([][]string, len(order))
	baseSet := map[string]bool{}
	for i, c := range order {
		for _, r := range c {
			seqs[i] = append(seqs[i], string(r))
			baseSet[string(r)] = true
		}
	}

	vocab := append([]string{"<unk>"}, sortedKeys(baseSet)...)
	var merges []Pair
	for len(merges) < numMerges {
		// 隣接ペアをチャンク頻度の重み付きで数える。
		count := map[Pair]int{}
		for i, s := range seqs {
			w := freq[order[i]]
			for j := 0; j+1 < len(s); j++ {
				count[Pair{s[j], s[j+1]}] += w
			}
		}
		best, ok := bestPair(count)
		if !ok {
			break // もう併合できる隣接ペアが無い
		}
		merges = append(merges, best)
		vocab = append(vocab, best.Left+best.Right)
		for i := range seqs {
			seqs[i] = applyMerge(seqs[i], best)
		}
	}

	ids := make(map[string]int, len(vocab))
	for i, v := range vocab {
		ids[v] = i
	}
	return &Tokenizer{merges: merges, vocab: vocab, ids: ids}
}

// bestPair は最多ペアを返す。同数は (Left, Right) の辞書順で決定的に選ぶ。
func bestPair(count map[Pair]int) (Pair, bool) {
	var best Pair
	bestN := 0
	for p, n := range count {
		if n > bestN || (n == bestN && less(p, best)) {
			best, bestN = p, n
		}
	}
	return best, bestN > 0
}

func less(a, b Pair) bool {
	if a.Left != b.Left {
		return a.Left < b.Left
	}
	return a.Right < b.Right
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// applyMerge はシンボル列の中の (Left, Right) 隣接をすべて 1 トークンに
// 置き換えた新しい列を返す。左から一度だけ走査する。
func applyMerge(s []string, p Pair) []string {
	out := make([]string, 0, len(s))
	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == p.Left && s[i+1] == p.Right {
			out = append(out, p.Left+p.Right)
			i += 2
		} else {
			out = append(out, s[i])
			i++
		}
	}
	return out
}

// #endregion train

// #region encode

// Tokens は学習した併合規則を順に適用してトークン文字列の列を返す。
// 学習時と同じ順で適用するので、同じ文字列は必ず同じ切り方になる。
func (t *Tokenizer) Tokens(text string) []string {
	var out []string
	for _, c := range chunks(text) {
		var s []string
		for _, r := range c {
			s = append(s, string(r))
		}
		for _, m := range t.merges {
			s = applyMerge(s, m)
		}
		out = append(out, s...)
	}
	return out
}

// Encode はテキストをトークン ID 列にする。学習コーパスに無かった rune は
// <unk>(ID 0) になる。実物は基底をバイトにすることで未知を根絶している
// (byte-level BPE)。ここでは教材のため rune 基底 + <unk> で単純化する。
func (t *Tokenizer) Encode(text string) []int {
	toks := t.Tokens(text)
	out := make([]int, len(toks))
	for i, tk := range toks {
		if id, ok := t.ids[tk]; ok {
			out[i] = id
		}
	}
	return out
}

// #endregion encode

// #region decode

// Decode はトークン ID 列を文字列に戻す。<unk> と範囲外は � になる。
func (t *Tokenizer) Decode(ids []int) string {
	var b strings.Builder
	for _, id := range ids {
		if id <= 0 || id >= len(t.vocab) {
			b.WriteString("�")
			continue
		}
		b.WriteString(t.vocab[id])
	}
	return b.String()
}

// #endregion decode

// Merges は学習した併合規則を適用順で返す。
func (t *Tokenizer) Merges() []Pair {
	out := make([]Pair, len(t.merges))
	copy(out, t.merges)
	return out
}

// Vocab は語彙(ID → トークン文字列)のコピーを返す。
func (t *Tokenizer) Vocab() []string {
	out := make([]string, len(t.vocab))
	copy(out, t.vocab)
	return out
}

// VocabSize は語彙数を返す。
func (t *Tokenizer) VocabSize() int { return len(t.vocab) }
