package container

import (
	"fmt"
	"sort"
)

// #region netns

// NetNamespace は独立したネットワークスタックの見え方。
// 別々の名前空間なら同じポート番号を同時に使える——だからコンテナごとに
// それぞれ :80 を bind できる。同じ名前空間内での二重 bind だけが衝突する。
type NetNamespace struct {
	ports map[int]string // port -> 使用者(プロセス名など)
}

// NewNetNamespace は何も bind されていないネットワーク名前空間を作る。
func NewNetNamespace() *NetNamespace {
	return &NetNamespace{ports: map[int]string{}}
}

// Bind はポートを確保する。同じ名前空間で既に使われていればエラー。
func (ns *NetNamespace) Bind(port int, who string) error {
	if owner, ok := ns.ports[port]; ok {
		return fmt.Errorf("ポート %d は %q が使用中", port, owner)
	}
	ns.ports[port] = who
	return nil
}

// Ports は bind 済みのポートを昇順で返す。
func (ns *NetNamespace) Ports() []int {
	ps := make([]int, 0, len(ns.ports))
	for p := range ns.ports {
		ps = append(ps, p)
	}
	sort.Ints(ps)
	return ps
}

// #endregion netns
