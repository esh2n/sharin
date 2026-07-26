// Package container は、コンテナ隔離(namespace と cgroup)の核を Go でモデル化する。
//
// 実物の Linux コンテナは、1つのカーネルを共有したまま、プロセスに
//   - 制限された「見え方」(namespace: PID / ネットワーク / マウント / ホスト名)
//   - 資源の「予算」(cgroup: メモリ・プロセス数)
//
// を与えたものにすぎない。VM のようにカーネルやハードウェアを丸ごと分ける
// わけではない。ここでは syscall(clone/unshare)を使わず、その仕組みを純粋な
// データ構造で決定的に再現する。
package container

import (
	"fmt"
	"sort"
)

// #region model

// Process は 1つのプロセス。host が付けた通し番号 GlobalPID と、
// 所属コンテナでの見かけの番号 LocalPID を持つ。
type Process struct {
	GlobalPID int
	LocalPID  int
	Name      string
	MemBytes  int64
}

// Host は共有された1つのカーネル(マシン)。global PID を全コンテナに通しで振り、
// 全プロセスとメモリの総量をルート cgroup で束ねる。
type Host struct {
	nextPID int
	procs   map[int]*Process // global PID -> Process(全コンテナ横断)
	root    *CGroup
}

// NewHost は総メモリ totalMem バイトのマシンを作る。
// global PID は 1000 から振る(local PID 1 と見分けやすくするため)。
func NewHost(totalMem int64) *Host {
	return &Host{
		nextPID: 1000,
		procs:   map[int]*Process{},
		root:    NewCGroup("host", totalMem, 0),
	}
}

// Config はコンテナの設定。MemLimit / PidsLimit が 0 なら、その資源は
// host 全体(ルート cgroup)の制限にだけ縛られる。
type Config struct {
	Name      string
	Hostname  string
	MemLimit  int64
	PidsLimit int
}

// Container は namespace 一式と cgroup を束ねた、隔離された実行環境。
type Container struct {
	Name     string
	host     *Host
	pids     *PIDNamespace
	net      *NetNamespace
	mnt      *MountNamespace
	cg       *CGroup
	hostname string // UTS namespace
}

// #endregion model

// #region container

// NewContainer は host の上に新しいコンテナを作る。それぞれ独立した
// PID / ネットワーク / マウント名前空間と、host 配下の子 cgroup を持つ。
func (h *Host) NewContainer(cfg Config) *Container {
	return &Container{
		Name:     cfg.Name,
		host:     h,
		pids:     NewPIDNamespace(),
		net:      NewNetNamespace(),
		mnt:      NewMountNamespace(),
		cg:       h.root.NewChild(cfg.Name, cfg.MemLimit, cfg.PidsLimit),
		hostname: cfg.Hostname,
	}
}

// Spawn は新しいプロセスを起こす。host から global PID を取り、PID 名前空間に
// 登録して local PID を得て、cgroup にプロセス数とメモリを計上する。
// pids 上限やメモリ上限を超えると起動を拒否し、計上は元に戻す(全か無か)。
func (c *Container) Spawn(name string, memBytes int64) (*Process, error) {
	if err := c.cg.AddProcess(); err != nil {
		return nil, err
	}
	if err := c.cg.Charge(memBytes); err != nil {
		c.cg.RemoveProcess()
		return nil, err
	}
	global := c.host.nextPID
	c.host.nextPID++
	local := c.pids.Add(global)
	p := &Process{GlobalPID: global, LocalPID: local, Name: name, MemBytes: memBytes}
	c.host.procs[global] = p
	return p, nil
}

// Kill は local PID のプロセスを終了させ、cgroup の計上を戻して回収する。
func (c *Container) Kill(localPID int) error {
	for _, g := range c.pids.Globals() {
		if l, _ := c.pids.Local(g); l == localPID {
			p := c.host.procs[g]
			c.cg.Uncharge(p.MemBytes)
			c.cg.RemoveProcess()
			c.pids.Remove(g)
			delete(c.host.procs, g)
			return nil
		}
	}
	return fmt.Errorf("PID %d は存在しない", localPID)
}

// Processes はコンテナから見えるプロセスを local PID の昇順で返す。
// 他のコンテナのプロセスは見えない(PID 名前空間の隔離)。
func (c *Container) Processes() []*Process {
	globals := c.pids.Globals()
	out := make([]*Process, 0, len(globals))
	for _, g := range globals {
		if p, ok := c.host.procs[g]; ok {
			out = append(out, p)
		}
	}
	return out
}

// #endregion container

// #region views

// Hostname は UTS 名前空間のホスト名を返す。
func (c *Container) Hostname() string { return c.hostname }

// SetHostname はこのコンテナのホスト名だけを変える(他コンテナに影響しない)。
func (c *Container) SetHostname(h string) { c.hostname = h }

// Bind はネットワーク名前空間でポートを確保する。
func (c *Container) Bind(port int, who string) error { return c.net.Bind(port, who) }

// Ports は bind 済みポートを昇順で返す。
func (c *Container) Ports() []int { return c.net.Ports() }

// Mount はマウント名前空間の target に source を割り当てる。
func (c *Container) Mount(target, source string) { c.mnt.Mount(target, source) }

// Resolve はコンテナ内パスを、マウント表に従って実体に解決する。
func (c *Container) Resolve(path string) string { return c.mnt.Resolve(path) }

// MemUsage はコンテナの cgroup が計上しているメモリ(バイト)。
func (c *Container) MemUsage() int64 { return c.cg.MemUsage() }

// MemLimit はコンテナの cgroup のメモリ上限(バイト、0 は無制限)。
func (c *Container) MemLimit() int64 { return c.cg.MemLimit() }

// PidCount はコンテナの cgroup が数えているプロセス数。
func (c *Container) PidCount() int { return c.cg.PidsUsage() }

// #endregion views

// #region host

// ProcessCount は host が見ている全プロセス数(全コンテナ横断)。
func (h *Host) ProcessCount() int { return len(h.procs) }

// RootUsage は host 全体(ルート cgroup)のメモリ使用量(バイト)。
func (h *Host) RootUsage() int64 { return h.root.MemUsage() }

// GlobalPIDs は host が見ている全 global PID を昇順で返す。
func (h *Host) GlobalPIDs() []int {
	ps := make([]int, 0, len(h.procs))
	for g := range h.procs {
		ps = append(ps, g)
	}
	sort.Ints(ps)
	return ps
}

// #endregion host
