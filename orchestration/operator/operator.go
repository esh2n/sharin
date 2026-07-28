// Package operator は Kubernetes の Operator パターンを最小構成で実装する。
//
// この編は調整ループから始まった。「Pod は3個であれ」と宣言すれば、
// コントローラが現状と見比べて差を埋める。以降の章で作ってきたものは、
// どれもこの形の変奏だった。スケジューラも、オートスケーラも、更新も、
// 宣言された状態へ寄せていく処理だった。
//
// ここで一段上がる。この形は Kubernetes が用意した資源にしか使えないのか。
// 使えるとしたら何が要るのか。答えは「型を1つ足すだけ」で、その足し方が
// Operator パターンになる。自分のドメインの状態を型として宣言し、その型を
// 現実へ寄せるコントローラを書く。
//
// 面白いのは、この仕組みが運用の手順そのものを対象にできることだ。
// 「バックアップから復元して、レプリカを繋ぎ直して、リーダーを選び直す」
// といった手順書は、状態として書き直せる。「復元済みで、レプリカが3台
// 追随していて、リーダーが1台いる」。あとは差を埋める処理を書けば、手順は
// 消えて状態だけが残る。人が真夜中に手順書を追う代わりに、ループが回る。
package operator

import "sort"

// #region resource

// Phase は自作リソースの状態。宣言された姿にどこまで近づいたかを表す。
type Phase int

const (
	Pending   Phase = iota // まだ何も作られていない
	Creating               // 部品を作っている途中
	Restoring              // バックアップから復元している
	Ready                  // 宣言どおりに揃った
	Degraded               // 揃っていたが崩れた
)

func (p Phase) String() string {
	switch p {
	case Pending:
		return "Pending"
	case Creating:
		return "Creating"
	case Restoring:
		return "Restoring"
	case Ready:
		return "Ready"
	case Degraded:
		return "Degraded"
	}
	return "Unknown"
}

// Spec は人が書く「あるべき姿」。手順ではなく状態だけを書く。
type Spec struct {
	Name string
	// Members はクラスタに欲しいメンバー数。
	Members int
	// RestoreFrom が空でなければ、そのバックアップから復元してから始める。
	RestoreFrom string
}

// Status はコントローラが観測して書き戻す「今の姿」。人は書かない。
type Status struct {
	Phase    Phase
	Members  int    // 実際に立ち上がったメンバー数
	Leader   string // 選ばれたリーダー(空なら未選出)
	Restored bool   // 復元が済んだか
}

// Resource は自作の型。Spec(あるべき姿)と Status(今の姿)を持つ。
// この2つを分けることが、宣言的な仕組みの最小の要件になる。
type Resource struct {
	Spec   Spec
	Status Status
}

// #endregion resource

// #region world

// Member は実際に立ち上がった1つの実体。
type Member struct {
	Name  string
	Ready bool
}

// World はコントローラの外にある現実。ここでは擬似的なクラスタ。
// コントローラはここを観測し、ここへ働きかける。
type World struct {
	members map[string]*Member
	backups map[string]bool
	seq     int
}

// NewWorld は空の現実を作る。
func NewWorld() *World { return &World{members: map[string]*Member{}, backups: map[string]bool{}} }

// PutBackup は復元元として使えるバックアップを1つ置く。
func (w *World) PutBackup(name string) { w.backups[name] = true }

// Members はメンバーを名前順に返す。
func (w *World) Members() []*Member {
	names := make([]string, 0, len(w.members))
	for n := range w.members {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Member, len(names))
	for i, n := range names {
		out[i] = w.members[n]
	}
	return out
}

// Kill はメンバーを1つ落とす。障害を起こすために使う。
func (w *World) Kill(name string) { delete(w.members, name) }

// StartPending は起動待ちのメンバーを立ち上げる。
func (w *World) StartPending() {
	for _, m := range w.members {
		m.Ready = true
	}
}

func (w *World) create(prefix string) *Member {
	w.seq++
	m := &Member{Name: prefix + "-" + itoa(w.seq)}
	w.members[m.Name] = m
	return m
}

