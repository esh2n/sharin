package lang

import "fmt"

// ツリーウォーク評価器: AST を根から再帰的にたどり、各ノードを実行時の値(Object)へ畳み込む。
// 「構文木を歩きながら値にしていく」のでツリーウォーク。最も素直な実行方式。

// 真偽値と null はシングルトンにして使い回す(生成コストと比較の簡単さ)。
var (
	TRUE_OBJ  = &BooleanObj{Value: true}
	FALSE_OBJ = &BooleanObj{Value: false}
	NULL_OBJ_ = &Null{}
)

// #region eval
// Eval はノードの種類で分岐し、子を再帰評価して値を組み上げる。
func Eval(node Node, env *Environment) Object {
	switch node := node.(type) {

	// --- 文 ---
	case *Program:
		return evalProgram(node, env)
	case *ExpressionStatement:
		return Eval(node.Expression, env)
	case *BlockStatement:
		return evalBlock(node, env)
	case *LetStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		env.Set(node.Name.Value, val) // 変数を環境に束縛
		return val
	case *ReturnStatement:
		val := Eval(node.ReturnValue, env)
		if isError(val) {
			return val
		}
		return &ReturnValue{Value: val} // 包んで持ち上げる

	// --- 式 ---
	case *IntegerLiteral:
		return &Integer{Value: node.Value}
	case *Boolean:
		return boolToObj(node.Value)
	case *Identifier:
		return evalIdentifier(node, env)
	case *PrefixExpression:
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefix(node.Operator, right)
	case *InfixExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalInfix(node.Operator, left, right)
	case *IfExpression:
		return evalIf(node, env)
	case *FunctionLiteral:
		// 定義時の環境 env を抱えるのがクロージャの肝
		return &Function{Parameters: node.Parameters, Body: node.Body, Env: env}
	case *CallExpression:
		return evalCall(node, env)
	}
	return nil
}
// #endregion eval

// evalProgram は文を順に評価し、return かエラーが出たら即座に打ち切る。
func evalProgram(program *Program, env *Environment) Object {
	var result Object
	for _, stmt := range program.Statements {
		result = Eval(stmt, env)
		switch result := result.(type) {
		case *ReturnValue:
			return result.Value // トップでは包みを剥がして返す
		case *Error:
			return result
		}
	}
	return result
}

// evalBlock はブロック内を評価。return は「包んだまま」持ち上げるのがポイント。
// そうしないと入れ子の if の中の return が外側の関数まで抜けられない。
func evalBlock(block *BlockStatement, env *Environment) Object {
	var result Object
	for _, stmt := range block.Statements {
		result = Eval(stmt, env)
		if result != nil {
			rt := result.Type()
			if rt == RETURN_OBJ || rt == ERROR_OBJ {
				return result // 剥がさずに持ち上げる
			}
		}
	}
	return result
}

func evalPrefix(op string, right Object) Object {
	switch op {
	case "!":
		return boolToObj(!isTruthy(right))
	case "-":
		if right.Type() != INTEGER_OBJ {
			return newError("未対応の演算: -%s", right.Type())
		}
		return &Integer{Value: -right.(*Integer).Value}
	default:
		return newError("未知の前置演算子: %s", op)
	}
}

// #region infix
func evalInfix(op string, left, right Object) Object {
	switch {
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerInfix(op, left.(*Integer), right.(*Integer))
	// == と != は真偽値・null にも使えるので、ポインタ同一性で比較(シングルトンだから成立)
	case op == "==":
		return boolToObj(left == right)
	case op == "!=":
		return boolToObj(left != right)
	case left.Type() != right.Type():
		return newError("型が違う: %s %s %s", left.Type(), op, right.Type())
	default:
		return newError("未対応の演算: %s %s %s", left.Type(), op, right.Type())
	}
}

func evalIntegerInfix(op string, left, right *Integer) Object {
	l, r := left.Value, right.Value
	switch op {
	case "+":
		return &Integer{Value: l + r}
	case "-":
		return &Integer{Value: l - r}
	case "*":
		return &Integer{Value: l * r}
	case "/":
		if r == 0 {
			return newError("ゼロ除算")
		}
		return &Integer{Value: l / r}
	case "<":
		return boolToObj(l < r)
	case ">":
		return boolToObj(l > r)
	case "==":
		return boolToObj(l == r)
	case "!=":
		return boolToObj(l != r)
	default:
		return newError("未知の演算子: %s", op)
	}
}
// #endregion infix

func evalIf(node *IfExpression, env *Environment) Object {
	cond := Eval(node.Condition, env)
	if isError(cond) {
		return cond
	}
	if isTruthy(cond) {
		return Eval(node.Consequence, env)
	} else if node.Alternative != nil {
		return Eval(node.Alternative, env)
	}
	return NULL_OBJ_ // else が無く条件も偽なら null
}

func evalIdentifier(node *Identifier, env *Environment) Object {
	if val, ok := env.Get(node.Value); ok {
		return val
	}
	return newError("未定義の変数: %s", node.Value)
}

// #region call
func evalCall(node *CallExpression, env *Environment) Object {
	fn := Eval(node.Function, env)
	if isError(fn) {
		return fn
	}
	args := evalExpressions(node.Arguments, env)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	function, ok := fn.(*Function)
	if !ok {
		return newError("関数ではない: %s", fn.Type())
	}
	if len(args) != len(function.Parameters) {
		return newError("引数の数が違う: 期待 %d, 実際 %d", len(function.Parameters), len(args))
	}

	// 呼び出しごとに、関数が抱える環境(定義時)を outer にした内側環境を作る。
	// ここに引数を束縛する。これでクロージャが外側の変数を見つつ、引数はローカルになる。
	inner := NewEnclosedEnvironment(function.Env)
	for i, param := range function.Parameters {
		inner.Set(param.Value, args[i])
	}
	result := Eval(function.Body, inner)
	// 関数の外へは、return の包みを剥がして返す
	if rv, ok := result.(*ReturnValue); ok {
		return rv.Value
	}
	return result
}
// #endregion call

func evalExpressions(exps []Expression, env *Environment) []Object {
	var result []Object
	for _, e := range exps {
		v := Eval(e, env)
		if isError(v) {
			return []Object{v}
		}
		result = append(result, v)
	}
	return result
}

// --- 小道具 ---

func boolToObj(b bool) *BooleanObj {
	if b {
		return TRUE_OBJ
	}
	return FALSE_OBJ
}

// isTruthy: false と null だけが偽。それ以外(0 含む)は真。
func isTruthy(obj Object) bool {
	switch obj {
	case NULL_OBJ_, FALSE_OBJ:
		return false
	default:
		return true
	}
}

func isError(obj Object) bool {
	return obj != nil && obj.Type() == ERROR_OBJ
}

func newError(format string, a ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, a...)}
}

// Run はソース文字列を字句解析→構文解析→評価する入口。エラーなら Error オブジェクト。
func Run(input string) Object {
	p := NewParser(NewLexer(input))
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return &Error{Message: "構文エラー: " + errs[0]}
	}
	return Eval(program, NewEnvironment())
}
