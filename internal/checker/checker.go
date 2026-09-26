// Package checker implements `nvs check`: a static, gradual type checker
// for NvS.
//
// NvS has annotations (`let x: int`), unions (`int | string`), and
// interfaces, but until now nothing verified them without running the
// program. This package checks them over the parser AST without executing
// anything.
//
// Gradual typing rules (mirroring the runtime contracts in
// internal/eval/wave5_types.go):
//   - Unannotated bindings and unknown expressions have type `any`.
//   - `any` is assignable to and from every type — no error is ever
//     reported on either side of an `any`.
//   - `int` and `float` are assignable to `number`.
//   - A union member check: a value of type `int | string` assigned to
//     `string` is an error unless the union is exactly covered.
//   - Named types resolve to declared classes/interfaces/records/enums;
//     anything else is an "unknown type" error (the runtime raises the
//     same error when the annotation executes).
//   - Interface satisfaction is structural: a class instance or hash
//     literal used where an interface is expected must provide every
//     interface method with a compatible arity.
//   - Function calls are arity-checked when the callee's signature is
//     known (user functions, interface/class methods, a few builtins);
//     annotated parameter types are checked against argument types.
//
// What is intentionally NOT checked (stays dynamic, like the runtime):
//   - member access on `any`, index expressions, imports, decorators'
//     runtime effects, match-arm exhaustiveness, exception flow.
//   - Bodies of functions are still walked so nested annotated code is
//     verified; unannotated code never produces errors by itself.
//
// The checker is advisory tooling. It never changes runtime behavior:
// `nvs run` does not consult it.
package checker

