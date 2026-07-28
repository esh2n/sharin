// Package admission は Kubernetes の admission webhook を最小構成で実装する。
//
// RBAC は「誰が」を見た。alice が Pod を作ってよいか。だが、作ってよい人が
// 作ろうとしている中身が妥当かは見ていない。resources を書き忘れた Pod も、
// 特権を要求する Pod も、権限さえあれば通る。
//
// そこで、作られる前にもう一段の関門を置く。admission webhook は、
// 保存される直前のオブジェクトを受け取り、書き換えるか、拒否する。
// 「誰が」でなく「何を」を見る層になる。
//
// 段が2つあることに意味がある。まず書き換え(mutating)、次に検証
// (validating)。この順序でなければならない。書き換えてから検証するので、
// 書き換えた結果も検証を通る。逆にすると、検証を通った後で書き換えられた
// ものが素通りしてしまう。
//
// そして、この仕組みには固有の危険がある。webhook 自身がクラスタの中で
// 動いている Pod だということだ。webhook が落ちているときに何が起きるかを
// 決めておかないと、webhook を動かす Pod の作成すら止まって、復旧できなく
// なる。
package admission

import "sort"

// #region model

// Object は作られようとしているオブジェクト。
type Object struct {
	Kind        string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

// NewObject は空のオブジェクトを作る。
func NewObject(kind, name string) *Object {
	return &Object{Kind: kind, Name: name,
		Labels: map[string]string{}, Annotations: map[string]string{}}
}

// clone は複製を返す。関門を通す前の姿を残しておくために使う。
func (o *Object) clone() *Object {
	c := NewObject(o.Kind, o.Name)
	for k, v := range o.Labels {
		c.Labels[k] = v
	}
	for k, v := range o.Annotations {
		c.Annotations[k] = v
	}
	return c
}

// Keys はラベルの鍵を名前順に返す(観測用)。
func (o *Object) Keys() []string {
	out := make([]string, 0, len(o.Labels))
	for k := range o.Labels {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Stage は関門の段。
type Stage int

const (
	// Mutating は書き換える段。先に走る。
	Mutating Stage = iota
	// Validating は検証する段。書き換えの後に走る。
	Validating
)

func (s Stage) String() string {
	if s == Validating {
		return "Validating"
	}
	return "Mutating"
}

// FailurePolicy は webhook が応答しないときの振る舞い。
type FailurePolicy int

const (
	// Fail は応答が無ければ拒否する。検証の抜けを許さない代わり、
	// webhook が落ちるとオブジェクトが1つも作れなくなる。
	Fail FailurePolicy = iota
	// Ignore は応答が無ければ素通しする。作成は止まらない代わり、
	// 検証されていないものが通る。
	Ignore
)

func (f FailurePolicy) String() string {
	if f == Ignore {
		return "Ignore"
	}
	return "Fail"
}

// Webhook は1つの関門。書き換えるか、検証するかのどちらかを担う。
type Webhook struct {
	Name  string
	Stage Stage
	// Kinds は対象の種類。空ならすべてに当たる。
	Kinds []string
	// Available が偽なら応答しない(webhook 自体が落ちている)。
	Available bool
	// Failure は応答しないときの扱い。
	Failure FailurePolicy
	// Mutate は書き換えを行い、何をしたかを返す(空なら何もしていない)。
	Mutate func(o *Object) string
	// Check は検証し、拒否理由を返す(空なら合格)。
	Check func(o *Object) string
}

// applies は obj がこの webhook の対象かを返す。
func (w *Webhook) applies(o *Object) bool {
	if len(w.Kinds) == 0 {
		return true
	}
	for _, k := range w.Kinds {
		if k == o.Kind {
			return true
		}
	}
	return false
}

// #endregion model

// #region chain

// Result は1回の関門通過の結果。
type Result struct {
	Allowed bool
	Object  *Object  // 通った場合、書き換え後の姿
	Applied []string // 書き換えの内容
	Reason  string
}

// Chain は関門の並び。
type Chain struct {
	hooks []*Webhook

	Admitted int
	Rejected int
	Bypassed int // 応答が無く素通しした回数
	Log      []string
}

// New は関門を持たない並びを作る。
func New() *Chain { return &Chain{} }

// Add は関門を1つ足す。
func (c *Chain) Add(w *Webhook) *Webhook {
	c.hooks = append(c.hooks, w)
	return w
}

// Hooks は関門を段の順(書き換え → 検証)に返す。
func (c *Chain) Hooks() []*Webhook {
	out := append([]*Webhook(nil), c.hooks...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Stage < out[j].Stage })
	return out
}

// Admit はオブジェクトを関門に通す。
//
// 段の順序が肝になる。まず書き換えの段を全部通し、その結果に対して検証の段を
// 通す。書き換えたものが検証されるので、書き換えで壊れたものは検証で止まる。
// 逆順にすると、検証を通った後で書き換えられたものが素通りしてしまう。
func (c *Chain) Admit(in *Object) Result {
	obj := in.clone()
	var applied []string

	for _, stage := range []Stage{Mutating, Validating} {
		for _, w := range c.hooks {
			if w.Stage != stage || !w.applies(obj) {
				continue
			}

			// 応答が無いときの扱いは、あらかじめ決めた方針で決まる。
			if !w.Available {
				if w.Failure == Fail {
					c.Rejected++
					c.logf(obj.Name + " を拒否(" + w.Name + " が応答せず、方針は Fail)")
					return Result{Object: obj, Applied: applied,
						Reason: w.Name + " が応答しない。方針が Fail なので拒否する"}
				}
				c.Bypassed++
				c.logf(w.Name + " が応答しないが、方針は Ignore なので素通しする")
				continue
			}

			if stage == Mutating && w.Mutate != nil {
				if msg := w.Mutate(obj); msg != "" {
					applied = append(applied, w.Name+": "+msg)
					c.logf(obj.Name + " を書き換え(" + w.Name + ": " + msg + ")")
				}
				continue
			}
			if stage == Validating && w.Check != nil {
				if why := w.Check(obj); why != "" {
					c.Rejected++
					c.logf(obj.Name + " を拒否(" + w.Name + ": " + why + ")")
					return Result{Object: obj, Applied: applied,
						Reason: w.Name + " が拒否: " + why}
				}
			}
		}
	}

	c.Admitted++
	return Result{Allowed: true, Object: obj, Applied: applied, Reason: "すべての関門を通った"}
}

// #endregion chain

func (c *Chain) logf(msg string) { c.Log = append(c.Log, msg) }
