package apiserver

import "testing"

func seeded() (*Store, *Informer) {
	s := NewStore()
	s.Put("Pod", "web-1", "running")
	s.Put("Pod", "web-2", "running")
	i := NewInformer(s, "Pod")
	i.Start()
	return s, i
}

// 最初に一度だけ全件を取り、以降は写しから読む。
func TestListsFromCacheNotServer(t *testing.T) {
	s, i := seeded()
	before := s.Reads

	for n := 0; n < 100; n++ {
		if len(i.List()) != 2 {
			t.Fatal("写しから 2 件読めるはず")
		}
	}
	if s.Reads != before {
		t.Fatalf("写しから読むので全件読み出しは増えないはずが %d → %d", before, s.Reads)
	}
}

// 変更は差分として届き、写しが追いつく。
func TestSyncAppliesChanges(t *testing.T) {
	s, i := seeded()
	s.Put("Pod", "web-3", "running")
	s.Delete("Pod", "web-1")

	if !i.Stale() {
		t.Fatal("反映前は遅れているはず")
	}
	i.Sync()
	if i.Stale() {
		t.Fatalf("追いついているはずが %d 版遅れ", i.Lag())
	}
	got := i.List()
	if len(got) != 2 || got[0].Name != "web-2" || got[1].Name != "web-3" {
		t.Fatalf("web-2 と web-3 のはずが %+v", got)
	}
}

// watch が切れている間の変更は届かない。写しは古いまま。
func TestDisconnectedCacheGoesStale(t *testing.T) {
	s, i := seeded()
	i.Disconnect()

	s.Put("Pod", "web-3", "running")
	s.Delete("Pod", "web-1")
	i.Sync() // 繋がっていないので何も起きない

	if len(i.List()) != 2 {
		t.Fatal("写しは切れる前のまま")
	}
	if _, ok := i.Get("web-1"); !ok {
		t.Fatal("消えたはずのものが写しには残っている")
	}
	if i.Lag() != 2 {
		t.Fatalf("2 版遅れているはずが %d", i.Lag())
	}
}

// 張り直せば、切れている間の変更にまとめて追いつく。
// 途中の経過は見ないが、最終の姿には追いつく。
func TestReconnectCatchesUp(t *testing.T) {
	s, i := seeded()
	i.Disconnect()
	s.Put("Pod", "web-3", "running")
	s.Put("Pod", "web-3", "stopped") // 同じものが2回変わった
	s.Delete("Pod", "web-1")

	i.Reconnect()
	if i.Stale() {
		t.Fatalf("追いつくはずが %d 版遅れ", i.Lag())
	}
	o, ok := i.Get("web-3")
	if !ok || o.Value != "stopped" {
		t.Fatalf("最終の姿になるはずが %+v", o)
	}
	if _, ok := i.Get("web-1"); ok {
		t.Fatal("消えたものは写しからも消えるはず")
	}
}

// 履歴が捨てられていると、差分では追いつけない。全件を取り直す。
func TestCompactedHistoryForcesResync(t *testing.T) {
	s, i := seeded()
	i.Disconnect()

	for n := 0; n < 10; n++ {
		s.Put("Pod", "churn", "v"+itoa(n))
	}
	s.Compact(2) // 古い履歴を捨てる

	beforeReads := s.Reads
	beforeResyncs := i.Resyncs
	i.Reconnect()

	if s.Reads == beforeReads {
		t.Fatal("全件を取り直すはず")
	}
	if i.Resyncs != beforeResyncs+1 {
		t.Fatalf("取り直しが数えられるはずが %d", i.Resyncs)
	}
	if i.Stale() {
		t.Fatal("取り直した後は追いついているはず")
	}
}

// 履歴が残っている範囲なら、差分で追いつける。取り直さない。
func TestRecentHistoryAvoidsResync(t *testing.T) {
	s, i := seeded()
	s.Put("Pod", "web-3", "running")

	beforeReads := s.Reads
	i.Sync()
	if s.Reads != beforeReads {
		t.Fatal("差分で足りるので全件は取らないはず")
	}
}

// 写しは古くなりうる。読み手は古い状態を見て判断することがある。
// 取りこぼしても次に数え直せば追いつく設計でなければ、この上では動けない。
func TestReaderSeesStaleState(t *testing.T) {
	s, i := seeded()
	i.Disconnect()
	s.Delete("Pod", "web-1")

	// 読み手から見れば、web-1 はまだ居る。
	if len(i.List()) != 2 {
		t.Fatal("写しの上では 2 件のまま")
	}
	// 置き場の上ではもう 1 件。
	if len(s.List("Pod")) != 1 {
		t.Fatal("真実は 1 件")
	}

	i.Reconnect()
	if len(i.List()) != 1 {
		t.Fatal("繋ぎ直せば真実に追いつく")
	}
}

// 種類が違う資源は写しに入らない。
func TestKindIsFiltered(t *testing.T) {
	s := NewStore()
	s.Put("Pod", "web-1", "running")
	s.Put("Service", "web", "10.0.0.1")
	i := NewInformer(s, "Pod")
	i.Start()

	if len(i.List()) != 1 {
		t.Fatalf("Pod だけのはずが %+v", i.List())
	}
}

// 存在しないものは消せない。版も上がらない。
func TestDeleteUnknownIsNoop(t *testing.T) {
	s := NewStore()
	s.Put("Pod", "web-1", "running")
	v := s.Version()
	if s.Delete("Pod", "nosuch") {
		t.Fatal("存在しないものは消せないはず")
	}
	if s.Version() != v {
		t.Fatal("版が上がってはいけない")
	}
}

// 何も無い置き場から始めても壊れない。
func TestEmptyStore(t *testing.T) {
	s := NewStore()
	i := NewInformer(s, "Pod")
	i.Start()
	if len(i.List()) != 0 || i.Stale() {
		t.Fatal("空でも追いついているはず")
	}
	i.Sync()
	if _, ok := i.Get("nosuch"); ok {
		t.Fatal("無いものは無い")
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(123) != "123" {
		t.Fatal("itoa が違う")
	}
}
