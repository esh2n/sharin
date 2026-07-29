package oom

import "testing"

// この章の中心その1。断るか、あとで殺すか。オーバーコミットはその取り替え。
func TestOvercommitTradesRefusalForAKill(t *testing.T) {
	// 物理 1000。4つのプロセスがそれぞれ 400 を申し込む。
	build := func(overcommit bool) *System {
		s := New(1000, overcommit)
		for _, n := range []string{"a", "b", "c", "d"} {
			s.Add(n, 0)
		}
		return s
	}

	// 切っていれば、3つめの申し込みで断られる。誰も死なない。
	off := build(false)
	var refusedAt int
	for i, n := range []string{"a", "b", "c", "d"} {
		if err := off.Reserve(n, 400); err != nil {
			refusedAt = i + 1
			break
		}
	}
	t.Logf("断る側:   %d つめの申し込みで断られた / 殺した数 %d", refusedAt, len(off.Kills))
	if refusedAt != 3 || len(off.Kills) != 0 {
		t.Fatalf("refusedAt=%d kills=%d", refusedAt, len(off.Kills))
	}

	// 入れていれば、4つとも通る。予約の合計は物理の 1.6 倍になる。
	on := build(true)
	for _, n := range []string{"a", "b", "c", "d"} {
		if err := on.Reserve(n, 400); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	t.Logf("殺す側:   予約 %d / 物理 %d (%.1f 倍)", on.Reserved(), on.Total,
		float64(on.Reserved())/float64(on.Total))
	if on.Reserved() != 1600 {
		t.Fatalf("予約 %d", on.Reserved())
	}

	// 触り始めると、どこかで足りなくなる。
	for _, n := range []string{"a", "b", "c", "d"} {
		err := on.Touch(n, 400)
		if err != nil {
			t.Logf("          %s を触ったところで %v", n, err)
		}
		if len(on.Kills) > 0 {
			t.Logf("          殺したのは %s(触ったのは %s)", on.Kills[0].Victim, on.Kills[0].Requester)
			break
		}
	}
	if len(on.Kills) == 0 {
		t.Fatal("誰も死んでいない")
	}
	// 断られるのは申し込んだ本人だが、殺されるのは本人とは限らない。
	if on.Kills[0].Victim == on.Kills[0].Requester {
		t.Log("(この筋書きでは本人が選ばれた)")
	}
}

// この章の中心その2。殺す相手は「足りなくした本人」ではなく「殺して効く相手」。
func TestVictimIsChosenByScoreNotByBlame(t *testing.T) {
	build := func(policy Policy) *System {
		s := New(1000, true)
		s.Policy = policy
		s.Add("大物", 0)
		s.Add("小物", 0)
		// 大物は 800 抱えている。小物はまだ何も持っていない。
		s.Touch("大物", 800)
		return s
	}

	// 本人を殺す方式: 小物を殺しても 100 しか空かないので、何度も殺すことになる。
	byBlame := build(Requester)
	byBlame.Touch("小物", 100)
	errBlame := byBlame.Touch("小物", 200) // 800 + 100 + 200 > 1000

	// 効く相手を殺す方式: 大物を1つ殺せば 800 空く。
	byScore := build(Biggest)
	byScore.Touch("小物", 100)
	errScore := byScore.Touch("小物", 200)

	t.Logf("本人を殺す:   殺した数 %d / 空いた量 %d / 結果 %v",
		len(byBlame.Kills), freed(byBlame), errBlame)
	t.Logf("効く相手:     殺した数 %d / 空いた量 %d / 結果 %v",
		len(byScore.Kills), freed(byScore), errScore)

	// 本人を殺すと、要求した本人が死ぬので仕事は進まない。
	if errBlame == nil {
		t.Error("本人を殺したのに要求が通った")
	}
	// 効く相手を殺せば、1回で足りて要求も通る。
	if errScore != nil {
		t.Errorf("効く相手を殺しても通らない: %v", errScore)
	}
	if len(byScore.Kills) != 1 || byScore.Kills[0].Victim != "大物" {
		t.Fatalf("%+v", byScore.Kills)
	}
	if freed(byScore) <= freed(byBlame) {
		t.Errorf("空いた量: %d, %d", freed(byBlame), freed(byScore))
	}
	// 足りなくしたのは小物なのに、死んだのは大物。
	if byScore.Kills[0].Requester != "小物" || byScore.Kills[0].Victim != "大物" {
		t.Errorf("%+v", byScore.Kills[0])
	}
}

func freed(s *System) int {
	n := 0
	for _, k := range s.Kills {
		n += k.Freed
	}
	return n
}

// この章の中心その3。oom_score_adj で選ばれ方が変わる。
func TestScoreAdjChangesTheVictim(t *testing.T) {
	build := func(adj int) *System {
		s := New(1000, true)
		s.Add("db", adj) // いちばん大きいが、殺されたくない
		s.Add("web", 0)
		s.Touch("db", 600)
		s.Touch("web", 300)
		return s
	}

	plain := build(0)
	for _, sc := range plain.Scores() {
		t.Logf("adj=0      %-4s score %4d", sc.Name, sc.Score)
	}
	plain.Add("job", 0)
	plain.Touch("job", 200) // 600 + 300 + 200 > 1000

	immune := build(-1000)
	for _, sc := range immune.Scores() {
		t.Logf("adj=-1000  %-4s score %4d", sc.Name, sc.Score)
	}
	immune.Add("job", 0)
	immune.Touch("job", 200)

	t.Logf("adj=0      殺したのは %s", plain.Kills[0].Victim)
	t.Logf("adj=-1000  殺したのは %s", immune.Kills[0].Victim)

	if plain.Kills[0].Victim != "db" {
		t.Errorf("いちばん大きい db が選ばれるはず: %+v", plain.Kills[0])
	}
	if immune.Kills[0].Victim != "web" {
		t.Errorf("免除したのに db が選ばれた: %+v", immune.Kills[0])
	}
	// 免除すると、より小さい相手を殺すので空く量が減る。
	if freed(immune) >= freed(plain) {
		t.Errorf("空いた量: %d, %d", freed(plain), freed(immune))
	}
}

// 1つ殺して足りなければ、足りるまで殺す。
func TestKillsUntilItFits(t *testing.T) {
	s := New(1000, true)
	for _, n := range []string{"a", "b", "c"} {
		s.Add(n, 0)
		s.Touch(n, 300)
	}
	s.Add("big", 0)
	if err := s.Touch("big", 700); err != nil {
		t.Fatal(err)
	}
	t.Logf("900 使っている状態で 700 を要求 → %d 個殺した(空いた %d)", len(s.Kills), freed(s))
	if len(s.Kills) != 2 {
		t.Errorf("殺した数 %d", len(s.Kills))
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	s := New(1000, true)
	if err := s.Reserve("nobody", 1); err == nil {
		t.Error("居ないプロセスに予約できた")
	}
	if err := s.Touch("nobody", 1); err == nil {
		t.Error("居ないプロセスが触れた")
	}
	// 全員免除だと、殺せる相手が居ないので打つ手が無くなる。
	s.Add("a", -1000)
	s.Touch("a", 900)
	s.Add("b", -1000)
	if err := s.Touch("b", 900); err == nil {
		t.Error("殺せないのに通った")
	}
	// 予約を超えて触ると、予約のほうが追いつく。
	s2 := New(1000, false)
	p := s2.Add("a", 0)
	s2.Reserve("a", 100)
	s2.Touch("a", 300)
	if p.Reserved != 300 {
		t.Errorf("予約 %d", p.Reserved)
	}
	// 断る側では予約の合計で断る。
	if err := s2.Reserve("a", 800); err == nil {
		t.Error("断られていない")
	}
	// 殺されたプロセスはもう触れない。
	s3 := New(100, true)
	s3.Add("x", 0)
	s3.Add("y", 0)
	s3.Touch("x", 100)
	s3.Policy = Requester
	if err := s3.Touch("x", 50); err == nil {
		t.Error("自分を殺して通った")
	}
	if err := s3.Touch("x", 10); err == nil {
		t.Error("死んだのに触れた")
	}
	if len(s3.Scores()) != 1 {
		t.Errorf("生き残り %v", s3.Scores())
	}
}
