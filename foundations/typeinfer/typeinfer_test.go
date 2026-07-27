package typeinfer

import (
	"errors"
	"testing"
)

// 便利な AST 構築ヘルパー。
func lam(p string, b Expr) Expr    { return &ELam{Param: p, Body: b} }
func app(f, a Expr) Expr           { return &EApp{Fn: f, Arg: a} }
func vr(n string) Expr             { return &EVar{Name: n} }
func let(n string, v, b Expr) Expr { return &ELet{Name: n, Value: v, Body: b} }
func iff(c, t, e Expr) Expr        { return &EIf{Cond: c, Then: t, Else: e} }
func int_(n int) Expr              { return &EInt{V: n} }
func bool_(b bool) Expr            { return &EBool{V: b} }

func infer(t *testing.T, e Expr) Type {
	t.Helper()
	ty, err := New().Infer(Env{}, e)
	if err != nil {
		t.Fatalf("inference failed: %v", err)
	}
	return ty
}

func TestLiterals(t *testing.T) {
	if got := infer(t, int_(42)).String(); got != "Int" {
		t.Fatalf("int literal: got %s want Int", got)
	}
	if got := infer(t, bool_(true)).String(); got != "Bool" {
		t.Fatalf("bool literal: got %s want Bool", got)
	}
}

// TestIdentityIsPolymorphic は、注釈なしの λx.x が「a -> a」と推論されることを確かめる。
func TestIdentityIsPolymorphic(t *testing.T) {
	// λx.x → (t1 -> t1) のような、入力と出力が同じ型変数の関数。
	ty := infer(t, lam("x", vr("x")))
	fn, ok := ty.(*TFun)
	if !ok {
		t.Fatalf("identity should be a function type, got %s", ty)
	}
	from, ok1 := fn.From.(*TVar)
	to, ok2 := fn.To.(*TVar)
	if !ok1 || !ok2 || from.Name != to.Name {
		t.Fatalf("identity should be (a -> a) with same var, got %s", ty)
	}
}

// TestApplicationResolvesType は、適用で型変数が具体型に解けることを確かめる。
func TestApplicationResolvesType(t *testing.T) {
	// (λx.x) 42 → Int
	if got := infer(t, app(lam("x", vr("x")), int_(42))).String(); got != "Int" {
		t.Fatalf("(λx.x) 42: got %s want Int", got)
	}
	// (λx. if x then 1 else 2) true → Int(x は Bool と推論される)
	e := app(lam("x", iff(vr("x"), int_(1), int_(2))), bool_(true))
	if got := infer(t, e).String(); got != "Int" {
		t.Fatalf("got %s want Int", got)
	}
}

// TestLetPolymorphism はこの章の主眼。let で束縛した id を、Int と Bool の
// 両方に使えることを固定する(let 多相)。単相ならここで型エラーになる。
func TestLetPolymorphism(t *testing.T) {
	// let id = λx.x in if (id true) then (id 1) else 2  → Int
	e := let("id", lam("x", vr("x")),
		iff(app(vr("id"), bool_(true)), // id を Bool -> Bool として使う
			app(vr("id"), int_(1)), //       id を Int -> Int として使う
			int_(2)))
	if got := infer(t, e).String(); got != "Int" {
		t.Fatalf("let-polymorphism failed: got %s want Int", got)
	}
}

func TestIfBranchesMustMatch(t *testing.T) {
	// if true then 1 else false → 型エラー(枝の型が違う)
	e := iff(bool_(true), int_(1), bool_(false))
	if _, err := New().Infer(Env{}, e); err == nil {
		t.Fatal("mismatched if branches should fail")
	}
}

func TestConditionMustBeBool(t *testing.T) {
	// if 1 then 2 else 3 → 型エラー(条件が Int)
	e := iff(int_(1), int_(2), int_(3))
	if _, err := New().Infer(Env{}, e); err == nil {
		t.Fatal("non-bool condition should fail")
	}
}

// TestOccursCheck は、自己適用 λx.x x が occurs check で弾かれることを確かめる。
func TestOccursCheck(t *testing.T) {
	// λx. x x は「x が x を受ける関数」= 無限型を要求する → occurs check で失敗。
	e := lam("x", app(vr("x"), vr("x")))
	if _, err := New().Infer(Env{}, e); err == nil {
		t.Fatal("self-application should fail occurs check")
	}
}

func TestApplyingNonFunctionFails(t *testing.T) {
	// 42 1 → Int を関数として適用しようとする → 型エラー
	e := app(int_(42), int_(1))
	if _, err := New().Infer(Env{}, e); err == nil {
		t.Fatal("applying a non-function should fail")
	}
}

func TestUnboundVariable(t *testing.T) {
	_, err := New().Infer(Env{}, vr("nope"))
	if !errors.Is(err, errUnbound) {
		t.Fatalf("expected unbound variable error, got %v", err)
	}
}

// TestConstFunction は const = λx.λy.x が (a -> b -> a) になることを確かめる。
func TestConstFunction(t *testing.T) {
	// λx.λy.x に Int, Bool を渡すと Int が返る。
	e := app(app(lam("x", lam("y", vr("x"))), int_(5)), bool_(true))
	if got := infer(t, e).String(); got != "Int" {
		t.Fatalf("const: got %s want Int", got)
	}
}
