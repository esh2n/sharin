// Package os は、最小カーネルと協調スケジューラの核を Go でモデル化する。
// 計算機の土台編の3つ目のパーツ。
//
// OS のいちばん奥にある仕事は「誰が CPU を握るかを決める」ことだ。ここでは
// その部分(スケジューラ)を、実スレッドや割り込みを使わず純粋なデータ構造で
// 決定的に再現する。採るのは協調(cooperative)方式——タスクが自発的に yield /
// sleep したときにだけスケジューラへ制御が戻り、強制的な横取り(プリエンプション)は
// 起きない。初期の Mac OS(〜9)や Windows 3.x、いまの async/await・goroutine の
// 手前にある考え方そのものである。
package os

// #region task

// State はタスクの状態。協調スケジューラでは、状態遷移は
// タスク自身の yield / sleep と、スケジューラの dispatch / wake でしか起きない。
type State int

const (
	Ready   State = iota // 実行可能。run queue で CPU の順番を待っている
	Running              // いま CPU を握っている(常に高々1つ)
	Blocked              // sleep 中。起床時刻になるまで run queue に戻らない
	Done                 // プログラムを最後まで実行し終えた
)

// String は状態の短い表示名を返す(トレースやデモ用)。
func (s State) String() string {
	switch s {
	case Ready:
		return "ready"
	case Running:
		return "running"
	case Blocked:
		return "blocked"
	case Done:
		return "done"
	}
	return "?"
}

// OpKind はタスクのプログラムを構成する命令の種類。
type OpKind int

const (
	OpRun   OpKind = iota // CPU を Arg tick 使う。この間 yield しない(協調の肝)
	OpYield               // 自発的に CPU を手放し、run queue の最後尾に並び直す
	OpSleep               // Arg tick 分ブロックする。時刻が来たら起こされる
)

// Op はタスクのプログラムの 1 命令。プログラムはこの命令の並びにすぎない。
type Op struct {
	Kind OpKind
	Arg  int
}

// Run は CPU を ticks 分使う命令。途中で yield しないので、この命令が長いほど
// 他タスクを待たせる——協調スケジューリングの弱点そのもの。ticks < 1 は 1 として扱う。
func Run(ticks int) Op { return Op{Kind: OpRun, Arg: ticks} }

// Yield は自発的に CPU を手放す命令。協調方式では、ここでだけスケジューラに制御が戻る。
func Yield() Op { return Op{Kind: OpYield} }

// Sleep は ticks 分ブロックする命令。起床時刻が来ると run queue に戻される。
func Sleep(ticks int) Op { return Op{Kind: OpSleep, Arg: ticks} }

// Task は 1 つの実行単位(プロセス/スレッド/コルーチン)。
//
// このタスクの「文脈(コンテキスト)」の実体は pc ——プログラムのどこまで
// 実行したか——だけである。実機ではレジスタやスタックポインタを退避するが、
// 本質は同じで、context switch とは pc の保存と復元にすぎない。
type Task struct {
	Name string
	prog []Op
	pc   int // 保存された文脈: 次に実行する命令の位置
	st   State
	cpu  int // これまでに使った CPU tick の合計
	wake int // Blocked のとき、起こされる時刻
	seq  int // 生成順。同時起床の決定的な順序づけに使う
}

// State はタスクの現在の状態を返す。
func (t *Task) State() State { return t.st }

// CPU はこのタスクがこれまでに使った CPU tick の合計を返す。
func (t *Task) CPU() int { return t.cpu }

// #endregion task
