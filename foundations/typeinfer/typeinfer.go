// Package typeinfer は Hindley–Milner 型推論(Algorithm W)を最小構成で実装する。
//
// 型注釈を一切書かなくても、コンパイラが式から型を導ける。let id = λx.x のような
// 定義に、人が「id は何型か」を書かなくても、「どんな型 a に対しても a→a」だと
// 分かる。種明かしは 3 つの道具にある。まず未知の型を型変数(t1, t2…)で置く。
// 次に、関数適用などの制約から「この型とこの型は等しい」を集め、単一化(unify)で
// 型変数の中身を解いていく。最後に let 束縛では、残った型変数を「どんな型でもよい」
// と一般化して(let 多相)、使うたびに新しい型変数へ具体化する。これで id を
// 整数にも真偽値にも使える。数式を解くように型が定まる。
package typeinfer

import (
	"errors"
	"fmt"
)

// #region types

// Type は型。型定数(Int/Bool)・型変数・関数型のいずれか。
type Type interface{ String() string }

// TCon は型定数(Int, Bool など)。
type TCon struct{ Name string }

func (t *TCon) String() string { return t.Name }

// TVar は型変数(まだ決まっていない型の穴)。
type TVar struct{ Name string }

func (t *TVar) String() string { return t.Name }

// TFun は関数型 From -> To。
type TFun struct{ From, To Type }

func (t *TFun) String() string { return "(" + t.From.String() + " -> " + t.To.String() + ")" }

var (
	tInt  = &TCon{Name: "Int"}
	tBool = &TCon{Name: "Bool"}
)

// Scheme は型スキーム ∀vars. Type(let 多相のための「量化された型」)。
type Scheme struct {
	Vars []string // どんな型でもよい型変数
	Type Type
}

// Env は変数名から型スキームへの環境。
type Env map[string]*Scheme

// #endregion types

// #region unify

// Subst は型変数名から型への置換。
type Subst map[string]Type

// apply は置換を型に適用する(型変数を中身で置き換える)。
func apply(s Subst, t Type) Type {
	switch t := t.(type) {
	case *TVar:
		if r, ok := s[t.Name]; ok {
			return apply(s, r) // 連鎖して解決
		}
		return t
	case *TFun:
		return &TFun{From: apply(s, t.From), To: apply(s, t.To)}
	default:
		return t // TCon はそのまま
	}
}

// compose は 2 つの置換を合成する(s1 を s2 の後に適用するのと同じ)。
func compose(s1, s2 Subst) Subst {
	out := Subst{}
	for k, v := range s2 {
		out[k] = apply(s1, v)
	}
	for k, v := range s1 {
		out[k] = v
	}
	return out
}

// occurs は型変数 name が型 t の中に現れるか(無限型を防ぐ occurs check)。
func occurs(name string, t Type) bool {
	switch t := t.(type) {
	case *TVar:
		return t.Name == name
	case *TFun:
		return occurs(name, t.From) || occurs(name, t.To)
	default:
		return false
	}
}

// bind は型変数 name を型 t に束縛する置換を作る。
func bind(name string, t Type) (Subst, error) {
	if tv, ok := t.(*TVar); ok && tv.Name == name {
		return Subst{}, nil // 自分自身なら何もしない
	}
	if occurs(name, t) {
		// t の中に name が出る = 無限型(例: t = t -> t)。λx.x x で起きる。
		return nil, fmt.Errorf("occurs check: %s appears in %s", name, t)
	}
	return Subst{name: t}, nil
}

// unify は 2 つの型を等しくする置換を求める(単一化)。合わなければ型エラー。
func unify(t1, t2 Type) (Subst, error) {
	switch a := t1.(type) {
	case *TVar:
		return bind(a.Name, t2)
	case *TCon:
		if b, ok := t2.(*TCon); ok && a.Name == b.Name {
			return Subst{}, nil
		}
		if _, ok := t2.(*TVar); ok {
			return unify(t2, t1)
		}
		return nil, fmt.Errorf("type mismatch: %s vs %s", t1, t2)
	case *TFun:
		b, ok := t2.(*TFun)
		if !ok {
			if _, isVar := t2.(*TVar); isVar {
				return unify(t2, t1)
			}
			return nil, fmt.Errorf("type mismatch: %s vs %s", t1, t2)
		}
		s1, err := unify(a.From, b.From)
		if err != nil {
			return nil, err
		}
		s2, err := unify(apply(s1, a.To), apply(s1, b.To))
		if err != nil {
			return nil, err
		}
		return compose(s2, s1), nil
	}
	return nil, fmt.Errorf("cannot unify %s and %s", t1, t2)
}

// #endregion unify

// #region infer

// Expr は推論対象の式。
type Expr interface{ isExpr() }

// EInt/EBool はリテラル。EVar は変数。ELam はラムダ、EApp は適用、
// ELet は let 束縛、EIf は条件分岐。
type EInt struct{ V int }
type EBool struct{ V bool }
type EVar struct{ Name string }
type ELam struct {
	Param string
	Body  Expr
}
type EApp struct{ Fn, Arg Expr }
type ELet struct {
	Name        string
	Value, Body Expr
}
type EIf struct{ Cond, Then, Else Expr }

func (*EInt) isExpr()  {}
func (*EBool) isExpr() {}
func (*EVar) isExpr()  {}
func (*ELam) isExpr()  {}
func (*EApp) isExpr()  {}
func (*ELet) isExpr()  {}
func (*EIf) isExpr()   {}