import (
	"fmt"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ---------------------------------------------------------------------------
// Static types
// ---------------------------------------------------------------------------

// Type is a static type. Name is one of the builtin names ("any", "int",
// "float", "number", "string", "bool", "null", "array", "hash", "tuple",
// "function") or a declared type name (class, interface, record, enum).
// A non-empty Union means the union of the members. Sig carries a known
// function signature; HashFuncs records function-valued keys of a hash
// literal (name -> arity) for structural interface checks.
type Type struct {
	Name      string
	Union     []Type
	Sig       *FuncSig
	HashFuncs map[string]FuncArity
	ClassName string // for Name=="instance": the class of `new C()`
}

// FuncArity is the static shape of a callable: total parameters and how
// many are required (no default). Mirrors the runtime implements() rule:
// callable with n args iff required <= n <= total.
type FuncArity struct {
	Total    int
	Required int
}

// FuncSig is a statically known function signature.
type FuncSig struct {
	Params []ParamSig
	Return *Type // nil = any
}

// ParamSig is one parameter: a nil Type means unannotated (any).
type ParamSig struct {
	Name     string
	Type     *Type
	Required bool
}

func anyType() Type            { return Type{Name: "any"} }
func simpleType(n string) Type { return Type{Name: n} }

func (t Type) String() string {
	if len(t.Union) > 0 {
		parts := make([]string, len(t.Union))
		for i, u := range t.Union {
			parts[i] = u.String()
		}
		return strings.Join(parts, " | ")
	}
	if t.Name == "instance" && t.ClassName != "" {
		return t.ClassName
	}
	return t.Name
}

func isAny(t Type) bool { return t.Name == "any" && len(t.Union) == 0 }

// builtinTypes mirrors internal/eval's builtinAnnotationChecks (lowercased).
var builtinTypes = map[string]bool{
	"int": true, "integer": true, "float": true, "number": true,
	"string": true, "str": true, "bool": true, "boolean": true,
	"null": true, "array": true, "list": true, "hash": true,
	"map": true, "dict": true, "tuple": true, "function": true,
	"fn": true, "callable": true, "record": true, "any": true,
}

// canonicalName maps annotation aliases to the canonical static name,
// mirroring the runtime predicates.
func canonicalName(name string) string {
	switch strings.ToLower(name) {
	case "integer":
		return "int"
	case "str":
		return "string"
	case "boolean":
		return "bool"
	case "list":
		return "array"
	case "map", "dict":
		return "hash"
	case "fn", "callable":
		return "function"
	}
	return strings.ToLower(name)
}

// annotationType converts a parsed *ast.TypeAnnotation into a static Type.
// Unknown names are reported via the returned error string ("" = ok);
// the caller turns that into a CheckError at the annotation's line.
func annotationType(ann *ast.TypeAnnotation, c *Checker) (Type, string) {
	if ann == nil {
		return anyType(), ""
	}
	if len(ann.Union) > 0 {
		var members []Type
		for _, m := range ann.Union {
			mt, unk := annotationType(m, c)
			if unk != "" {
				return anyType(), unk
			}
			members = append(members, mt)
		}
		return Type{Name: "union", Union: members}, ""
	}
	name := ann.Name
	if strings.EqualFold(name, "any") {
		return anyType(), ""
	}
	canon := canonicalName(name)
	if builtinTypes[canon] {
		return simpleType(canon), ""
	}
	// Declared type names (class/interface/record/enum) are case-sensitive,
	// exactly like the runtime's lexical lookup.
	if c.isDeclaredType(name) {
		return simpleType(name), ""
	}
	return anyType(), fmt.Sprintf("unknown type '%s'", name)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// CheckError is one static type error.
type CheckError struct {
	File string
	Line int
	Msg  string
}

func (e CheckError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Msg)
}

// ---------------------------------------------------------------------------
// Checker
// ---------------------------------------------------------------------------

// varInfo records a binding: its static type and, when the binding was
// declared with an annotation, the declared (checked) type.
type varInfo struct {
	typ      Type
	declared *Type // non-nil when an annotation was written
}

// classInfo is a declared class: its methods and parent.
type classInfo struct {
	methods map[string]*FuncSig
	parent  string
}

// ifaceInfo is a declared interface: its methods in order.
type ifaceInfo struct {
	methods map[string]*ifaceMethod
	order   []string
}

type ifaceMethod struct {
	name       string
	arity      int
	paramTypes []*Type // nil entry = any
	ret        *Type
}

// Checker holds the state for checking one file.
type Checker struct {
	file    string
	errs    []CheckError
	scopes  []map[string]varInfo
	classes map[string]*classInfo
	ifaces  map[string]*ifaceInfo
	records map[string]bool
	enums   map[string]map[string]bool
	// fnStack tracks enclosing functions' declared return types (nil = any).
	fnStack []*Type
}

func newChecker(file string) *Checker {
	return &Checker{
		file:    file,
		scopes:  []map[string]varInfo{{}},
		classes: map[string]*classInfo{},
		ifaces:  map[string]*ifaceInfo{},
		records: map[string]bool{},
		enums:   map[string]map[string]bool{},
	}
}

func (c *Checker) errorf(line int, format string, args ...interface{}) {
	c.errs = append(c.errs, CheckError{File: c.file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (c *Checker) pushScope() { c.scopes = append(c.scopes, map[string]varInfo{}) }
func (c *Checker) popScope()  { c.scopes = c.scopes[:len(c.scopes)-1] }

func (c *Checker) define(name string, v varInfo) { c.scopes[len(c.scopes)-1][name] = v }

func (c *Checker) lookup(name string) (varInfo, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if v, ok := c.scopes[i][name]; ok {
			return v, true
		}
	}
	return varInfo{}, false
}

func (c *Checker) isDeclaredType(name string) bool {
	if _, ok := c.classes[name]; ok {
		return true
	}
	if _, ok := c.ifaces[name]; ok {
		return true
	}
	if c.records[name] {
		return true
	}
	if _, ok := c.enums[name]; ok {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Subtyping (gradual)
// ---------------------------------------------------------------------------

// assignableTo reports whether a value of type src may flow into a position
// declared as dst. `any` on either side always succeeds (gradual typing).
func (c *Checker) assignableTo(src, dst Type) bool {
	if isAny(src) || isAny(dst) {
		return true
	}
	// Union destination: every source member must match some dest member.
	// (When src is not a union this reduces to "matches a member".)
	if len(dst.Union) > 0 {
		srcMembers := src.Union
		if len(srcMembers) == 0 {
			srcMembers = []Type{src}
		}
		for _, sm := range srcMembers {
			matched := false
			for _, dm := range dst.Union {
				if c.assignableTo(sm, dm) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}
	// Union source, simple destination: every member must be assignable
	// (sound for declared positions; inference unions stay precise).
	if len(src.Union) > 0 {
		for _, m := range src.Union {
			if !c.assignableTo(m, dst) {
				return false
			}
		}
		return true
	}
	if src.Name == dst.Name {
		// Named class/interface/record/enum: same declaration.
		// Instances: allow subclass -> parent via the extends chain.
		if src.Name == "instance" {
			return c.instanceOf(src.ClassName, dst.ClassName)
		}
		return true
	}
	// Numeric tower.
	if dst.Name == "number" && (src.Name == "int" || src.Name == "float") {
		return true
	}
	// Interface destination: structural check.
	if _, ok := c.ifaces[dst.Name]; ok {
		return c.satisfiesIface(src, dst.Name)
	}
	// Class destination: only instances of it (or subclasses).
	if _, ok := c.classes[dst.Name]; ok {
		return src.Name == "instance" && c.instanceOf(src.ClassName, dst.Name)
	}
	return false
}

// instanceOf walks the extends chain: is child the same class as, or a
// subclass of, parent? Unknown classes are treated conservatively (false),
// which only matters when both sides are fully known.
func (c *Checker) instanceOf(child, parent string) bool {
	for cur := child; cur != ""; {
		if cur == parent {
			return true
		}
		ci, ok := c.classes[cur]
		if !ok {
			return false
		}
		cur = ci.parent
	}
	return false
}

// satisfiesIface reports whether a value of static type src structurally
// satisfies the interface: every method present with compatible arity.
// Hash literals carry their function-valued keys; class instances consult
// the class method table (including inherited methods).
func (c *Checker) satisfiesIface(src Type, ifaceName string) bool {
	iface, ok := c.ifaces[ifaceName]
	if !ok {
		return false
	}
	switch src.Name {
	case "hash":
		for _, mname := range iface.order {
			im := iface.methods[mname]
			arity, found := src.HashFuncs[mname]
			if !found {
				return false
			}
			// Runtime rule: required_impl <= arity_iface <= total_impl.
			if !(arity.Required <= im.arity && im.arity <= arity.Total) {
				return false
			}
		}
		return true
	case "instance":
		ci, ok := c.classes[src.ClassName]
		if !ok {
			return false
		}
		for _, mname := range iface.order {
			im := iface.methods[mname]
			sig := c.lookupMethod(ci, mname)
			if sig == nil {
				return false
			}
			if !c.arityCompatible(sig, im.arity) {
				return false
			}
		}
		return true
	}
	return false
}

// arityCompatible mirrors the runtime implements() rule: a function with
// required..total parameters can be called with n arguments.
func (c *Checker) arityCompatible(sig *FuncSig, n int) bool {
	required := 0
	for _, p := range sig.Params {
		if p.Required {
			required++
		}
	}
	return required <= n && n <= len(sig.Params)
}

// lookupMethod finds a method on a class or its parents.
func (c *Checker) lookupMethod(ci *classInfo, name string) *FuncSig {
	for cur := ci; cur != nil; {
		if sig, ok := cur.methods[name]; ok {
			return sig
		}
		if cur.parent == "" {
			break
		}
		p, ok := c.classes[cur.parent]
		if !ok {
			break
		}
		cur = p
	}
	return nil
}

// ---------------------------------------------------------------------------
// Declaration collection (pass 1)
// ---------------------------------------------------------------------------

// collectDecls registers top-level declarations so forward references work.
// It runs in two loops: first all type NAMES (class/interface/record/enum),
// then signatures and variable bindings, so annotations may reference types
// declared later in the file.
func (c *Checker) collectDecls(prog *ast.Program) {
	for _, s := range prog.Statements {
		switch n := s.(type) {
		case *ast.ClassStatement:
			if _, ok := c.classes[n.Name.Value]; !ok {
				c.classes[n.Name.Value] = &classInfo{methods: map[string]*FuncSig{}}
			}
		case *ast.InterfaceDecl:
			if _, ok := c.ifaces[n.Name.Value]; !ok {
				c.ifaces[n.Name.Value] = &ifaceInfo{methods: map[string]*ifaceMethod{}}
			}
		case *ast.RecordStatement:
			c.records[n.Name.Value] = true
		case *ast.EnumStatement:
			members := map[string]bool{}
			for _, m := range n.Members {
				members[m.Value] = true
			}
			c.enums[n.Name.Value] = members
		}
	}
	for _, s := range prog.Statements {
		switch n := s.(type) {
		case *ast.LetStatement:
			if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
				c.define(n.Name.Value, varInfo{typ: Type{Name: "function", Sig: c.funcSigOf(fl)}})
			} else {
				c.define(n.Name.Value, varInfo{typ: c.infer(n.Value)})
			}
		case *ast.TypedLetStatement:
			// Unknown names resolve to any here; reported once in pass 2.
			t, unk := annotationType(n.Type, c)
			if unk == "" {
				c.define(n.Name.Value, varInfo{typ: t, declared: &t})
			} else {
				c.define(n.Name.Value, varInfo{typ: anyType()})
			}
		case *ast.ConstStatement:
			c.define(n.Name.Value, varInfo{typ: c.infer(n.Value)})
		case *ast.ClassStatement:
			ci := c.classes[n.Name.Value]
			if n.Parent != nil {
				ci.parent = n.Parent.Value
			}
			for _, m := range n.Methods {
				ci.methods[m.Name.Value] = c.methodSigOf(m)
			}
			c.define(n.Name.Value, varInfo{typ: simpleType(n.Name.Value)})
		case *ast.InterfaceDecl:
			ii := c.ifaces[n.Name.Value]
			for _, m := range n.Methods {
				im := &ifaceMethod{name: m.Name.Value, arity: len(m.ParamNames)}
				for _, pt := range m.ParamTypes {
					t, _ := annotationType(pt, c)
					p := t
					im.paramTypes = append(im.paramTypes, &p)
				}
				if m.ReturnType != nil {
					if t, unk := annotationType(m.ReturnType, c); unk == "" {
						im.ret = &t
					}
				}
				ii.methods[m.Name.Value] = im
				ii.order = append(ii.order, m.Name.Value)
			}
			c.define(n.Name.Value, varInfo{typ: simpleType(n.Name.Value)})
		case *ast.RecordStatement:
			c.define(n.Name.Value, varInfo{typ: simpleType(n.Name.Value)})
		case *ast.EnumStatement:
			c.define(n.Name.Value, varInfo{typ: simpleType(n.Name.Value)})
		case *ast.DecoratorStatement:
			// @dec \n fn name(): unwrap like LetStatement.
			if ls, ok := n.Function.(*ast.LetStatement); ok {
				if fl, ok := ls.Value.(*ast.FunctionLiteral); ok {
					c.define(ls.Name.Value, varInfo{typ: Type{Name: "function", Sig: c.funcSigOf(fl)}})
				} else {
					c.define(ls.Name.Value, varInfo{typ: c.infer(ls.Value)})
				}
			}
		}
	}
}

// funcSigOf builds the static signature of a function literal. It is pure:
// unknown annotation names resolve to any here; unknown names are reported
// exactly once by the pass-2 validation in checkFunctionBody.
func (c *Checker) funcSigOf(fl *ast.FunctionLiteral) *FuncSig {
	sig := &FuncSig{}
	for i, p := range fl.Parameters {
		ps := ParamSig{Name: p.Value, Required: true}
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			ps.Required = false
		}
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != nil {
			t, unk := annotationType(fl.ParamTypes[i], c)
			if unk == "" {
				tt := t
				ps.Type = &tt
			}
			// Unknown: left as any; reported by checkFunctionBody.
		}
		sig.Params = append(sig.Params, ps)
	}
	if fl.ReturnType != nil {
		t, unk := annotationType(fl.ReturnType, c)
		if unk == "" {
			tt := t
			sig.Return = &tt
		}
	}
	return sig
}

func (c *Checker) methodSigOf(m *ast.ClassMethod) *FuncSig {
	fl := &ast.FunctionLiteral{
		Token:      m.Token,
		Parameters: m.Parameters,
		ParamTypes: m.ParamTypes,
		ReturnType: m.ReturnType,
	}
	return c.funcSigOf(fl)
}

// ---------------------------------------------------------------------------
// Statement checking (pass 2)
// ---------------------------------------------------------------------------

func (c *Checker) checkProgram(prog *ast.Program) {
	for _, s := range prog.Statements {
		c.checkStatement(s)
	}
}

func (c *Checker) checkStatement(s ast.Statement) {
	switch n := s.(type) {
	case *ast.LetStatement:
		vt := c.infer(n.Value)
		if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
			c.checkFunctionBody(n.Name.Value, fl)
			c.define(n.Name.Value, varInfo{typ: Type{Name: "function", Sig: c.funcSigOf(fl)}})
		} else {
			c.checkExpression(n.Value)
			c.define(n.Name.Value, varInfo{typ: vt})
		}
	case *ast.TypedLetStatement:
		t, unk := annotationType(n.Type, c)
		if unk != "" {
			// Reported here, exactly once: pass 1 no longer reports, so
			// forward-referenced type names resolve before this runs.
			c.errorf(lineOf(n), "%s", unk)
			c.checkExpression(n.Value)
			c.define(n.Name.Value, varInfo{typ: anyType()})
			return
		}
		vt := c.infer(n.Value)
		c.checkExpression(n.Value)
		if !c.assignableTo(vt, t) {
			c.errorf(lineOf(n), "cannot assign %s to variable '%s' of type %s", vt, n.Name.Value, t)
		}
		c.define(n.Name.Value, varInfo{typ: t, declared: &t})
	case *ast.ConstStatement:
		c.checkExpression(n.Value)
		c.define(n.Name.Value, varInfo{typ: c.infer(n.Value)})
	case *ast.ReturnStatement:
		if len(c.fnStack) == 0 {
			return // `return` at top level: runtime handles it; not a type error.
		}
		want := c.fnStack[len(c.fnStack)-1]
		if want == nil {
			return // unannotated return: anything goes.
		}
		var got Type
		if n.ReturnValue != nil {
			got = c.infer(n.ReturnValue)
			c.checkExpression(n.ReturnValue)
		} else {
			got = simpleType("null")
		}
		if !c.assignableTo(got, *want) {
			c.errorf(lineOf(n), "return type mismatch: got %s, want %s", got, *want)
		}
	case *ast.ExpressionStatement:
		c.checkExpression(n.Expression)
	case *ast.PrintStatement:
		c.checkExpression(n.Value)
	case *ast.BlockStatement:
		c.pushScope()
		for _, st := range n.Statements {
			c.checkStatement(st)
		}
		c.popScope()
	case *ast.WhileStatement:
		c.checkExpression(n.Condition)
		c.checkBlock(n.Body)
	case *ast.ForStatement:
		c.pushScope()
		if n.Init != nil {
			c.checkStatement(n.Init)
		}
		if n.Condition != nil {
			c.checkExpression(n.Condition)
		}
		if n.Post != nil {
			c.checkExpression(n.Post)
		}
		c.checkBlock(n.Body)
		if n.OrElse != nil {
			c.checkBlock(n.OrElse)
		}
		c.popScope()
	case *ast.ForInStatement:
		c.checkExpression(n.Iterable)
		c.pushScope()
		c.define(n.Name.Value, varInfo{typ: anyType()})
		c.checkBlock(n.Body)
		if n.OrElse != nil {
			c.checkBlock(n.OrElse)
		}
		c.popScope()
	case *ast.TryStatement:
		c.checkBlock(n.Body)
		if n.Catch != nil {
			c.checkBlock(n.Catch)
		}
		if n.Finally != nil {
			c.checkBlock(n.Finally)
		}
	case *ast.ThrowStatement:
		if n.Value != nil {
			c.checkExpression(n.Value)
		}
	case *ast.ClassStatement, *ast.InterfaceDecl, *ast.RecordStatement, *ast.EnumStatement:
		// Declarations: check method bodies for classes; validate
		// interface method annotations (unknown names reported once here).
		if cs, ok := s.(*ast.ClassStatement); ok {
			for _, m := range cs.Methods {
				fl := &ast.FunctionLiteral{Token: m.Token, Parameters: m.Parameters,
					ParamTypes: m.ParamTypes, ReturnType: m.ReturnType, Body: m.Body}
				c.checkFunctionBody(m.Name.Value, fl)
			}
		}
		if id, ok := s.(*ast.InterfaceDecl); ok {
			for _, m := range id.Methods {
				for _, pt := range m.ParamTypes {
					if pt != nil {
						if _, unk := annotationType(pt, c); unk != "" {
							c.errorf(m.Token.Line, "%s in parameter of '%s'", unk, m.Name.Value)
						}
					}
				}
				if m.ReturnType != nil {
					if _, unk := annotationType(m.ReturnType, c); unk != "" {
						c.errorf(m.Token.Line, "%s in return type of '%s'", unk, m.Name.Value)
					}
				}
			}
		}
	case *ast.ImportStatement:
		// Dynamic; nothing to check.
	case *ast.BreakStatement, *ast.ContinueStatement, *ast.YieldStatement:
		if ys, ok := s.(*ast.YieldStatement); ok && ys.Value != nil {
			c.checkExpression(ys.Value)
		}
	case *ast.DestructureLetStatement:
		// Bindings get type any; check the source expression.
		if n.Value != nil {
			c.checkExpression(n.Value)
		}
	case *ast.DecoratorStatement:
		c.checkStatement(n.Function)
	case *ast.DeferStatement:
		c.checkExpression(n.Call)
	default:
		// Other statements: nothing type-specific.
	}
}

func (c *Checker) checkBlock(b *ast.BlockStatement) {
	if b == nil {
		return
	}
	c.pushScope()
	for _, s := range b.Statements {
		c.checkStatement(s)
	}
	c.popScope()
}

// checkFunctionBody checks a function literal's body with its parameters in
// scope, enforcing the declared return type. Unknown annotation names are
// reported here, exactly once per function literal.
func (c *Checker) checkFunctionBody(name string, fl *ast.FunctionLiteral) {
	sig := c.funcSigOf(fl)
	for i, p := range fl.Parameters {
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != nil {
			if _, unk := annotationType(fl.ParamTypes[i], c); unk != "" {
				c.errorf(p.Token.Line, "%s in parameter '%s'", unk, p.Value)
			}
		}
	}
	if fl.ReturnType != nil {
		if _, unk := annotationType(fl.ReturnType, c); unk != "" {
			c.errorf(lineOf(fl), "%s in return type", unk)
		}
	}
	c.pushScope()
	for _, p := range sig.Params {
		pt := anyType()
		if p.Type != nil {
			pt = *p.Type
		}
		c.define(p.Name, varInfo{typ: pt})
	}
	c.fnStack = append(c.fnStack, sig.Return)
	if fl.Body != nil {
		for _, s := range fl.Body.Statements {
			c.checkStatement(s)
		}
	}
	c.fnStack = c.fnStack[:len(c.fnStack)-1]
	c.popScope()
	_ = name
}

// ---------------------------------------------------------------------------
// Expression checking + inference
// ---------------------------------------------------------------------------

// checkExpression walks an expression for nested errors (calls, literals
// with subexpressions, etc.).
func (c *Checker) checkExpression(e ast.Expression) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *ast.CallExpression:
		c.checkCall(n)
	case *ast.InfixExpression:
		c.checkExpression(n.Left)
		c.checkExpression(n.Right)
	case *ast.PrefixExpression:
		c.checkExpression(n.Right)
	case *ast.IfExpression:
		c.checkExpression(n.Condition)
		c.checkBlock(n.Consequence)
		if n.Alternative != nil {
			c.checkBlock(n.Alternative)
		}
	case *ast.FunctionLiteral:
		// Anonymous fn: check body (params are any unless annotated).
		c.checkFunctionBody("", n)
	case *ast.ArrayLiteral:
		for _, el := range n.Elements {
			c.checkExpression(el)
		}
	case *ast.HashLiteral:
		for k, v := range n.Pairs {
			c.checkExpression(k)
			c.checkExpression(v)
		}
		for _, s := range n.Spreads {
			c.checkExpression(s)
		}
	case *ast.TupleLiteral:
		for _, el := range n.Elements {
			c.checkExpression(el)
		}
	case *ast.IndexExpression:
		c.checkExpression(n.Left)
		c.checkExpression(n.Index)
	case *ast.IndexAssignExpression:
		c.checkExpression(n.Left)
		c.checkExpression(n.Value)
	case *ast.AssignExpression:
		c.checkExpression(n.Value)
		c.checkAssign(n.Name.Value, n.Value, lineOf(n))
	case *ast.MemberExpression:
		c.checkExpression(n.Object)
	case *ast.MemberAssignExpression:
		c.checkExpression(n.Object)
		c.checkExpression(n.Value)
	case *ast.NewExpression:
		for _, a := range n.Arguments {
			c.checkExpression(a)
		}
	case *ast.TernaryExpression:
		c.checkExpression(n.Condition)
		c.checkExpression(n.Consequence)
		c.checkExpression(n.Alternative)
	case *ast.MatchExpression:
		c.checkExpression(n.Value)
		for _, a := range n.Arms {
			if a.Guard != nil {
				c.checkExpression(a.Guard)
			}
			c.checkBlock(a.Body)
		}
		if n.Default != nil {
			c.checkBlock(n.Default)
		}
	case *ast.InterpolatedString:
		for _, p := range n.Parts {
			c.checkExpression(p)
		}
	case *ast.SpreadExpression:
		c.checkExpression(n.Value)
	case *ast.OptionalChainExpression:
		c.checkExpression(n.Base)
		for _, l := range n.Links {
			if l.Index != nil {
				c.checkExpression(l.Index)
			}
			for _, a := range l.Arguments {
				c.checkExpression(a)
			}
		}
	case *ast.RangeExpression:
		c.checkExpression(n.Start)
		c.checkExpression(n.End)
	default:
		// Literals and identifiers: nothing to descend into.
	}
}

// checkAssign enforces a declared annotation on re-assignment.
func (c *Checker) checkAssign(name string, value ast.Expression, line int) {
	vt := c.infer(value)
	if v, ok := c.lookup(name); ok && v.declared != nil {
		if !c.assignableTo(vt, *v.declared) {
			c.errorf(line, "cannot assign %s to variable '%s' of type %s", vt, name, *v.declared)
		}
	}
}

// checkCall arity-checks and arg-checks calls with a known callee.
func (c *Checker) checkCall(n *ast.CallExpression) {
	for _, a := range n.Arguments {
		c.checkExpression(a)
	}
	// Skip strict checks when named arguments are used (dynamic dispatch).
	for _, a := range n.Arguments {
		if _, ok := a.(*ast.NamedArgument); ok {
			return
		}
	}
	sig := c.calleeSig(n.Function)
	if sig == nil {
		return // unknown callee: gradual, no checks.
	}
	required := 0
	for _, p := range sig.Params {
		if p.Required {
			required++
		}
	}
	if len(n.Arguments) < required || len(n.Arguments) > len(sig.Params) {
		c.errorf(lineOf(n), "wrong number of arguments: got %d, want %d..%d",
			len(n.Arguments), required, len(sig.Params))
		return // don't cascade arg-type errors on arity mismatch.
	}
	for i, a := range n.Arguments {
		if i >= len(sig.Params) || sig.Params[i].Type == nil {
			continue
		}
		at := c.infer(a)
		if !c.assignableTo(at, *sig.Params[i].Type) {
			c.errorf(lineOf(a), "argument %d: got %s, want %s", i+1, at, *sig.Params[i].Type)
		}
	}
}

// calleeSig resolves a call target to a static signature when possible.
func (c *Checker) calleeSig(fn ast.Expression) *FuncSig {
	switch f := fn.(type) {
	case *ast.Identifier:
		if v, ok := c.lookup(f.Value); ok && v.typ.Name == "function" && v.typ.Sig != nil {
			return v.typ.Sig
		}
		if bs, ok := builtinSigs[f.Value]; ok {
			return bs
		}
	case *ast.MemberExpression:
		// obj.method(...) where obj has a known class: method signature.
		ot := c.infer(f.Object)
		if ot.Name == "instance" {
			if ci, ok := c.classes[ot.ClassName]; ok {
				if sig := c.lookupMethod(ci, f.Property.Value); sig != nil {
					return sig
				}
			}
		}
		// hash.method(...) where the hash literal had that function key:
		// known callable, but the signature isn't fully known — return nil
		// so no arity/type checks fire (gradual typing).
		if ot.Name == "hash" && ot.HashFuncs != nil {
			if _, ok := ot.HashFuncs[f.Property.Value]; ok {
				return nil
			}
		}
	}
	return nil
}

// builtinSigs: static signatures for a few common pure builtins. Anything
// not listed is `any` (no arity or type checks) — honest gradual typing.
// `print` is deliberately absent: it is variadic, so a fixed signature
// would produce false arity errors.
var builtinSigs = map[string]*FuncSig{
	"len":   {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "int"}},
	"str":   {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "string"}},
	"int":   {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "int"}},
	"float": {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "float"}},
	"bool":  {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "bool"}},
	"abs":   {Params: []ParamSig{{Name: "x", Required: true}}, Return: &Type{Name: "number"}},
}

