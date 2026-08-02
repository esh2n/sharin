package deadline

import "testing"

// 壊れたままの末端。何回呼んでも失敗する。
const broken = 1 << 20

func base(hops int, p Policy) Chain {
	// 予算 45 は末端 4 回ぶん。3 段・各段 3 回なら 9 回まで試せてしまうので、
	// 締め切りが効いているかどうかが結果に出る。
	return Chain{Hops: hops, Tries: 3, Cost: 10, Budget: 45, Fails: broken, Policy: p}
}

// 締め切りを置かないと、末端への呼び出しは段ごとに掛け算になる。
// #region multiply
func TestRetriesMultiplyPerHop(t *testing.T) {
	want := map[int]int{2: 3, 3: 9, 4: 27, 5: 81}
	for _, hops := range []int{2, 3, 4, 5} {
		c := base(hops, None)
		r := Run(c)
		t.Logf("%d 段・各段 %d 回: 末端 %3d 回 / 全段 %3d 回 / 経過 %4d",
			hops, c.Tries, r.Leaf, r.Total, r.Elapsed)
		if r.Leaf != want[hops] {
			t.Fatalf("%d 段で末端 %d 回。%d 回のはず", hops, r.Leaf, want[hops])
		}
		if r.Elapsed != r.Leaf*c.Cost {
			t.Fatalf("経過 %d が末端の回数と合わない", r.Elapsed)
		}
		if r.OK {
			t.Fatal("壊れた末端なのに成功している")
		}
	}
}

// #endregion multiply

// 1 段増やすと、末端への呼び出しは試行回数ぶん倍になる。
func TestOneMoreHopMultipliesByTries(t *testing.T) {
	for _, tries := range []int{2, 3, 4} {
		c := base(3, None)
		c.Tries = tries
		three := Run(c).Leaf
		c.Hops = 4
		four := Run(c).Leaf
		t.Logf("各段 %d 回: 3 段で %2d 回 → 4 段で %3d 回(%d 倍)", tries, three, four, four/three)
		if four != three*tries {
			t.Fatalf("段を増やしたとき %d 倍になっていない", tries)
		}
	}
}

// 段ごとに同じ長さの締め切りを持つと、入口が諦めたあとも下が働き続ける。
// #region wasted
func TestEachTimeoutKeepsWorkingAfterEntryGaveUp(t *testing.T) {
	each := Run(base(3, Each))
	pass := Run(base(3, Pass))
	t.Logf("段ごとに持つ: 末端 %2d 回 / 経過 %3d / 誰も待っていない仕事 %3d", each.Leaf, each.Elapsed, each.Wasted)
	t.Logf("下へ渡す    : 末端 %2d 回 / 経過 %3d / 誰も待っていない仕事 %3d", pass.Leaf, pass.Elapsed, pass.Wasted)

	if each.Wasted == 0 {
		t.Fatal("段ごとに持つ形で無駄仕事が出ていない")
	}
	if pass.Wasted != 0 {
		t.Fatalf("下へ渡したのに無駄仕事が %d 出ている", pass.Wasted)
	}
	if each.Elapsed <= pass.Elapsed {
		t.Fatal("段ごとに持つほうが早く終わってしまっている")
	}
	// 入口の締め切りそのものを超えている。渡す形は超えない
	budget := base(3, Each).Budget
	if each.Elapsed <= budget {
		t.Fatalf("段ごとに持つ形で、経過 %d が締め切り %d を超えていない", each.Elapsed, budget)
	}
	if pass.Elapsed > budget {
		t.Fatalf("渡す形で、経過 %d が締め切り %d を超えた", pass.Elapsed, budget)
	}
	t.Logf("入口の締め切りは %d。段ごとに持つ形は %d まで延びて、うち %d は誰も待っていない",
		budget, each.Elapsed, each.Wasted)
}

// #endregion wasted

