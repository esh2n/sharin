// Package rbac は Kubernetes の RBAC を最小構成で実装する。
//
// 前章の NetworkPolicy は、Pod どうしが繋いでよいかを絞った。こちらは、
// 誰が API を叩いてよいかを絞る。似た話に見えるが、既定の向きが逆になっている。
// 通信は何も書かなければ全通しだった。API は何も書かなければ何も通らない。
//
// この差は、扱っているものの違いから来ている。通信を既定で塞ぐと何も動かない。
// 一方 API は、叩けないことが正常な初期状態になる。新しく作った利用者が、
// いきなりクラスタ全体を消せては困る。
//
// 仕組みは単純な足し算の許可リストで、拒否は書けない。「この人はこれをして
// よい」しか書けず、「これはしてはいけない」は書けない。書けないほうが安全に
// なるのは、拒否と許可が混ざると、どちらが勝つかの規則が要るからだ。規則が
// 増えれば、実際に何が通るのかを人が追えなくなる。足し算だけなら、通るものは
// どこかに書いてあり、書いていないものは通らない。
package rbac

import "sort"

// #region model

// Verb は API に対する操作。
type Verb string

const (
	Get    Verb = "get"
	List   Verb = "list"
	Create Verb = "create"
	Update Verb = "update"
	Delete Verb = "delete"
)

// PolicyRule は「この資源にこの操作をしてよい」という許可1つ。
type PolicyRule struct {
	// Resources は対象の資源。"*" ならすべて。
	Resources []string
	// Verbs は許す操作。"*" ならすべて。
	Verbs []Verb
}

// allows は資源 res への操作 verb を許すかを返す。
func (r PolicyRule) allows(res string, verb Verb) bool {
	return contains(r.Resources, res) && containsVerb(r.Verbs, verb)
}

// Role は許可の束。名前をつけて、まとめて渡せるようにしたもの。
//
// 役割と、それを誰に与えるかを分けているのが肝になる。役割の中身を直せば、
// それを持つ全員に一度に効く。人ごとに許可を書くと、方針が変わったときに
// 全員ぶんを直すことになる。
type Role struct {
	Name  string
	Rules []PolicyRule
}

// Binding は「この役割を、この人たちに与える」という結びつけ。
type Binding struct {
	Role     string
	Subjects []string
}

// #endregion model

// #region decide

// Decision は1回の判定の結果と、その理由。
type Decision struct {
	Allowed bool
	Role    string // 効いた役割(拒否のときは空)
	Reason  string
}

// Authorizer は役割と結びつけを持ち、可否を判定する。
type Authorizer struct {
	roles    map[string]*Role
	bindings []Binding

	Allowed int
	Denied  int
	Log     []string
}

// New は何も許可されていない状態から始める。
// 何も書かなければ何も通らない、というのが既定になる。
func New() *Authorizer { return &Authorizer{roles: map[string]*Role{}} }

// AddRole は役割を1つ定義する。定義しただけでは誰にも効かない。
func (a *Authorizer) AddRole(r *Role) *Role {
	a.roles[r.Name] = r
	return r
}

// Bind は役割を人に与える。ここで初めて効く。
func (a *Authorizer) Bind(role string, subjects ...string) {
	a.bindings = append(a.bindings, Binding{Role: role, Subjects: subjects})
	a.logf(role + " を " + join(subjects) + " に与えた")
}

// Roles は名前順に役割を返す。
func (a *Authorizer) Roles() []*Role {
	names := make([]string, 0, len(a.roles))
	for n := range a.roles {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Role, len(names))
	for i, n := range names {
		out[i] = a.roles[n]
	}
	return out
}

// RolesOf は subject が持つ役割の名前を返す。
func (a *Authorizer) RolesOf(subject string) []string {
	var out []string
	for _, b := range a.bindings {
		if contains(b.Subjects, subject) {
			out = append(out, b.Role)
		}
	}
	sort.Strings(out)
	return out
}

// Can は subject が資源 res に操作 verb をしてよいかを判定する。
//
// 判定は足し算で、持っている役割のどれか1つが許していれば通る。拒否は
// 書けないので、通らないものは「どこにも書いていない」だけになる。
// 何が通るかを知りたければ、書いてあるものを全部見ればよい。
func (a *Authorizer) Can(subject, res string, verb Verb) Decision {
	for _, name := range a.RolesOf(subject) {
		role := a.roles[name]
		if role == nil {
			continue
		}
		for _, rule := range role.Rules {
			if rule.allows(res, verb) {
				return Decision{Allowed: true, Role: name,
					Reason: name + " が " + res + " への " + string(verb) + " を許可している"}
			}
		}
	}
	return Decision{Reason: subject + " に " + res + " への " + string(verb) + " を許す役割が無い"}
}

// Do は判定したうえで数を記録する。
func (a *Authorizer) Do(subject, res string, verb Verb) Decision {
	d := a.Can(subject, res, verb)
	if d.Allowed {
		a.Allowed++
	} else {
		a.Denied++
		a.logf("拒否: " + d.Reason)
	}
	return d
}

// #endregion decide

// #region matrix

// Cell は誰が何をしてよいかの1マス。
type Cell struct {
	Subject  string
	Resource string
	Verb     Verb
	Allowed  bool
	Role     string
}

// Matrix は subjects × resources について、verb の可否を並べて返す。
// 誰に何を与えているかを一望し、与えすぎに気づくために使う。
func (a *Authorizer) Matrix(subjects, resources []string, verb Verb) []Cell {
	var out []Cell
	for _, s := range subjects {
		for _, r := range resources {
			d := a.Can(s, r, verb)
			out = append(out, Cell{Subject: s, Resource: r, Verb: verb, Allowed: d.Allowed, Role: d.Role})
		}
	}
	return out
}

// #endregion matrix

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v || x == "*" {
			return true
		}
	}
	return false
}

func containsVerb(list []Verb, v Verb) bool {
	for _, x := range list {
		if x == v || x == "*" {
			return true
		}
	}
	return false
}

func join(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

func (a *Authorizer) logf(msg string) { a.Log = append(a.Log, msg) }
