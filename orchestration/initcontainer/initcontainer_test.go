package initcontainer

import "testing"

// ordered は順序を宣言した書き方。プロキシを Sidecar 枠に置く。
func ordered() *Pod {
	return New([]Spec{
		{Name: "config-fetch", Kind: Init, Boot: 2},
		{Name: "proxy", Kind: Sidecar, Boot: 3, Drain: 1, Proxy: true},
		{Name: "web", Kind: App, Boot: 1, Drain: 3},
	})
}

// flat は順序を宣言しない書き方。同じ3つを、プロキシも本体と同じ枠に置く。
func flat() *Pod {
	return New([]Spec{
		{Name: "config-fetch", Kind: Init, Boot: 2},
		{Name: "proxy", Kind: App, Boot: 3, Drain: 1, Proxy: true},
		{Name: "web", Kind: App, Boot: 1, Drain: 3},
	})
}

func tickUntilReady(t *testing.T, p *Pod, limit int) int {
	t.Helper()
	for i := 0; i < limit; i++ {
		if p.Ready() {
			return i
		}
		p.Tick()
	}
	if !p.Ready() {
		t.Fatalf("%d tick 回しても Ready にならない", limit)
	}
	return limit
}

func tickUntilFinished(t *testing.T, p *Pod, limit int) {
	t.Helper()
	for i := 0; i < limit && !p.Finished(); i++ {
		p.Tick()
	}
	if !p.Finished() {
		t.Fatalf("%d tick 回しても終了しない", limit)
	}
}

