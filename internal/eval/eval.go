package eval

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"io"
	"net/http"
	"golang.org/x/net/websocket"
	"os"
	"os/exec"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
	"github.com/navescript/nvs/internal/sqlite"
)

var (
	CLIArgs []string
	metricCounters = map[string]int64{}
	metricGauges   = map[string]float64{}
	TRUE  = &object.Boolean{Value: true}
	FALSE = &object.Boolean{Value: false}
	NULL  = &object.Null{}
)

func Eval(node ast.Node, env *object.Environment) object.Object {
	switch node := node.(type) {

	// Statements
	case *ast.Program:
		return evalProgram(node, env)
	case *ast.BlockStatement:
		return evalBlockStatement(node, env)
	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)
	case *ast.ReturnStatement:
		val := Eval(node.ReturnValue, env)
		if isError(val) {
			return val
		}
		return &object.ReturnValue{Value: val}
	case *ast.LetStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		env.Set(node.Name.Value, val)
		return NULL
	case *ast.PrintStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		fmt.Println(val.Inspect())
		return NULL
	case *ast.WhileStatement:
		return evalWhile(node, env)

	// Expressions
	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}
	case *ast.StringLiteral:
		return &object.String{Value: node.Value}
	case *ast.Boolean:
		return nativeBoolToBooleanObject(node.Value)
	case *ast.NullLiteral:
		return NULL
	case *ast.PrefixExpression:
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)
	case *ast.InfixExpression:
		// Short-circuit operators
		if node.Operator == "&&" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if !isTruthy(left) {
				return FALSE
			}
			right := Eval(node.Right, env)
			if isError(right) {
				return right
			}
			return nativeBoolToBooleanObject(isTruthy(right))
		}
		if node.Operator == "||" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if isTruthy(left) {
				return TRUE
			}
			right := Eval(node.Right, env)
			if isError(right) {
				return right
			}
			return nativeBoolToBooleanObject(isTruthy(right))
		}
		if node.Operator == "??" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if left.Type() != object.NULL_OBJ {
				return left
			}
			return Eval(node.Right, env)
		}
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalInfixExpression(node.Operator, left, right)
	case *ast.IfExpression:
		return evalIfExpression(node, env)
	case *ast.Identifier:
		return evalIdentifier(node, env)
	case *ast.FunctionLiteral:
		params := node.Parameters
		body := node.Body
		return &object.Function{Parameters: params, Body: body, Env: env}
	case *ast.CallExpression:
		// Method call: obj.method(args)
		if mem, ok := node.Function.(*ast.MemberExpression); ok {
			obj := Eval(mem.Object, env)
			if isError(obj) {
				return obj
			}
			args := evalExpressions(node.Arguments, env)
			if len(args) == 1 && isError(args[0]) {
				return args[0]
			}
			return evalMethodCall(obj, mem.Property.Value, args, env)
		}
		function := Eval(node.Function, env)
		if isError(function) {
			return function
		}
		args := evalExpressions(node.Arguments, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}
		return applyFunction(function, args)
	case *ast.ArrayLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}
	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(left, index)
	case *ast.AssignExpression:
		return evalAssignExpression(node, env)
	case *ast.ForStatement:
		return evalForStatement(node, env)
	case *ast.ForInStatement:
		return evalForInStatement(node, env)
	case *ast.BreakStatement:
		return &object.Break{}
	case *ast.ContinueStatement:
		return &object.Continue{}
	case *ast.HashLiteral:
		return evalHashLiteral(node, env)
	case *ast.IndexAssignExpression:
		return evalIndexAssignExpression(node, env)
	case *ast.ImportStatement:
		return evalImportStatement(node, env)
	case *ast.ClassStatement:
		return evalClassStatement(node, env)
	case *ast.NewExpression:
		return evalNewExpression(node, env)
	case *ast.ThisExpression:
		return evalThisExpression(env)
	case *ast.MemberExpression:
		return evalMemberExpression(node, env)
	case *ast.MemberAssignExpression:
		return evalMemberAssignExpression(node, env)
	case *ast.TryStatement:
		return evalTryStatement(node, env)
	case *ast.ThrowStatement:
		return evalThrowStatement(node, env)
	case *ast.ConstStatement:
		return evalConstStatement(node, env)
	case *ast.MatchExpression:
		return evalMatchExpression(node, env)
	case *ast.YieldStatement:
		return evalYieldStatement(node, env)
	case *ast.DecoratorStatement:
		return evalDecoratorStatement(node, env)
	case *ast.TypedLetStatement:
		return evalTypedLetStatement(node, env)
	case *ast.TernaryExpression:
		return evalTernaryExpression(node, env)
	}

	return NULL
}

func evalProgram(program *ast.Program, env *object.Environment) object.Object {
	var result object.Object
	for _, statement := range program.Statements {
		result = Eval(statement, env)
		switch result := result.(type) {
		case *object.ReturnValue:
			return result.Value
		case *object.Error:
			return result
		}
	}
	return result
}

func evalBlockStatement(block *ast.BlockStatement, env *object.Environment) object.Object {
	var result object.Object
	for _, statement := range block.Statements {
		result = Eval(statement, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ ||
				rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ {
				return result
			}
		}
	}
	return result
}

func evalWhile(ws *ast.WhileStatement, env *object.Environment) object.Object {
	var result object.Object = NULL
	for {
		cond := Eval(ws.Condition, env)
		if isError(cond) {
			return cond
		}
		if !isTruthy(cond) {
			break
		}
		result = Eval(ws.Body, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
			if rt == object.BREAK_OBJ {
				return NULL
			}
			if rt == object.CONTINUE_OBJ {
				continue
			}
		}
	}
	return result
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalPrefixExpression(operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	default:
		return newError("unknown operator: %s%s", operator, right.Type())
	}
}

func evalBangOperatorExpression(right object.Object) object.Object {
	switch right {
	case TRUE:
		return FALSE
	case FALSE:
		return TRUE
	case NULL:
		return TRUE
	default:
		return FALSE
	}
}

func evalMinusPrefixOperatorExpression(right object.Object) object.Object {
	if right.Type() == object.INTEGER_OBJ {
		value := right.(*object.Integer).Value
		return &object.Integer{Value: -value}
	}
	if right.Type() == object.FLOAT_OBJ {
		value := right.(*object.Float).Value
		return &object.Float{Value: -value}
	}
	return newError("unknown operator: -%s", right.Type())
}

func evalInfixExpression(operator string, left, right object.Object) object.Object {
	if operator == "??" {
		if left.Type() == object.NULL_OBJ {
			return right
		}
		return left
	}
	if operator == "in" {
		return evalInOperator(left, right)
	}
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ || right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case left.Type() == object.BOOLEAN_OBJ && right.Type() == object.BOOLEAN_OBJ:
		return evalBooleanInfixExpression(operator, left, right)
	case operator == "==":
		return nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return nativeBoolToBooleanObject(left != right)
	case left.Type() != right.Type():
		return newError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalBooleanInfixExpression(operator string, left, right object.Object) object.Object {
	l := left.(*object.Boolean).Value
	r := right.(*object.Boolean).Value
	switch operator {
	case "&&":
		return nativeBoolToBooleanObject(l && r)
	case "||":
		return nativeBoolToBooleanObject(l || r)
	case "==":
		return nativeBoolToBooleanObject(l == r)
	case "!=":
		return nativeBoolToBooleanObject(l != r)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value

	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &object.Integer{Value: leftVal / rightVal}
	case "%":
		return &object.Integer{Value: leftVal % rightVal}
	case "&":
		return &object.Integer{Value: leftVal & rightVal}
	case "|":
		return &object.Integer{Value: leftVal | rightVal}
	case "^":
		return &object.Integer{Value: leftVal ^ rightVal}
	case "<<":
		return &object.Integer{Value: leftVal << rightVal}
	case ">>":
		return &object.Integer{Value: leftVal >> rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func toFloat(obj object.Object) (float64, bool) {
	switch o := obj.(type) {
	case *object.Integer:
		return float64(o.Value), true
	case *object.Float:
		return o.Value, true
	default:
		return 0, false
	}
}

func evalFloatInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal, ok1 := toFloat(left)
	rightVal, ok2 := toFloat(right)
	if !ok1 || !ok2 {
		return newError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	}

	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalStringInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value
	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIfExpression(ie *ast.IfExpression, env *object.Environment) object.Object {
	condition := Eval(ie.Condition, env)
	if isError(condition) {
		return condition
	}
	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	} else if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	}
	return NULL
}

func evalIdentifier(node *ast.Identifier, env *object.Environment) object.Object {
	if val, ok := env.Get(node.Value); ok {
		return val
	}
	ensureBuiltins()
	if builtin, ok := builtins[node.Value]; ok {
		return builtin
	}
	return newError("identifier not found: " + node.Value)
}

func isTruthy(obj object.Object) bool {
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		return true
	}
}

func newError(format string, a ...interface{}) *object.Error {
	return &object.Error{Message: fmt.Sprintf(format, a...)}
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func evalExpressions(exps []ast.Expression, env *object.Environment) []object.Object {
	var result []object.Object
	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}
	return result
}

func applyFunction(fn object.Object, args []object.Object) object.Object {
	switch fn := fn.(type) {
	case *object.Function:
		extendedEnv := extendFunctionEnv(fn, args)
		// Collect yields for generator functions
		values, isGen, result := evalCollectingYields(fn.Body, extendedEnv)
		if isGen {
			return &object.Generator{Values: values}
		}
		return unwrapReturnValue(result)
	case *object.Builtin:
		return fn.Fn(args...)
	default:
		return newError("not a function: %s", fn.Type())
	}
}

func evalCollectingYields(body *ast.BlockStatement, env *object.Environment) ([]object.Object, bool, object.Object) {
	var yields []object.Object
	var last object.Object = NULL
	hasYield := false
	for _, stmt := range body.Statements {
		last = Eval(stmt, env)
		if last == nil {
			continue
		}
		if yv, ok := last.(*object.YieldValue); ok {
			hasYield = true
			yields = append(yields, yv.Value)
			continue
		}
		rt := last.Type()
		if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
			return yields, hasYield, last
		}
		if rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ {
			return yields, hasYield, last
		}
	}
	return yields, hasYield, last
}


func extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	for paramIdx, param := range fn.Parameters {
		if paramIdx < len(args) {
			env.Set(param.Value, args[paramIdx])
		}
	}
	return env
}

func unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}


func evalIndexExpression(left, index object.Object) object.Object {
	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalArrayIndexExpression(left, index)
	case left.Type() == object.STRING_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalStringIndexExpression(left, index)
	case left.Type() == object.HASH_OBJ:
		return evalHashIndexExpression(left, index)
	default:
		return newError("index operator not supported: %s[%s]", left.Type(), index.Type())
	}
}

func evalHashIndexExpression(hash, index object.Object) object.Object {
	hashObject := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return newError("unusable as hash key: %s", index.Type())
	}
	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		return NULL
	}
	return pair.Value
}

func evalArrayIndexExpression(array, index object.Object) object.Object {
	arrayObject := array.(*object.Array)
	idx := index.(*object.Integer).Value
	max := int64(len(arrayObject.Elements) - 1)
	if idx < 0 || idx > max {
		return NULL
	}
	return arrayObject.Elements[idx]
}

func evalStringIndexExpression(str, index object.Object) object.Object {
	stringObject := str.(*object.String)
	idx := index.(*object.Integer).Value
	runes := []rune(stringObject.Value)
	max := int64(len(runes) - 1)
	if idx < 0 || idx > max {
		return NULL
	}
	return &object.String{Value: string(runes[idx])}
}

func evalAssignExpression(node *ast.AssignExpression, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	if ok, errMsg := env.AssignStrict(node.Name.Value, val); ok {
		return val
	} else if errMsg != "" {
		return newError(errMsg)
	}
	// Not found: create in current env (like implicit let)
	env.Set(node.Name.Value, val)
	return val
}

func evalForStatement(fs *ast.ForStatement, env *object.Environment) object.Object {
	if fs.Init != nil {
		res := Eval(fs.Init, env)
		if isError(res) {
			return res
		}
	}

	var result object.Object = NULL
	for {
		if fs.Condition != nil {
			cond := Eval(fs.Condition, env)
			if isError(cond) {
				return cond
			}
			if !isTruthy(cond) {
				break
			}
		}

		result = Eval(fs.Body, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
			if rt == object.BREAK_OBJ {
				return NULL
			}
			if rt == object.CONTINUE_OBJ {
				// still run post
				if fs.Post != nil {
					post := Eval(fs.Post, env)
					if isError(post) {
						return post
					}
				}
				continue
			}
		}

		if fs.Post != nil {
			post := Eval(fs.Post, env)
			if isError(post) {
				return post
			}
		}

		if fs.Condition == nil {
			break
		}
	}
	return result
}


func evalHashLiteral(node *ast.HashLiteral, env *object.Environment) object.Object {
	pairs := make(map[object.HashKey]object.HashPair)
	for keyNode, valueNode := range node.Pairs {
		key := Eval(keyNode, env)
		if isError(key) {
			return key
		}
		hashKey, ok := key.(object.Hashable)
		if !ok {
			return newError("unusable as hash key: %s", key.Type())
		}
		value := Eval(valueNode, env)
		if isError(value) {
			return value
		}
		hashed := hashKey.HashKey()
		pairs[hashed] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func evalIndexAssignExpression(node *ast.IndexAssignExpression, env *object.Environment) object.Object {
	left := Eval(node.Left.Left, env)
	if isError(left) {
		return left
	}
	index := Eval(node.Left.Index, env)
	if isError(index) {
		return index
	}
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}

	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		arr := left.(*object.Array)
		idx := index.(*object.Integer).Value
		if idx < 0 || int(idx) >= len(arr.Elements) {
			return newError("index out of bounds: %d", idx)
		}
		arr.Elements[idx] = val
		return val
	case left.Type() == object.HASH_OBJ:
		hash := left.(*object.Hash)
		key, ok := index.(object.Hashable)
		if !ok {
			return newError("unusable as hash key: %s", index.Type())
		}
		hash.Pairs[key.HashKey()] = object.HashPair{Key: index, Value: val}
		return val
	default:
		return newError("index assignment not supported: %s[%s]", left.Type(), index.Type())
	}
}

func evalForInStatement(fs *ast.ForInStatement, env *object.Environment) object.Object {
	iterable := Eval(fs.Iterable, env)
	if isError(iterable) {
		return iterable
	}

	var result object.Object = NULL

	switch it := iterable.(type) {
	case *object.Array:
		for _, el := range it.Elements {
			env.Set(fs.Name.Value, el)
			result = Eval(fs.Body, env)
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return NULL
				}
				if rt == object.CONTINUE_OBJ {
					continue
				}
			}
		}
	case *object.String:
		for _, ch := range it.Value {
			env.Set(fs.Name.Value, &object.String{Value: string(ch)})
			result = Eval(fs.Body, env)
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return NULL
				}
				if rt == object.CONTINUE_OBJ {
					continue
				}
			}
		}
	case *object.Hash:
		for _, pair := range it.Pairs {
			env.Set(fs.Name.Value, pair.Key)
			result = Eval(fs.Body, env)
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return NULL
				}
				if rt == object.CONTINUE_OBJ {
					continue
				}
			}
		}
	default:
		return newError("for-in not supported on %s", iterable.Type())
	}
	return result
}

// Module loading — tracks loaded files to avoid cycles
var loadedModules = map[string]bool{}

func evalImportStatement(node *ast.ImportStatement, env *object.Environment) object.Object {
	path := node.Path.Value
	abs, err := filepath.Abs(path)
	if err != nil {
		return newError("import: %s", err.Error())
	}
	if loadedModules[abs] {
		return NULL // already loaded
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return newError("import failed: %s", err.Error())
	}
	loadedModules[abs] = true

	l := lexer.New(string(data))
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return newError("import parse errors in %s: %v", path, p.Errors())
	}
	result := Eval(program, env)
	if isError(result) {
		return result
	}
	return NULL
}


func evalClassStatement(node *ast.ClassStatement, env *object.Environment) object.Object {
	methods := map[string]*object.Function{}
	for _, m := range node.Methods {
		methods[m.Name.Value] = &object.Function{
			Parameters: m.Parameters,
			Body:       m.Body,
			Env:        env,
		}
	}
	var parent *object.Class
	if node.Parent != nil {
		parentObj, ok := env.Get(node.Parent.Value)
		if !ok {
			return newError("parent class not found: %s", node.Parent.Value)
		}
		parent, ok = parentObj.(*object.Class)
		if !ok {
			return newError("%s is not a class", node.Parent.Value)
		}
	}
	class := &object.Class{
		Name:    node.Name.Value,
		Parent:  parent,
		Methods: methods,
	}
	env.Set(node.Name.Value, class)
	return class
}

func evalNewExpression(node *ast.NewExpression, env *object.Environment) object.Object {
	classObj, ok := env.Get(node.ClassName.Value)
	if !ok {
		return newError("class not found: %s", node.ClassName.Value)
	}
	class, ok := classObj.(*object.Class)
	if !ok {
		return newError("%s is not a class", node.ClassName.Value)
	}
	instance := &object.Instance{
		Class:  class,
		Fields: map[string]object.Object{},
	}
	args := evalExpressions(node.Arguments, env)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}
	// Call constructor if present
	if init, ok := class.GetMethod("init"); ok {
		result := applyMethod(init, instance, args)
		if isError(result) {
			return result
		}
	}
	return instance
}

func evalThisExpression(env *object.Environment) object.Object {
	if this, ok := env.Get("this"); ok {
		return this
	}
	return newError("this used outside of method")
}