func (w *World) readyMembers() []*Member {
	var out []*Member
	for _, m := range w.Members() {
		if m.Ready {
			out = append(out, m)
		}
	}
	return out
}

// #endregion world

// #region reconcile

// Action は1回の調整で打った手(観測・説明用)。
type Action struct {
	Kind   string // "restore" / "create" / "elect" / "noop"
	Target string
}

// Operator は自作リソースを現実へ寄せるコントローラ。
//
// 中身は調整ループそのもので、違うのは扱う型だけになる。Pod の数を数える
// 代わりに、復元が済んだか、メンバーが揃ったか、リーダーが居るかを数える。
// 差を埋める手順は、順序のある運用の手順そのものだが、書き方は「今どの差が
// 残っているか」の判定に変わっている。
type Operator struct {
	Log []string
}

// New はコントローラを作る。
func New() *Operator { return &Operator{} }

// Reconcile は宣言と現実を見比べ、差を1つ埋める。
//
// 1回の呼び出しで1手しか打たないのが肝になる。全部を一気にやろうとすると、
// 途中で失敗したときにどこまで進んだかが分からなくなる。1手ずつ打って
// 状態を書き戻せば、次の呼び出しは必ず現状から再開できる。何度呼んでも
// 安全で、途中で落ちても続きから進む。
func (o *Operator) Reconcile(r *Resource, w *World) Action {
	// ① 復元が指定されていて、まだ済んでいないなら、まず復元する。
	// 順序のある手順は、こうして「まだ済んでいない差」として表現できる。
	if r.Spec.RestoreFrom != "" && !r.Status.Restored {
		if !w.backups[r.Spec.RestoreFrom] {
			r.Status.Phase = Degraded
			o.logf("復元元 " + r.Spec.RestoreFrom + " が見つからない")
			return Action{Kind: "noop"}
		}
		r.Status.Restored = true
		r.Status.Phase = Restoring
		o.logf(r.Spec.RestoreFrom + " から復元した")
		return Action{Kind: "restore", Target: r.Spec.RestoreFrom}
	}

	// ② メンバーが足りなければ1つ作る。
	ready := w.readyMembers()
	r.Status.Members = len(ready)
	if len(w.Members()) < r.Spec.Members {
		m := w.create(r.Spec.Name)
		r.Status.Phase = Creating
		o.logf(m.Name + " を作成")
		return Action{Kind: "create", Target: m.Name}
	}

	// ③ 全員が立ち上がるまでは、まだ次へ進まない。
	if len(ready) < r.Spec.Members {
		r.Status.Phase = Creating
		return Action{Kind: "noop"}
	}

	// ④ リーダーが居ないか、居たはずの者が消えていれば選び直す。
	if r.Status.Leader == "" || !o.alive(w, r.Status.Leader) {
		leader := ready[0].Name // 名前順で決定的に選ぶ
		if r.Status.Leader != "" {
			o.logf("リーダー " + r.Status.Leader + " が居なくなった")
		}
		r.Status.Leader = leader
		r.Status.Phase = Creating
		o.logf(leader + " をリーダーに選出")
		return Action{Kind: "elect", Target: leader}
	}

	// ⑤ 差が無い。宣言どおりに揃っている。
	r.Status.Phase = Ready
	return Action{Kind: "noop"}
}

// Run は差が無くなるまで最大 max 回まわす。
func (o *Operator) Run(r *Resource, w *World, max int) {
	for i := 0; i < max; i++ {
		act := o.Reconcile(r, w)
		w.StartPending() // 作ったメンバーが立ち上がる
		if act.Kind == "noop" && r.Status.Phase == Ready {
			return
		}
	}
}

func (o *Operator) alive(w *World, name string) bool {
	m, ok := w.members[name]
	return ok && m.Ready
}

func (o *Operator) logf(msg string) { o.Log = append(o.Log, msg) }

// #endregion reconcile

// itoa は小さな非負整数を文字列にする(strconv を避ける)。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