func find(p *Pod, name string) *Container {
	for _, c := range p.Containers() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// Init が終わるまで App は1つも始まらない。これが Init 枠の定義そのもの。
func TestInitBlocksApp(t *testing.T) {
	p := ordered()
	for i := 0; i < 2; i++ {
		p.Tick()
	}
	if got := find(p, "web").Phase(); got != Waiting {
		t.Fatalf("Init が終わる前に web が %v になっている", got)
	}
	if got := find(p, "config-fetch").Phase(); got != Booting {
		t.Fatalf("config-fetch は起動中のはずが %v", got)
	}
}

// Init は宣言順に1つずつ。前が完了するまで次は始まらない。
func TestInitRunsOneAtATime(t *testing.T) {
	p := New([]Spec{
		{Name: "a", Kind: Init, Boot: 2},
		{Name: "b", Kind: Init, Boot: 2},
		{Name: "web", Kind: App, Boot: 1},
	})
	for i := 0; i < 2; i++ {
		p.Tick()
	}
	if got := find(p, "b").Phase(); got != Waiting {
		t.Fatalf("a の完了前に b が %v になっている", got)
	}
	if got := find(p, "a").Phase(); got != Booting {
		t.Fatalf("a は起動中のはずが %v", got)
	}
	p.Tick() // a が完了し、同じ時刻に b が始まる
	if got := find(p, "a").Phase(); got != Done {
		t.Fatalf("a が完了していない: %v", got)
	}
	if got := find(p, "b").Phase(); got != Booting {
		t.Fatalf("a の完了と同じ時刻に b が始まっていない: %v", got)
	}
}

// Sidecar は完了を待たずに次へ進ませる。動き続けたまま本体を起動させる。
func TestSidecarDoesNotBlockButStartsFirst(t *testing.T) {
	p := ordered()
	tickUntilReady(t, p, 20)
	if got := find(p, "proxy").Phase(); got != Running {
		t.Fatalf("Ready の時点で proxy が %v", got)
	}
	// proxy が Running になった時刻より、web が Running になった時刻が後。
	if p.Exposed != 0 {
		t.Fatalf("順序を宣言したのに出入口なしの時間が %d tick ある", p.Exposed)
	}
}

// 順序を宣言しないと、起動の速い本体が出入口より先に立ち上がる。
func TestFlatExposesOnStartup(t *testing.T) {
	p := flat()
	tickUntilReady(t, p, 20)
	if p.Exposed == 0 {
		t.Fatal("並べただけなのに出入口なしの時間が 0 になっている")
	}
	if p.Exposed != 2 {
		t.Fatalf("起動側の露出は 2 tick のはずが %d", p.Exposed)
	}
}

// 停止のときは向きが逆。Sidecar は本体が終わってから止まる。
func TestSidecarStopsAfterApp(t *testing.T) {
	p := ordered()
	tickUntilReady(t, p, 20)
	p.Terminate()
	web, proxy := find(p, "web"), find(p, "proxy")
	for !p.Finished() {
		if web.Phase() != Done && proxy.Phase() == Done {
			t.Fatal("本体がまだ動いているのに出入口が消えた")
		}
		p.Tick()
	}
	if p.Exposed != 0 {
		t.Fatalf("順序を宣言したのに出入口なしの時間が %d tick ある", p.Exposed)
	}
}

// 同じ枠に並べると、停止の速い出入口が本体より先に消える。
func TestFlatExposesOnShutdown(t *testing.T) {
	p := flat()
	tickUntilReady(t, p, 20)
	atReady := p.Exposed
	p.Terminate()
	tickUntilFinished(t, p, 20)
	if p.Exposed-atReady != 2 {
		t.Fatalf("停止側の露出は 2 tick のはずが %d", p.Exposed-atReady)
	}
	if p.Exposed != 4 {
		t.Fatalf("合計の露出は 4 tick のはずが %d", p.Exposed)
	}
}

// 対照の要点。宣言したほうが起動は遅く、露出は 0 になる。
func TestOrderedIsSlowerButSafe(t *testing.T) {
	o, f := ordered(), flat()
	oReady := tickUntilReady(t, o, 20)
	fReady := tickUntilReady(t, f, 20)
	if oReady <= fReady {
		t.Fatalf("宣言したほうが速く Ready になっている(%d vs %d)", oReady, fReady)
	}
	o.Terminate()
	f.Terminate()
	tickUntilFinished(t, o, 20)
	tickUntilFinished(t, f, 20)
	if o.Exposed != 0 || f.Exposed == 0 {
		t.Fatalf("露出の対照が出ていない: ordered=%d flat=%d", o.Exposed, f.Exposed)
	}
}

// Init が失敗すると、後続は1つも始まらないまま再試行を繰り返す。
func TestInitFailureBlocksEverything(t *testing.T) {
	p := New([]Spec{
		{Name: "config-fetch", Kind: Init, Boot: 1, Fails: 2},
		{Name: "web", Kind: App, Boot: 1},
	})
	for i := 0; i < 5; i++ {
		p.Tick()
	}
	cf := find(p, "config-fetch")
	if cf.Attempts() == 0 {
		t.Fatal("失敗が記録されていない")
	}
	if got := find(p, "web").Phase(); got != Waiting {
		t.Fatalf("Init が失敗しているのに web が %v になっている", got)
	}
	tickUntilReady(t, p, 20)
	if cf.Attempts() != 2 {
		t.Fatalf("失敗回数が台本と合わない: %d", cf.Attempts())
	}
	if cf.Phase() != Done {
		t.Fatalf("再試行後に完了していない: %v", cf.Phase())
	}
}

// 再試行の待ち時間は倍に伸びていき、上限で止まる。
func TestBackoffGrows(t *testing.T) {
	cases := []struct{ attempts, want int }{
		{1, 1}, {2, 2}, {3, 4}, {4, 8}, {5, 8}, {9, 8},
	}
	for _, c := range cases {
		if got := backoffFor(c.attempts); got != c.want {
			t.Errorf("backoffFor(%d) = %d, want %d", c.attempts, got, c.want)
		}
	}
}

// Boot が 0 のコンテナは、開始したその時刻に片付く。
func TestZeroBootSettlesImmediately(t *testing.T) {
	p := New([]Spec{
		{Name: "noop", Kind: Init, Boot: 0},
		{Name: "web", Kind: App, Boot: 0},
	})
	p.Tick()
	if got := find(p, "noop").Phase(); got != Done {
		t.Fatalf("Boot 0 の Init が同じ時刻に完了していない: %v", got)
	}
	tickUntilReady(t, p, 5)
}

// Drain が 0 なら停止要求と同じ時刻に消える。
func TestZeroDrainStopsImmediately(t *testing.T) {
	p := New([]Spec{{Name: "web", Kind: App, Boot: 1, Drain: 0}})
	tickUntilReady(t, p, 5)
	p.Terminate()
	p.Tick()
	if got := find(p, "web").Phase(); got != Done {
		t.Fatalf("Drain 0 なのに %v のまま", got)
	}
}

// 起動しきる前に止められたコンテナも、そのまま終わる。
func TestTerminateDuringStartup(t *testing.T) {
	p := ordered()
	p.Tick() // config-fetch が起動中
	p.Terminate()
	tickUntilFinished(t, p, 20)
	for _, c := range p.Containers() {
		if c.Phase() != Done {
			t.Fatalf("%s が %v のまま残った", c.Name(), c.Phase())
		}
	}
}

// Terminate は二度呼んでも一度しか効かない。
func TestTerminateIsIdempotent(t *testing.T) {
	p := ordered()
	tickUntilReady(t, p, 20)
	p.Terminate()
	n := len(p.Log)
	p.Terminate()
	if len(p.Log) != n {
		t.Fatal("二度目の Terminate が記録された")
	}
}

// 本体が無い Pod は Ready にならない(Job のような一発ものは別の話)。
func TestNoAppIsNeverReady(t *testing.T) {
	p := New([]Spec{{Name: "config-fetch", Kind: Init, Boot: 1}})
	for i := 0; i < 5; i++ {
		p.Tick()
	}
	if p.Ready() {
		t.Fatal("本体が無いのに Ready になっている")
	}
	if !p.Finished() {
		t.Fatal("Init だけの Pod が終了していない")
	}
}

func TestPhaseAndKindStrings(t *testing.T) {
	phases := []struct {
		p    Phase
		want string
	}{
		{Waiting, "Waiting"}, {Booting, "Booting"}, {Running, "Running"},
		{Failed, "Failed"}, {Draining, "Draining"}, {Done, "Done"},
		{Phase(99), "Unknown"},
	}
	for _, c := range phases {
		if got := c.p.String(); got != c.want {
			t.Errorf("Phase(%d) = %q, want %q", c.p, got, c.want)
		}
	}
	p := ordered()
	if find(p, "proxy").Kind() != Sidecar {
		t.Error("Kind が取れていない")
	}
	if p.Now() != 0 {
		t.Error("開始時刻が 0 でない")
	}
}

func TestItoa(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{{0, "0"}, {7, "7"}, {42, "42"}, {-3, "-3"}}
	for _, c := range cases {
		if got := itoa(c.n); got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
