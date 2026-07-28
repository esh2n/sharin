// Package csi はボリュームの確保から Pod に見えるまでを最小構成で実装する。
//
// [StatefulSet](statefulset)の章では、ボリュームは Pod より長生きだと書いた。
// 消えない置き場があるおかげで、同じ序数の Pod が作り直されても続きから始められる。
// その「消えない置き場」を用意している下の層が、この章になる。
//
// まず、要求と実体が分かれている。使う側が書くのは「20GiB を1ノードから読み書きで
// 欲しい」という要求だけで、それがどのディスクになるかは知らない。要求に合う実体を
// 用意するのは別の担当になる。
//
// そして、用意してから Pod に見えるまでが3段に分かれている。作る、ノードに繋ぐ、
// Pod から見えるようにする。分かれているのは、それぞれ担当も失敗の仕方も違うからだ。
// この分かれ方を知らないと、いちばん有名な誤解にはまる。ReadWriteOnce は「1つの
// Pod から」ではなく「1つのノードから」を意味する。繋ぐのがノード単位だからだ。
//
// もう1つ、順序の問題がある。素朴には、先にディスクを用意してから Pod を置けば
// よさそうに見える。だが区画([topology](topology))がある環境では、先に用意すると
// その区画から出られなくなる。だから待つ、という選択肢が要る。
package csi

import "sort"

// #region model

// AccessMode は同時にどう使えるか。
type AccessMode int

const (
	// ReadWriteOnce は1つの「ノード」から読み書きできる。
	// 1つの Pod から、ではない。同じノードに載った2つの Pod は共有できる。
	ReadWriteOnce AccessMode = iota
	// ReadOnlyMany は複数ノードから読める。
	ReadOnlyMany
	// ReadWriteMany は複数ノードから読み書きできる。
	ReadWriteMany
)

func (a AccessMode) String() string {
	return [...]string{"ReadWriteOnce", "ReadOnlyMany", "ReadWriteMany"}[a]
}

// Binding は確保の時機。
type Binding int

const (
	// Immediate は要求が来た時点で実体を作る。
	Immediate Binding = iota
	// WaitForFirstConsumer は、使う Pod の置き場所が決まるまで作らない。
	WaitForFirstConsumer
)

// Reclaim は要求が消えたときに実体をどうするか。
type Reclaim int

const (
	// Delete は実体も消す。
	Delete Reclaim = iota
	// Retain は実体を残す。中身を捨てたくないときに使う。
	Retain
)

// Class は実体の作り方の型。運用側が用意する。
type Class struct {
	Name    string
	Binding Binding
	Reclaim Reclaim
}

// Claim は使う側が書く要求。どのディスクかは書かない。
type Claim struct {
	Name  string
	Class string
	Size  int
	Mode  AccessMode
	// Zone は要求した区画(空なら指定なし)。
	Zone string
}

// Volume は実体。区画を持つので、後から他の区画へは動かせない。
type Volume struct {
	Name    string
	Claim   string
	Size    int
	Mode    AccessMode
	Zone    string
	Reclaim Reclaim
	// AttachedTo は今どのノードに繋がっているか(空なら繋がっていない)。
	AttachedTo string
	// MountedBy は今どの Pod から見えているか。
	MountedBy []string
	Released  bool // 要求が消えた後も残っている
}

// #endregion model

// #region driver

// Driver はボリュームの確保から取り外しまでを担う。
//
// 3つの段に分かれた操作を持つのが、この面の形になる。実物の CSI も
// Controller 側(作る・繋ぐ)と Node 側(見えるようにする)で別の面になっている。
type Driver struct {
	classes map[string]Class
	claims  map[string]Claim
	vols    map[string]*Volume
	seq     int

	Log []string
}

// New は class を登録した Driver を作る。
func New(cs ...Class) *Driver {
	d := &Driver{classes: map[string]Class{}, claims: map[string]Claim{}, vols: map[string]*Volume{}}
	for _, c := range cs {
		d.classes[c.Name] = c
	}
	return d
}

