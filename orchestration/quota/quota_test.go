package quota

import "testing"

func std() *Namespace {
	return New("team-a",
		ResourceQuota{Hard: Resources{CPU: 2000, Mem: 2048}, MaxPods: 6},
		LimitRange{Default: Resources{CPU: 200, Mem: 256}, Max: Resources{CPU: 1000, Mem: 1024}})
}

func small() Resources { return Resources{CPU: 400, Mem: 400} }

// 上限の内なら受け入れ、超えたら断る。1つずつでなく合計で判断する。
func TestTotalIsWhatMatters(t *testing.T) {
	n := std()
	for i := 1; i <= 5; i++ {
		if r := n.Admit("p"+itoa(i), small()); !r.Admitted {
			t.Fatalf("%d 個目で断られた: %s", i, r.Reason)
		}
	}
	// 合計 2000m。次はどれだけ小さくても入らない。
	if r := n.Admit("p6", Resources{CPU: 1, Mem: 1}); r.Admitted {
		t.Fatal("総量の上限を超えるので断られるはず")
	}
	if n.Used().CPU != 2000 {
		t.Fatalf("合計が 2000m のはずが %d", n.Used().CPU)
	}
}

// 要求を書かない Pod には既定値が入り、その値で総量に数えられる。
// 入れなければ 0 として通り、上限が意味を失う。
func TestDefaultsAreCountedInTotal(t *testing.T) {
	n := std()
	r := n.Admit("no-req", Resources{})
	if !r.Defaulted {
		t.Fatal("既定値が入るはず")
	}
	if n.Used().CPU != 200 {
		t.Fatalf("既定値で数えるはずが %d", n.Used().CPU)
	}

	// 既定値の無い区画では、書き忘れが 0 として通り続ける。
	loose := New("loose", ResourceQuota{Hard: Resources{CPU: 100, Mem: 100}}, LimitRange{})
	for i := 0; i < 20; i++ {
		if r := loose.Admit("p"+itoa(i), Resources{}); !r.Admitted {
			t.Fatal("0 として数えられるので、いくつでも入ってしまう")
		}
	}
	if !loose.Used().IsZero() {
		t.Fatalf("合計は 0 のまま: %+v", loose.Used())
	}
}

// 1つあたりの上限は、総量に余裕があっても効く。
func TestPerPodMaxAppliesEvenWithRoom(t *testing.T) {
	n := std()
	if r := n.Admit("huge", Resources{CPU: 1500, Mem: 1500}); r.Admitted {
		t.Fatal("1つあたりの上限を超えるので断られるはず")
	}
	if !n.Used().IsZero() {
		t.Fatal("断ったのに数えられている")
	}
	// 上限の内なら、同じ総量でも通る。
	if r := n.Admit("ok", Resources{CPU: 1000, Mem: 1024}); !r.Admitted {
		t.Fatalf("上限ちょうどは通るはず: %s", r.Reason)
	}
}

// 個数の上限も別に効く。小さくても数で止まる。
func TestPodCountLimit(t *testing.T) {
	n := New("cnt", ResourceQuota{Hard: Resources{CPU: 9999, Mem: 9999}, MaxPods: 3}, LimitRange{})
	for i := 1; i <= 3; i++ {
		if r := n.Admit("p"+itoa(i), Resources{CPU: 1, Mem: 1}); !r.Admitted {
			t.Fatalf("%d 個目で断られた", i)
		}
	}
	if r := n.Admit("p4", Resources{CPU: 1, Mem: 1}); r.Admitted {
		t.Fatal("個数の上限で断られるはず")
	}
}

// 消せば分が戻り、また入れられるようになる。
func TestRemovingFreesQuota(t *testing.T) {
	n := std()
	for i := 1; i <= 5; i++ {
		n.Admit("p"+itoa(i), small())
	}
	if r := n.Admit("p6", small()); r.Admitted {
		t.Fatal("満杯のはず")
	}
	if !n.Remove("p1") {
		t.Fatal("消せるはず")
	}
	if r := n.Admit("p6", small()); !r.Admitted {
		t.Fatalf("空いたので入るはず: %s", r.Reason)
	}
	if n.Remove("nosuch") {
		t.Fatal("存在しない Pod は消せないはず")
	}
}

// 既定値を入れてから上限を見る。順序が逆だと、書き忘れが
// 1つあたりの上限も素通りする。
func TestDefaultThenCheck(t *testing.T) {
	// 既定値が1つあたりの上限を超えている、という壊れた設定。
	n := New("bad", ResourceQuota{Hard: Resources{CPU: 9999, Mem: 9999}},
		LimitRange{Default: Resources{CPU: 500, Mem: 500}, Max: Resources{CPU: 100, Mem: 100}})
	r := n.Admit("no-req", Resources{})
	if r.Admitted {
		t.Fatal("既定値を入れた後で上限に照らすので、断られるはず")
	}
	if !r.Defaulted {
		t.Fatal("既定値は入っているはず")
	}
}

// 断られた分は数えられない。断った後も次が入る。
func TestRejectionDoesNotConsume(t *testing.T) {
	n := std()
	n.Admit("p1", small())
	n.Admit("huge", Resources{CPU: 1500, Mem: 1500}) // 断られる
	if n.Used().CPU != 400 {
		t.Fatalf("断った分は数えないはずが %d", n.Used().CPU)
	}
	if n.Rejected != 1 || n.Admitted != 1 {
		t.Fatalf("admitted=%d rejected=%d", n.Admitted, n.Rejected)
	}
}

// 上限がぴったりなら通る。境界で切り上がらない。
func TestExactFitIsAllowed(t *testing.T) {
	n := New("exact", ResourceQuota{Hard: Resources{CPU: 1000, Mem: 1000}}, LimitRange{})
	if r := n.Admit("p", Resources{CPU: 1000, Mem: 1000}); !r.Admitted {
		t.Fatalf("ぴったりは通るはず: %s", r.Reason)
	}
	if !n.Free().IsZero() {
		t.Fatalf("残りは 0 のはずが %+v", n.Free())
	}
}

// 要求を書いてあれば、既定値は入らない。
func TestExplicitRequestWins(t *testing.T) {
	n := std()
	r := n.Admit("p", Resources{CPU: 700, Mem: 700})
	if r.Defaulted {
		t.Fatal("書いてあるなら既定値は入らないはず")
	}
	if n.Used().CPU != 700 {
		t.Fatalf("書いた値で数えるはずが %d", n.Used().CPU)
	}
}

func TestHelpers(t *testing.T) {
	if res(Resources{CPU: 100, Mem: 200}) != "100m/200Mi" {
		t.Fatal("res が違う")
	}
	if itoa(0) != "0" || itoa(2048) != "2048" {
		t.Fatal("itoa が違う")
	}
	n := std()
	if n.Quota().MaxPods != 6 || len(n.Pods()) != 0 {
		t.Fatal("初期状態が違う")
	}
}