func evalMemberExpression(node *ast.MemberExpression, env *object.Environment) object.Object {
	obj := Eval(node.Object, env)
	if isError(obj) {
		return obj
	}
	if inst, ok := obj.(*object.Instance); ok {
		if val, ok := inst.Get(node.Property.Value); ok {
			return val
		}
		return newError("property not found: %s", node.Property.Value)
	}
	if hash, ok := obj.(*object.Hash); ok {
		// allow obj.key as sugar for obj["key"]
		key := &object.String{Value: node.Property.Value}
		return evalHashIndexExpression(hash, key)
	}
	return newError("member access on non-instance: %s", obj.Type())
}

func evalMemberAssignExpression(node *ast.MemberAssignExpression, env *object.Environment) object.Object {
	obj := Eval(node.Object, env)
	if isError(obj) {
		return obj
	}
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	if inst, ok := obj.(*object.Instance); ok {
		inst.Set(node.Property.Value, val)
		return val
	}
	if hash, ok := obj.(*object.Hash); ok {
		key := &object.String{Value: node.Property.Value}
		hash.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: val}
		return val
	}
	return newError("member assign on non-instance: %s", obj.Type())
}

func evalMethodCall(obj object.Object, name string, args []object.Object, env *object.Environment) object.Object {
	inst, ok := obj.(*object.Instance)
	if !ok {
		// try builtin-style on other types later
		return newError("method call on non-instance: %s", obj.Type())
	}
	method, ok := inst.Class.GetMethod(name)
	if !ok {
		return newError("method not found: %s.%s", inst.Class.Name, name)
	}
	return applyMethod(method, inst, args)
}

func applyMethod(fn *object.Function, inst *object.Instance, args []object.Object) object.Object {
	extended := object.NewEnclosedEnvironment(fn.Env)
	extended.Set("this", inst)
	for i, param := range fn.Parameters {
		if i < len(args) {
			extended.Set(param.Value, args[i])
		}
	}
	result := Eval(fn.Body, extended)
	return unwrapReturnValue(result)
}


func jsonToObject(data []byte) object.Object {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return newError("json_parse: %s", err.Error())
	}
	return interfaceToObject(v)
}

func interfaceToObject(v interface{}) object.Object {
	switch val := v.(type) {
	case nil:
		return NULL
	case bool:
		return nativeBoolToBooleanObject(val)
	case float64:
		if val == float64(int64(val)) {
			return &object.Integer{Value: int64(val)}
		}
		return &object.Float{Value: val}
	case string:
		return &object.String{Value: val}
	case []interface{}:
		elements := make([]object.Object, len(val))
		for i, e := range val {
			elements[i] = interfaceToObject(e)
		}
		return &object.Array{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair)
		for k, v := range val {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: interfaceToObject(v)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return &object.String{Value: fmt.Sprintf("%v", val)}
	}
}

func objectToJSON(obj object.Object) ([]byte, error) {
	return json.Marshal(objectToInterface(obj))
}

func objectToInterface(obj object.Object) interface{} {
	switch o := obj.(type) {
	case *object.Null:
		return nil
	case *object.Boolean:
		return o.Value
	case *object.Integer:
		return o.Value
	case *object.Float:
		return o.Value
	case *object.String:
		return o.Value
	case *object.Array:
		arr := make([]interface{}, len(o.Elements))
		for i, e := range o.Elements {
			arr[i] = objectToInterface(e)
		}
		return arr
	case *object.Hash:
		m := map[string]interface{}{}
		for _, pair := range o.Pairs {
			m[pair.Key.Inspect()] = objectToInterface(pair.Value)
		}
		return m
	default:
		return o.Inspect()
	}
}


func evalConstStatement(node *ast.ConstStatement, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	env.SetConst(node.Name.Value, val)
	return NULL
}

func evalThrowStatement(node *ast.ThrowStatement, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	return newError("%s", val.Inspect())
}

func evalTryStatement(node *ast.TryStatement, env *object.Environment) object.Object {
	result := Eval(node.Body, env)
	if isError(result) {
		if node.Catch != nil {
			catchEnv := object.NewEnclosedEnvironment(env)
			if node.CatchId != nil {
				catchEnv.Set(node.CatchId.Value, &object.String{Value: result.(*object.Error).Message})
			}
			return Eval(node.Catch, catchEnv)
		}
		return result
	}
	return result
}

func evalMatchExpression(node *ast.MatchExpression, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	for _, arm := range node.Arms {
		pat := Eval(arm.Pattern, env)
		if isError(pat) {
			return pat
		}
		if matchEquals(val, pat) {
			return Eval(arm.Body, env)
		}
		// identifier pattern binds
		if ident, ok := arm.Pattern.(*ast.Identifier); ok {
			if ident.Value == "_" {
				return Eval(arm.Body, env)
			}
			// bind and match any
			env.Set(ident.Value, val)
			return Eval(arm.Body, env)
		}
	}
	if node.Default != nil {
		return Eval(node.Default, env)
	}
	return NULL
}

func matchEquals(a, b object.Object) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch av := a.(type) {
	case *object.Integer:
		return av.Value == b.(*object.Integer).Value
	case *object.Float:
		return av.Value == b.(*object.Float).Value
	case *object.String:
		return av.Value == b.(*object.String).Value
	case *object.Boolean:
		return av.Value == b.(*object.Boolean).Value
	case *object.Null:
		return true
	default:
		return a.Inspect() == b.Inspect()
	}
}


func evalYieldStatement(node *ast.YieldStatement, env *object.Environment) object.Object {
	var val object.Object = NULL
	if node.Value != nil {
		val = Eval(node.Value, env)
		if isError(val) {
			return val
		}
	}
	return &object.YieldValue{Value: val}
}

func evalDecoratorStatement(node *ast.DecoratorStatement, env *object.Environment) object.Object {
	// Evaluate the function statement first (binds name)
	result := Eval(node.Function, env)
	if isError(result) {
		return result
	}
	// Get the function we just defined
	// parseFunctionStatement returns LetStatement
	letStmt, ok := node.Function.(*ast.LetStatement)
	if !ok {
		return newError("@decorator: expected function statement")
	}
	fnObj, ok := env.Get(letStmt.Name.Value)
	if !ok {
		return newError("@decorator: function not bound")
	}
	dec, ok := env.Get(node.Decorator.Value)
	if !ok {
		return newError("@decorator: decorator not found: %s", node.Decorator.Value)
	}
	// Call decorator(fn) and rebind
	wrapped := applyFunction(dec, []object.Object{fnObj})
	if isError(wrapped) {
		return wrapped
	}
	env.Set(letStmt.Name.Value, wrapped)
	return wrapped
}

func evalTypedLetStatement(node *ast.TypedLetStatement, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	// Light runtime type check
	expected := node.TypeName.Value
	actual := string(val.Type())
	typeMap := map[string]string{
		"int": "INTEGER", "integer": "INTEGER", "float": "FLOAT",
		"string": "STRING", "str": "STRING", "bool": "BOOLEAN", "boolean": "BOOLEAN",
		"array": "ARRAY", "map": "HASH", "hash": "HASH", "fn": "FUNCTION", "any": actual,
	}
	if exp, ok := typeMap[expected]; ok && expected != "any" {
		if exp != actual {
			return newError("type error: expected %s, got %s", expected, actual)
		}
	}
	env.Set(node.Name.Value, val)
	return NULL
}


func evalTernaryExpression(node *ast.TernaryExpression, env *object.Environment) object.Object {
	cond := Eval(node.Condition, env)
	if isError(cond) {
		return cond
	}
	if isTruthy(cond) {
		return Eval(node.Consequence, env)
	}
	return Eval(node.Alternative, env)
}


