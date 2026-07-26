package lang

import (
	"strconv"
	"strings"
)

// 実行時の値(Object)と、変数を保持する環境(Environment)。
// AST は「プログラムの形」、Object は「実行して得られる値」。この2つを分けるのが肝。

// #region object
type ObjectType string

const (
	INTEGER_OBJ  = "INTEGER"
	BOOLEAN_OBJ  = "BOOLEAN"
	NULL_OBJ     = "NULL"
	RETURN_OBJ   = "RETURN_VALUE"
	FUNCTION_OBJ = "FUNCTION"
	ERROR_OBJ    = "ERROR"
)

type Object interface {
	Type() ObjectType
	Inspect() string // 表示用
}

type Integer struct{ Value int64 }

func (*Integer) Type() ObjectType  { return INTEGER_OBJ }
func (i *Integer) Inspect() string { return strconv.FormatInt(i.Value, 10) }

// 実行時の真偽値。AST の Boolean(構文上の true/false)とは別物なので BooleanObj にする。
type BooleanObj struct{ Value bool }
// #endregion object

func (*BooleanObj) Type() ObjectType  { return BOOLEAN_OBJ }
func (b *BooleanObj) Inspect() string {
	if b.Value {
		return "true"
	}
	return "false"
}

type Null struct{}

func (*Null) Type() ObjectType  { return NULL_OBJ }
func (*Null) Inspect() string   { return "null" }

// return は「評価を打ち切って値を持ち上げる」ための包み。
type ReturnValue struct{ Value Object }

func (*ReturnValue) Type() ObjectType  { return RETURN_OBJ }
func (rv *ReturnValue) Inspect() string { return rv.Value.Inspect() }

// Error は実行時エラー。以降の評価を止めて持ち上がる。
type Error struct{ Message string }

func (*Error) Type() ObjectType  { return ERROR_OBJ }
func (e *Error) Inspect() string { return "ERROR: " + e.Message }

// #region function
// Function は関数値。パラメータと本体に加えて、定義時の環境(Env)を抱える。
// この Env の抱え込みがクロージャの正体——「どこで定義されたか」を値が覚えている。
type Function struct {
	Parameters []*Identifier
	Body       *BlockStatement
	Env        *Environment
}

func (*Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	params := make([]string, len(f.Parameters))
	for i, p := range f.Parameters {
		params[i] = p.String()
	}
	return "fn(" + strings.Join(params, ", ") + ") { ... }"
}
// #endregion function

// #region env
// Environment は変数名 → 値の対応表。outer を辿れる入れ子構造にすることで、
// 内側のスコープから外側の変数を見られる(レキシカルスコープ)。
type Environment struct {
	store map[string]Object
	outer *Environment
}

func NewEnvironment() *Environment {
	return &Environment{store: map[string]Object{}}
}

// NewEnclosedEnvironment は関数呼び出しごとに作る「内側の環境」。
// outer に定義時の環境を置くので、外側の変数が見える=クロージャになる。
func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

// Get は自分の store を見て、無ければ outer を再帰的に辿る。
func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil {
		return e.outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}
// #endregion env
