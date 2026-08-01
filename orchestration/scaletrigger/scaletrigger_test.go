package scaletrigger

import "testing"

// この章の中心その1。同じ「200 件溜まっている」でも、取れる情報が違う。
func TestLogHasLagButQueueDoesNot(t *testing.T) {
	// ログ型: 200 件書いて、まだ 1 件も読んでいない。
	l := &Log{}
	l.Append(200)

	// キュー型: 200 件入れて、まだ 1 件も取り出していない。
	q := &Queue{}
	q.Enqueue(200)

	t.Logf("ログ型  書いた %d / 読んだ %d / ラグ %d", l.Written(), l.Committed(), l.Lag())
	t.Logf("キュー型 見えている %d / 処理中 %d(書いた位置という概念が無い)", q.Visible(), q.InFlight())

	if l.Lag() != 200 || q.Visible() != 200 {
		t.Fatalf("初期状態が違う: lag=%d visible=%d", l.Lag(), q.Visible())
	}

	// 50 件ずつ処理する。ログ型は読んでも消えないので、書いた位置は動かない。
	l.Commit(50)
	q.Receive(50)
	q.Ack(50)

	t.Logf("50 件処理後")
	t.Logf("  ログ型  書いた %d / 読んだ %d / ラグ %d ← 書いた総数が残る", l.Written(), l.Committed(), l.Lag())
	t.Logf("  キュー型 見えている %d ← 消えたので、何件流れたかは分からない", q.Visible())

	// ログ型は「何件流れたか」を保持し続ける。
	if l.Written() != 200 || l.Committed() != 50 || l.Lag() != 150 {
		t.Fatalf("%+v", l)
	}
	// キュー型は残りしか分からない。総数 200 という情報はどこにも無い。
	if q.Visible() != 150 || q.Outstanding() != 150 {
		t.Fatalf("%+v", q)
	}
	// どちらも「残り 150」は取れる。違うのは、そこに至る位置情報を持つかどうか。
	if l.Lag() != q.Outstanding() {
		t.Fatalf("残りが一致しない: %d %d", l.Lag(), q.Outstanding())
	}
}

// この章の中心その2。見えている数だけで判断すると、縮めすぎる。
func TestVisibleOnlyUnderCountsTheWork(t *testing.T) {
	q := &Queue{}
	q.Enqueue(200)

	// 20 レプリカが 10 件ずつ取り出して処理中。まだ確定していない。
	q.Receive(200)

	t.Logf("200 件を全部取り出した直後")
	t.Logf("  見えている %d / 処理中 %d / まだ終わっていない %d",
		q.Visible(), q.InFlight(), q.Outstanding())

	const perReplica = 10
	byVisible := Desired(q.Visible(), perReplica)
	byOutstanding := Desired(q.Outstanding(), perReplica)
	t.Logf("  見えている数で決める      → %d 個", byVisible)
	t.Logf("  終わっていない数で決める   → %d 個", byOutstanding)

	// 見えている数は 0 なので、「もう仕事が無い」と読んでしまう。
	if q.Visible() != 0 || byVisible != 0 {
		t.Fatalf("visible=%d desired=%d", q.Visible(), byVisible)
	}
	// 実際には 200 件が処理中で、20 個ぶんの仕事が残っている。
	if q.Outstanding() != 200 || byOutstanding != 20 {
		t.Fatalf("outstanding=%d desired=%d", q.Outstanding(), byOutstanding)
	}

	// 処理に失敗すると、見えている数に戻ってくる。
	q.Nack(200)
	t.Logf("全部失敗して戻ると 見えている %d に戻る", q.Visible())
	if q.Visible() != 200 || q.InFlight() != 0 {
		t.Fatalf("%+v", q)
	}
}

// 式は現在のレプリカ数に依存しない(カスタム指標の AverageValue と同じ)。
func TestDesiredIgnoresCurrentReplicas(t *testing.T) {
	const backlog, perReplica = 200, 10
	want := 20
	if got := Desired(backlog, perReplica); got != want {
		t.Fatalf("Desired = %d, want %d", got, want)
	}
	// 端数は切り上げる。足りないより多いほうがまし。
	if got := Desired(201, 10); got != 21 {
		t.Fatalf("切り上げていない: %d", got)
	}
	if Desired(0, 10) != 0 || Desired(200, 0) != 0 {
		t.Fatal("端の扱いが違う")
	}
}