func deepCopy(obj object.Object) object.Object {
	switch o := obj.(type) {
	case *object.Integer:
		return &object.Integer{Value: o.Value}
	case *object.Float:
		return &object.Float{Value: o.Value}
	case *object.String:
		return &object.String{Value: o.Value}
	case *object.Boolean:
		return nativeBoolToBooleanObject(o.Value)
	case *object.Array:
		el := make([]object.Object, len(o.Elements))
		for i, e := range o.Elements {
			el[i] = deepCopy(e)
		}
		return &object.Array{Elements: el}
	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair)
		for k, p := range o.Pairs {
			pairs[k] = object.HashPair{Key: deepCopy(p.Key), Value: deepCopy(p.Value)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return o
	}
}


func evalInOperator(left, right object.Object) object.Object {
	switch r := right.(type) {
	case *object.Array:
		for _, el := range r.Elements {
			if el.Type() == left.Type() && el.Inspect() == left.Inspect() {
				return TRUE
			}
		}
		return FALSE
	case *object.String:
		if l, ok := left.(*object.String); ok {
			return nativeBoolToBooleanObject(strings.Contains(r.Value, l.Value))
		}
		return nativeBoolToBooleanObject(strings.Contains(r.Value, left.Inspect()))
	case *object.Hash:
		if key, ok := left.(object.Hashable); ok {
			_, exists := r.Pairs[key.HashKey()]
			return nativeBoolToBooleanObject(exists)
		}
		return FALSE
	default:
		return newError("in: right side must be array, string, or map")
	}
}

// Builtins

var builtins map[string]*object.Builtin

func initBuiltins() {
	if builtins != nil { return }
	builtins = map[string]*object.Builtin{
	"len": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("wrong number of arguments. got=%d, want=1", len(args))
			}
			switch arg := args[0].(type) {
			case *object.String:
				return &object.Integer{Value: int64(len(arg.Value))}
			case *object.Array:
				return &object.Integer{Value: int64(len(arg.Elements))}
			default:
				return newError("argument to `len` not supported, got %s", args[0].Type())
			}
		},
	},
	"str": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("wrong number of arguments. got=%d, want=1", len(args))
			}
			return &object.String{Value: args[0].Inspect()}
		},
	},
	"read_file": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("read_file: want 1 argument")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("read_file: path must be string")
			}
			data, err := os.ReadFile(path.Value)
			if err != nil {
				return newError("read_file: %s", err.Error())
			}
			return &object.String{Value: string(data)}
		},
	},
	"write_file": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("write_file: want 2 arguments (path, content)")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("write_file: path must be string")
			}
			content := args[1].Inspect()
			if s, ok := args[1].(*object.String); ok {
				content = s.Value
			}
			err := os.WriteFile(path.Value, []byte(content), 0644)
			if err != nil {
				return newError("write_file: %s", err.Error())
			}
			return TRUE
		},
	},
	"http_get": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("http_get: want 1 argument (url)")
			}
			url, ok := args[0].(*object.String)
			if !ok {
				return newError("http_get: url must be string")
			}
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(url.Value)
			if err != nil {
				return newError("http_get: %s", err.Error())
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return newError("http_get: read body: %s", err.Error())
			}
			return &object.String{Value: string(body)}
		},
	},
	"json_parse": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("json_parse: want 1 argument")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("json_parse: argument must be string")
			}
			return jsonToObject([]byte(s.Value))
		},
	},
	"json_stringify": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("json_stringify: want 1 argument")
			}
			data, err := objectToJSON(args[0])
			if err != nil {
				return newError("json_stringify: %s", err.Error())
			}
			return &object.String{Value: string(data)}
		},
	},
	"type": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("type: want 1 argument")
			}
			return &object.String{Value: string(args[0].Type())}
		},
	},
	"push": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("push: want 2 arguments (array, value)")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("push: first arg must be array")
			}
			arr.Elements = append(arr.Elements, args[1])
			return arr
		},
	},
	"range": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 || len(args) > 3 {
				return newError("range: want 1-3 arguments")
			}
			toInt := func(o object.Object) (int64, bool) {
				if i, ok := o.(*object.Integer); ok {
					return i.Value, true
				}
				return 0, false
			}
			var start, end, step int64 = 0, 0, 1
			if len(args) == 1 {
				end, _ = toInt(args[0])
			} else if len(args) == 2 {
				start, _ = toInt(args[0])
				end, _ = toInt(args[1])
			} else {
				start, _ = toInt(args[0])
				end, _ = toInt(args[1])
				step, _ = toInt(args[2])
				if step == 0 {
					return newError("range: step cannot be 0")
				}
			}
			var elements []object.Object
			if step > 0 {
				for i := start; i < end; i += step {
					elements = append(elements, &object.Integer{Value: i})
				}
			} else {
				for i := start; i > end; i += step {
					elements = append(elements, &object.Integer{Value: i})
				}
			}
			return &object.Array{Elements: elements}
		},
	},
	"keys": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("keys: want 1 argument")
			}
			hash, ok := args[0].(*object.Hash)
			if !ok {
				return newError("keys: argument must be hash")
			}
			var elements []object.Object
			for _, pair := range hash.Pairs {
				elements = append(elements, pair.Key)
			}
			return &object.Array{Elements: elements}
		},
	},
	"values": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("values: want 1 argument")
			}
			hash, ok := args[0].(*object.Hash)
			if !ok {
				return newError("values: argument must be hash")
			}
			var elements []object.Object
			for _, pair := range hash.Pairs {
				elements = append(elements, pair.Value)
			}
			return &object.Array{Elements: elements}
		},
	},
	"system": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("system: want command string")
			}
			cmdStr, ok := args[0].(*object.String)
			if !ok {
				return newError("system: command must be string")
			}
			cmd := exec.Command("sh", "-c", cmdStr.Value)
			out, err := cmd.CombinedOutput()
			result := string(out)
			if err != nil {
				return &object.String{Value: result + "\n[exit error: " + err.Error() + "]"}
			}
			return &object.String{Value: result}
		},
	},
	"python": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("python: want 1 argument (code string)")
			}
			code, ok := args[0].(*object.String)
			if !ok {
				return newError("python: code must be string")
			}
			cmd := exec.Command("python3", "-c", code.Value)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return newError("python: %s\n%s", err.Error(), string(out))
			}
			return &object.String{Value: string(out)}
		},
	},
	
	"js": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("js: want 1 argument (code string)")
			}
			code, ok := args[0].(*object.String)
			if !ok {
				return newError("js: code must be string")
			}
			cmd := exec.Command("node", "-e", code.Value)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return newError("js: %s\n%s", err.Error(), string(out))
			}
			return &object.String{Value: string(out)}
		},
	},
	"ruby": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("ruby: want 1 argument (code string)")
			}
			code, ok := args[0].(*object.String)
			if !ok {
				return newError("ruby: code must be string")
			}
			cmd := exec.Command("ruby", "-e", code.Value)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return newError("ruby: %s\n%s", err.Error(), string(out))
			}
			return &object.String{Value: string(out)}
		},
	},
	"next": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("next: want generator")
			}
			gen, ok := args[0].(*object.Generator)
			if !ok {
				return newError("next: not a generator")
			}
			return gen.Next()
		},
	},
	"metric_inc": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("metric_inc: want name")
			}
			name := args[0].Inspect()
			if s, ok := args[0].(*object.String); ok {
				name = s.Value
			}
			by := int64(1)
			if len(args) >= 2 {
				if i, ok := args[1].(*object.Integer); ok {
					by = i.Value
				}
			}
			metricCounters[name] += by
			return &object.Integer{Value: metricCounters[name]}
		},
	},
	"metric_get": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("metric_get: want name")
			}
			name := args[0].Inspect()
			if s, ok := args[0].(*object.String); ok {
				name = s.Value
			}
			return &object.Integer{Value: metricCounters[name]}
		},
	},
	"metric_gauge": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("metric_gauge: want name, value")
			}
			name := args[0].Inspect()
			if s, ok := args[0].(*object.String); ok {
				name = s.Value
			}
			var v float64
			switch n := args[1].(type) {
			case *object.Integer:
				v = float64(n.Value)
			case *object.Float:
				v = n.Value
			default:
				return newError("metric_gauge: value must be number")
			}
			metricGauges[name] = v
			return &object.Float{Value: v}
		},
	},
	"trace": {
		Fn: func(args ...object.Object) object.Object {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.Inspect()
			}
			fmt.Println("[trace]", strings.Join(parts, " "))
			return NULL
		},
	},
	"hardware_info": {
		Fn: func(args ...object.Object) object.Object {
			info := "{\"usb\":\"unsupported in this build\",\"spi\":\"unsupported\",\"i2c\":\"unsupported\",\"serial\":\"unsupported\",\"hid\":\"unsupported\",\"note\":\"hardware APIs are stubs matching original claims\"}"
			return &object.String{Value: info}
		},
	},
	"wasi_info": {
		Fn: func(args ...object.Object) object.Object {
			return &object.String{Value: "{\"wasi\":\"not embedded\",\"component_model\":\"not embedded\",\"note\":\"use system/python/js for host interop\"}"}
		},
	},

	
	"replace": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("replace: want string, old, new")
			}
			s, a, b := args[0].(*object.String), args[1].(*object.String), args[2].(*object.String)
			if s == nil || a == nil || b == nil {
				return newError("replace: want strings")
			}
			return &object.String{Value: strings.ReplaceAll(s.Value, a.Value, b.Value)}
		},
	},
	"trim": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("trim: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("trim: want string")
			}
			return &object.String{Value: strings.TrimSpace(s.Value)}
		},
	},
	"starts_with": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("starts_with: want string, prefix")
			}
			s, ok1 := args[0].(*object.String)
			p, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("starts_with: want strings")
			}
			return nativeBoolToBooleanObject(strings.HasPrefix(s.Value, p.Value))
		},
	},
	"ends_with": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("ends_with: want string, suffix")
			}
			s, ok1 := args[0].(*object.String)
			p, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("ends_with: want strings")
			}
			return nativeBoolToBooleanObject(strings.HasSuffix(s.Value, p.Value))
		},
	},
	"substr": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 || len(args) > 3 {
				return newError("substr: want string, start [, len]")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("substr: want string")
			}
			runes := []rune(s.Value)
			start := int(args[1].(*object.Integer).Value)
			if start < 0 {
				start = 0
			}
			if start > len(runes) {
				return &object.String{Value: ""}
			}
			length := len(runes) - start
			if len(args) == 3 {
				length = int(args[2].(*object.Integer).Value)
			}
			end := start + length
			if end > len(runes) {
				end = len(runes)
			}
			return &object.String{Value: string(runes[start:end])}
		},
	},
	"pop": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("pop: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("pop: want array")
			}
			if len(arr.Elements) == 0 {
				return NULL
			}
			last := arr.Elements[len(arr.Elements)-1]
			arr.Elements = arr.Elements[:len(arr.Elements)-1]
			return last
		},
	},
	"reverse": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("reverse: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("reverse: want array")
			}
			n := len(arr.Elements)
			for i := 0; i < n/2; i++ {
				arr.Elements[i], arr.Elements[n-1-i] = arr.Elements[n-1-i], arr.Elements[i]
			}
			return arr
		},
	},
	"sort": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sort: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("sort: want array")
			}
			sort.SliceStable(arr.Elements, func(i, j int) bool {
				a, b := arr.Elements[i], arr.Elements[j]
				if ai, ok := a.(*object.Integer); ok {
					if bi, ok := b.(*object.Integer); ok {
						return ai.Value < bi.Value
					}
				}
				if af, ok := a.(*object.Float); ok {
					if bf, ok := b.(*object.Float); ok {
						return af.Value < bf.Value
					}
				}
				return a.Inspect() < b.Inspect()
			})
			return arr
		},
	},
	"has": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("has: want map, key")
			}
			hash, ok := args[0].(*object.Hash)
			if !ok {
				return newError("has: want map")
			}
			key, ok := args[1].(object.Hashable)
			if !ok {
				return newError("has: unusable key")
			}
			_, exists := hash.Pairs[key.HashKey()]
			return nativeBoolToBooleanObject(exists)
		},
	},
	"delete": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("delete: want map, key")
			}
			hash, ok := args[0].(*object.Hash)
			if !ok {
				return newError("delete: want map")
			}
			key, ok := args[1].(object.Hashable)
			if !ok {
				return newError("delete: unusable key")
			}
			delete(hash.Pairs, key.HashKey())
			return TRUE
		},
	},
	"min": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return newError("min: want args")
			}
			// support min(array) or min(a,b,c)
			vals := args
			if len(args) == 1 {
				if arr, ok := args[0].(*object.Array); ok {
					vals = arr.Elements
				}
			}
			if len(vals) == 0 {
				return NULL
			}
			best := vals[0]
			for _, v := range vals[1:] {
				if bi, ok := best.(*object.Integer); ok {
					if vi, ok := v.(*object.Integer); ok && vi.Value < bi.Value {
						best = v
					}
				}
			}
			return best
		},
	},
	"max": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return newError("max: want args")
			}
			vals := args
			if len(args) == 1 {
				if arr, ok := args[0].(*object.Array); ok {
					vals = arr.Elements
				}
			}
			if len(vals) == 0 {
				return NULL
			}
			best := vals[0]
			for _, v := range vals[1:] {
				if bi, ok := best.(*object.Integer); ok {
					if vi, ok := v.(*object.Integer); ok && vi.Value > bi.Value {
						best = v
					}
				}
			}
			return best
		},
	},
	"pow": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("pow: want base, exp")
			}
			var b, e float64
			switch v := args[0].(type) {
			case *object.Integer:
				b = float64(v.Value)
			case *object.Float:
				b = v.Value
			default:
				return newError("pow: want numbers")
			}
			switch v := args[1].(type) {
			case *object.Integer:
				e = float64(v.Value)
			case *object.Float:
				e = v.Value
			default:
				return newError("pow: want numbers")
			}
			return &object.Float{Value: math.Pow(b, e)}
		},
	},
	"sqrt": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sqrt: want 1 number")
			}
			var v float64
			switch n := args[0].(type) {
			case *object.Integer:
				v = float64(n.Value)
			case *object.Float:
				v = n.Value
			default:
				return newError("sqrt: want number")
			}
			return &object.Float{Value: math.Sqrt(v)}
		},
	},
	"floor": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("floor: want number")
			}
			switch n := args[0].(type) {
			case *object.Integer:
				return n
			case *object.Float:
				return &object.Integer{Value: int64(math.Floor(n.Value))}
			default:
				return newError("floor: want number")
			}
		},
	},
	"ceil": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("ceil: want number")
			}
			switch n := args[0].(type) {
			case *object.Integer:
				return n
			case *object.Float:
				return &object.Integer{Value: int64(math.Ceil(n.Value))}
			default:
				return newError("ceil: want number")
			}
		},
	},
	"exists": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("exists: want path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("exists: want string path")
			}
			_, err := os.Stat(path.Value)
			return nativeBoolToBooleanObject(err == nil)
		},
	},
	"listdir": {
		Fn: func(args ...object.Object) object.Object {
			path := "."
			if len(args) >= 1 {
				if s, ok := args[0].(*object.String); ok {
					path = s.Value
				}
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return newError("listdir: %s", err.Error())
			}
			el := make([]object.Object, 0, len(entries))
			for _, e := range entries {
				el = append(el, &object.String{Value: e.Name()})
			}
			return &object.Array{Elements: el}
		},
	},
	"sleep": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sleep: want milliseconds")
			}
			ms, ok := args[0].(*object.Integer)
			if !ok {
				return newError("sleep: want integer ms")
			}
			time.Sleep(time.Duration(ms.Value) * time.Millisecond)
			return NULL
		},
	},
	"exit": {
		Fn: func(args ...object.Object) object.Object {
			code := 0
			if len(args) >= 1 {
				if i, ok := args[0].(*object.Integer); ok {
					code = int(i.Value)
				}
			}
			os.Exit(code)
			return NULL
		},
	},
	"format": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("format: want format string")
			}
			fmtStr, ok := args[0].(*object.String)
			if !ok {
				return newError("format: first arg must be string")
			}
			// Simple {0} {1} replacement
			out := fmtStr.Value
			for i, a := range args[1:] {
				token := fmt.Sprintf("{%d}", i)
				out = strings.ReplaceAll(out, token, a.Inspect())
			}
			return &object.String{Value: out}
		},
	},
