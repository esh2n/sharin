package container

import "testing"

const (
	kib = 1 << 10
	mib = 1 << 20
	gib = 1 << 30
)

func TestPIDIsolation(t *testing.T) {
	h := NewHost(gib)
	web := h.NewContainer(Config{Name: "web", Hostname: "web"})
	db := h.NewContainer(Config{Name: "db", Hostname: "db"})

	// 各コンテナの最初のプロセスは local PID 1(init)
	winit, err := web.Spawn("init", 4*mib)
	if err != nil {
		t.Fatal(err)
	}
	dinit, err := db.Spawn("init", 4*mib)
	if err != nil {
		t.Fatal(err)
	}
	if winit.LocalPID != 1 || dinit.LocalPID != 1 {
		t.Fatalf("両コンテナの init は local PID 1 のはず: web=%d db=%d", winit.LocalPID, dinit.LocalPID)
	}
	// だが global PID は別物(1つのカーネルを共有)
	if winit.GlobalPID == dinit.GlobalPID {
		t.Fatalf("global PID は衝突しないはず: %d", winit.GlobalPID)
	}
	// 2つ目のプロセスは local PID 2
	w2, _ := web.Spawn("nginx", 8*mib)
	if w2.LocalPID != 2 {
		t.Fatalf("2つ目は local PID 2 のはず: %d", w2.LocalPID)
	}
	// web は db のプロセスを見られない
	if got := len(web.Processes()); got != 2 {
		t.Fatalf("web からは 2 プロセスのはず: %d", got)
	}
	if got := len(db.Processes()); got != 1 {
		t.Fatalf("db からは 1 プロセスのはず: %d", got)
	}
	// host は全部見える
	if h.ProcessCount() != 3 {
		t.Fatalf("host は 3 プロセス見えるはず: %d", h.ProcessCount())
	}
	if len(h.GlobalPIDs()) != 3 {
		t.Fatalf("host の global PID は 3 個: %v", h.GlobalPIDs())
	}
	// web から見える最初のプロセスは init(local PID 1)
	if ps := web.Processes(); ps[0].LocalPID != 1 || ps[0].Name != "init" {
		t.Fatalf("先頭は init(local 1)のはず: %+v", ps[0])
	}
}

func TestKillReapsProcess(t *testing.T) {
	h := NewHost(gib)
	c := h.NewContainer(Config{Name: "app", MemLimit: 100 * mib})
	c.Spawn("init", 4*mib)
	p, _ := c.Spawn("worker", 8*mib)
	before := c.MemUsage()
	if err := c.Kill(p.LocalPID); err != nil {
		t.Fatal(err)
	}
	if c.MemUsage() != before-8*mib {
		t.Fatalf("kill 後にメモリが戻るはず: %d", c.MemUsage())
	}
	if got := len(c.Processes()); got != 1 {
		t.Fatalf("残り 1 プロセスのはず: %d", got)
	}
	if h.ProcessCount() != 1 {
		t.Fatalf("host からも消えるはず: %d", h.ProcessCount())
	}
	if err := c.Kill(999); err == nil {
		t.Fatal("存在しない PID の kill はエラーのはず")
	}
}

func TestCgroupMemoryLimit(t *testing.T) {
	h := NewHost(gib)
	c := h.NewContainer(Config{Name: "limited", MemLimit: 32 * mib})
	if _, err := c.Spawn("a", 20*mib); err != nil {
		t.Fatalf("20 MiB は入るはず: %v", err)
	}
	// もう 20 MiB は 32 を超えるので拒否(OOM)
	if _, err := c.Spawn("b", 20*mib); err == nil {
		t.Fatal("上限超過は拒否されるはず(OOM)")
	}
	// 拒否されたプロセスは計上されない(全か無か)
	if c.PidCount() != 1 {
		t.Fatalf("拒否分は数えないはず: %d", c.PidCount())
	}
	if c.MemUsage() != 20*mib {
		t.Fatalf("拒否分のメモリは計上しないはず: %d", c.MemUsage())
	}
	if c.MemLimit() != 32*mib {
		t.Fatalf("上限は 32 MiB: %d", c.MemLimit())
	}
}

func TestCgroupHierarchy(t *testing.T) {
	// 親 48 MiB の下に子2つ。子単体では収まっても、親の合計で頭打ちになる
	parent := NewCGroup("pod", 48*mib, 0)
	a := parent.NewChild("a", 40*mib, 0)
	b := parent.NewChild("b", 40*mib, 0)

	if err := a.Charge(40 * mib); err != nil {
		t.Fatalf("a は 40 入るはず: %v", err)
	}
	// b は自分の上限 40 には収まるが、親 48 を超えるので拒否
	if err := b.Charge(40 * mib); err == nil {
		t.Fatal("親の上限で拒否されるはず")
	}
	if b.MemUsage() != 0 {
		t.Fatalf("拒否された b は 0 のはず: %d", b.MemUsage())
	}
	// 親の使用量は a のぶんだけ(子の合計がロールアップ)
	if parent.MemUsage() != 40*mib {
		t.Fatalf("親は子の合計を計上: %d", parent.MemUsage())
	}
	// a を解放すれば b が入る
	a.Uncharge(40 * mib)
	if err := b.Charge(40 * mib); err != nil {
		t.Fatalf("解放後は b が入るはず: %v", err)
	}
}