// infer computes the static type of an expression (any when unknown).
func (c *Checker) infer(e ast.Expression) Type {
	if e == nil {
		return anyType()
	}
	switch n := e.(type) {
	case *ast.IntegerLiteral:
		return simpleType("int")
	case *ast.FloatLiteral:
		return simpleType("float")
	case *ast.StringLiteral:
		return simpleType("string")
	case *ast.Boolean:
		return simpleType("bool")
	case *ast.NullLiteral:
		return simpleType("null")
	case *ast.InterpolatedString:
		return simpleType("string")
	case *ast.ArrayLiteral:
		return simpleType("array")
	case *ast.TupleLiteral:
		return simpleType("tuple")
	case *ast.HashLiteral:
		t := simpleType("hash")
		t.HashFuncs = map[string]FuncArity{}
		for k, v := range n.Pairs {
			if fl, ok := v.(*ast.FunctionLiteral); ok {
				if ks, ok := k.(*ast.StringLiteral); ok {
					required := 0
					for i := range fl.Parameters {
						if i >= len(fl.Defaults) || fl.Defaults[i] == nil {
							required++
						}
					}
					t.HashFuncs[ks.Value] = FuncArity{Total: len(fl.Parameters), Required: required}
				}
			}
		}
		return t
	case *ast.Identifier:
		// Enum member access is `Color.Red` (member expr); a bare enum
		// name refers to the enum itself.
		if v, ok := c.lookup(n.Value); ok {
			return v.typ
		}
		return anyType()
	case *ast.PrefixExpression:
		switch n.Operator {
		case "!":
			return simpleType("bool")
		case "-":
			rt := c.infer(n.Right)
			if rt.Name == "int" {
				return simpleType("int")
			}
			if rt.Name == "float" || rt.Name == "number" {
				return simpleType("number")
			}
			return anyType()
		}
		return anyType()
	case *ast.InfixExpression:
		return c.inferInfix(n)
	case *ast.IfExpression:
		t := c.inferBlock(n.Consequence)
		if n.Alternative != nil {
			a := c.inferBlock(n.Alternative)
			return unionOf(t, a)
		}
		return unionOf(t, simpleType("null"))
	case *ast.TernaryExpression:
		return unionOf(c.infer(n.Consequence), c.infer(n.Alternative))
	case *ast.CallExpression:
		if sig := c.calleeSig(n.Function); sig != nil && sig.Return != nil {
			return *sig.Return
		}
		return anyType()
	case *ast.FunctionLiteral:
		return Type{Name: "function", Sig: c.funcSigOf(n)}
	case *ast.NewExpression:
		if c.isDeclaredType(n.ClassName.Value) {
			return Type{Name: "instance", ClassName: n.ClassName.Value}
		}
		return anyType()
	case *ast.MemberExpression:
		return c.inferMember(n)
	case *ast.RangeExpression:
		return simpleType("array")
	case *ast.MatchExpression:
		var out *Type
		for _, a := range n.Arms {
			bt := c.inferBlock(a.Body)
			if out == nil {
				o := bt
				out = &o
			} else {
				u := unionOf(*out, bt)
				out = &u
			}
		}
		if n.Default != nil {
			dt := c.inferBlock(n.Default)
			if out == nil {
				out = &dt
			} else {
				u := unionOf(*out, dt)
				out = &u
			}
		}
		if out == nil {
			return anyType()
		}
		return *out
	default:
		return anyType()
	}
}