"split": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("split: want string, sep")
			}
			s, ok1 := args[0].(*object.String)
			sep, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("split: want strings")
			}
			parts := strings.Split(s.Value, sep.Value)
			el := make([]object.Object, len(parts))
			for i, p := range parts {
				el[i] = &object.String{Value: p}
			}
			return &object.Array{Elements: el}
		},
	},
	"join": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("join: want array, sep")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("join: first arg must be array")
			}
			sep := ","
			if s, ok := args[1].(*object.String); ok {
				sep = s.Value
			}
			parts := make([]string, len(arr.Elements))
			for i, e := range arr.Elements {
				parts[i] = e.Inspect()
			}
			return &object.String{Value: strings.Join(parts, sep)}
		},
	},
	"upper": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("upper: want 1 string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("upper: want string")
			}
			return &object.String{Value: strings.ToUpper(s.Value)}
		},
	},
	"lower": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("lower: want 1 string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("lower: want string")
			}
			return &object.String{Value: strings.ToLower(s.Value)}
		},
	},
	"contains": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("contains: want string, substr")
			}
			s, ok1 := args[0].(*object.String)
			sub, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("contains: want strings")
			}
			return nativeBoolToBooleanObject(strings.Contains(s.Value, sub.Value))
		},
	},
	"slice": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 || len(args) > 3 {
				return newError("slice: want arr, start [, end]")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("slice: first arg must be array")
			}
			start := int(args[1].(*object.Integer).Value)
			end := len(arr.Elements)
			if len(args) == 3 {
				end = int(args[2].(*object.Integer).Value)
			}
			if start < 0 {
				start = 0
			}
			if end > len(arr.Elements) {
				end = len(arr.Elements)
			}
			if start > end {
				return &object.Array{Elements: []object.Object{}}
			}
			return &object.Array{Elements: arr.Elements[start:end]}
		},
	},
	"assert": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("assert: want condition")
			}
			if !isTruthy(args[0]) {
				msg := "assertion failed"
				if len(args) >= 2 {
					msg = args[1].Inspect()
				}
				return newError("%s", msg)
			}
			return TRUE
		},
	},
	"now": {
		Fn: func(args ...object.Object) object.Object {
			return &object.Integer{Value: time.Now().Unix()}
		},
	},
	"env": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("env: want name")
			}
			name := args[0].Inspect()
			if s, ok := args[0].(*object.String); ok {
				name = s.Value
			}
			return &object.String{Value: os.Getenv(name)}
		},
	},
	"args": {
		Fn: func(args ...object.Object) object.Object {
			el := make([]object.Object, len(CLIArgs))
			for i, a := range CLIArgs {
				el[i] = &object.String{Value: a}
			}
			return &object.Array{Elements: el}
		},
	},
	"int": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("int: want 1 arg")
			}
			switch v := args[0].(type) {
			case *object.Integer:
				return v
			case *object.Float:
				return &object.Integer{Value: int64(v.Value)}
			case *object.String:
				var n int64
				fmt.Sscan(v.Value, &n)
				return &object.Integer{Value: n}
			case *object.Boolean:
				if v.Value {
					return &object.Integer{Value: 1}
				}
				return &object.Integer{Value: 0}
			default:
				return newError("int: cannot convert %s", args[0].Type())
			}
		},
	},
	
	
	
	
	"group_by": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("group_by: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("group_by: want array")
			}
			groups := map[string]*object.Array{}
			order := []string{}
			for _, el := range arr.Elements {
				keyObj := applyFunction(args[1], []object.Object{el})
				if isError(keyObj) {
					return keyObj
				}
				k := keyObj.Inspect()
				if _, ok := groups[k]; !ok {
					groups[k] = &object.Array{Elements: []object.Object{}}
					order = append(order, k)
				}
				groups[k].Elements = append(groups[k].Elements, el)
			}
			pairs := make(map[object.HashKey]object.HashPair)
			for _, k := range order {
				key := &object.String{Value: k}
				pairs[key.HashKey()] = object.HashPair{Key: key, Value: groups[k]}
			}
			return &object.Hash{Pairs: pairs}
		},
	},
	"sort_by": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("sort_by: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("sort_by: want array")
			}
			type pair struct {
				el  object.Object
				key string
			}
			items := make([]pair, len(arr.Elements))
			for i, el := range arr.Elements {
				k := applyFunction(args[1], []object.Object{el})
				if isError(k) {
					return k
				}
				items[i] = pair{el: el, key: k.Inspect()}
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i].key < items[j].key })
			out := make([]object.Object, len(items))
			for i, it := range items {
				out[i] = it.el
			}
			return &object.Array{Elements: out}
		},
	},
	"glob": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("glob: want pattern")
			}
			pat, ok := args[0].(*object.String)
			if !ok {
				return newError("glob: want string pattern")
			}
			matches, err := filepath.Glob(pat.Value)
			if err != nil {
				return newError("glob: %s", err.Error())
			}
			out := make([]object.Object, len(matches))
			for i, m := range matches {
				out[i] = &object.String{Value: m}
			}
			return &object.Array{Elements: out}
		},
	},
	"set_env": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("set_env: want name, value")
			}
			name, ok1 := args[0].(*object.String)
			if !ok1 {
				return newError("set_env: name must be string")
			}
			val := args[1].Inspect()
			if s, ok := args[1].(*object.String); ok {
				val = s.Value
			}
			if err := os.Setenv(name.Value, val); err != nil {
				return newError("set_env: %s", err.Error())
			}
			return TRUE
		},
	},
	"is_null": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("is_null: want 1 arg")
			}
			return nativeBoolToBooleanObject(args[0].Type() == object.NULL_OBJ)
		},
	},
	"is_empty": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("is_empty: want 1 arg")
			}
			switch v := args[0].(type) {
			case *object.Null:
				return TRUE
			case *object.String:
				return nativeBoolToBooleanObject(v.Value == "")
			case *object.Array:
				return nativeBoolToBooleanObject(len(v.Elements) == 0)
			case *object.Hash:
				return nativeBoolToBooleanObject(len(v.Pairs) == 0)
			default:
				return FALSE
			}
		},
	},
	"pad_left": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return newError("pad_left: want string, width [, pad]")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("pad_left: want string")
			}
			width := int(args[1].(*object.Integer).Value)
			pad := " "
			if len(args) >= 3 {
				if p, ok := args[2].(*object.String); ok {
					pad = p.Value
				}
			}
			if pad == "" {
				pad = " "
			}
			out := s.Value
			for len([]rune(out)) < width {
				out = pad + out
			}
			return &object.String{Value: out}
		},
	},
	"pad_right": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return newError("pad_right: want string, width [, pad]")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("pad_right: want string")
			}
			width := int(args[1].(*object.Integer).Value)
			pad := " "
			if len(args) >= 3 {
				if p, ok := args[2].(*object.String); ok {
					pad = p.Value
				}
			}
			if pad == "" {
				pad = " "
			}
			out := s.Value
			for len([]rune(out)) < width {
				out = out + pad
			}
			return &object.String{Value: out}
		},
	},
	"count": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("count: want array/string, value")
			}
			if arr, ok := args[0].(*object.Array); ok {
				n := 0
				for _, el := range arr.Elements {
					if el.Type() == args[1].Type() && el.Inspect() == args[1].Inspect() {
						n++
					}
				}
				return &object.Integer{Value: int64(n)}
			}
			if s, ok := args[0].(*object.String); ok {
				sub, ok := args[1].(*object.String)
				if !ok {
					return newError("count: substring must be string")
				}
				return &object.Integer{Value: int64(strings.Count(s.Value, sub.Value))}
			}
			return newError("count: want array or string")
		},
	},