// 締め切りを下へ渡すと、掛け算が予算で止まる。
// #region capped
func TestPassCapsTheMultiplication(t *testing.T) {
	for _, hops := range []int{2, 3, 4, 5} {
		none := Run(base(hops, None))
		pass := Run(base(hops, Pass))
		t.Logf("%d 段: 締め切り無し 末端 %3d 回・経過 %4d → 下へ渡す 末端 %2d 回・経過 %3d",
			hops, none.Leaf, none.Elapsed, pass.Leaf, pass.Elapsed)
		if pass.Elapsed > base(hops, Pass).Budget {
			t.Fatalf("%d 段で経過 %d が予算を超えた", hops, pass.Elapsed)
		}
		if pass.Leaf > none.Leaf {
			t.Fatalf("%d 段で渡したほうが呼び出しが増えた", hops)
		}
	}
	// 段数を増やしても、渡す形なら末端の回数は増えない
	a := Run(base(3, Pass)).Leaf
	b := Run(base(6, Pass)).Leaf
	if b > a {
		t.Fatalf("段数を倍にしたら末端が %d から %d へ増えた", a, b)
	}
}

// #endregion capped

// 予算が短すぎると、あと 1 回で通ったはずのものを落とす。
// #region tooshort
func TestTooShortBudgetDropsWhatWouldHavePassed(t *testing.T) {
	c := base(3, Pass)
	c.Fails = 4 // 5 回目で通る末端
	for _, budget := range []int{20, 40, 50, 60, 100} {
		c.Budget = budget
		r := Run(c)
		t.Logf("予算 %3d: 末端 %d 回 / 経過 %3d / 成功 %v", budget, r.Leaf, r.Elapsed, r.OK)
		if r.Elapsed > budget {
			t.Fatalf("予算 %d を超えて %d 使った", budget, r.Elapsed)
		}
	}
	c.Budget = 40
	if Run(c).OK {
		t.Fatal("5 回要るのに予算 40(4 回ぶん)で成功している")
	}
	c.Budget = 50
	if !Run(c).OK {
		t.Fatal("予算 50(5 回ぶん)で成功していない")
	}
}

// #endregion tooshort

// 3 つの方針を並べる。
// #region compare
func TestThreePolicies(t *testing.T) {
	t.Logf("             末端 経過 無駄")
	var rows []Result
	for _, p := range []Policy{None, Each, Pass} {
		r := Run(base(3, p))
		rows = append(rows, r)
	}
	for i, name := range []string{"締め切り無し", "段ごとに持つ", "下へ渡す    "} {
		t.Logf("%s %4d %4d %4d", name, rows[i].Leaf, rows[i].Elapsed, rows[i].Wasted)
	}
	none, each, pass := rows[0], rows[1], rows[2]
	// 締め切り無しがいちばん長い
	if none.Elapsed <= each.Elapsed || none.Elapsed <= pass.Elapsed {
		t.Fatal("締め切り無しがいちばん長くなっていない")
	}
	// 無駄仕事が出るのは、段ごとに持つ形だけ
	if none.Wasted != 0 || pass.Wasted != 0 || each.Wasted == 0 {
		t.Fatal("無駄仕事が出る形が「段ごとに持つ」だけになっていない")
	}
}

// #endregion compare

// 端の値で落ちない。
func TestEdges(t *testing.T) {
	if r := Run(Chain{}); r.Total != 0 {
		t.Fatal("空の連なりで壊れる")
	}
	if r := Run(Chain{Hops: 3, Tries: 0}); r.Total != 0 {
		t.Fatal("試行 0 回で壊れる")
	}
	// 1 段だけなら、入口がそのまま末端になる
	r := Run(Chain{Hops: 1, Tries: 3, Cost: 10, Budget: 100, Fails: 0, Policy: Pass})
	if r.Leaf != 1 || !r.OK {
		t.Fatalf("1 段で末端 %d 回・成功 %v", r.Leaf, r.OK)
	}
}
