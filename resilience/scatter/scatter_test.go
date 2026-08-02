package scatter

import "testing"

// 1 台あたりの応答は、たまに遅い。
func TestTookHasATail(t *testing.T) {
	slow := 0
	for i := range 100 {
		if Took(i, 0) >= Slow {
			slow++
		}
	}
	if slow == 0 || slow > 20 {
		t.Fatalf("遅い応答が %d 台。裾のある分布になっていない", slow)
	}
	// 同じ台でも本数が違えば別の値になる。投げ直しに意味が出る条件
	same := 0
	for i := range 100 {
		if Took(i, 0) == Took(i, 1) {
			same++
		}
	}
	if same > 50 {
		t.Fatalf("1 本目と 2 本目が %d 台で同じ。投げ直す意味が無い", same)
	}
}

// 全部揃うまで待つ形は、台数を増やすほど遅い1台を踏みやすくなる。
// #region grow
func TestAllGetsSlowerWithMoreNodes(t *testing.T) {
	prev := 0
	for _, n := range []int{1, 5, 20, 100} {
		r := All(n)
		t.Logf("台数 %3d: 揃うまで %3d / 投げた %3d / 遅い台 %d / 誰かが遅い割合 %d.%02d%%",
			n, r.Wait, r.Sent, r.Slows, Tail(n)/100, Tail(n)%100)
		if r.Wait < prev {
			t.Fatalf("台数 %d で揃うまでが縮んだ", n)
		}
		prev = r.Wait
		if r.Sent != n {
			t.Fatalf("台数 %d なのに %d 本投げている", n, r.Sent)
		}
	}
	// 100 台では、いちばん遅い1台が全体を決める
	if All(100).Wait != Slow {
		t.Fatalf("100 台で遅い応答が全体を決めていない")
	}
}

// #endregion grow

// 誰か1台が遅い割合は、台数とともに 1 に近づく。
func TestTailApproachesOne(t *testing.T) {
	if Tail(1) != 500 {
		t.Fatalf("1 台の裾が %d。5%% にならない", Tail(1))
	}
	if Tail(20) < 5000 || Tail(20) > 7000 {
		t.Fatalf("20 台の裾が %d。5 割から 7 割の間に入らない", Tail(20))
	}
	if Tail(100) < 9900 {
		t.Fatalf("100 台の裾が %d。ほぼ確実にならない", Tail(100))
	}
}

// 揃うのを待たずに打ち切ると、遅い台を待たなくて済む。
// 代わりに、集まる答えが減る。
// #region cut
func TestFirstKTradesAnswersForTime(t *testing.T) {
	const n = 100
	all := All(n)
	for _, k := range []int{50, 90, 99, 100} {
		r := FirstK(n, k)
		t.Logf("%3d 台中 %3d 揃えば打ち切り: 揃うまで %3d / 集まった答え %3d / 投げた %d",
			n, k, r.Wait, r.Got, r.Sent)
		if r.Got != k {
			t.Fatalf("k=%d なのに %d 件集まっている", k, r.Got)
		}
		if r.Sent != n {
			t.Fatalf("打ち切っても投げた本数は減らないはずが %d 本", r.Sent)
		}
		if r.Wait > all.Wait {
			t.Fatalf("k=%d で全部待つより遅くなった", k)
		}
	}
	// 全部揃えるまで待つのと同じ k は、全部待つのと同じ結果になる
	if FirstK(n, n).Wait != all.Wait {
		t.Fatal("k=n が全部待つのと一致しない")
	}
	// 打ち切ると、遅い台を踏まずに済む
	if FirstK(n, 90).Wait >= Slow {
		t.Fatal("9 割で打ち切っても遅い応答を待っている")
	}
}

// #endregion cut

// 遅い台にだけ 2 本目を投げると、答えを減らさずに裾を切れる。
// 代わりに投げる本数が増える。
// #region hedge
func TestHedgeBuysTimeWithLoad(t *testing.T) {
	const n = 100
	all := All(n)
	h := Hedged(n, Fast+FastWidth)
	t.Logf("全部待つ  : 揃うまで %3d / 投げた %3d / 集まった答え %d", all.Wait, all.Sent, all.Got)
	t.Logf("2 本目あり: 揃うまで %3d / 投げた %3d / 集まった答え %d", h.Wait, h.Sent, h.Got)
	t.Logf("増えた本数: %d 本(%d%%)", h.Sent-all.Sent, (h.Sent-all.Sent)*100/all.Sent)

	if h.Wait >= all.Wait {
		t.Fatalf("2 本目を投げても揃うまでが縮んでいない(%d → %d)", all.Wait, h.Wait)
	}
	if h.Got != all.Got {
		t.Fatalf("答えの数が変わった(%d → %d)", all.Got, h.Got)
	}
	if h.Sent <= all.Sent {
		t.Fatal("2 本目を投げたのに本数が増えていない")
	}
	// 増えるのは遅かった台のぶんだけで、全部が倍になるわけではない
	if h.Sent > all.Sent*3/2 {
		t.Fatalf("投げた本数が %d 本。遅い台のぶんに収まっていない", h.Sent)
	}
}

// #endregion hedge

// 待つ時刻を早めるほど裾は切れるが、投げる本数は増える。
func TestHedgeAfterTradesLoadForTime(t *testing.T) {
	const n = 100
	prevWait, prevSent := 0, 1<<30
	for _, after := range []int{5, 20, 50, 150} {
		r := Hedged(n, after)
		t.Logf("%3d を過ぎたら 2 本目: 揃うまで %3d / 投げた %3d", after, r.Wait, r.Sent)
		if r.Wait < prevWait {
			t.Fatalf("after=%d で揃うまでが縮んだ", after)
		}
		if r.Sent > prevSent {
			t.Fatalf("after=%d で投げた本数が増えた", after)
		}
		prevWait, prevSent = r.Wait, r.Sent
	}
}

// 打ち切りと 2 本目は、買っているものが違う。
// #region compare
func TestCutAndHedgeBuyDifferentThings(t *testing.T) {
	const n = 100
	all := All(n)
	cut := FirstK(n, 90)
	hedge := Hedged(n, Fast+FastWidth)

	if cut.Got >= all.Got {
		t.Fatal("打ち切ったのに答えが減っていない")
	}
	if cut.Sent != all.Sent {
		t.Fatal("打ち切ると投げた本数が変わってしまっている")
	}
	if hedge.Got != all.Got {
		t.Fatal("2 本目で答えが減ってしまっている")
	}
	if hedge.Sent == all.Sent {
		t.Fatal("2 本目で本数が増えていない")
	}
	t.Logf("         揃うまで 答え 投げた")
	t.Logf("全部待つ   %5d %4d %5d", all.Wait, all.Got, all.Sent)
	t.Logf("打ち切る   %5d %4d %5d", cut.Wait, cut.Got, cut.Sent)
	t.Logf("2 本目     %5d %4d %5d", hedge.Wait, hedge.Got, hedge.Sent)
}

// #endregion compare

// 台数 0 と k=0 で落ちない。
func TestEdges(t *testing.T) {
	if r := All(0); r.Wait != 0 || r.Sent != 0 {
		t.Fatal("台数 0 で壊れる")
	}
	if r := FirstK(0, 0); r.Wait != 0 {
		t.Fatal("k=0 で壊れる")
	}
	if r := FirstK(5, 99); r.Got != 5 {
		t.Fatal("k が台数を超えたとき台数に丸めていない")
	}
	if r := Hedged(0, 10); r.Sent != 0 {
		t.Fatal("台数 0 の 2 本目で壊れる")
	}
}