func (c *Checker) inferInfix(n *ast.InfixExpression) Type {
	l, r := c.infer(n.Left), c.infer(n.Right)
	switch n.Operator {
	case "==", "!=":
		return simpleType("bool")
	case "<", ">", "<=", ">=":
		return simpleType("bool")
	case "+":
		if l.Name == "string" && r.Name == "string" {
			return simpleType("string")
		}
		if l.Name == "int" && r.Name == "int" {
			return simpleType("int")
		}
		if isNumeric(l) && isNumeric(r) {
			return simpleType("number")
		}
		return anyType()
	case "-", "*", "%":
		if l.Name == "int" && r.Name == "int" {
			return simpleType("int")
		}
		if isNumeric(l) && isNumeric(r) {
			return simpleType("number")
		}
		return anyType()
	case "/":
		// Runtime int/int is int division; keep "number" to stay safe.
		if isNumeric(l) && isNumeric(r) {
			return simpleType("number")
		}
		return anyType()
	case "and", "or":
		return simpleType("bool")
	}
	return anyType()
}

func isNumeric(t Type) bool {
	return t.Name == "int" || t.Name == "float" || t.Name == "number" || isAny(t)
}

// inferMember resolves `obj.prop`: enum members, class method call types
// stay any (calls are checked via calleeSig), hash literal keys are any.
func (c *Checker) inferMember(n *ast.MemberExpression) Type {
	ot := c.infer(n.Object)
	// Enum access: Color.Red has the enum's type.
	if ot.Name != "" && ot.Name != "any" {
		if members, ok := c.enums[ot.Name]; ok {
			if members[n.Property.Value] {
				return simpleType(ot.Name)
			}
		}
	}
	return anyType()
}

