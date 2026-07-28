package statefulset

import "testing"

func newSet(replicas, startup int) *Set {
	return New(Config{Name: "db", Replicas: replicas, StartupTicks: startup})
}

// 名前は序数から決まる。何度作り直しても同じ名前になる。
func TestStableNames(t *testing.T) {
	s := newSet(3, 0)
	s.Run(20)
	got := s.Pods()
	if len(got) != 3 {
		t.Fatalf("3 個のはずが %d", len(got))
	}
	for i, p := range got {
		if p.Name != "db-"+itoa(i) || p.Ordinal != i {
			t.Fatalf("序数 %d の名前が %s", i, p.Name)
		}
	}

	// 消して作り直しても、同じ序数には同じ名前が戻る。
	s.DeletePod(1)
	s.Run(20)
	if p := s.Pods()[1]; p.Name != "db-1" {
		t.Fatalf("作り直しても db-1 のはずが %s", p.Name)
	}
}

// 増やすのは序数の小さいほうから、1つずつ。
// 前のものが ready になるまで次を作らない。
func TestCreatesInOrderOneAtATime(t *testing.T) {
	s := newSet(3, 2)
	s.Step() // db-0 を作る
	if len(s.Pods()) != 1 || s.Pods()[0].Name != "db-0" {
		t.Fatalf("最初は db-0 だけのはずが %d 個", len(s.Pods()))
	}
	s.Step() // db-0 はまだ起動中。db-1 は作らない
	if len(s.Pods()) != 1 {
		t.Fatalf("手前が ready でないので増えないはずが %d 個\n%v", len(s.Pods()), s.Log)
	}
	s.Step() // db-0 が ready になる
	if !s.Pods()[0].Ready {
		t.Fatal("db-0 が ready になっているはず")
	}
	s.Step() // ここで db-1 を作る
	if len(s.Pods()) != 2 || s.Pods()[1].Name != "db-1" {
		t.Fatalf("db-1 が作られるはずが %d 個", len(s.Pods()))
	}
}

// 減らすのは序数の大きいほうから。
func TestScalesDownInReverseOrder(t *testing.T) {
	s := newSet(3, 0)
	s.Run(20)
	s.Scale(1)
	s.Step()
	if len(s.Pods()) != 2 || s.maxOrdinal() != 1 {
		t.Fatalf("db-2 から消えるはずが %v", names(s))
	}
	s.Step()
	if len(s.Pods()) != 1 || s.Pods()[0].Name != "db-0" {
		t.Fatalf("db-0 だけ残るはずが %v", names(s))
	}
}

// 途中の1つが ready にならないと、それ以降は永久に作られない。
// 順序を守るとは、詰まったら止まるということでもある。
func TestBrokenOrdinalBlocksTheRest(t *testing.T) {
	s := newSet(4, 1)
	s.SetBroken(1, true)
	s.Run(40)
	if len(s.Pods()) != 2 {
		t.Fatalf("db-0 と db-1 で止まるはずが %v\n%v", names(s), s.Log)
	}
	if s.Converged() {
		t.Fatal("揃っていないはず")
	}

	// 直せば、そこから先が進む。
	s.SetBroken(1, false)
	s.DeletePod(1) // 作り直させる
	s.Run(40)
	if !s.Converged() {
		t.Fatalf("直せば揃うはずが %v\n%v", names(s), s.Log)
	}
}

// ボリュームは Pod より長生きする。Pod を消してもデータは残り、
// 同じ序数の Pod が作り直されると、また同じボリュームが繋がる。
func TestVolumeOutlivesPod(t *testing.T) {
	s := newSet(2, 0)
	s.Run(20)
	s.Write(1, "important")

	s.DeletePod(1)
	if len(s.PVCs()) != 2 {
		t.Fatalf("Pod を消してもボリュームは残るはずが %d 個", len(s.PVCs()))
	}
	if s.Read(1) != "important" {
		t.Fatal("データが消えている")
	}

	s.Run(20)
	if p := s.Pods()[1]; p.PVC != "data-db-1" {
		t.Fatalf("同じボリュームに繋がるはずが %s", p.PVC)
	}
	if s.Read(1) != "important" {
		t.Fatal("作り直した後もデータが残るはず")
	}
}

// 縮小してもボリュームは残る。戻すと同じデータが繋がる。
// 意図された動きだが、使っていないボリュームの費用は払い続ける。
func TestVolumeSurvivesScaleDown(t *testing.T) {
	s := newSet(3, 0)
	s.Run(20)
	s.Write(2, "shard-2")

	s.Scale(1)
	s.Run(20)
	if len(s.Pods()) != 1 {
		t.Fatalf("1 個に減るはずが %d", len(s.Pods()))
	}
	if len(s.PVCs()) != 3 {
		t.Fatalf("ボリュームは 3 個残るはずが %d", len(s.PVCs()))
	}

	s.Scale(3)
	s.Run(20)
	if s.Read(2) != "shard-2" {
		t.Fatalf("戻せば同じデータが繋がるはずが %q", s.Read(2))
	}
}

// ボリュームを明示的に消すと、そこで初めてデータが失われる。
func TestDeletePVCLosesData(t *testing.T) {
	s := newSet(2, 0)
	s.Run(20)
	s.Write(1, "gone")
	s.DeletePod(1)
	s.DeletePVC(1)
	s.Run(20)
	if s.Read(1) != "" {
		t.Fatalf("消した後は空のはずが %q", s.Read(1))
	}
	if len(s.PVCs()) != 2 {
		t.Fatal("作り直しで新しいボリュームができるはず")
	}
}

// 目標 0 まで縮めても、ボリュームは全部残る。
func TestScaleToZeroKeepsVolumes(t *testing.T) {
	s := newSet(2, 0)
	s.Run(20)
	s.Scale(0)
	s.Run(20)
	if len(s.Pods()) != 0 {
		t.Fatalf("Pod は 0 になるはずが %d", len(s.Pods()))
	}
	if len(s.PVCs()) != 2 {
		t.Fatalf("ボリュームは残るはずが %d", len(s.PVCs()))
	}
	if !s.Converged() {
		t.Fatal("目標 0 で揃っているはず")
	}
}

// 負の目標は 0 に丸める。存在しない序数への操作は何もしない。
func TestGuards(t *testing.T) {
	s := newSet(1, 0)
	s.Run(10)
	s.Scale(-3)
	s.Run(10)
	if len(s.Pods()) != 0 {
		t.Fatal("負の目標は 0 扱いのはず")
	}
	s.DeletePod(99)
	s.DeletePVC(99)
	s.Write(99, "x")
	if s.Read(99) != "" {
		t.Fatal("存在しない序数は空のはず")
	}
}

func TestItoa(t *testing.T) {
	if itoa(0) != "0" || itoa(12) != "12" {
		t.Fatal("itoa が違う")
	}
}

func names(s *Set) []string {
	out := make([]string, 0, len(s.Pods()))
	for _, p := range s.Pods() {
		out = append(out, p.Name)
	}
	return out
}