func TestCgroupPidsLimit(t *testing.T) {
	h := NewHost(gib)
	c := h.NewContainer(Config{Name: "capped", PidsLimit: 2})
	if _, err := c.Spawn("p1", mib); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Spawn("p2", mib); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Spawn("p3", mib); err == nil {
		t.Fatal("pids 上限で 3つ目は拒否されるはず")
	}
	if c.PidCount() != 2 {
		t.Fatalf("2 のはず: %d", c.PidCount())
	}
	// メモリも計上されていない(全か無かで戻したから)
	if c.MemUsage() != 2*mib {
		t.Fatalf("拒否分のメモリは戻るはず: %d", c.MemUsage())
	}
}

func TestHostSharesOneMemoryPool(t *testing.T) {
	h := NewHost(64 * mib) // マシン全体 64 MiB
	a := h.NewContainer(Config{Name: "a"})
	b := h.NewContainer(Config{Name: "b"})
	if _, err := a.Spawn("x", 40*mib); err != nil {
		t.Fatal(err)
	}
	// b はコンテナ制限が無くても、host 全体 64 のうち残り 24 しか使えない
	if _, err := b.Spawn("y", 40*mib); err == nil {
		t.Fatal("host 全体のメモリを共有するので拒否されるはず")
	}
	if _, err := b.Spawn("y", 20*mib); err != nil {
		t.Fatalf("20 なら入る: %v", err)
	}
	if h.RootUsage() != 60*mib {
		t.Fatalf("host 合計は 60 MiB: %d", h.RootUsage())
	}
}

func TestNetNamespaceIsolation(t *testing.T) {
	h := NewHost(gib)
	web := h.NewContainer(Config{Name: "web"})
	db := h.NewContainer(Config{Name: "db"})

	// 両方が :80 を bind できる(別々のネットワーク名前空間)
	if err := web.Bind(80, "nginx"); err != nil {
		t.Fatal(err)
	}
	if err := db.Bind(80, "pg"); err != nil {
		t.Fatalf("別コンテナなら同じ :80 を bind できるはず: %v", err)
	}
	// 同じ名前空間内での二重 bind は衝突
	if err := web.Bind(80, "apache"); err == nil {
		t.Fatal("同一名前空間の二重 bind は拒否されるはず")
	}
	if got := web.Ports(); len(got) != 1 || got[0] != 80 {
		t.Fatalf("web は :80 のみ: %v", got)
	}
}

func TestUTSNamespace(t *testing.T) {
	h := NewHost(gib)
	a := h.NewContainer(Config{Name: "a", Hostname: "alpha"})
	b := h.NewContainer(Config{Name: "b", Hostname: "beta"})
	if a.Hostname() != "alpha" || b.Hostname() != "beta" {
		t.Fatal("ホスト名は独立のはず")
	}
	a.SetHostname("gamma")
	if a.Hostname() != "gamma" || b.Hostname() != "beta" {
		t.Fatal("片方の変更は他方に影響しないはず")
	}
}

func TestMountNamespace(t *testing.T) {
	h := NewHost(gib)
	web := h.NewContainer(Config{Name: "web"})
	db := h.NewContainer(Config{Name: "db"})

	// 同じパス /data に別々の実体をマウント
	web.Mount("/data", "web-volume")
	db.Mount("/data", "db-volume")
	if got := web.Resolve("/data/index.html"); got != "web-volume" {
		t.Fatalf("web の /data は web-volume: %s", got)
	}
	if got := db.Resolve("/data/table.db"); got != "db-volume" {
		t.Fatalf("db の /data は db-volume: %s", got)
	}
	// マウントしていないパスは既定の rootfs
	if got := web.Resolve("/bin/sh"); got != "rootfs" {
		t.Fatalf("既定は rootfs: %s", got)
	}
	// 最長一致: /data/cache の別マウントが勝つ
	web.Mount("/data/cache", "tmpfs")
	if got := web.Resolve("/data/cache/x"); got != "tmpfs" {
		t.Fatalf("最長一致で tmpfs: %s", got)
	}
	// セグメント境界: /database は /data にマッチしない
	if got := web.Resolve("/database"); got != "rootfs" {
		t.Fatalf("セグメント境界を守る: %s", got)
	}
	// マウント先そのもの(末尾スラッシュなし)も一致する
	if got := web.Resolve("/data"); got != "web-volume" {
		t.Fatalf("/data ちょうども一致: %s", got)
	}
}