"sum": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sum: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("sum: want array")
			}
			var total int64
			var ftotal float64
			useFloat := false
			for _, e := range arr.Elements {
				switch v := e.(type) {
				case *object.Integer:
					total += v.Value
					ftotal += float64(v.Value)
				case *object.Float:
					useFloat = true
					ftotal += v.Value
				default:
					return newError("sum: non-numeric element")
				}
			}
			if useFloat {
				return &object.Float{Value: ftotal}
			}
			return &object.Integer{Value: total}
		},
	},
	"avg": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("avg: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok || len(arr.Elements) == 0 {
				return newError("avg: want non-empty array")
			}
			s := builtins["sum"].Fn(arr)
			if isError(s) {
				return s
			}
			n := float64(len(arr.Elements))
			switch v := s.(type) {
			case *object.Integer:
				return &object.Float{Value: float64(v.Value) / n}
			case *object.Float:
				return &object.Float{Value: v.Value / n}
			default:
				return newError("avg: bad sum")
			}
		},
	},
	"take": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("take: want array, n")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("take: want array")
			}
			n := int(args[1].(*object.Integer).Value)
			if n < 0 {
				n = 0
			}
			if n > len(arr.Elements) {
				n = len(arr.Elements)
			}
			return &object.Array{Elements: arr.Elements[:n]}
		},
	},
	"drop": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("drop: want array, n")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("drop: want array")
			}
			n := int(args[1].(*object.Integer).Value)
			if n < 0 {
				n = 0
			}
			if n > len(arr.Elements) {
				return &object.Array{Elements: []object.Object{}}
			}
			return &object.Array{Elements: arr.Elements[n:]}
		},
	},
	"chunk": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("chunk: want array, size")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("chunk: want array")
			}
			size := int(args[1].(*object.Integer).Value)
			if size <= 0 {
				return newError("chunk: size > 0")
			}
			var out []object.Object
			for i := 0; i < len(arr.Elements); i += size {
				end := i + size
				if end > len(arr.Elements) {
					end = len(arr.Elements)
				}
				chunk := make([]object.Object, end-i)
				copy(chunk, arr.Elements[i:end])
				out = append(out, &object.Array{Elements: chunk})
			}
			return &object.Array{Elements: out}
		},
	},
	"lines": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("lines: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("lines: want string")
			}
			parts := strings.Split(strings.ReplaceAll(s.Value, "\r\n", "\n"), "\n")
			out := make([]object.Object, len(parts))
			for i, p := range parts {
				out[i] = &object.String{Value: p}
			}
			return &object.Array{Elements: out}
		},
	},
	"read_json": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("read_json: want path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("read_json: want string path")
			}
			data, err := os.ReadFile(path.Value)
			if err != nil {
				return newError("read_json: %s", err.Error())
			}
			return jsonToObject(data)
		},
	},
	"write_json": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("write_json: want path, value")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("write_json: want path string")
			}
			data, err := objectToJSON(args[1])
			if err != nil {
				return newError("write_json: %s", err.Error())
			}
			if err := os.WriteFile(path.Value, data, 0644); err != nil {
				return newError("write_json: %s", err.Error())
			}
			return TRUE
		},
	},
	"mkdir": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("mkdir: want path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("mkdir: want string")
			}
			if err := os.MkdirAll(path.Value, 0755); err != nil {
				return newError("mkdir: %s", err.Error())
			}
			return TRUE
		},
	},
	"remove": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("remove: want path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("remove: want string")
			}
			if err := os.RemoveAll(path.Value); err != nil {
				return newError("remove: %s", err.Error())
			}
			return TRUE
		},
	},
	"cwd": {
		Fn: func(args ...object.Object) object.Object {
			d, err := os.Getwd()
			if err != nil {
				return newError("cwd: %s", err.Error())
			}
			return &object.String{Value: d}
		},
	},
	"cd": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("cd: want path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return newError("cd: want string")
			}
			if err := os.Chdir(path.Value); err != nil {
				return newError("cd: %s", err.Error())
			}
			return TRUE
		},
	},
	"find": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("find: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("find: want array")
			}
			for _, el := range arr.Elements {
				res := applyFunction(args[1], []object.Object{el})
				if isError(res) {
					return res
				}
				if isTruthy(res) {
					return el
				}
			}
			return NULL
		},
	},
	"index_of": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("index_of: want array/string, value")
			}
			if arr, ok := args[0].(*object.Array); ok {
				for i, el := range arr.Elements {
					if el.Inspect() == args[1].Inspect() && el.Type() == args[1].Type() {
						return &object.Integer{Value: int64(i)}
					}
				}
				return &object.Integer{Value: -1}
			}
			if s, ok := args[0].(*object.String); ok {
				sub, ok := args[1].(*object.String)
				if !ok {
					return newError("index_of: substring must be string")
				}
				return &object.Integer{Value: int64(strings.Index(s.Value, sub.Value))}
			}
			return newError("index_of: want array or string")
		},
	},
