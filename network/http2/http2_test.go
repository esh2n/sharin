package http2

import "testing"

// TestHPACKRoundTrip はヘッダの圧縮・復元が一致することを確かめる。
func TestHPACKRoundTrip(t *testing.T) {
	enc := NewEncoder()
	dec := NewDecoder()
	headers := []Header{
		{":method", "GET"},
		{":path", "/"},
		{"host", "example.com"},
	}
	fields := enc.Encode(headers)
	got := dec.Decode(fields)
	if len(got) != len(headers) {
		t.Fatalf("got %d headers want %d", len(got), len(headers))
	}
	for i := range headers {
		if got[i] != headers[i] {
			t.Fatalf("header %d: got %v want %v", i, got[i], headers[i])
		}
	}
}

// TestHPACKCompressesRepeats はこの章の主眼その 1。二度目に同じヘッダを送ると
// 索引参照になって、バイト数が大きく減ることを固定する。
func TestHPACKCompressesRepeats(t *testing.T) {
	enc := NewEncoder()
	headers := []Header{
		{":method", "GET"},
		{"host", "example.com"},
		{"user-agent", "sharin/1.0 (a fairly long user agent string)"},
	}
	first := enc.Encode(headers)  // すべて literal(初出)
	second := enc.Encode(headers) // すべて索引参照

	raw := RawSize(headers)
	firstSize := EncodedSize(first)
	secondSize := EncodedSize(second)

	// 初回は literal なので raw と同程度、二度目は索引だけでずっと小さい。
	if secondSize >= firstSize {
		t.Fatalf("repeat should compress: first=%d second=%d", firstSize, secondSize)
	}
	// 二度目は各ヘッダ 1 バイト = ヘッダ数ぶんに縮む。
	if secondSize != len(headers) {
		t.Fatalf("second encode should be %d bytes (indices), got %d", len(headers), secondSize)
	}
	if secondSize >= raw {
		t.Fatalf("indexed size %d should be far below raw %d", secondSize, raw)
	}
}

// TestHeadOfLineBlocking はこの章の主眼その 2。大きな応答 1 つと小さな応答 2 つを
// 混ぜたとき、HTTP/1.1 では小さな応答が大きいものの後ろで待たされ、HTTP/2 では
// 多重化で先に完了することを固定する。
func TestHeadOfLineBlocking(t *testing.T) {
	// stream 0 が大(10 フレーム)、stream 1・2 が小(各 1 フレーム)。
	sizes := []int{10, 1, 1}

	h1 := CompletionTicksH1(sizes)
	h2 := CompletionTicksH2(sizes)

	// HTTP/1.1: 小さな応答は大きいものの後ろ。完了は 11, 12 tick。
	if h1[1] != 11 || h1[2] != 12 {
		t.Fatalf("H1 small streams should finish late: %v", h1)
	}
	// HTTP/2: 小さな応答は最初の順繰りで完了。2, 3 tick。
	if h2[1] != 2 || h2[2] != 3 {
		t.Fatalf("H2 small streams should finish early: %v", h2)
	}
	// 小さな応答は H2 の方がはるかに早く終わる。
	if h2[1] >= h1[1] || h2[2] >= h1[2] {
		t.Fatalf("H2 must beat H1 for small streams: h1=%v h2=%v", h1, h2)
	}
	// 全応答が終わる総 tick 数は同じ(仕事量は変わらない)。
	if h1[0] != 10 || max(h2) != 12 {
		t.Fatalf("totals: h1=%v h2=%v", h1, h2)
	}
}

func max(xs []int) int {
	m := 0
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

// TestMultiplexInterleaves はフレームが順繰りに交互配置されることを確かめる。
func TestMultiplexInterleaves(t *testing.T) {
	frames := Multiplex([]int{3, 1})
	// 期待順: s1,s2,s1,s1(順繰り。2 本目は 1 フレームで終わる)。
	wantStreams := []int{1, 2, 1, 1}
	if len(frames) != len(wantStreams) {
		t.Fatalf("got %d frames want %d", len(frames), len(wantStreams))
	}
	for i, w := range wantStreams {
		if frames[i].StreamID != w {
			t.Fatalf("frame %d: stream %d want %d", i, frames[i].StreamID, w)
		}
	}
	// 最後のフレームは stream1 の終端。
	last := frames[len(frames)-1]
	if !last.End || last.StreamID != 1 {
		t.Fatalf("last frame should end stream 1: %+v", last)
	}
}

func TestEmptyAndSingle(t *testing.T) {
	if len(Multiplex(nil)) != 0 {
		t.Fatal("no streams -> no frames")
	}
	if got := CompletionTicksH2([]int{5}); got[0] != 5 {
		t.Fatalf("single stream should finish at its size: %v", got)
	}
}
