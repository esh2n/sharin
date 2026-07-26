package container

import "fmt"

// #region cgroup

// CGroup は資源(メモリ・プロセス数)の階層的な予算。
// 親の制限は子孫すべてに効く。charge のたびに自分から先祖までさかのぼって
// どこかの上限を超えないか確認し、超えるなら「全か無か」で拒否する。
// 使用量は先祖にロールアップされるので、親は配下の合計を常に把握できる。
type CGroup struct {
	name      string
	parent    *CGroup
	memLimit  int64 // バイト。0 は無制限
	memUsage  int64 // このグループ配下の合計メモリ
	pidsLimit int   // 0 は無制限
	pidsUsage int   // このグループ配下の合計プロセス数
}

// NewCGroup はルート(制限の根)となる cgroup を作る。
func NewCGroup(name string, memLimit int64, pidsLimit int) *CGroup {
	return &CGroup{name: name, memLimit: memLimit, pidsLimit: pidsLimit}
}

// NewChild は自分の下にぶら下がる子 cgroup を作る。子の使用量は親にも積まれる。
func (cg *CGroup) NewChild(name string, memLimit int64, pidsLimit int) *CGroup {
	return &CGroup{name: name, parent: cg, memLimit: memLimit, pidsLimit: pidsLimit}
}

// Charge はメモリを計上する。自分から先祖までのどれかの上限を超えるなら
// 何も変えずにエラーを返す(OOM = 全か無か)。
func (cg *CGroup) Charge(bytes int64) error {
	for c := cg; c != nil; c = c.parent {
		if c.memLimit > 0 && c.memUsage+bytes > c.memLimit {
			return fmt.Errorf("cgroup %q: メモリ上限超過 (%d + %d > %d)", c.name, c.memUsage, bytes, c.memLimit)
		}
	}
	for c := cg; c != nil; c = c.parent {
		c.memUsage += bytes
	}
	return nil
}

// Uncharge は計上したメモリを戻す(プロセスの終了)。先祖まで減らす。
func (cg *CGroup) Uncharge(bytes int64) {
	for c := cg; c != nil; c = c.parent {
		c.memUsage -= bytes
		if c.memUsage < 0 {
			c.memUsage = 0
		}
	}
}

// AddProcess はプロセス数を 1 増やす。pids 上限を超えるなら拒否する。
func (cg *CGroup) AddProcess() error {
	for c := cg; c != nil; c = c.parent {
		if c.pidsLimit > 0 && c.pidsUsage+1 > c.pidsLimit {
			return fmt.Errorf("cgroup %q: プロセス数の上限 (%d)", c.name, c.pidsLimit)
		}
	}
	for c := cg; c != nil; c = c.parent {
		c.pidsUsage++
	}
	return nil
}

// RemoveProcess はプロセス数を 1 減らす。先祖まで減らす。
func (cg *CGroup) RemoveProcess() {
	for c := cg; c != nil; c = c.parent {
		if c.pidsUsage > 0 {
			c.pidsUsage--
		}
	}
}

// MemUsage はこのグループ配下の合計メモリ使用量(バイト)。
func (cg *CGroup) MemUsage() int64 { return cg.memUsage }

// MemLimit はこのグループのメモリ上限(バイト、0 は無制限)。
func (cg *CGroup) MemLimit() int64 { return cg.memLimit }

// PidsUsage はこのグループ配下の合計プロセス数。
func (cg *CGroup) PidsUsage() int { return cg.pidsUsage }

// #endregion cgroup