"map_fn": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("map_fn: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("map_fn: want array")
			}
			var out []object.Object
			for _, el := range arr.Elements {
				res := applyFunction(args[1], []object.Object{el})
				if isError(res) {
					return res
				}
				out = append(out, res)
			}
			return &object.Array{Elements: out}
		},
	},
	"filter": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("filter: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("filter: want array")
			}
			var out []object.Object
			for _, el := range arr.Elements {
				res := applyFunction(args[1], []object.Object{el})
				if isError(res) {
					return res
				}
				if isTruthy(res) {
					out = append(out, el)
				}
			}
			return &object.Array{Elements: out}
		},
	},
	"reduce": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 || len(args) > 3 {
				return newError("reduce: want array, function [, init]")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("reduce: want array")
			}
			fn := args[1]
			var acc object.Object
			start := 0
			if len(args) == 3 {
				acc = args[2]
			} else {
				if len(arr.Elements) == 0 {
					return NULL
				}
				acc = arr.Elements[0]
				start = 1
			}
			for i := start; i < len(arr.Elements); i++ {
				res := applyFunction(fn, []object.Object{acc, arr.Elements[i]})
				if isError(res) {
					return res
				}
				acc = res
			}
			return acc
		},
	},
	"zip": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("zip: want two arrays")
			}
			a, ok1 := args[0].(*object.Array)
			b, ok2 := args[1].(*object.Array)
			if !ok1 || !ok2 {
				return newError("zip: want arrays")
			}
			n := len(a.Elements)
			if len(b.Elements) < n {
				n = len(b.Elements)
			}
			out := make([]object.Object, n)
			for i := 0; i < n; i++ {
				out[i] = &object.Array{Elements: []object.Object{a.Elements[i], b.Elements[i]}}
			}
			return &object.Array{Elements: out}
		},
	},
	"repeat": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("repeat: want string, count")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("repeat: want string")
			}
			n, ok := args[1].(*object.Integer)
			if !ok {
				return newError("repeat: want count int")
			}
			if n.Value < 0 {
				return newError("repeat: count >= 0")
			}
			return &object.String{Value: strings.Repeat(s.Value, int(n.Value))}
		},
	},
	"enumerate": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("enumerate: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("enumerate: want array")
			}
			out := make([]object.Object, len(arr.Elements))
			for i, el := range arr.Elements {
				out[i] = &object.Array{Elements: []object.Object{
					&object.Integer{Value: int64(i)},
					el,
				}}
			}
			return &object.Array{Elements: out}
		},
	},
	"regex_match": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("regex_match: want pattern, string")
			}
			pat, ok1 := args[0].(*object.String)
			s, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("regex_match: want strings")
			}
			re, err := regexp.Compile(pat.Value)
			if err != nil {
				return newError("regex_match: %s", err.Error())
			}
			return nativeBoolToBooleanObject(re.MatchString(s.Value))
		},
	},
	"regex_find": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("regex_find: want pattern, string")
			}
			pat, ok1 := args[0].(*object.String)
			s, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return newError("regex_find: want strings")
			}
			re, err := regexp.Compile(pat.Value)
			if err != nil {
				return newError("regex_find: %s", err.Error())
			}
			all := re.FindAllString(s.Value, -1)
			out := make([]object.Object, len(all))
			for i, m := range all {
				out[i] = &object.String{Value: m}
			}
			return &object.Array{Elements: out}
		},
	},
	"regex_replace": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("regex_replace: want pattern, string, repl")
			}
			pat, ok1 := args[0].(*object.String)
			s, ok2 := args[1].(*object.String)
			repl, ok3 := args[2].(*object.String)
			if !ok1 || !ok2 || !ok3 {
				return newError("regex_replace: want strings")
			}
			re, err := regexp.Compile(pat.Value)
			if err != nil {
				return newError("regex_replace: %s", err.Error())
			}
			return &object.String{Value: re.ReplaceAllString(s.Value, repl.Value)}
		},
	},
	"csv_parse": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("csv_parse: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("csv_parse: want string")
			}
			lines := strings.Split(strings.ReplaceAll(s.Value, "\r\n", "\n"), "\n")
			var rows []object.Object
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.Split(line, ",")
				cells := make([]object.Object, len(parts))
				for i, c := range parts {
					cells[i] = &object.String{Value: strings.TrimSpace(c)}
				}
				rows = append(rows, &object.Array{Elements: cells})
			}
			return &object.Array{Elements: rows}
		},
	},
	"url_encode": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("url_encode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("url_encode: want string")
			}
			return &object.String{Value: url.QueryEscape(s.Value)}
		},
	},
	"url_decode": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("url_decode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("url_decode: want string")
			}
			out, err := url.QueryUnescape(s.Value)
			if err != nil {
				return newError("url_decode: %s", err.Error())
			}
			return &object.String{Value: out}
		},
	},
	
	"log": {
		Fn: func(args ...object.Object) object.Object {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.Inspect()
			}
			fmt.Println(strings.Join(parts, " "))
			return NULL
		},
	},
	"printf": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("printf: want format")
			}
			fmtStr, ok := args[0].(*object.String)
			if !ok {
				return newError("printf: format must be string")
			}
			out := fmtStr.Value
			for i, a := range args[1:] {
				token := fmt.Sprintf("{%d}", i)
				out = strings.ReplaceAll(out, token, a.Inspect())
			}
			fmt.Print(out)
			return NULL
		},
	},
	"metrics_text": {
		Fn: func(args ...object.Object) object.Object {
			var b strings.Builder
			for name, v := range metricCounters {
				b.WriteString(fmt.Sprintf("# TYPE %s counter\n%s %d\n", name, name, v))
			}
			for name, v := range metricGauges {
				b.WriteString(fmt.Sprintf("# TYPE %s gauge\n%s %g\n", name, name, v))
			}
			return &object.String{Value: b.String()}
		},
	},
	
	"ws_connect": {
		Fn: func(args ...object.Object) object.Object {
			// ws_connect(url, message) -> response string (single request/response style)
			if len(args) < 1 {
				return newError("ws_connect: want url [, message]")
			}
			urlStr, ok := args[0].(*object.String)
			if !ok {
				return newError("ws_connect: url must be string")
			}
			msg := ""
			if len(args) >= 2 {
				if s, ok := args[1].(*object.String); ok {
					msg = s.Value
				} else {
					msg = args[1].Inspect()
				}
			}
			ws, err := websocket.Dial(urlStr.Value, "", "http://localhost/")
			if err != nil {
				return newError("ws_connect: %s", err.Error())
			}
			defer ws.Close()
			if msg != "" {
				if err := websocket.Message.Send(ws, msg); err != nil {
					return newError("ws_connect send: %s", err.Error())
				}
			}
			var reply string
			if err := websocket.Message.Receive(ws, &reply); err != nil {
				return newError("ws_connect recv: %s", err.Error())
			}
			return &object.String{Value: reply}
		},
	},
"ws_info": {
		Fn: func(args ...object.Object) object.Object {
			return &object.String{Value: "{\"websocket\":\"partial\",\"hint\":\"use python/js polyglot for full ws\"}"}
		},
	},
	"grpc_info": {
		Fn: func(args ...object.Object) object.Object {
			return &object.String{Value: "{\"grpc\":\"stub\",\"status\":\"not embedded\"}"}
		},
	},
	"otel_info": {
		Fn: func(args ...object.Object) object.Object {
			return &object.String{Value: "{\"opentelemetry\":\"partial\",\"metrics\":\"metric_* + metrics_text\",\"traces\":\"trace\"}"}
		},
	},
