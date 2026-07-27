package txn

// saga.go は 2PC の対極——ロックを持たない分散トランザクション——を実装する。
//
// 2PC は prepare で資源をロックして原子性を守るが、調整役が落ちるとロックを抱えて
// 止まる(ブロッキング)。Saga は逆の割り切りをする。各ステップをローカルに即コミットし、
// 途中で失敗したら「完了済みステップの補償(打ち消し)」を逆順に実行して辻褄を合わせる。
//
// 旅行予約が定番の例だ。航空券を取り、ホテルを取り、レンタカーを取る。レンタカーで
// 失敗したら、ホテルをキャンセルし、航空券をキャンセルする。各予約は即確定するので
// ロックは無く、他の客を待たせない。代わりに「航空券だけ取れている」途中状態が外から
// 見える——原子性は諦め、補償で結果整合に持ち込む。

// #region saga

// Step は Saga の 1 ステップ。本処理(Do)と、それを打ち消す補償(Compensate)の対で定義する。
// 補償は「逆操作」であって「ロールバック」ではない——Do は既にコミット済みなので、
// 取り消しではなく打ち消しの新しい操作を実行する(予約に対するキャンセルのように)。
type Step struct {
	Name       string
	Do         func() error
	Compensate func()
}

// LogEntry は Saga の実行記録。何が実行され、何が補償されたかを順に残す。
type LogEntry struct {
	Step   string
	Action string // "do" / "do-failed" / "compensate"
	Err    string
}

// Saga はステップ列を順に実行し、失敗時は完了済みを逆順に補償する実行機。
type Saga struct {
	steps []Step
	log   []LogEntry
}

// NewSaga はステップ列から Saga を作る。
func NewSaga(steps ...Step) *Saga { return &Saga{steps: steps} }

// Log は実行記録(表示・検査用)。
func (s *Saga) Log() []LogEntry { return s.log }

// Run はステップを先頭から実行する。あるステップが失敗したら、そこで前進をやめ、
// 完了済みステップの補償を逆順に実行してから、元の失敗を返す。
//
// 逆順なのは依存関係のためだ。後のステップは前のステップの結果の上に成り立っている
// ことが多い(ホテルは航空券が取れている前提)。積み上げた順の逆に崩すのが安全になる。
func (s *Saga) Run() error {
	var completed []Step
	for _, st := range s.steps {
		if err := st.Do(); err != nil {
			s.log = append(s.log, LogEntry{Step: st.Name, Action: "do-failed", Err: err.Error()})
			// 完了済みを逆順に補償する。
			for i := len(completed) - 1; i >= 0; i-- {
				completed[i].Compensate()
				s.log = append(s.log, LogEntry{Step: completed[i].Name, Action: "compensate"})
			}
			return err
		}
		s.log = append(s.log, LogEntry{Step: st.Name, Action: "do"})
		completed = append(completed, st)
	}
	return nil
}

// #endregion saga
