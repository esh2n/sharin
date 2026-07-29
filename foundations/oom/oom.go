// Package oom は OOM killer の最小実装。
//
// メモリが足りないとき、素直な答えは「断る」になる。だが Linux は既定で断らない。
// 予約の合計が物理メモリを超えても通してしまう(オーバーコミット)。
// 断らなかったぶんの帳尻は、実際にページを触った時点で合わせることになり、
// そこで足りなければ誰かを殺す。これが OOM killer。
//
// 「断る」を「あとで殺す」に取り替えた、という言い方ができる。
// 断るのは申し込んだ本人にしか影響しないが、殺すのは誰に当たるか分からない。
//
// ここでは実時間も乱数も使わない。触る順を台本で決めるので、何回やっても同じ結果になる。
package oom

import (
	"fmt"
	"sort"
)

// #region proc

// Proc は1つのプロセス。
type Proc struct {
	Name string
	// Reserved は確保を申し込んだ量。まだ触っていないぶんを含む。
	Reserved int
	// Touched は実際に触った量。物理メモリを消費しているのはこちらだけ。
	Touched int
	// ScoreAdj は oom_score_adj。-1000 なら決して選ばれない。
	ScoreAdj int
	// Dead は殺されたかどうか。
	Dead bool
}

// Score は oom_score。使っている量が物理メモリに占める割合を 0〜1000 で表し、
// oom_score_adj を足す。
//
// 見ているのは Touched であって Reserved ではない。**申し込んだ量ではなく、
// 実際に抱えている量**で選ぶ。殺して効くのはそこだけだからになる。
func Score(p *Proc, total int) int {
	if p.ScoreAdj <= -1000 {
		return -1000 // 免除
	}
	return p.Touched*1000/total + p.ScoreAdj
}

// #endregion proc

// #region system

// Policy は誰を殺すかの決め方。
type Policy int

const (
	// Biggest は oom_score がいちばん高い相手を選ぶ。Linux はこちら。
	Biggest Policy = iota
	// Requester は足りなくした本人を殺す。比較用。
	Requester
)

// Kill は1回ぶんの処刑の記録。
type Kill struct {
	// Requester は足りなくした本人。Victim と一致するとは限らない。
	Requester string
	Victim    string
	Score     int
	// Freed は殺して空いた量。
	Freed int
}

// System は物理メモリと、その上のプロセス。
type System struct {
	Total int
	// Overcommit が false なら、予約の合計が物理を超える申し込みを断る。
	// Linux の既定は true 相当で、断らない。
	Overcommit bool
	Policy     Policy

	procs []*Proc
	Kills []Kill
}

// New はシステムを作る。
func New(total int, overcommit bool) *System {
	return &System{Total: total, Overcommit: overcommit}
}

// Add はプロセスを1つ足す。
func (s *System) Add(name string, scoreAdj int) *Proc {
	p := &Proc{Name: name, ScoreAdj: scoreAdj}
	s.procs = append(s.procs, p)
	return p
}

// Reserved は予約の合計。
func (s *System) Reserved() int { return s.sum(func(p *Proc) int { return p.Reserved }) }

// Touched は実際に触られている合計。物理メモリを食っているのはこれ。
func (s *System) Touched() int { return s.sum(func(p *Proc) int { return p.Touched }) }

func (s *System) sum(f func(*Proc) int) int {
	n := 0
	for _, p := range s.procs {
		if !p.Dead {
			n += f(p)
		}
	}
	return n
}

// #endregion system

// #region reserve

// Reserve は確保を申し込む。
//
// オーバーコミットを切っていれば、予約の合計が物理を超えた時点で断る。
// 入れていれば通る。**通ったことは、あとで触れることを何も約束しない**。
func (s *System) Reserve(name string, n int) error {
	p := s.find(name)
	if p == nil || p.Dead {
		return fmt.Errorf("oom: %s は居ない", name)
	}
	if !s.Overcommit && s.Reserved()+n > s.Total {
		return fmt.Errorf("oom: 予約を断る (%d + %d > %d)", s.Reserved(), n, s.Total)
	}
	p.Reserved += n
	return nil
}

// Touch は予約したぶんを実際に触る。ここで初めて物理メモリを使う。
//
// 足りなければ、足りるまで殺す。1つ殺せば済むとは限らない。
func (s *System) Touch(name string, n int) error {
	p := s.find(name)
	if p == nil || p.Dead {
		return fmt.Errorf("oom: %s は居ない", name)
	}
	for s.Touched()+n > s.Total {
		victim := s.pick(p)
		if victim == nil {
			// 殺せる相手が居ない。ここまで来ると打つ手が無い。
			return fmt.Errorf("oom: 殺せる相手が居ない (%d + %d > %d)", s.Touched(), n, s.Total)
		}
		s.Kills = append(s.Kills, Kill{
			Requester: p.Name,
			Victim:    victim.Name,
			Score:     Score(victim, s.Total),
			Freed:     victim.Touched,
		})
		victim.Dead = true
		victim.Touched, victim.Reserved = 0, 0
		if p.Dead {
			return fmt.Errorf("oom: %s が殺された", p.Name)
		}
	}
	p.Touched += n
	if p.Touched > p.Reserved {
		p.Reserved = p.Touched
	}
	return nil
}

// pick は殺す相手を選ぶ。
func (s *System) pick(requester *Proc) *Proc {
	if s.Policy == Requester {
		if requester.ScoreAdj <= -1000 || requester.Touched == 0 {
			return nil
		}
		return requester
	}
	var best *Proc
	for _, p := range s.procs {
		if p.Dead || p.ScoreAdj <= -1000 || p.Touched == 0 {
			continue
		}
		if best == nil || Score(p, s.Total) > Score(best, s.Total) {
			best = p
		}
	}
	return best
}

func (s *System) find(name string) *Proc {
	for _, p := range s.procs {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// #endregion reserve

// #region report

// Scores は生きているプロセスの oom_score を高い順に返す。
func (s *System) Scores() []struct {
	Name  string
	Score int
} {
	var out []struct {
		Name  string
		Score int
	}
	for _, p := range s.procs {
		if p.Dead {
			continue
		}
		out = append(out, struct {
			Name  string
			Score int
		}{p.Name, Score(p, s.Total)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// #endregion report