"eprint": {
		Fn: func(args ...object.Object) object.Object {
			parts := make([]string, len(args))
			for i, a := range args {
				parts[i] = a.Inspect()
			}
			fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
			return NULL
		},
	},
	"timeit": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("timeit: want function [, times]")
			}
			times := 1
			if len(args) >= 2 {
				if i, ok := args[1].(*object.Integer); ok {
					times = int(i.Value)
				}
			}
			start := time.Now()
			var last object.Object = NULL
			for i := 0; i < times; i++ {
				last = applyFunction(args[0], []object.Object{})
				if isError(last) {
					return last
				}
			}
			elapsed := time.Since(start).Milliseconds()
			return &object.Integer{Value: elapsed}
		},
	},
	"any": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("any: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("any: want array")
			}
			for _, el := range arr.Elements {
				res := applyFunction(args[1], []object.Object{el})
				if isError(res) {
					return res
				}
				if isTruthy(res) {
					return TRUE
				}
			}
			return FALSE
		},
	},
	"all": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("all: want array, function")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("all: want array")
			}
			for _, el := range arr.Elements {
				res := applyFunction(args[1], []object.Object{el})
				if isError(res) {
					return res
				}
				if !isTruthy(res) {
					return FALSE
				}
			}
			return TRUE
		},
	},
"random": {
		Fn: func(args ...object.Object) object.Object {
			return &object.Float{Value: rand.Float64()}
		},
	},
	"rand_int": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) == 1 {
				n := int(args[0].(*object.Integer).Value)
				if n <= 0 {
					return newError("rand_int: n must be > 0")
				}
				return &object.Integer{Value: int64(rand.Intn(n))}
			}
			if len(args) == 2 {
				lo := int(args[0].(*object.Integer).Value)
				hi := int(args[1].(*object.Integer).Value)
				if hi <= lo {
					return newError("rand_int: hi must be > lo")
				}
				return &object.Integer{Value: int64(lo + rand.Intn(hi-lo))}
			}
			return newError("rand_int: want max or lo, hi")
		},
	},
	"uuid": {
		Fn: func(args ...object.Object) object.Object {
			b := make([]byte, 16)
			rand.Read(b)
			b[6] = (b[6] & 0x0f) | 0x40
			b[8] = (b[8] & 0x3f) | 0x80
			s := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
			return &object.String{Value: s}
		},
	},
	"date": {
		Fn: func(args ...object.Object) object.Object {
			t := time.Now()
			if len(args) >= 1 {
				if i, ok := args[0].(*object.Integer); ok {
					t = time.Unix(i.Value, 0)
				}
			}
			layout := time.RFC3339
			if len(args) >= 2 {
				if s, ok := args[1].(*object.String); ok {
					layout = s.Value
				}
			}
			return &object.String{Value: t.Format(layout)}
		},
	},
	"clamp": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("clamp: want val, lo, hi")
			}
			v := args[0].(*object.Integer).Value
			lo := args[1].(*object.Integer).Value
			hi := args[2].(*object.Integer).Value
			if v < lo {
				v = lo
			}
			if v > hi {
				v = hi
			}
			return &object.Integer{Value: v}
		},
	},
	"unique": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("unique: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("unique: want array")
			}
			seen := map[string]bool{}
			var out []object.Object
			for _, e := range arr.Elements {
				k := string(e.Type()) + ":" + e.Inspect()
				if !seen[k] {
					seen[k] = true
					out = append(out, e)
				}
			}
			return &object.Array{Elements: out}
		},
	},
	"flatten": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("flatten: want array")
			}
			arr, ok := args[0].(*object.Array)
			if !ok {
				return newError("flatten: want array")
			}
			var out []object.Object
			var walk func(object.Object)
			walk = func(o object.Object) {
				if a, ok := o.(*object.Array); ok {
					for _, e := range a.Elements {
						walk(e)
					}
				} else {
					out = append(out, o)
				}
			}
			walk(arr)
			return &object.Array{Elements: out}
		},
	},
	"base64_encode": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("base64_encode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("base64_encode: want string")
			}
			return &object.String{Value: base64.StdEncoding.EncodeToString([]byte(s.Value))}
		},
	},
	"base64_decode": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("base64_decode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("base64_decode: want string")
			}
			b, err := base64.StdEncoding.DecodeString(s.Value)
			if err != nil {
				return newError("base64_decode: %s", err.Error())
			}
			return &object.String{Value: string(b)}
		},
	},
	"md5": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("md5: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("md5: want string")
			}
			sum := md5.Sum([]byte(s.Value))
			return &object.String{Value: hex.EncodeToString(sum[:])}
		},
	},
	"sha256": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sha256: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("sha256: want string")
			}
			sum := sha256.Sum256([]byte(s.Value))
			return &object.String{Value: hex.EncodeToString(sum[:])}
		},
	},
	"basename": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("basename: want path")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("basename: want string")
			}
			return &object.String{Value: filepath.Base(s.Value)}
		},
	},
	"dirname": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("dirname: want path")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("dirname: want string")
			}
			return &object.String{Value: filepath.Dir(s.Value)}
		},
	},
	"join_path": {
		Fn: func(args ...object.Object) object.Object {
			parts := make([]string, 0, len(args))
			for _, a := range args {
				if s, ok := a.(*object.String); ok {
					parts = append(parts, s.Value)
				} else {
					parts = append(parts, a.Inspect())
				}
			}
			return &object.String{Value: filepath.Join(parts...)}
		},
	},
	"http_serve": {
		Fn: func(args ...object.Object) object.Object {
			// http_serve(port, body_or_handler_message)
			if len(args) < 1 {
				return newError("http_serve: want port [, body]")
			}
			port := args[0].Inspect()
			if i, ok := args[0].(*object.Integer); ok {
				port = fmt.Sprintf("%d", i.Value)
			} else if s, ok := args[0].(*object.String); ok {
				port = s.Value
			}
			body := "Navescript OK"
			if len(args) >= 2 {
				if s, ok := args[1].(*object.String); ok {
					body = s.Value
				} else {
					body = args[1].Inspect()
				}
			}
			addr := ":" + port
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("X-Powered-By", "Navescript")
				fmt.Fprint(w, body)
			})
			// non-blocking start
			go func() {
				_ = http.ListenAndServe(addr, mux)
			}()
			time.Sleep(50 * time.Millisecond)
			return &object.String{Value: "listening on " + addr}
		},
	},
	"copy": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("copy: want 1 value")
			}
			return deepCopy(args[0])
		},
	},
"abs": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("abs: want 1 number")
			}
			switch v := args[0].(type) {
			case *object.Integer:
				if v.Value < 0 {
					return &object.Integer{Value: -v.Value}
				}
				return v
			case *object.Float:
				if v.Value < 0 {
					return &object.Float{Value: -v.Value}
				}
				return v
			default:
				return newError("abs: want number")
			}
		},
	},
"sqlite_open": {
		Fn: func(args ...object.Object) object.Object {
			path := ":memory:"
			if len(args) >= 1 {
				if s, ok := args[0].(*object.String); ok {
					path = s.Value
				}
			}
			id, err := sqlite.Open(path)
			if err != nil {
				return newError("sqlite_open: %s", err.Error())
			}
			return &object.String{Value: id}
		},
	},
	"sqlite_exec": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("sqlite_exec: want handle, query")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("sqlite_exec: handle must be string")
			}
			query, ok := args[1].(*object.String)
			if !ok {
				return newError("sqlite_exec: query must be string")
			}
			n, err := sqlite.Exec(id.Value, query.Value)
			if err != nil {
				return newError("sqlite_exec: %s", err.Error())
			}
			return &object.Integer{Value: n}
		},
	},
	"sqlite_query": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("sqlite_query: want handle, query")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("sqlite_query: handle must be string")
			}
			query, ok := args[1].(*object.String)
			if !ok {
				return newError("sqlite_query: query must be string")
			}
			rows, cols, err := sqlite.Query(id.Value, query.Value)
			if err != nil {
				return newError("sqlite_query: %s", err.Error())
			}
			// Return array of hashes
			var result []object.Object
			for _, row := range rows {
				pairs := make(map[object.HashKey]object.HashPair)
				for i, col := range cols {
					k := &object.String{Value: col}
					v := &object.String{Value: row[i]}
					pairs[k.HashKey()] = object.HashPair{Key: k, Value: v}
				}
				result = append(result, &object.Hash{Pairs: pairs})
			}
			return &object.Array{Elements: result}
		},
	},
	"sqlite_close": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sqlite_close: want handle")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("sqlite_close: handle must be string")
			}
			if err := sqlite.Close(id.Value); err != nil {
				return newError("sqlite_close: %s", err.Error())
			}
			return TRUE
		},
	},
	"http_post": {
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return newError("http_post: want url, body [, content_type]")
			}
			url, ok := args[0].(*object.String)
			if !ok {
				return newError("http_post: url must be string")
			}
			body := args[1].Inspect()
			if s, ok := args[1].(*object.String); ok {
				body = s.Value
			}
			ct := "application/json"
			if len(args) >= 3 {
				if s, ok := args[2].(*object.String); ok {
					ct = s.Value
				}
			}
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Post(url.Value, ct, strings.NewReader(body))
			if err != nil {
				return newError("http_post: %s", err.Error())
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return newError("http_post: %s", err.Error())
			}
			return &object.String{Value: string(data)}
		},
	},
}
}

func ensureBuiltins() {
	if builtins == nil {
		initBuiltins()
	}
}


