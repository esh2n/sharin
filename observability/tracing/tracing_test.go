package tracing

import "testing"

// buildSampleTrace は 1 リクエストの典型的なトレースを組む。
//
//	gateway(0-100)
//	├─ auth(10-30)
//	└─ handler(30-95)
//	   ├─ inventory(35-50)
//	   └─ billing(50-90)   ← ここが遅い(クリティカルパス)
func buildSampleTrace() *Tracer {
	t := New(NewIDGen(0))
	root := t.StartRoot("gateway")

	t.Advance(10)
	auth := t.Start(root, "auth")
	t.Advance(20)
	t.End(auth) // auth 10-30

	handler := t.Start(root, "handler") // 30-
	t.Advance(5)
	inv := t.Start(handler, "inventory") // 35-
	t.Advance(15)
	t.End(inv) // 35-50
	bill := t.Start(handler, "billing")
	t.Advance(40)
	t.End(bill) // 50-90
	t.Advance(5)
	t.End(handler) // 30-95
	t.Advance(5)
	t.End(root) // 0-100
	return t
}

func TestTraceIDPropagates(t *testing.T) {
	tr := buildSampleTrace()
	spans := tr.Spans()
	if len(spans) != 5 {
		t.Fatalf("got %d spans want 5", len(spans))
	}
	// 全 span が同じ trace_id を共有する。
	tid := spans[0].TraceID
	for _, s := range spans {
		if s.TraceID != tid {
			t.Fatalf("span %q trace_id %d != %d", s.Name, s.TraceID, tid)
		}
	}
}

func TestParentChildLinks(t *testing.T) {
	tr := buildSampleTrace()
	byName := map[string]Span{}
	for _, s := range tr.Spans() {
		byName[s.Name] = s
	}
	// gateway は根(親なし)。
	if byName["gateway"].ParentID != 0 {
		t.Fatalf("gateway should be root")
	}
	// auth と handler の親は gateway。
	if byName["auth"].ParentID != byName["gateway"].SpanID {
		t.Fatal("auth parent should be gateway")
	}
	if byName["handler"].ParentID != byName["gateway"].SpanID {
		t.Fatal("handler parent should be gateway")
	}
	// billing の親は handler。
	if byName["billing"].ParentID != byName["handler"].SpanID {
		t.Fatal("billing parent should be handler")
	}
}

func TestDurations(t *testing.T) {
	tr := buildSampleTrace()
	byName := map[string]Span{}
	for _, s := range tr.Spans() {
		byName[s.Name] = s
	}
	cases := map[string]int{"gateway": 100, "auth": 20, "handler": 65, "inventory": 15, "billing": 40}
	for name, want := range cases {
		if got := byName[name].Duration(); got != want {
			t.Fatalf("%s duration got %d want %d", name, got, want)
		}
	}
}

func TestBuildTree(t *testing.T) {
	tr := buildSampleTrace()
	root := BuildTree(tr.Spans())
	if root == nil || root.Span.Name != "gateway" {
		t.Fatal("root should be gateway")
	}
	if len(root.Children) != 2 {
		t.Fatalf("gateway should have 2 children, got %d", len(root.Children))
	}
	// handler の子は 2 つ(inventory, billing)。
	var handler *Node
	for _, c := range root.Children {
		if c.Span.Name == "handler" {
			handler = c
		}
	}
	if handler == nil || len(handler.Children) != 2 {
		t.Fatal("handler should have 2 children")
	}
}

// TestCriticalPath はこの章の主眼。全体の所要時間を決めているのは、
// 各段で最も遅く終わる子の連なり。ここでは gateway→handler→billing。
func TestCriticalPath(t *testing.T) {
	tr := buildSampleTrace()
	root := BuildTree(tr.Spans())
	path := CriticalPath(root)
	want := []string{"gateway", "handler", "billing"}
	if len(path) != len(want) {
		t.Fatalf("path len got %d want %d", len(path), len(want))
	}
	for i, name := range want {
		if path[i].Name != name {
			t.Fatalf("path[%d] got %q want %q", i, path[i].Name, name)
		}
	}
}

func TestInjectExtractRoundTrip(t *testing.T) {
	sc := SpanContext{TraceID: 42, SpanID: 1234}
	h := Inject(sc)
	if h != "42-1234" {
		t.Fatalf("inject got %q want 42-1234", h)
	}
	got, ok := Extract(h)
	if !ok || got != sc {
		t.Fatalf("extract got %+v ok=%v want %+v", got, ok, sc)
	}
}

func TestExtractRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "abc", "12-", "-34", "12", "1x-2", "1-2x"} {
		if _, ok := Extract(bad); ok {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

// TestPropagationStitchesServices は、境界を越えた span が
// Inject/Extract を経ても 1 つのトレースに繋がることを示す。
func TestPropagationStitchesServices(t *testing.T) {
	// サービス A: 入口 span を作り、ヘッダに詰めて「送る」。
	a := New(NewIDGen(0))
	root := a.StartRoot("service-a")
	header := Inject(root)

	// サービス B: ヘッダから復元し、その子として span を作る。
	ctx, ok := Extract(header)
	if !ok {
		t.Fatal("extract failed")
	}
	b := New(NewIDGen(100)) // 別プロセス = 別の ID 生成器
	child := b.Start(ctx, "service-b")
	b.End(child)

	// B の span は A の trace_id を引き継ぎ、親は A の入口 span。
	bs := b.Spans()[0]
	if bs.TraceID != root.TraceID {
		t.Fatalf("trace_id not propagated: %d != %d", bs.TraceID, root.TraceID)
	}
	if bs.ParentID != root.SpanID {
		t.Fatalf("parent not linked: %d != %d", bs.ParentID, root.SpanID)
	}
}

func TestEndUnknownSpanNoop(t *testing.T) {
	tr := New(NewIDGen(0))
	tr.End(SpanContext{TraceID: 1, SpanID: 999}) // 開いていない span
	if len(tr.Spans()) != 0 {
		t.Fatal("ending unknown span should be a no-op")
	}
}
