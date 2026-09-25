// Wave 5 — runtime type contracts.
//
// NvS has NO static type checker, and this file does not add one. Everything
// here is enforced while the program runs:
//
//   - `fn f(a: int, b: string): bool` — parameter and return annotations are
//     checked at call time / on return. A violation is a runtime error.
//   - `int | string` unions and `T?` (= `T | null`) in annotation position.
//   - `interface Shape { area(): number }` — structural contracts checked by
//     implements()/assert_implements(), usable as annotations.
//   - `let x: int = 5` — the initial binding is checked and the declared type
//     is remembered; later assignments to x are checked too.
//   - is_int/is_float/... type-guard builtins and type_of().
//
// What is deliberately NOT done: no arity/signature variance analysis on
// interfaces beyond "the method can be called with the declared number of
// arguments"; no checking of generic parameters; no compile-time errors.

package eval

import (
	"fmt"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/object"
)

// friendlyTypeName is the lowercase, user-facing type name used by type_of()
// and by annotation error messages. It complements the existing type() /
// typeof() builtins, which return the uppercase internal names ("INTEGER").
func friendlyTypeName(obj object.Object) string {
	switch obj.(type) {
	case *object.Integer:
		return "int"
	case *object.Float:
		return "float"
	case *object.String:
		return "string"
	case *object.Boolean:
		return "bool"
	case *object.Null:
		return "null"
	case *object.Array:
		return "array"
	case *object.Hash:
		return "hash"
	case *object.Tuple:
		return "tuple"
	case *object.Record:
		return "record"
	case *object.RecordDef:
		return "record_def"
	case *object.Function:
		return "function"
	case *object.Builtin:
		return "builtin"
	case *object.Class:
		return "class"
	case *object.Instance:
		return "instance"
	case *object.Interface:
		return "interface"
	case *object.Generator:
		return "generator"
	case *object.Error:
		return "error"
	default:
		return strings.ToLower(string(obj.Type()))
	}
}

// isCallable is function or builtin — anything applyFunction can invoke.
func isCallable(obj object.Object) bool {
	switch obj.(type) {
	case *object.Function, *object.Builtin:
		return true
	}
	return false
}

// builtinAnnotationChecks maps lowercase annotation names to predicates.
// "number" is int-or-float; "function"/"fn"/"callable" accept user functions
// AND builtins (both are callable); "any" is handled before this table.
var builtinAnnotationChecks = map[string]func(object.Object) bool{
	"int":     func(o object.Object) bool { _, ok := o.(*object.Integer); return ok },
	"integer": func(o object.Object) bool { _, ok := o.(*object.Integer); return ok },
	"float":   func(o object.Object) bool { _, ok := o.(*object.Float); return ok },
	"number": func(o object.Object) bool {
		switch o.(type) {
		case *object.Integer, *object.Float:
			return true
		}
		return false
	},
	"string":   func(o object.Object) bool { _, ok := o.(*object.String); return ok },
	"str":      func(o object.Object) bool { _, ok := o.(*object.String); return ok },
	"bool":     func(o object.Object) bool { _, ok := o.(*object.Boolean); return ok },
	"boolean":  func(o object.Object) bool { _, ok := o.(*object.Boolean); return ok },
	"null":     func(o object.Object) bool { _, ok := o.(*object.Null); return ok },
	"array":    func(o object.Object) bool { _, ok := o.(*object.Array); return ok },
	"list":     func(o object.Object) bool { _, ok := o.(*object.Array); return ok },
	"hash":     func(o object.Object) bool { _, ok := o.(*object.Hash); return ok },
	"map":      func(o object.Object) bool { _, ok := o.(*object.Hash); return ok },
	"dict":     func(o object.Object) bool { _, ok := o.(*object.Hash); return ok },
	"tuple":    func(o object.Object) bool { _, ok := o.(*object.Tuple); return ok },
	"function": func(o object.Object) bool { return isCallable(o) },
	"fn":       func(o object.Object) bool { return isCallable(o) },
	"callable": func(o object.Object) bool { return isCallable(o) },
	"record":   func(o object.Object) bool { _, ok := o.(*object.Record); return ok },
}