// Volumes は実体を名前順で返す。
func (d *Driver) Volumes() []*Volume {
	var out []*Volume
	for _, v := range d.vols {
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// VolumeFor は要求に結びついた実体を返す(無ければ nil)。
func (d *Driver) VolumeFor(claim string) *Volume {
	for _, v := range d.vols {
		if v.Claim == claim && !v.Released {
			return v
		}
	}
	return nil
}

// Request は要求を受け取る。
//
// ここで実体ができるとは限らないのが大事なところになる。時機が
// WaitForFirstConsumer なら、使う Pod の置き場所が決まるまで何も作らない。
func (d *Driver) Request(c Claim) (bool, string) {
	cl, ok := d.classes[c.Class]
	if !ok {
		return false, "そんな class は無い"
	}
	d.claims[c.Name] = c
	if cl.Binding == WaitForFirstConsumer {
		d.logf(c.Name + " は使う Pod の置き場所が決まるまで待つ")
		return true, ""
	}
	d.provision(c, cl, c.Zone)
	return true, ""
}

// provision は実体を作る。区画はここで決まり、後から変えられない。
func (d *Driver) provision(c Claim, cl Class, zone string) *Volume {
	d.seq++
	v := &Volume{
		Name: "pv-" + itoa(d.seq), Claim: c.Name, Size: c.Size,
		Mode: c.Mode, Zone: zone, Reclaim: cl.Reclaim,
	}
	d.vols[v.Name] = v
	if zone == "" {
		d.logf(v.Name + " を作った(" + itoa(c.Size) + "GiB、区画の指定なし)")
	} else {
		d.logf(v.Name + " を作った(" + itoa(c.Size) + "GiB、区画 " + zone + ")")
	}
	return v
}

// #endregion driver

// #region stages

// Attach は実体をノードに繋ぐ。ノード単位の操作になる。
//
// 待つ設定の要求なら、ここで初めて実体ができる。Pod の置き場所が決まって
// はじめて区画が決まるので、区画をまたいで繋がらない事故を避けられる。
func (d *Driver) Attach(claim, node, zone string) (bool, string) {
	c, ok := d.claims[claim]
	if !ok {
		return false, "そんな要求は無い"
	}
	v := d.VolumeFor(claim)
	if v == nil {
		cl := d.classes[c.Class]
		if cl.Binding != WaitForFirstConsumer {
			return false, "実体がまだ無い"
		}
		v = d.provision(c, cl, zone) // 置き場所が決まったので、ここで作る
	}
	if v.Zone != "" && zone != "" && v.Zone != zone {
		return false, "実体は区画 " + v.Zone + " にある。区画 " + zone + " のノードからは繋がらない"
	}
	if v.AttachedTo == node {
		return true, ""
	}
	if v.AttachedTo != "" {
		if v.Mode == ReadWriteOnce {
			return false, "すでに " + v.AttachedTo + " に繋がっている。" +
				ReadWriteOnce.String() + " は1つのノードからしか繋げない"
		}
	}
	v.AttachedTo = node
	d.logf(v.Name + " を " + node + " に繋いだ")
	return true, ""
}

// Mount は Pod から見えるようにする。Pod 単位の操作になる。
//
// 繋がっているノードの上の Pod なら、何個からでも見える。
// ReadWriteOnce が「1つの Pod から」ではないことが、ここに出る。
func (d *Driver) Mount(claim, node, pod string) (bool, string) {
	v := d.VolumeFor(claim)
	if v == nil {
		return false, "実体がまだ無い"
	}
	if v.AttachedTo != node {
		return false, "このノードに繋がっていない(今は " + orNone(v.AttachedTo) + ")"
	}
	for _, p := range v.MountedBy {
		if p == pod {
			return true, ""
		}
	}
	v.MountedBy = append(v.MountedBy, pod)
	sort.Strings(v.MountedBy)
	d.logf(v.Name + " が " + pod + " から見えるようになった")
	return true, ""
}

// Unmount は Pod から見えなくする。
func (d *Driver) Unmount(claim, pod string) {
	v := d.VolumeFor(claim)
	if v == nil {
		return
	}
	var rest []string
	for _, p := range v.MountedBy {
		if p != pod {
			rest = append(rest, p)
		}
	}
	v.MountedBy = rest
}

// Detach はノードから外す。まだ見えている Pod があれば外せない。
//
// 外れないことが、ノードが死んだときの引き継ぎの遅さの正体になる。
// ノードが応答しないと、外れたことを確かめられない。
func (d *Driver) Detach(claim string) (bool, string) {
	v := d.VolumeFor(claim)
	if v == nil {
		return false, "実体がまだ無い"
	}
	if len(v.MountedBy) > 0 {
		return false, "まだ " + itoa(len(v.MountedBy)) + " 個の Pod から見えている"
	}
	if v.AttachedTo == "" {
		return true, ""
	}
	d.logf(v.Name + " を " + v.AttachedTo + " から外した")
	v.AttachedTo = ""
	return true, ""
}

// ForceDetach はノードの応答を待たずに外す。ノードが死んだときの最後の手段。
func (d *Driver) ForceDetach(claim string) {
	v := d.VolumeFor(claim)
	if v == nil {
		return
	}
	v.MountedBy = nil
	v.AttachedTo = ""
	d.logf(v.Name + " をノードの応答を待たずに外した")
}

// #endregion stages

// #region release

// DeleteClaim は要求を消す。実体をどうするかは class が決めてある。
func (d *Driver) DeleteClaim(claim string) {
	v := d.VolumeFor(claim)
	delete(d.claims, claim)
	if v == nil {
		return
	}
	if v.Reclaim == Retain {
		v.Released = true
		d.logf(v.Name + " は残す設定なので、要求から切り離して残した")
		return
	}
	delete(d.vols, v.Name)
	d.logf(v.Name + " を消した")
}

// #endregion release

func orNone(s string) string {
	if s == "" {
		return "どこにも繋がっていない"
	}
	return s
}

func (d *Driver) logf(msg string) { d.Log = append(d.Log, msg) }

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
