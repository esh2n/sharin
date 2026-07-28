// Package kubelet は kubelet とコンテナランタイムの境界を最小構成で実装する。
//
// [スケジューラ](scheduler)は Pod をどのノードに置くかを決めた。だが決めただけでは
// 何も動かない。決まった配置を実際にプロセスとして起こすのが、各ノードで動いている
// kubelet になる。この編でずっと扱ってきた話の、いちばん下の端になる。
//
// kubelet も調整ループになっている。あるべき姿と現状を比べて差を埋める。ただし
// 今までのコントローラと違うところが1つあって、現状の取得先が置き場ではない。
// ランタイムに聞く。「宣言ではこうなっている」と「実際にプロセスが動いている」を
// 突き合わせるのは、この層が初めてになる。
//
// 聞き方も面白い。ランタイムは何かあったら教えてくれる仕組みを持っているが、
// kubelet はそれを主に使わず、定期的に一覧を取り直す。[調整ループ](reconcile)の
// 章で見た level-triggered が、いちばん下の層でも同じ形で現れている。
//
// そしてもう1つ、この層にしかないものがある。ファイルから読む Pod だ。置き場を
// 経由せず、ノード上のディレクトリに置いたファイルから直接起動する。これが
// 鶏と卵を解く。[API サーバ](apiserver)自身が Pod として動いているとき、
// その API サーバを誰が起動するのか。kubelet が、ファイルから起動する。
package kubelet

import "sort"

// #region cri

// State はランタイムから見たコンテナの状態。
type State int

const (
	Creating State = iota // 起動中
	Running               // 稼働中
	Exited                // 終了した
)

func (s State) String() string {
	return [...]string{"Creating", "Running", "Exited"}[s]
}

// ContainerSpec は1つのコンテナの宣言。挙動は台本で与える。
type ContainerSpec struct {
	Name string
	// StartupTicks は起動にかかる時間。
	StartupTicks int
	// FailAfter は稼働に入ってから落ちるまでの時間(0 なら落ちない)。
	FailAfter int
}

// ContainerStatus はランタイムが返す一覧の1件。
type ContainerStatus struct {
	ID    string
	Pod   string
	Name  string
	State State
}

type instance struct {
	id    string
	pod   string
	spec  ContainerSpec
	state State
	left  int // 起動までの残り、または落ちるまでの残り
}

// Runtime は CRI の向こう側。kubelet はここに直接触らず、この面越しにだけ扱う。
//
// 境界を1枚置いたことが、実装を差し替えられることの正体になる。kubelet が
// 知っているのは「作れ」「消せ」「一覧をくれ」の3つだけで、その先が何であるかは
// 知らない。
type Runtime struct {
	seq   int
	items []*instance

	// Relists は一覧を取り直した回数。イベントに頼っていないことがここに出る。
	Relists int
	// Creates と Removes は呼び出しの回数。
	Creates int
	Removes int
}

// NewRuntime は空のランタイムを作る。
func NewRuntime() *Runtime { return &Runtime{} }

// Create はコンテナを作り、識別子を返す。
func (r *Runtime) Create(pod string, spec ContainerSpec) string {
	r.seq++
	r.Creates++
	id := "c" + itoa(r.seq)
	st := Creating
	left := spec.StartupTicks
	if left <= 0 {
		st, left = Running, spec.FailAfter
	}
	r.items = append(r.items, &instance{id: id, pod: pod, spec: spec, state: st, left: left})
	return id
}

// Remove はコンテナを消す。
func (r *Runtime) Remove(id string) {
	var rest []*instance
	for _, it := range r.items {
		if it.id != id {
			rest = append(rest, it)
			continue
		}
		r.Removes++
	}
	r.items = rest
}

