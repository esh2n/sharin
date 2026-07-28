// Package statefulset は Kubernetes の StatefulSet を最小構成で実装する。
//
// ここまでの章は、ある前提の上に成り立っていた。どの Pod も同じで、区別が
// つかず、どれを消してもよい。だから調整ループは「3個であれ」とだけ言えば
// よかったし、ローリング更新はどれから入れ替えても構わなかった。
//
// だがデータベースのクラスタは、そうはいかない。3台のうち1台がリーダーで、
// 残りが追随する。各台は自分のディスクを持ち、そこに自分だけのデータがある。
// 消えた1台の代わりに新しい1台を立てても、ディスクが空なら同じものにならない。
// 名前も要る。他のメンバーが「web-1 に繋げ」と設定を持っているなら、作り直した
// 相手も web-1 でなければならない。
//
// StatefulSet はこの前提を3つ置き換える。序数のついた安定した名前、作る順と
// 消す順の保証、そして Pod と一対一で結びついたボリューム。どれも「どの Pod も
// 同じ」を捨てることで得られるもので、そのぶん失うものもある。順序を守るとは、
// 1つ詰まれば以降が全部止まるということでもある。
package statefulset

import "sort"

// #region model

// PVC は Pod に紐づくボリューム。序数と一対一で結びつく。
//
// 肝は、これが Pod より長生きすることだ。Pod が消えても PVC は残り、同じ
// 序数の Pod が作り直されたときに、また同じ PVC が繋がる。データが残るのは
// この寿命の差による。
type PVC struct {
	Name    string
	Ordinal int
	Data    string
}

// Pod は序数で識別されるレプリカ。名前は序数から決まる。
type Pod struct {
	Name    string
	Ordinal int
	Ready   bool
	PVC     string

	broken  bool
	readyAt int
}

// #endregion model

// #region set

// Config は集合の設定。
type Config struct {
	// Name は名前の前半。Pod 名は "<Name>-<序数>" になる。
	Name string
	// Replicas は保ちたい数。
	Replicas int
	// StartupTicks は作られてから ready になるまでの周期。
	StartupTicks int
}

// Set は序数で管理されるレプリカの集合。
type Set struct {
	cfg  Config
	pods map[int]*Pod
	pvcs map[int]*PVC
	now  int

	broken map[int]bool // この序数の Pod は ready にならない

	Log []string
}

// New は空の集合を作る。Pod は Step を回すと順に立ち上がる。
func New(cfg Config) *Set {
	return &Set{cfg: cfg, pods: map[int]*Pod{}, pvcs: map[int]*PVC{}, broken: map[int]bool{}}
}

// Scale は目標数を変える。宣言を書き換えるだけで、増減は Step が進める。
func (s *Set) Scale(n int) {
	if n < 0 {
		n = 0
	}
	s.cfg.Replicas = n
	s.logf("目標を " + itoa(n) + " に変更")
}

// SetBroken は、その序数の Pod が ready にならないようにする。
// 壊れた版をその序数だけに入れた状況を作るために使う。
func (s *Set) SetBroken(ordinal int, broken bool) { s.broken[ordinal] = broken }

// Pods は Pod を序数順に返す。
func (s *Set) Pods() []*Pod {
	ords := make([]int, 0, len(s.pods))
	for o := range s.pods {
		ords = append(ords, o)
	}
	sort.Ints(ords)
	out := make([]*Pod, len(ords))
	for i, o := range ords {
		out[i] = s.pods[o]
	}
	return out
}

// PVCs はボリュームを序数順に返す。Pod が無くても残っていることがある。
func (s *Set) PVCs() []*PVC {
	ords := make([]int, 0, len(s.pvcs))
	for o := range s.pvcs {
		ords = append(ords, o)
	}
	sort.Ints(ords)
	out := make([]*PVC, len(ords))
	for i, o := range ords {
		out[i] = s.pvcs[o]
	}
	return out
}

// Ready は ready な Pod の数を返す。
func (s *Set) Ready() int {
	n := 0
	for _, p := range s.pods {
		if p.Ready {
			n++
		}
	}
	return n
}

// Converged は目標数だけ ready に揃ったかを返す。
func (s *Set) Converged() bool { return s.Ready() == s.cfg.Replicas && len(s.pods) == s.cfg.Replicas }

// Write は序数 ordinal のボリュームに書き込む。Pod でなくボリュームに残る。
func (s *Set) Write(ordinal int, data string) {
	if v, ok := s.pvcs[ordinal]; ok {
		v.Data = data
	}
}

