package container

import "sort"

// #region pidns

// PIDNamespace は「プロセス番号(PID)の独立した見え方」を表す。
// 新しい名前空間では最初のプロセスが必ず local PID 1(init)になり、
// 中のプロセスは同じ名前空間のプロセスしか番号で参照できない。
// host が付ける global PID とは別に、コンテナごとの local PID を割り振る。
type PIDNamespace struct {
	nextLocal int
	local     map[int]int // global PID -> local PID
}

// NewPIDNamespace は空の名前空間を作る。最初の Add は local PID 1 を返す。
func NewPIDNamespace() *PIDNamespace {
	return &PIDNamespace{nextLocal: 1, local: map[int]int{}}
}

// Add は host が割り当てた global PID を名前空間に登録し、local PID を返す。
func (ns *PIDNamespace) Add(global int) int {
	l := ns.nextLocal
	ns.nextLocal++
	ns.local[global] = l
	return l
}

// Remove は global PID を名前空間から外す(プロセスの回収)。
func (ns *PIDNamespace) Remove(global int) {
	delete(ns.local, global)
}

// Local は global PID に対応する local PID を返す(未登録なら false)。
func (ns *PIDNamespace) Local(global int) (int, bool) {
	l, ok := ns.local[global]
	return l, ok
}

// Globals は名前空間に見えている global PID を local PID の昇順で返す。
func (ns *PIDNamespace) Globals() []int {
	gs := make([]int, 0, len(ns.local))
	for g := range ns.local {
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool { return ns.local[gs[i]] < ns.local[gs[j]] })
	return gs
}

// #endregion pidns
