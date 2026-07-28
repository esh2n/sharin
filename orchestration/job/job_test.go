package job

import "testing"

// 必要な数だけ成功したら終わる。動き続けるものと違い、終わりが正常な結末。
func TestCompletesWhenEnoughSucceed(t *testing.T) {
	j := New(Config{Completions: 3, Parallelism: 1, BackoffLimit: 2})
	j.Run(20)
	if j.Phase != Complete {
		t.Fatalf("Complete のはずが %s\n%v", j.Phase, j.Log)
	}
	if j.Succeeded != 3 {
		t.Fatalf("3 個成功のはずが %d", j.Succeeded)
	}
	if j.Active() != 0 {
		t.Fatalf("終わった後に走っているものがある: %d", j.Active())
	}
}

// 同時実行を増やすと、同じ仕事量が少ない周期で終わる。
// 仕事の量(completions)と速さ(parallelism)は別の軸になっている。
func TestParallelismShortensTheRun(t *testing.T) {
	steps := func(par int) int {
		j := New(Config{Completions: 6, Parallelism: par, BackoffLimit: 0})
		n := 0
		for ; !j.Done() && n < 50; n++ {
			j.Step()
		}
		return n
	}
	slow, fast := steps(1), steps(3)
	if fast >= slow {
		t.Fatalf("並列を増やせば短くなるはず: par1=%d par3=%d", slow, fast)
	}
}

// 要る数より多くは走らせない。残り1個なら1個だけ起動する。
func TestDoesNotOverrunCompletions(t *testing.T) {
	j := New(Config{Completions: 2, Parallelism: 5, BackoffLimit: 0})
	j.Step() // 2 個だけ起動するはず
	if j.Active() != 2 {
		t.Fatalf("残り 2 なので 2 個のはずが %d", j.Active())
	}
	j.Run(10)
	if j.Succeeded != 2 {
		t.Fatalf("余計に成功させないはずが %d", j.Succeeded)
	}
}

// 失敗しても、上限の内なら再試行して最後には終わる。
func TestRetriesWithinBackoffLimit(t *testing.T) {
	j := New(Config{Completions: 2, Parallelism: 1, BackoffLimit: 3, FailFirst: 2})
	j.Run(30)
	if j.Phase != Complete {
		t.Fatalf("再試行して終わるはずが %s\n%v", j.Phase, j.Log)
	}
	if j.Failed != 2 {
		t.Fatalf("2 回失敗したはずが %d", j.Failed)
	}
}

// 上限を超えたら Job ごと失敗にする。決めておかないと永久に再試行し続ける。
func TestGivesUpAfterBackoffLimit(t *testing.T) {
	j := New(Config{Completions: 1, Parallelism: 1, BackoffLimit: 2, FailFirst: 99})
	j.Run(50)
	if j.Phase != Failed {
		t.Fatalf("Failed のはずが %s\n%v", j.Phase, j.Log)
	}
	if j.Succeeded != 0 {
		t.Fatal("1 つも成功していないはず")
	}
	if j.Failed <= 2 {
		t.Fatalf("上限を超えて初めて諦めるはずが %d 回で止まった", j.Failed)
	}
}

// 結末がついた後の Step は何もしない。
func TestStepAfterDoneIsNoop(t *testing.T) {
	j := New(Config{Completions: 1, Parallelism: 1})
	j.Run(10)
	before := j.Succeeded
	for i := 0; i < 5; i++ {
		j.Step()
	}
	if j.Succeeded != before {
		t.Fatalf("終わった後に進んだ: %d → %d", before, j.Succeeded)
	}
}

// 壊れた設定は最小限に補正する。
func TestConfigIsSanitized(t *testing.T) {
	j := New(Config{Completions: 0, Parallelism: 0})
	if j.cfg.Completions != 1 || j.cfg.Parallelism != 1 {
		t.Fatalf("補正されていない: %+v", j.cfg)
	}
}

// 周期ごとに起動し、終わった実行が積み上がる。
func TestCronStartsOnSchedule(t *testing.T) {
	c := NewCron(CronConfig{Every: 3, Policy: Allow,
		Job: Config{Completions: 1, Parallelism: 1}})
	// t=3,6,9 で起動する。最後の実行が終わるまで進める。
	for i := 0; i < 11; i++ {
		c.Tick()
	}
	if c.Started != 3 {
		t.Fatalf("3 回起動するはずが %d\n%v", c.Started, c.Log)
	}
	if c.Completed() != 3 {
		t.Fatalf("3 回とも完了するはずが %d\n%v", c.Completed(), c.Log)
	}
}

// 前の実行が終わらないまま次の時刻が来たとき、方針で振る舞いが分かれる。
func TestConcurrencyPolicies(t *testing.T) {
	// 長引く仕事(必要数が多く、並列が1)を短い周期で起動する。
	mk := func(p Policy) *CronJob {
		c := NewCron(CronConfig{Every: 2, Policy: p,
			Job: Config{Completions: 6, Parallelism: 1}})
		for i := 0; i < 10; i++ {
			c.Tick()
		}
		return c
	}

	allow := mk(Allow)
	if allow.Skipped != 0 || allow.Replaced != 0 {
		t.Fatalf("Allow は重ねるはず: skipped=%d replaced=%d", allow.Skipped, allow.Replaced)
	}
	if allow.Started < 4 {
		t.Fatalf("Allow は毎回起動するはずが %d", allow.Started)
	}

	forbid := mk(Forbid)
	if forbid.Skipped == 0 {
		t.Fatalf("Forbid は飛ばすはずが 0\n%v", forbid.Log)
	}
	if forbid.Started >= allow.Started {
		t.Fatalf("Forbid のほうが起動数は少ないはず: %d vs %d", forbid.Started, allow.Started)
	}

	replace := mk(Replace)
	if replace.Replaced == 0 {
		t.Fatalf("Replace は置き換えるはずが 0\n%v", replace.Log)
	}
	// 置き換えられた実行は完了しない。最新だけが残る。
	if replace.Active() > 1 {
		t.Fatalf("Replace なら同時に走るのは 1 つのはずが %d", replace.Active())
	}
}

// Forbid は飛ばすので、実行そのものが起きない回がある。
// 「毎回必ず走る」ことは保証されない。
func TestForbidSkipsRuns(t *testing.T) {
	c := NewCron(CronConfig{Every: 2, Policy: Forbid,
		Job: Config{Completions: 5, Parallelism: 1}})
	for i := 0; i < 12; i++ {
		c.Tick()
	}
	if c.Started+c.Skipped != 6 {
		t.Fatalf("起動と飛ばしの合計が予定回数のはずが %d + %d", c.Started, c.Skipped)
	}
	if c.Skipped == 0 {
		t.Fatal("飛ばされた回があるはず")
	}
}

func TestStrings(t *testing.T) {
	if Running.String() != "Running" || Complete.String() != "Complete" ||
		Failed.String() != "Failed" || Phase(9).String() != "Unknown" {
		t.Fatal("Phase の文字列が違う")
	}
	if Allow.String() != "Allow" || Forbid.String() != "Forbid" ||
		Replace.String() != "Replace" || Policy(9).String() != "Unknown" {
		t.Fatal("Policy の文字列が違う")
	}
	if itoa(0) != "0" || itoa(31) != "31" {
		t.Fatal("itoa が違う")
	}
	if NewCron(CronConfig{Every: 0}).cfg.Every != 1 {
		t.Fatal("Every は最低 1 に補正されるはず")
	}
}