// checkAnnotation tests obj against the runtime contract ann.
//
// Returns:
//   - ok=true when the value satisfies the contract.
//   - ok=false with expected=a human-readable description ("int",
//     "int | string", "Shape") when it does not.
//   - unknown=a complete problem description when ann names something that
//     is neither a builtin type nor a class/interface/record visible in
//     env ("unknown type 'Nope'", or "not a type: 'x' (got int)" when the
//     name is bound to a non-type value). Callers turn this into a loud
//     runtime error rather than a confusing mismatch.
//
// A nil ann (no annotation written) always matches. "any" matches everything.
func checkAnnotation(ann *ast.TypeAnnotation, obj object.Object, env *object.Environment) (ok bool, expected string, unknown string) {
	if ann == nil {
		return true, "any", ""
	}
	if len(ann.Union) > 0 {
		for _, member := range ann.Union {
			mOk, _, mUnknown := checkAnnotation(member, obj, env)
			if mUnknown != "" {
				return false, "", mUnknown
			}
			if mOk {
				return true, ann.String(), ""
			}
		}
		return false, ann.String(), ""
	}
	name := ann.Name
	if strings.EqualFold(name, "any") {
		return true, "any", ""
	}
	if pred, found := builtinAnnotationChecks[strings.ToLower(name)]; found {
		return pred(obj), name, ""
	}
	// Class / interface / record names resolve lexically in env.
	if env != nil {
		if target, found := env.Get(name); found {
			switch t := target.(type) {
			case *object.Interface:
				if len(checkImplements(obj, t)) == 0 {
					return true, name, ""
				}
				return false, name, ""
			case *object.Class:
				if inst, isInst := obj.(*object.Instance); isInst && instanceOf(inst, t) {
					return true, name, ""
				}
				return false, name, ""
			case *object.RecordDef:
				if rec, isRec := obj.(*object.Record); isRec && rec.Def == t {
					return true, name, ""
				}
				return false, name, ""
			default:
				// Bound, but not a type at all.
				return false, "", "not a type: '" + name + "' (got " + friendlyTypeName(target) + ")"
			}
		}
	}
	return false, "", "unknown type '" + name + "'"
}

// instanceOf reports whether inst is an instance of class or one of its
// subclasses (walks the extends chain).
func instanceOf(inst *object.Instance, class *object.Class) bool {
	for c := inst.Class; c != nil; c = c.Parent {
		if c == class {
			return true
		}
	}
	return false
}

// checkValueAnnotation enforces one annotation, returning the canonical
// runtime type error (or nil when the value satisfies the contract). `what`
// describes the checked position, e.g. "parameter 'x'".
func checkValueAnnotation(ann *ast.TypeAnnotation, val object.Object, env *object.Environment, what string) object.Object {
	ok, expected, unknown := checkAnnotation(ann, val, env)
	if unknown != "" {
		return newError("%s in annotation for %s", unknown, what)
	}
	if !ok {
		return newError("type error: %s expects %s, got %s", what, expected, friendlyTypeName(val))
	}
	return nil
}

// fnDisplayName is the function name when known ("" for anonymous).
func fnDisplayName(fn *object.Function) string {
	return fn.Name
}

// paramWhat describes a parameter position for error messages.
func paramWhat(fn *object.Function, paramName string) string {
	if n := fnDisplayName(fn); n != "" {
		return fmt.Sprintf("parameter '%s' of function '%s'", paramName, n)
	}
	return fmt.Sprintf("parameter '%s'", paramName)
}

// checkReturnType enforces a function's return annotation. Errors and
// generator functions are never checked (a thrown error is not a return
// value, and generator results flow through yield).
func checkReturnType(fn *object.Function, val object.Object) object.Object {
	if fn.ReturnType == nil || isError(val) {
		return nil
	}
	what := "return value"
	if n := fnDisplayName(fn); n != "" {
		what = fmt.Sprintf("return value of function '%s'", n)
	}
	return checkValueAnnotation(fn.ReturnType, val, fn.Env, what)
}

// ---- structural interfaces ----

// lookupMember finds a named member on the shapes interfaces support:
// class instances (fields, then methods incl. inherited), hashes
// (string keys — the maps-of-functions idiom), and records (fields).
func lookupMember(obj object.Object, name string) (object.Object, bool) {
	switch o := obj.(type) {
	case *object.Instance:
		return o.Get(name)
	case *object.Hash:
		key := &object.String{Value: name}
		if pair, ok := o.Pairs[key.HashKey()]; ok {
			return pair.Value, true
		}
		return nil, false
	case *object.Record:
		if idx, ok := o.Def.FieldIndex(name); ok {
			return o.Values[idx], true
		}
		return nil, false
	}
	return nil, false
}

