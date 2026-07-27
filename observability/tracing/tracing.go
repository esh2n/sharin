// Package tracing は分散トレーシングの中核(span・trace_id・親子関係・伝播)を
// 最小構成で実装する。
//
// 1 つのリクエストは、いくつものサービスを渡り歩く。API ゲートウェイが認証を
// 呼び、認証がユーザ DB を引き、本体が在庫と課金を呼ぶ。全体が 800ms かかった
// とき、どこで時間を使ったのかは、1 台のログを見ても分からない。分散トレーシングは、
// リクエストに 1 つの trace_id を振り、各サービスでの処理を span として記録する。
// span は trace_id と親 span の id を持ち、どのサービスがどの呼び出しの子かを繋ぐ。
// 境界を越えるときは trace_id と現在の span_id をヘッダで伝える(伝播)。集めた
// span を親子で組み立て直すと、リクエスト全体の時間の使われ方が 1 本の木になる。
package tracing

import "strings"

// #region model

// ID は trace / span の識別子。
type ID = uint64

// IDGen は決定的な ID 生成器(テスト再現性のため実乱数を使わない)。
type IDGen struct{ n uint64 }

// NewIDGen は seed から ID 生成器を作る。
func NewIDGen(seed uint64) *IDGen { return &IDGen{n: seed} }

// next は次の ID を返す(0 は「親なし」を表すので 1 から始める)。
func (g *IDGen) next() ID {
	g.n++
	return g.n
}

// Span は 1 つのサービスでの 1 区間の処理。
type Span struct {
	TraceID  ID     // このリクエスト全体で共通
	SpanID   ID     // この span 固有
	ParentID ID     // 親 span(0 なら根 = リクエストの入口)
	Name     string // 何をしていたか(例 "auth.verify")
	Start    int    // 開始の論理時刻
	End      int    // 終了の論理時刻
}

// Duration は span の所要時間。
func (s Span) Duration() int { return s.End - s.Start }

// SpanContext は境界を越えて伝播する最小情報。
// trace_id で「どのリクエストか」、span_id で「誰が親か」を伝える。
type SpanContext struct {
	TraceID ID
	SpanID  ID
}

// #endregion model

// #region tracer

// Tracer は span を開始・終了し、完了した span を集めるコレクタ役。
// 時刻は論理時計(Advance で進める)で、テストと決定性のため実時計を使わない。
type Tracer struct {
	ids  *IDGen
	now  int
	open map[ID]*Span // 進行中の span(SpanID → span)
	done []Span       // 完了した span
}

// New は ID 生成器を注入して Tracer を作る。
func New(ids *IDGen) *Tracer {
	return &Tracer{ids: ids, open: make(map[ID]*Span)}
}

// Advance は論理時計を d だけ進める。
func (t *Tracer) Advance(d int) { t.now += d }

// StartRoot は新しいトレースを開始する(親なし = リクエストの入口)。
func (t *Tracer) StartRoot(name string) SpanContext {
	tid := t.ids.next()
	sid := t.ids.next()
	t.open[sid] = &Span{TraceID: tid, SpanID: sid, ParentID: 0, Name: name, Start: t.now}
	return SpanContext{TraceID: tid, SpanID: sid}
}

// Start は parent の下に子 span を開始する。
// trace_id は親から受け継ぎ、親の span_id を ParentID に記録する。
func (t *Tracer) Start(parent SpanContext, name string) SpanContext {
	sid := t.ids.next()
	t.open[sid] = &Span{
		TraceID:  parent.TraceID, // 同じトレース
		SpanID:   sid,
		ParentID: parent.SpanID, // 親を指す
		Name:     name,
		Start:    t.now,
	}
	return SpanContext{TraceID: parent.TraceID, SpanID: sid}
}

// End は span を閉じ、完了一覧に移す。
func (t *Tracer) End(sc SpanContext) {
	s, ok := t.open[sc.SpanID]
	if !ok {
		return
	}
	s.End = t.now
	t.done = append(t.done, *s)
	delete(t.open, sc.SpanID)
}

// Spans は完了した span 一覧を返す(集めた生データ)。
func (t *Tracer) Spans() []Span { return t.done }

// Inject は境界を越えるため SpanContext をヘッダ文字列にする("traceid-spanid")。
// 実物は W3C Trace Context の traceparent ヘッダ。ここでは最小形。
func Inject(sc SpanContext) string {
	return itoa(sc.TraceID) + "-" + itoa(sc.SpanID)
}

// Extract はヘッダ文字列を SpanContext に戻す。壊れていれば false。
func Extract(s string) (SpanContext, bool) {
	i := strings.IndexByte(s, '-')
	if i <= 0 || i == len(s)-1 {
		return SpanContext{}, false
	}
	tid, ok1 := atoi(s[:i])
	sid, ok2 := atoi(s[i+1:])
	if !ok1 || !ok2 {
		return SpanContext{}, false
	}
	return SpanContext{TraceID: tid, SpanID: sid}, true
}

// #endregion tracer

// #region tree

// Node は組み立て直したトレース木の 1 ノード。
type Node struct {
	Span     Span
	Children []*Node
}

// BuildTree は span 一覧を親子で木に組み立てる。根(ParentID==0)を返す。
func BuildTree(spans []Span) *Node {
	byID := make(map[ID]*Node, len(spans))
	for _, s := range spans {
		byID[s.SpanID] = &Node{Span: s}
	}
	var root *Node
	for _, s := range spans {
		n := byID[s.SpanID]
		if s.ParentID == 0 {
			root = n
			continue
		}
		if p, ok := byID[s.ParentID]; ok {
			p.Children = append(p.Children, n)
		}
	}
	return root
}

// CriticalPath は根から「最も遅く終わる子」を辿る鎖を返す。
// 親は子(並列に走ることもある)を待って初めて終われるので、全体の所要時間を
// 決めているのは、各段でいちばん最後に終わる子の連なり = クリティカルパスだ。
// ここを速くしない限り、他をいくら速くしても全体は縮まない。
func CriticalPath(root *Node) []Span {
	var path []Span
	for n := root; n != nil; {
		path = append(path, n.Span)
		var slowest *Node
		for _, c := range n.Children {
			if slowest == nil || c.Span.End > slowest.Span.End {
				slowest = c
			}
		}
		n = slowest
	}
	return path
}

// #endregion tree

// itoa / atoi は strconv を避けた最小実装(非負のみ)。
func itoa(n ID) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func atoi(s string) (ID, bool) {
	if len(s) == 0 {
		return 0, false
	}
	var n ID
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + ID(c-'0')
	}
	return n, true
}