func (c *Checker) inferBlock(b *ast.BlockStatement) Type {
	if b == nil || len(b.Statements) == 0 {
		return simpleType("null")
	}
	// The block's value is its last statement's value (best effort).
	last := b.Statements[len(b.Statements)-1]
	if es, ok := last.(*ast.ExpressionStatement); ok {
		return c.infer(es.Expression)
	}
	return anyType()
}

// unionOf builds a union, flattening nested unions and dropping duplicates.
func unionOf(a, b Type) Type {
	if isAny(a) || isAny(b) {
		return anyType()
	}
	var members []Type
	var flatten func(t Type)
	flatten = func(t Type) {
		if len(t.Union) > 0 {
			for _, m := range t.Union {
				flatten(m)
			}
			return
		}
		for _, m := range members {
			if m.Name == t.Name && m.ClassName == t.ClassName {
				return
			}
		}
		members = append(members, t)
	}
	flatten(a)
	flatten(b)
	if len(members) == 1 {
		return members[0]
	}
	return Type{Name: "union", Union: members}
}

// ---------------------------------------------------------------------------
// lineOf: source line for an AST node (0 when unknown)
// ---------------------------------------------------------------------------

func lineOf(n ast.Node) int {
	switch t := n.(type) {
	case *ast.Program:
		return 1
	case *ast.LetStatement:
		return t.Token.Line
	case *ast.TypedLetStatement:
		return t.Token.Line
	case *ast.ConstStatement:
		return t.Token.Line
	case *ast.ReturnStatement:
		return t.Token.Line
	case *ast.ExpressionStatement:
		return t.Token.Line
	case *ast.PrintStatement:
		return t.Token.Line
	case *ast.BlockStatement:
		return t.Token.Line
	case *ast.IfExpression:
		return t.Token.Line
	case *ast.WhileStatement:
		return t.Token.Line
	case *ast.ForStatement:
		return t.Token.Line
	case *ast.ForInStatement:
		return t.Token.Line
	case *ast.FunctionLiteral:
		return t.Token.Line
	case *ast.CallExpression:
		return t.Token.Line
	case *ast.Identifier:
		return t.Token.Line
	case *ast.IntegerLiteral:
		return t.Token.Line
	case *ast.FloatLiteral:
		return t.Token.Line
	case *ast.StringLiteral:
		return t.Token.Line
	case *ast.Boolean:
		return t.Token.Line
	case *ast.NullLiteral:
		return t.Token.Line
	case *ast.ClassStatement:
		return t.Token.Line
	case *ast.InterfaceDecl:
		return t.Token.Line
	case *ast.NewExpression:
		return t.Token.Line
	case *ast.AssignExpression:
		return t.Token.Line
	case *ast.EnumStatement:
		return t.Token.Line
	case *ast.RecordStatement:
		return t.Token.Line
	case *ast.MatchExpression:
		return t.Token.Line
	case *ast.ThrowStatement:
		return t.Token.Line
	case *ast.TryStatement:
		return t.Token.Line
	}
	return 0
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// CheckSource parses src and returns the list of static type errors.
// Parse errors are returned as CheckErrors too (line from the parser).
func CheckSource(file, src string) []CheckError {
	p := parser.New(lexer.New(src))
	prog := p.ParseProgram()
	var errs []CheckError
	for _, pe := range p.Errors() {
		errs = append(errs, CheckError{File: file, Msg: "parse error: " + pe})
	}
	if len(errs) > 0 {
		return errs
	}
	c := newChecker(file)
	c.collectDecls(prog)
	c.checkProgram(prog)
	return c.errs
}
