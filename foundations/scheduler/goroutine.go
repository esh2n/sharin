// Package scheduler は Go ランタイムの M:N スケジューラ(GMP モデル)の肝——
// **複数のプロセッサ(P)がそれぞれローカル実行キューを持ち、暇な P が忙しい P から
// 仕事を横取りする(work-stealing)**——を、実スレッド不使用で決定的にモデル化する。
// ランタイム内部編のパーツ。
//
// GMP: G = goroutine(走らせたい仕事)、M = OS スレッド、P = プロセッサ
// (スケジューリングの文脈。ローカル実行キューを持つ)。M は P を1つ握らないと
// G を走らせられない——これが「M 個のスレッドで N 本の goroutine を回す」M:N の要。
//
// os 編の協調スケジューラは CPU 1 個・キュー 1 本だった。本章の主題はそこから先——
// **CPU(P)が複数あるとき、仕事をどう均すか**。素朴に「生成した P のキューに積む」だと
// 偏るので、暇な P が仕事を盗んで回る。これが Go が数百万の goroutine をさばく仕組みだ。
package scheduler

import "strconv"

// #region gp

// State は G の状態。
type State int

const (
	Runnable State = iota // 実行待ち(どこかのキューにいる)
	Running               // いま P 上で走っている
	Done                  // 実行し終えた
)

func (s State) String() string {
	switch s {
	case Runnable:
		return "runnable"
	case Running:
		return "running"
	case Done:
		return "done"
	default:
		return "?"
	}
}

// G は goroutine。work は必要な仕事量(tick)、executed は消化済み量。
// 本モデルでは「実際の計算」はせず、work tick を消費し切ったら Done になる。
type G struct {
	ID       int
	Name     string
	work     int
	executed int
	st       State
}

// State は現在の状態を返す。
func (g *G) State() State { return g.st }

// Remaining は残りの仕事量を返す。
func (g *G) Remaining() int { return g.work }

// Executed は消化済みの仕事量を返す。
func (g *G) Executed() int { return g.executed }

// P はプロセッサ(スケジューリング文脈)。ローカル実行キューを持ち、その先頭から
// G を取り出して走らせる。ローカルキューがあることで、G の出し入れに毎回グローバルな
// ロックを取らずに済む——マルチコアでスケールする鍵。
type P struct {
	ID     int
	local  []*G // ローカル実行キュー
	ran    int  // この P が実行した総 tick(負荷の観察用)
	steals int  // この P が横取りに成功した回数
}

// QueueLen はローカルキューの長さを返す。
func (p *P) QueueLen() int { return len(p.local) }

// Ran はこの P が実行した総 tick を返す(負荷分散が効いたかの指標)。
func (p *P) Ran() int { return p.ran }

// Steals はこの P が横取りに成功した回数を返す。
func (p *P) Steals() int { return p.steals }

// QueueNames はローカルキューの G 名を先頭(次に走る)から返す。
func (p *P) QueueNames() []string {
	out := make([]string, len(p.local))
	for i, g := range p.local {
		out[i] = g.Name
	}
	return out
}

// tag は P を "P0" のように表す(トレースで横取り元を示すのに使う)。
func (p *P) tag() string { return "P" + strconv.Itoa(p.ID) }

// #endregion gp