// List は今あるコンテナの一覧を返す。Pod 名、コンテナ名の順で決定的にする。
func (r *Runtime) List() []ContainerStatus {
	r.Relists++
	out := make([]ContainerStatus, 0, len(r.items))
	for _, it := range r.items {
		out = append(out, ContainerStatus{ID: it.id, Pod: it.pod, Name: it.spec.Name, State: it.state})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pod != out[j].Pod {
			return out[i].Pod < out[j].Pod
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Step は実際の世界を1つ進める。起動が完了し、台本どおりにプロセスが落ちる。
// kubelet はこれを呼ばない。世界が勝手に進むことを表している。
func (r *Runtime) Step() {
	for _, it := range r.items {
		switch it.state {
		case Creating:
			it.left--
			if it.left <= 0 {
				it.state = Running
				it.left = it.spec.FailAfter
			}
		case Running:
			if it.spec.FailAfter <= 0 {
				continue
			}
			it.left--
			if it.left <= 0 {
				it.state = Exited
			}
		}
	}
}

// #endregion cri

// #region spec

// Source は Pod の宣言がどこから来たか。
type Source int

const (
	// FromAPIServer は置き場から届いたもの。
	FromAPIServer Source = iota
	// FromFile はノード上のファイルから読んだもの。置き場を経由しない。
	FromFile
)

func (s Source) String() string {
	return [...]string{"apiserver", "file"}[s]
}

// PodSpec はこのノードで動かすべき Pod。
type PodSpec struct {
	Name       string
	Source     Source
	Containers []ContainerSpec
	// Restart が偽なら、落ちても作り直さない。
	Restart bool
}

// #endregion spec

// #region kubelet

// Kubelet は1台のノードで、宣言と現実の差を埋め続ける。
type Kubelet struct {
	rt *Runtime

	filePods []PodSpec // ファイルから読んだもの。置き場と無関係に動く
	apiPods  []PodSpec // 置き場から届いたもの。最後に届いた内容を持ち続ける
	linked   bool      // 置き場に届くか

	now       int
	restarts  map[string]int
	waitUntil map[string]int // 再作成を待つ時刻

	Log []string
}

// New はランタイムに繋がった kubelet を作る。置き場とは最初は繋がっていない。
func New(rt *Runtime) *Kubelet {
	return &Kubelet{rt: rt, restarts: map[string]int{}, waitUntil: map[string]int{}}
}

// SetFilePods はノード上のファイルを置き換える。置き場とは無関係に効く。
func (k *Kubelet) SetFilePods(ps []PodSpec) {
	k.filePods = normalize(ps, FromFile)
	k.logf("ファイルの宣言を読み直した(" + itoa(len(ps)) + " 件)")
}

// Link は置き場との接続を切り替える。
func (k *Kubelet) Link(up bool) {
	if k.linked == up {
		return
	}
	k.linked = up
	if up {
		k.logf("置き場に届くようになった")
	} else {
		k.logf("置き場に届かなくなった。ファイルの宣言だけで動き続ける")
	}
}

// Linked は置き場に届くかを返す。
func (k *Kubelet) Linked() bool { return k.linked }

// Deliver は置き場から届いた宣言を受け取る。届かない状態なら何も起きない。
//
// 届かない間、最後に受け取った宣言が残り続けるのが大事なところになる。
// kubelet は置き場を見失っても止まらない。知っている宣言を守り続ける。
func (k *Kubelet) Deliver(ps []PodSpec) bool {
	if !k.linked {
		return false
	}
	k.apiPods = normalize(ps, FromAPIServer)
	return true
}

func normalize(ps []PodSpec, src Source) []PodSpec {
	out := append([]PodSpec(nil), ps...)
	for i := range out {
		out[i].Source = src
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Desired はこのノードで動いているべき Pod を返す。2つの出所を合わせたもの。
func (k *Kubelet) Desired() []PodSpec {
	out := append([]PodSpec(nil), k.filePods...)
	out = append(out, k.apiPods...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Now は現在の論理時刻を返す。
func (k *Kubelet) Now() int { return k.now }

// Restarts はコンテナが作り直された回数を返す。
func (k *Kubelet) Restarts(pod, name string) int { return k.restarts[pod+"/"+name] }

// #endregion kubelet

// #region sync

// Sync は1周ぶんの調整を行う。
//
// 一覧を取り直すところから始まるのが肝になる。前回から何が変わったかを
// 覚えておいて差分で進めるのではなく、毎回まるごと見る。取りこぼしても
// 次の周で必ず気づく。
func (k *Kubelet) Sync() {
	actual := k.rt.List()

	// 今あるものを、Pod とコンテナの組で引けるようにする。
	live := map[string]ContainerStatus{}
	for _, s := range actual {
		live[s.Pod+"/"+s.Name] = s
	}

	wanted := map[string]bool{}
	for _, p := range k.Desired() {
		for _, c := range p.Containers {
			key := p.Name + "/" + c.Name
			wanted[key] = true

			cur, ok := live[key]
			switch {
			case !ok:
				k.start(p, c, key)
			case cur.State == Exited:
				k.rt.Remove(cur.ID)
				if !p.Restart {
					k.logf(key + " は終了した。作り直さない宣言なのでそのまま")
					continue
				}
				k.restarts[key]++
				k.waitUntil[key] = k.now + backoffFor(k.restarts[key])
				k.logf(key + " が落ちた。" + itoa(backoffFor(k.restarts[key])) +
					" 待って作り直す(" + itoa(k.restarts[key]) + " 回目)")
			}
		}
	}

	// 宣言に無いものは消す。集合の差を埋める形は[DaemonSet](daemonset)と同じ。
	for _, s := range actual {
		if !wanted[s.Pod+"/"+s.Name] {
			k.rt.Remove(s.ID)
			k.logf(s.Pod + "/" + s.Name + " は宣言に無い。消す")
		}
	}
}

// start は待ち時間を見たうえでコンテナを作る。
func (k *Kubelet) start(p PodSpec, c ContainerSpec, key string) {
	if until, ok := k.waitUntil[key]; ok && k.now < until {
		return // まだ待ち時間の内側
	}
	k.rt.Create(p.Name, c)
	if k.restarts[key] == 0 {
		k.logf(key + " を作った(" + p.Source.String() + " の宣言)")
	} else {
		k.logf(key + " を作り直した")
	}
}

// backoffFor は作り直しの待ち時間を倍に伸ばしていく(上限つき)。
func backoffFor(restarts int) int {
	d := 1
	for i := 1; i < restarts; i++ {
		d *= 2
		if d >= 8 {
			return 8
		}
	}
	return d
}

// Tick は世界を1つ進めてから調整する。
//
// 順序が大事で、先に世界が動く。kubelet は起きたことを後から知る。
func (k *Kubelet) Tick() {
	k.rt.Step()
	k.Sync()
	k.now++
}

// Running は今稼働しているコンテナの「Pod/名前」を返す。
func (k *Kubelet) Running() []string {
	var out []string
	for _, s := range k.rt.List() {
		if s.State == Running {
			out = append(out, s.Pod+"/"+s.Name)
		}
	}
	return out
}

// #endregion sync

func (k *Kubelet) logf(msg string) { k.Log = append(k.Log, "t="+itoa(k.now)+" "+msg) }

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