// Inferer は型変数の採番を持つ(決定的にするためのカウンタ)。
type Inferer struct{ counter int }

// New は新しい推論器を作る。
func New() *Inferer { return &Inferer{} }

// fresh は新しい型変数を作る(t1, t2, …)。
func (inf *Inferer) fresh() *TVar {
	inf.counter++
	return &TVar{Name: fmt.Sprintf("t%d", inf.counter)}
}

// freeVars は型に含まれる自由な型変数を集める。
func freeVars(t Type, out map[string]bool) {
	switch t := t.(type) {
	case *TVar:
		out[t.Name] = true
	case *TFun:
		freeVars(t.From, out)
		freeVars(t.To, out)
	}
}

// instantiate はスキーム ∀vars.T の量化変数を新しい型変数に置き換える(具体化)。
// 使うたびに新鮮な変数にするので、id を Int にも Bool にも使える。
func (inf *Inferer) instantiate(sc *Scheme) Type {
	sub := Subst{}
	for _, v := range sc.Vars {
		sub[v] = inf.fresh()
	}
	return apply(sub, sc.Type)
}

// generalize は型 t のうち、環境に現れない自由変数を量化する(一般化 = let 多相)。
func generalize(env Env, t Type) *Scheme {
	tvars := map[string]bool{}
	freeVars(t, tvars)
	// 環境で使われている型変数は量化しない。
	envVars := map[string]bool{}
	for _, sc := range env {
		ft := map[string]bool{}
		freeVars(sc.Type, ft)
		for v := range ft {
			if !contains(sc.Vars, v) {
				envVars[v] = true
			}
		}
	}
	var vars []string
	for v := range tvars {
		if !envVars[v] {
			vars = append(vars, v)
		}
	}
	return &Scheme{Vars: vars, Type: t}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

// applyEnv は環境の全スキームに置換を適用する。
func applyEnv(s Subst, env Env) Env {
	out := Env{}
	for k, sc := range env {
		out[k] = &Scheme{Vars: sc.Vars, Type: apply(s, sc.Type)}
	}
	return out
}

var errUnbound = errors.New("unbound variable")

// Infer は式の型を推論する(公開入口)。
func (inf *Inferer) Infer(env Env, e Expr) (Type, error) {
	s, t, err := inf.infer(env, e)
	if err != nil {
		return nil, err
	}
	return apply(s, t), nil
}

// infer は Algorithm W の本体。置換と型を返しながら制約を解いていく。
func (inf *Inferer) infer(env Env, e Expr) (Subst, Type, error) {
	switch e := e.(type) {
	case *EInt:
		return Subst{}, tInt, nil
	case *EBool:
		return Subst{}, tBool, nil

	case *EVar:
		sc, ok := env[e.Name]
		if !ok {
			return nil, nil, fmt.Errorf("%w: %s", errUnbound, e.Name)
		}
		return Subst{}, inf.instantiate(sc), nil // 使うたび新鮮な型に

	case *ELam:
		tv := inf.fresh()
		env2 := Env{}
		for k, v := range env {
			env2[k] = v
		}
		env2[e.Param] = &Scheme{Type: tv} // 引数は単相の型変数
		s1, tbody, err := inf.infer(env2, e.Body)
		if err != nil {
			return nil, nil, err
		}
		return s1, &TFun{From: apply(s1, tv), To: tbody}, nil

	case *EApp:
		s1, tfn, err := inf.infer(env, e.Fn)
		if err != nil {
			return nil, nil, err
		}
		s2, targ, err := inf.infer(applyEnv(s1, env), e.Arg)
		if err != nil {
			return nil, nil, err
		}
		tv := inf.fresh()
		// tfn は「targ を受け tv を返す関数」でなければならない、という制約。
		s3, err := unify(apply(s2, tfn), &TFun{From: targ, To: tv})
		if err != nil {
			return nil, nil, err
		}
		return compose(s3, compose(s2, s1)), apply(s3, tv), nil

	case *ELet:
		// value の型を推論し、一般化してから body を推論する(let 多相)。
		s1, tval, err := inf.infer(env, e.Value)
		if err != nil {
			return nil, nil, err
		}
		env1 := applyEnv(s1, env)
		scheme := generalize(env1, tval)
		env2 := Env{}
		for k, v := range env1 {
			env2[k] = v
		}
		env2[e.Name] = scheme
		s2, tbody, err := inf.infer(env2, e.Body)
		if err != nil {
			return nil, nil, err
		}
		return compose(s2, s1), tbody, nil

	case *EIf:
		sc, tc, err := inf.infer(env, e.Cond)
		if err != nil {
			return nil, nil, err
		}
		s1, err := unify(tc, tBool) // 条件は Bool
		if err != nil {
			return nil, nil, err
		}
		sc = compose(s1, sc)
		st, tt, err := inf.infer(applyEnv(sc, env), e.Then)
		if err != nil {
			return nil, nil, err
		}
		sc = compose(st, sc)
		se, te, err := inf.infer(applyEnv(sc, env), e.Else)
		if err != nil {
			return nil, nil, err
		}
		sc = compose(se, sc)
		s2, err := unify(apply(sc, tt), apply(sc, te)) // then と else は同じ型
		if err != nil {
			return nil, nil, err
		}
		return compose(s2, sc), apply(s2, apply(sc, tt)), nil
	}
	return nil, nil, fmt.Errorf("unknown expression")
}

// #endregion infer