// この章の中心その3。パーティションを超えたレプリカは何もしない。
func TestPartitionsCapTheUsefulReplicas(t *testing.T) {
	topic := Topic{Partitions: 10, PerReplica: 10}

	t.Logf("パーティション %d / 1 レプリカ %d 件", topic.Partitions, topic.PerReplica)
	t.Logf("%-10s %10s %8s %12s", "レプリカ", "実際に働く", "遊ぶ", "捌ける件数")
	for _, r := range []int{5, 10, 15, 20} {
		t.Logf("%-10d %10d %8d %12d", r, topic.EffectiveReplicas(r), topic.Idle(r), topic.Throughput(r))
	}

	// 10 までは増やしたぶん伸びる。
	if topic.Throughput(5) != 50 || topic.Throughput(10) != 100 {
		t.Fatal("上限の内側で伸びていない")
	}
	// 超えると1件も増えない。
	if topic.Throughput(20) != topic.Throughput(10) {
		t.Fatalf("上限を超えて伸びている: %d %d", topic.Throughput(20), topic.Throughput(10))
	}
	// 超えたぶんは丸ごと遊ぶ。
	if topic.Idle(20) != 10 {
		t.Fatalf("遊ぶ数が違う: %d", topic.Idle(20))
	}
	if topic.MaxUseful() != 10 {
		t.Fatal("有効な上限が違う")
	}
}

// 滞留から出した必要数が、パーティション上限に当たる場面。
func TestBacklogAsksForMoreThanPartitionsAllow(t *testing.T) {
	topic := Topic{Partitions: 10, PerReplica: 10}
	const backlog, perReplica = 2000, 10

	desired := Desired(backlog, perReplica)
	effective := topic.EffectiveReplicas(desired)

	t.Logf("滞留 %d 件 / 1 レプリカ %d 件 → 式は %d 個を要求", backlog, perReplica, desired)
	t.Logf("だがパーティションは %d なので、働くのは %d 個。%d 個は遊ぶ",
		topic.Partitions, effective, topic.Idle(desired))
	t.Logf("捌け終わるまで %d 単位時間(%d 個でも %d 個でも同じ)",
		Drain(backlog, topic, desired), desired, topic.Partitions)

	// 式は 200 個を要求する。
	if desired != 200 {
		t.Fatalf("desired = %d", desired)
	}
	// 実際に働くのは 10 個だけで、190 個は割り当てが無い。
	if effective != 10 || topic.Idle(desired) != 190 {
		t.Fatalf("effective=%d idle=%d", effective, topic.Idle(desired))
	}
	// 200 個立てても、10 個のときと捌ける時間が変わらない。
	if Drain(backlog, topic, desired) != Drain(backlog, topic, topic.Partitions) {
		t.Fatal("上限を超えて速くなっている")
	}
	// 分割を増やせば、初めて速くなる。
	wider := Topic{Partitions: 50, PerReplica: 10}
	t.Logf("分割を %d に増やすと %d 単位時間", wider.Partitions, Drain(backlog, wider, desired))
	if Drain(backlog, wider, desired) >= Drain(backlog, topic, desired) {
		t.Fatal("分割を増やしても速くなっていない")
	}
}

// 端の振る舞い。
func TestEdges(t *testing.T) {
	// 空のログ・キュー。
	l := &Log{}
	if l.Lag() != 0 {
		t.Fatal("空のラグが 0 でない")
	}
	l.Commit(10) // 書いていないのに読めない
	if l.Committed() != 0 || l.Lag() != 0 {
		t.Fatalf("%+v", l)
	}
	q := &Queue{}
	if got := q.Receive(10); got != 0 {
		t.Fatalf("空から %d 件取れた", got)
	}
	q.Ack(10)
	q.Nack(10)
	if q.Outstanding() != 0 {
		t.Fatalf("%+v", q)
	}
	// 取り出せる数より多く要求しても、あるぶんだけ。
	q.Enqueue(3)
	if got := q.Receive(10); got != 3 {
		t.Fatalf("%d 件取れた", got)
	}
	// パーティション 0 なら誰も働けず、捌け終わらない。
	dead := Topic{Partitions: 0, PerReplica: 10}
	if dead.Throughput(100) != 0 || Drain(100, dead, 100) != -1 {
		t.Fatal("パーティション 0 の扱いが違う")
	}
	// レプリカ 0。
	topic := Topic{Partitions: 10, PerReplica: 10}
	if topic.Throughput(0) != 0 || topic.EffectiveReplicas(-1) != 0 {
		t.Fatal("レプリカ 0 の扱いが違う")
	}
}
