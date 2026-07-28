package kubelet

import (
	"sort"
	"testing"
)

func pod(name string, restart bool, cs ...ContainerSpec) PodSpec {
	return PodSpec{Name: name, Containers: cs, Restart: restart}
}

func web() PodSpec {
	return pod("web", true, ContainerSpec{Name: "app", StartupTicks: 1})
}

// apiserver は、置き場そのものを動かす Pod。ファイルから読む。
func apiserver() PodSpec {
	return pod("kube-apiserver", true, ContainerSpec{Name: "apiserver", StartupTicks: 2})
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newPair() (*Runtime, *Kubelet) {
	rt := NewRuntime()
	return rt, New(rt)
}

func run(k *Kubelet, n int) {
	for i := 0; i < n; i++ {
		k.Tick()
	}
}

// 宣言されたものが無ければ作る。あれば作らない。
func TestCreatesWhatIsDeclared(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{web()})

	run(k, 4)
	if !eq(k.Running(), []string{"web/app"}) {
		t.Fatalf("宣言したコンテナが動いていない: %v", k.Running())
	}
	created := rt.Creates
	run(k, 5)
	if rt.Creates != created {
		t.Fatalf("動いているのに作り直した: %d → %d", created, rt.Creates)
	}
}

// 宣言に無いものは消す。集合の差を埋める形になっている。
func TestRemovesWhatIsNotDeclared(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{web(), pod("old", true, ContainerSpec{Name: "app"})})
	run(k, 3)
	if len(k.Running()) != 2 {
		t.Fatalf("2 つ動いているはず: %v", k.Running())
	}

	k.Deliver([]PodSpec{web()}) // old が宣言から消えた
	run(k, 1)
	if !eq(k.Running(), []string{"web/app"}) {
		t.Fatalf("宣言から消えたものが残っている: %v", k.Running())
	}
	if rt.Removes == 0 {
		t.Fatal("消す呼び出しが行われていない")
	}
}

// 誰も知らないうちにランタイムに現れたものも、次の周で消える。
// 差分でなく毎回まるごと見ているから気づける。
func TestRelistFindsUnknownContainers(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{web()})
	run(k, 3)

	rt.Create("stray", ContainerSpec{Name: "intruder"}) // kubelet の知らないところで増えた
	run(k, 1)

	for _, s := range rt.List() {
		if s.Pod == "stray" {
			t.Fatal("知らないコンテナが残っている")
		}
	}
}

// 一覧は毎周取り直す。イベントに頼っていない。
func TestSyncAlwaysRelists(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{web()})

	before := rt.Relists
	run(k, 5)
	if rt.Relists-before < 5 {
		t.Fatalf("毎周取り直していない: %d 周で %d 回", 5, rt.Relists-before)
	}
}