// Read は序数 ordinal のボリュームの中身を返す。
func (s *Set) Read(ordinal int) string {
	if v, ok := s.pvcs[ordinal]; ok {
		return v.Data
	}
	return ""
}

// DeletePod は Pod を1つ消す。ボリュームは消さない。
// 作り直された Pod は、同じ序数のボリュームに再び繋がる。
func (s *Set) DeletePod(ordinal int) {
	if _, ok := s.pods[ordinal]; !ok {
		return
	}
	delete(s.pods, ordinal)
	s.logf(s.name(ordinal) + " を削除(ボリュームは残る)")
}

// DeletePVC はボリュームを明示的に消す。ここで初めてデータが失われる。
func (s *Set) DeletePVC(ordinal int) {
	if _, ok := s.pvcs[ordinal]; !ok {
		return
	}
	delete(s.pvcs, ordinal)
	s.logf(s.pvcName(ordinal) + " を削除(ここで初めてデータが消える)")
}

// #endregion set

// #region step

// Step は1周期進める。起動を進めてから、順序に従って作る手か消す手を1つ打つ。
//
// 増やすときは序数の小さいほうから、減らすときは大きいほうから。しかも
// 一度に1つずつで、前のものが ready になるまで次へ進まない。この待ちが
// 「1台目が立ち上がってから2台目」という順序を保証する。
func (s *Set) Step() {
	s.now++
	for _, p := range s.pods {
		if !p.Ready && !p.broken && s.now >= p.readyAt {
			p.Ready = true
			s.logf(p.Name + " が ready になった")
		}
	}

	// 減らす: いちばん大きい序数から1つずつ。
	if len(s.pods) > s.cfg.Replicas {
		last := s.maxOrdinal()
		s.DeletePod(last)
		return
	}

	// 増やす: 次に埋めるべき序数は、まだ Pod が無い最小の序数。
	if len(s.pods) < s.cfg.Replicas {
		next := s.nextOrdinal()
		// それより小さい序数が全部 ready でなければ、まだ作らない。
		if !s.allReadyBelow(next) {
			s.logf("序数 " + itoa(next) + " はまだ作らない(手前が ready でない)")
			return
		}
		s.create(next)
	}
}

// Run は最大 max 周期まで進め、揃ったらそこで止める。
func (s *Set) Run(max int) {
	for i := 0; i < max && !s.Converged(); i++ {
		s.Step()
	}
}

// allReadyBelow は ordinal 未満の序数がすべて ready かを返す。
func (s *Set) allReadyBelow(ordinal int) bool {
	for o := 0; o < ordinal; o++ {
		p, ok := s.pods[o]
		if !ok || !p.Ready {
			return false
		}
	}
	return true
}

// create は序数 ordinal の Pod を作り、同じ序数のボリュームを繋ぐ。
// ボリュームが無ければ新しく作り、あればそれを再利用する。
func (s *Set) create(ordinal int) {
	v, existed := s.pvcs[ordinal]
	if !existed {
		v = &PVC{Name: s.pvcName(ordinal), Ordinal: ordinal}
		s.pvcs[ordinal] = v
	}
	p := &Pod{
		Name: s.name(ordinal), Ordinal: ordinal, PVC: v.Name,
		broken: s.broken[ordinal], readyAt: s.now + s.cfg.StartupTicks,
	}
	if s.cfg.StartupTicks == 0 && !p.broken {
		p.Ready = true
	}
	s.pods[ordinal] = p
	if existed {
		s.logf(p.Name + " を作成(既存のボリューム " + v.Name + " を再接続)")
	} else {
		s.logf(p.Name + " を作成(ボリューム " + v.Name + " を新規作成)")
	}
}

// #endregion step

func (s *Set) nextOrdinal() int {
	for o := 0; ; o++ {
		if _, ok := s.pods[o]; !ok {
			return o
		}
	}
}

func (s *Set) maxOrdinal() int {
	max := -1
	for o := range s.pods {
		if o > max {
			max = o
		}
	}
	return max
}

func (s *Set) name(o int) string    { return s.cfg.Name + "-" + itoa(o) }
func (s *Set) pvcName(o int) string { return "data-" + s.cfg.Name + "-" + itoa(o) }

func (s *Set) logf(msg string) { s.Log = append(s.Log, "step "+itoa(s.now)+": "+msg) }

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