// checkImplements structurally tests obj against iface. Returns nil when obj
// satisfies the interface, otherwise a list of human-readable problems.
//
// What IS verified, honestly:
//   - every required member is present,
//   - every required member is callable (a function or builtin),
//   - when the member is a user function with knowable arity, it can be
//     invoked with exactly the number of arguments the interface declares
//     (required params <= declared arity <= total params).
//
// What is NOT verified: parameter/return type compatibility (signature
// variance) — an annotated implementation enforces its own contracts at
// call time; an unannotated one is dynamically typed by design.
func checkImplements(obj object.Object, iface *object.Interface) []string {
	var problems []string
	for _, mname := range iface.Order {
		im := iface.Methods[mname]
		member, found := lookupMember(obj, mname)
		if !found {
			problems = append(problems, fmt.Sprintf("missing method '%s'", mname))
			continue
		}
		if !isCallable(member) {
			problems = append(problems, fmt.Sprintf("'%s' is not callable (got %s)", mname, friendlyTypeName(member)))
			continue
		}
		if fn, ok := member.(*object.Function); ok {
			required := 0
			for i := range fn.Parameters {
				if i >= len(fn.Defaults) || fn.Defaults[i] == nil {
					required++
				}
			}
			if !(required <= im.Arity && im.Arity <= len(fn.Parameters)) {
				problems = append(problems, fmt.Sprintf("method '%s' cannot be called with %d argument(s)", mname, im.Arity))
			}
		}
		// Builtins have no knowable arity — skipped honestly, not guessed.
	}
	return problems
}

// describeValue is a short, safe description for implements() errors.
func describeValue(obj object.Object) string {
	if inst, ok := obj.(*object.Instance); ok {
		return "instance of " + inst.Class.Name
	}
	if rec, ok := obj.(*object.Record); ok {
		return "record " + rec.Def.Name
	}
	return friendlyTypeName(obj)
}

// evalInterfaceStatement evaluates `interface Name { ... }`, binding the
// interface object in the current environment.
func evalInterfaceStatement(node *ast.InterfaceDecl, env *object.Environment) object.Object {
	methods := make(map[string]*object.InterfaceMethod, len(node.Methods))
	order := make([]string, 0, len(node.Methods))
	for _, m := range node.Methods {
		if _, dup := methods[m.Name.Value]; dup {
			return newError("duplicate method '%s' in interface %s", m.Name.Value, node.Name.Value)
		}
		methods[m.Name.Value] = &object.InterfaceMethod{
			Name:       m.Name.Value,
			Arity:      len(m.ParamNames),
			ParamTypes: m.ParamTypes,
			ReturnType: m.ReturnType,
		}
		order = append(order, m.Name.Value)
	}
	iface := &object.Interface{Name: node.Name.Value, Methods: methods, Order: order}
	env.Set(node.Name.Value, iface)
	return iface
}

// registerWave5Builtins adds the wave-5 type builtins. Called once from
// initBuiltins(); is_null already exists, so only the gaps are filled.
func registerWave5Builtins() {
	wave5 := map[string]*object.Builtin{
		"implements": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return newError("implements: want 2 arguments (value, interface)")
				}
				iface, ok := args[1].(*object.Interface)
				if !ok {
					return newError("implements: second argument must be an interface, got %s", friendlyTypeName(args[1]))
				}
				return nativeBoolToBooleanObject(len(checkImplements(args[0], iface)) == 0)
			},
		},
		"assert_implements": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return newError("assert_implements: want 2 arguments (value, interface)")
				}
				iface, ok := args[1].(*object.Interface)
				if !ok {
					return newError("assert_implements: second argument must be an interface, got %s", friendlyTypeName(args[1]))
				}
				if problems := checkImplements(args[0], iface); len(problems) > 0 {
					return newError("assert_implements: %s does not implement %s: %s",
						describeValue(args[0]), iface.Name, strings.Join(problems, "; "))
				}
				return NULL
			},
		},
		"is_int": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_int: want 1 argument")
				}
				_, ok := args[0].(*object.Integer)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_float": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_float: want 1 argument")
				}
				_, ok := args[0].(*object.Float)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_number": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_number: want 1 argument")
				}
				switch args[0].(type) {
				case *object.Integer, *object.Float:
					return nativeBoolToBooleanObject(true)
				}
				return nativeBoolToBooleanObject(false)
			},
		},
		"is_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_string: want 1 argument")
				}
				_, ok := args[0].(*object.String)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_bool": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_bool: want 1 argument")
				}
				_, ok := args[0].(*object.Boolean)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_array": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_array: want 1 argument")
				}
				_, ok := args[0].(*object.Array)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_hash": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_hash: want 1 argument")
				}
				_, ok := args[0].(*object.Hash)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_tuple": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_tuple: want 1 argument")
				}
				_, ok := args[0].(*object.Tuple)
				return nativeBoolToBooleanObject(ok)
			},
		},
		"is_function": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("is_function: want 1 argument")
				}
				// True for user functions and builtins alike: both are
				// callable. Documented as "can this be called".
				return nativeBoolToBooleanObject(isCallable(args[0]))
			},
		},
		"type_of": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return newError("type_of: want 1 argument")
				}
				// Lowercase companion to type()/typeof() (which return the
				// uppercase internal names like "INTEGER").
				return &object.String{Value: friendlyTypeName(args[0])}
			},
		},
	}
	for name, b := range wave5 {
		builtins[name] = b
	}
}