// 落ちたら作り直す。待ち時間は倍に伸びる。
func TestRestartsWithBackoff(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{pod("flaky", true, ContainerSpec{Name: "app", StartupTicks: 1, FailAfter: 1})})

	run(k, 30)
	if k.Restarts("flaky", "app") < 3 {
		t.Fatalf("作り直しが記録されていない: %d", k.Restarts("flaky", "app"))
	}
	// 待ち時間が伸びるので、作り直しの回数は経過時間より少なくなる。
	if rt.Creates >= 30 {
		t.Fatalf("待ち時間が効いていない: %d 回作った", rt.Creates)
	}
	cases := []struct{ n, want int }{{1, 1}, {2, 2}, {3, 4}, {4, 8}, {5, 8}}
	for _, c := range cases {
		if got := backoffFor(c.n); got != c.want {
			t.Errorf("backoffFor(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}

// 作り直さない宣言なら、落ちたままにする。
func TestNoRestartLeavesItDown(t *testing.T) {
	_, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{pod("once", false, ContainerSpec{Name: "app", StartupTicks: 1, FailAfter: 1})})

	run(k, 10)
	if len(k.Running()) != 0 {
		t.Fatalf("作り直さない宣言なのに動いている: %v", k.Running())
	}
	if k.Restarts("once", "app") != 0 {
		t.Fatalf("作り直しが記録された: %d", k.Restarts("once", "app"))
	}
}

// 置き場に届かなくても、ファイルの宣言だけで起動できる。
// これが鶏と卵を解く。置き場そのものを、置き場を経由せずに起こす。
func TestFilePodsStartWithoutTheStore(t *testing.T) {
	_, k := newPair()
	if k.Linked() {
		t.Fatal("最初から繋がっていることになっている")
	}
	k.SetFilePods([]PodSpec{apiserver()})

	run(k, 4)
	if !eq(k.Running(), []string{"kube-apiserver/apiserver"}) {
		t.Fatalf("置き場なしで起動できていない: %v", k.Running())
	}
	// 届かない状態では、置き場からの宣言は受け取れない。
	if k.Deliver([]PodSpec{web()}) {
		t.Fatal("届かないのに受け取れてしまった")
	}
	if len(k.Running()) != 1 {
		t.Fatalf("受け取れていないのに動いている: %v", k.Running())
	}
}

// 起動の順序が自己参照になっている。
// ファイルから置き場を起こし、起きたら置き場から他の Pod が届き始める。
func TestBootstrapSequence(t *testing.T) {
	_, k := newPair()
	k.SetFilePods([]PodSpec{apiserver()})

	// 置き場が立つまでは、他の Pod は1つも動かない。
	for i := 0; i < 10; i++ {
		if len(k.Running()) > 0 && !k.Linked() {
			// 置き場が立った。ここから届くようになる。
			k.Link(true)
			k.Deliver([]PodSpec{web()})
		}
		k.Tick()
	}

	if !eq(k.Running(), []string{"kube-apiserver/apiserver", "web/app"}) {
		t.Fatalf("順に立ち上がっていない: %v", k.Running())
	}
}

// 置き場を見失っても止まらない。最後に知った宣言を守り続ける。
func TestKeepsRunningWhenTheStoreIsLost(t *testing.T) {
	rt, k := newPair()
	k.SetFilePods([]PodSpec{apiserver()})
	k.Link(true)
	k.Deliver([]PodSpec{web()})
	run(k, 4)
	if len(k.Running()) != 2 {
		t.Fatalf("2 つ動いているはず: %v", k.Running())
	}

	k.Link(false)
	run(k, 10)
	if !eq(k.Running(), []string{"kube-apiserver/apiserver", "web/app"}) {
		t.Fatalf("見失ったら止まってしまった: %v", k.Running())
	}

	// 見失っている間に落ちても、知っている宣言のとおり作り直す。
	before := rt.Creates
	for _, s := range rt.List() {
		if s.Pod == "web" {
			rt.Remove(s.ID)
		}
	}
	run(k, 3)
	if rt.Creates <= before {
		t.Fatal("見失っている間は作り直さなくなっている")
	}
	if !eq(k.Running(), []string{"kube-apiserver/apiserver", "web/app"}) {
		t.Fatalf("作り直せていない: %v", k.Running())
	}
}

// 見失っている間の変更は届かない。届くようになれば追いつく。
func TestChangesArriveOnlyWhenLinked(t *testing.T) {
	_, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{web()})
	run(k, 3)

	k.Link(false)
	if k.Deliver([]PodSpec{web(), pod("added", true, ContainerSpec{Name: "app"})}) {
		t.Fatal("届かないのに受け取れた")
	}
	run(k, 3)
	if !eq(k.Running(), []string{"web/app"}) {
		t.Fatalf("届かない間に増えた: %v", k.Running())
	}

	k.Link(true)
	k.Deliver([]PodSpec{web(), pod("added", true, ContainerSpec{Name: "app"})})
	run(k, 2)
	if !eq(k.Running(), []string{"web/app", "added/app"}) {
		t.Fatalf("届くようになっても追いつかない: %v", k.Running())
	}
}

// ファイルの宣言は置き場からは消せない。出所が違うので混ざらない。
func TestFilePodsSurviveStoreUpdates(t *testing.T) {
	_, k := newPair()
	k.SetFilePods([]PodSpec{apiserver()})
	k.Link(true)
	k.Deliver([]PodSpec{web()})
	run(k, 4)

	k.Deliver(nil) // 置き場の宣言が空になった
	run(k, 2)
	if !eq(k.Running(), []string{"kube-apiserver/apiserver"}) {
		t.Fatalf("ファイルの宣言まで消えた、または置き場の宣言が残った: %v", k.Running())
	}

	k.SetFilePods(nil) // ファイルからも消した
	run(k, 2)
	if len(k.Running()) != 0 {
		t.Fatalf("ファイルから消しても残っている: %v", k.Running())
	}
}

// 出所は宣言に押し込まれる。呼び出し側の指定は上書きされる。
func TestSourceIsStamped(t *testing.T) {
	_, k := newPair()
	k.SetFilePods([]PodSpec{{Name: "a", Source: FromAPIServer}})
	k.Link(true)
	k.Deliver([]PodSpec{{Name: "b", Source: FromFile}})

	for _, p := range k.Desired() {
		switch p.Name {
		case "a":
			if p.Source != FromFile {
				t.Fatalf("ファイル由来になっていない: %v", p.Source)
			}
		case "b":
			if p.Source != FromAPIServer {
				t.Fatalf("置き場由来になっていない: %v", p.Source)
			}
		}
	}
}

// 起動に時間がかかる間は Creating で、稼働にはまだ数えない。
func TestStartupTakesTime(t *testing.T) {
	rt, k := newPair()
	k.Link(true)
	k.Deliver([]PodSpec{pod("slow", true, ContainerSpec{Name: "app", StartupTicks: 3})})

	k.Tick()
	if len(k.Running()) != 0 {
		t.Fatalf("すぐ稼働になった: %v", k.Running())
	}
	found := false
	for _, s := range rt.List() {
		if s.State == Creating {
			found = true
		}
	}
	if !found {
		t.Fatal("起動中の状態が無い")
	}
	run(k, 4)
	if len(k.Running()) != 1 {
		t.Fatalf("いつまでも起動しない: %v", k.Running())
	}
}

// 一覧は Pod 名、コンテナ名の順で決定的。
func TestListIsOrdered(t *testing.T) {
	rt := NewRuntime()
	rt.Create("zeta", ContainerSpec{Name: "b"})
	rt.Create("alpha", ContainerSpec{Name: "z"})
	rt.Create("alpha", ContainerSpec{Name: "a"})

	got := rt.List()
	want := []string{"alpha/a", "alpha/z", "zeta/b"}
	for i, s := range got {
		if s.Pod+"/"+s.Name != want[i] {
			t.Fatalf("並びが違う: %v", got)
		}
	}
}

// 表示まわりと、繰り返し呼んでも効かない操作。
func TestNamesAndIdempotence(t *testing.T) {
	rt, k := newPair()
	if Creating.String() != "Creating" || Running.String() != "Running" || Exited.String() != "Exited" {
		t.Fatal("状態の名前が違う")
	}
	if FromAPIServer.String() != "apiserver" || FromFile.String() != "file" {
		t.Fatal("出所の名前が違う")
	}
	if k.Now() != 0 {
		t.Fatal("開始時刻が 0 でない")
	}

	k.Link(true)
	n := len(k.Log)
	k.Link(true) // 二度目は記録しない
	if len(k.Log) != n {
		t.Fatal("二度目の接続が記録された")
	}
	rt.Remove("nope") // 知らない識別子
	if rt.Removes != 0 {
		t.Fatal("知らない識別子で消したことになっている")
	}
	if itoa(0) != "0" || itoa(120) != "120" {
		t.Fatal("itoa が違う")
	}
}
