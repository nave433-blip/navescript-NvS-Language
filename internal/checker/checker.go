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
// Wave 19: TypeParam (non-empty) is a reference to an in-scope generic
// type parameter — a named hole, `any`-compatible; TypeArgs carries
// explicit type arguments on instance types (`Vec<int>`).
type Type struct {
	Name      string
	Union     []Type
	Sig       *FuncSig
	HashFuncs map[string]FuncArity
	ClassName string // for Name=="instance": the class of `new C()`
	TypeParam string // wave 19: non-empty = reference to type parameter `T`
	TypeArgs  []Type // wave 19: `Vec<int>` → [int]
}

// FuncArity is the static shape of a callable: total parameters and how
// many are required (no default). Mirrors the runtime implements() rule:
// callable with n args iff required <= n <= total.
type FuncArity struct {
	Total    int
	Required int
}

// FuncSig is a statically known function signature.
// Wave 19: TypeParams holds the declared generic parameters (`fn<T>`);
// a nil/empty list means the function is not generic.
type FuncSig struct {
	Params     []ParamSig
	Return     *Type // nil = any
	TypeParams []*GenericParam
}

// GenericParam is one declared type parameter: `T` or `T: Bound`.
// A nil Bound means unbounded (any). BoundSrc is the original AST bound,
// used to report unknown bound names exactly once in pass 2.
type GenericParam struct {
	Name     string
	Bound    *Type
	BoundSrc *ast.TypeAnnotation
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
	if t.TypeParam != "" {
		return t.TypeParam
	}
	if len(t.Union) > 0 {
		parts := make([]string, len(t.Union))
		for i, u := range t.Union {
			parts[i] = u.String()
		}
		return strings.Join(parts, " | ")
	}
	if t.Name == "instance" && t.ClassName != "" {
		s := t.ClassName
		if len(t.TypeArgs) > 0 {
			args := make([]string, len(t.TypeArgs))
			for i, a := range t.TypeArgs {
				args[i] = a.String()
			}
			s += "<" + strings.Join(args, ", ") + ">"
		}
		return s
	}
	return t.Name
}

func isAny(t Type) bool { return t.Name == "any" && len(t.Union) == 0 }

// builtinTypes mirrors internal/eval's builtinAnnotationChecks (lowercased).
var builtinTypes = map[string]bool{
	"int": true, "integer": true, "float": true, "number": true,
	"numeric": true, "comparable": true, "printable": true,
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
	case "numeric":
		// Wave 19: `numeric` is the `number` tower (int/float/number).
		return "number"
	case "comparable", "printable":
		// Wave 19: every NvS value is comparable (`==` never errors) and
		// printable (every value stringifies) — honestly `any`.
		return "any"
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
	// Wave 19: `fn(A, B) -> R` function-type annotations.
	if ann.FnSig != nil {
		return c.fnTypeAnnotation(ann)
	}
	// Wave 19: a reference to an in-scope generic type parameter — a named
	// hole, `any`-compatible (gradual typing, rule 9).
	if gp := c.lookupTypeParam(name); gp != nil {
		if len(ann.TypeArgs) > 0 {
			return anyType(), fmt.Sprintf("type parameter '%s' cannot take type arguments", name)
		}
		if ann.IsArray {
			return simpleType("array"), ""
		}
		return Type{TypeParam: name}, ""
	}
	canon := canonicalName(name)
	if builtinTypes[canon] {
		// Wave 19: builtins don't take type arguments (`int<string>` is
		// nonsense; say `[int]`/`string[]` for arrays of int).
		if len(ann.TypeArgs) > 0 {
			return anyType(), fmt.Sprintf("only classes can take type arguments, not '%s'", name)
		}
		// `string[]` etc: the element type is erased, the shape is array.
		if ann.IsArray {
			return simpleType("array"), ""
		}
		return simpleType(canon), ""
	}
	// Declared type names (class/interface/record/enum) are case-sensitive,
	// exactly like the runtime's lexical lookup.
	if c.isDeclaredType(name) {
		// Wave 19: parameterized types — `Vec<int>`.
		if len(ann.TypeArgs) > 0 {
			if _, ok := c.classes[name]; !ok {
				return anyType(), fmt.Sprintf("only classes can take type arguments, not '%s'", name)
			}
			var targs []Type
			for _, ta := range ann.TypeArgs {
				t, unk := annotationType(ta, c)
				if unk != "" {
					return anyType(), unk
				}
				targs = append(targs, t)
			}
			if ann.IsArray {
				return simpleType("array"), ""
			}
			return Type{Name: "instance", ClassName: name, TypeArgs: targs}, ""
		}
		if ann.IsArray {
			return simpleType("array"), ""
		}
		return simpleType(name), ""
	}
	return anyType(), fmt.Sprintf("unknown type '%s'", name)
}

// fnTypeAnnotation converts a `fn(A, B) -> R` annotation into a static
// function type. Wave 19.
func (c *Checker) fnTypeAnnotation(ann *ast.TypeAnnotation) (Type, string) {
	sig := &FuncSig{}
	for _, p := range ann.FnSig.Params {
		t, unk := annotationType(p, c)
		if unk != "" {
			return anyType(), unk
		}
		tt := t
		sig.Params = append(sig.Params, ParamSig{Type: &tt, Required: true})
	}
	if ann.FnSig.Return != nil {
		t, unk := annotationType(ann.FnSig.Return, c)
		if unk != "" {
			return anyType(), unk
		}
		tt := t
		sig.Return = &tt
	}
	return Type{Name: "function", Sig: sig}, ""
}

// genericParamsOf converts AST type parameters to checker GenericParams,
// resolving bounds left-to-right with earlier params in scope
// (`<T, U: Vec<T>>`). Unknown bound names resolve to any here and are
// reported exactly once by checkTypeParamDecls in pass 2.
func (c *Checker) genericParamsOf(tps []*ast.TypeParam) []*GenericParam {
	var out []*GenericParam
	for _, tp := range tps {
		gp := &GenericParam{Name: tp.Name.Value, BoundSrc: tp.Bound}
		if tp.Bound != nil {
			if bt, unk := annotationType(tp.Bound, c); unk == "" {
				b := bt
				gp.Bound = &b
			}
		}
		out = append(out, gp)
		// This param is in scope for the bounds of later params.
		c.pushTypeParams([]*GenericParam{gp})
	}
	for range out {
		c.popTypeParams()
	}
	return out
}

// checkTypeParamDecls enforces checker rules 3 (no duplicates) and 4
// (bounds must name known types) for one declaration's type parameters.
// Call with the params' own scope pushed (plus any outer scopes).
func (c *Checker) checkTypeParamDecls(tps []*GenericParam, line int) {
	seen := map[string]bool{}
	for _, tp := range tps {
		if seen[tp.Name] {
			c.errorf(line, "duplicate type parameter '%s'", tp.Name)
		}
		seen[tp.Name] = true
		if tp.BoundSrc != nil {
			if _, unk := annotationType(tp.BoundSrc, c); unk != "" {
				c.errorf(line, "%s in bound of type parameter '%s'", unk, tp.Name)
			}
		}
	}
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

// classInfo is a declared class: its methods, parent, and (wave 19)
// generic type parameters.
type classInfo struct {
	methods    map[string]*FuncSig
	parent     string
	typeParams []*GenericParam
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
	warns   []CheckError // wave 19: advisory warnings (e.g. type-param shadowing)
	scopes  []map[string]varInfo
	classes map[string]*classInfo
	ifaces  map[string]*ifaceInfo
	records map[string]bool
	enums   map[string]map[string]bool
	// fnStack tracks enclosing functions' declared return types (nil = any).
	fnStack []*Type
	// Wave 19: tparamStack is the stack of in-scope generic type parameters
	// (innermost last); resolvedCalls caches instantiated call signatures
	// per call node so inference runs once; resolveFailed marks calls whose
	// generic resolution already reported an error.
	tparamStack   []map[string]*GenericParam
	resolvedCalls map[*ast.CallExpression]*FuncSig
	resolveFailed map[*ast.CallExpression]bool
}

func newChecker(file string) *Checker {
	return &Checker{
		file:          file,
		scopes:        []map[string]varInfo{{}},
		classes:       map[string]*classInfo{},
		ifaces:        map[string]*ifaceInfo{},
		records:       map[string]bool{},
		enums:         map[string]map[string]bool{},
		resolvedCalls: map[*ast.CallExpression]*FuncSig{},
		resolveFailed: map[*ast.CallExpression]bool{},
	}
}

func (c *Checker) errorf(line int, format string, args ...interface{}) {
	c.errs = append(c.errs, CheckError{File: c.file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

// warnf records an advisory warning. Warnings never fail `nvs check`.
func (c *Checker) warnf(line int, format string, args ...interface{}) {
	c.warns = append(c.warns, CheckError{File: c.file, Line: line, Msg: "warning: " + fmt.Sprintf(format, args...)})
}

// pushTypeParams pushes one scope of generic type parameters.
func (c *Checker) pushTypeParams(params []*GenericParam) {
	m := map[string]*GenericParam{}
	for _, gp := range params {
		m[gp.Name] = gp
	}
	c.tparamStack = append(c.tparamStack, m)
}

func (c *Checker) popTypeParams() {
	c.tparamStack = c.tparamStack[:len(c.tparamStack)-1]
}

// lookupTypeParam finds an in-scope type parameter (innermost first).
func (c *Checker) lookupTypeParam(name string) *GenericParam {
	for i := len(c.tparamStack) - 1; i >= 0; i-- {
		if gp, ok := c.tparamStack[i][name]; ok {
			return gp
		}
	}
	return nil
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
	// Wave 19: a type parameter is a named hole — `any`-compatible in both
	// directions (rule 9's spirit: never error on generic code paths the
	// checker cannot see through).
	if src.TypeParam != "" || dst.TypeParam != "" {
		return true
	}
	// Wave 19: function types — arity must match; parameters are checked
	// bivariantly (gradual), returns covariantly.
	if src.Name == "function" && dst.Name == "function" {
		return c.funcTypesCompatible(src, dst)
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
			if !c.instanceOf(src.ClassName, dst.ClassName) {
				return false
			}
			// Wave 19, rule 10: invariance — `Box<int>` is not
			// `Box<number>`. Unknown (nil) type args are gradual wildcards.
			return c.typeArgsInvariant(src.TypeArgs, dst.TypeArgs)
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

// typeArgsInvariant enforces wave-19 rule 10 (invariance): two
// parameterized types are compatible only when their type arguments match
// exactly. A side with unknown (nil) arguments is a gradual wildcard.
func (c *Checker) typeArgsInvariant(src, dst []Type) bool {
	if len(src) == 0 || len(dst) == 0 {
		return true
	}
	if len(src) != len(dst) {
		return false
	}
	for i := range src {
		if !c.typesEqual(src[i], dst[i]) {
			return false
		}
	}
	return true
}

// typesEqual is exact type equality modulo gradual `any`.
func (c *Checker) typesEqual(a, b Type) bool {
	if isAny(a) || isAny(b) {
		return true
	}
	if a.TypeParam != "" || b.TypeParam != "" {
		return a.TypeParam == b.TypeParam
	}
	if a.Name != b.Name || a.ClassName != b.ClassName {
		return false
	}
	if len(a.Union) != len(b.Union) || len(a.TypeArgs) != len(b.TypeArgs) {
		return false
	}
	for i := range a.Union {
		if !c.typesEqual(a.Union[i], b.Union[i]) {
			return false
		}
	}
	for i := range a.TypeArgs {
		if !c.typesEqual(a.TypeArgs[i], b.TypeArgs[i]) {
			return false
		}
	}
	return true
}

// funcTypesCompatible checks function-type assignability: same arity,
// bivariant parameters (gradual), covariant return.
func (c *Checker) funcTypesCompatible(src, dst Type) bool {
	if src.Sig == nil || dst.Sig == nil {
		return true // unknown signature: gradual
	}
	if len(src.Sig.Params) != len(dst.Sig.Params) {
		return false
	}
	for i := range src.Sig.Params {
		sp, dp := src.Sig.Params[i].Type, dst.Sig.Params[i].Type
		if sp != nil && dp != nil && !isAny(*sp) && !isAny(*dp) {
			if !c.assignableTo(*sp, *dp) && !c.assignableTo(*dp, *sp) {
				return false
			}
		}
	}
	if src.Sig.Return != nil && dst.Sig.Return != nil {
		return c.assignableTo(*src.Sig.Return, *dst.Sig.Return)
	}
	return true
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
			// Wave 19, rule 8 (Go's honest boundary): a generic method is
			// an infinite family of methods, not one entry in a dispatch
			// table — it cannot satisfy an interface method.
			if len(sig.TypeParams) > 0 {
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

// ifaceMismatchReason explains why an instance doesn't satisfy an interface
// ("" when it does). Used to sharpen the assignment error.
func (c *Checker) ifaceMismatchReason(src Type, ifaceName string) string {
	iface, ok := c.ifaces[ifaceName]
	if !ok || src.Name != "instance" {
		return ""
	}
	ci, ok := c.classes[src.ClassName]
	if !ok {
		return ""
	}
	for _, mname := range iface.order {
		im := iface.methods[mname]
		sig := c.lookupMethod(ci, mname)
		if sig == nil {
			return fmt.Sprintf("method '%s' is missing", mname)
		}
		if len(sig.TypeParams) > 0 {
			return fmt.Sprintf("method '%s' is generic and cannot satisfy interface method '%s'", mname, mname)
		}
		if !c.arityCompatible(sig, im.arity) {
			return fmt.Sprintf("method '%s' has incompatible arity", mname)
		}
	}
	return ""
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
			// Wave 19: class type params are in scope for every method
			// signature (rule 1).
			ci.typeParams = c.genericParamsOf(n.TypeParams)
			c.pushTypeParams(ci.typeParams)
			for _, m := range n.Methods {
				ci.methods[m.Name.Value] = c.methodSigOf(m)
			}
			c.popTypeParams()
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
// Wave 19: the literal's type parameters are in scope for its annotations.
func (c *Checker) funcSigOf(fl *ast.FunctionLiteral) *FuncSig {
	tparams := c.genericParamsOf(fl.TypeParams)
	c.pushTypeParams(tparams)
	defer c.popTypeParams()
	sig := &FuncSig{TypeParams: tparams}
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
		TypeParams: m.TypeParams,
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
			c.assignErrorf(lineOf(n), vt, t, fmt.Sprintf("variable '%s'", n.Name.Value))
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
	case *ast.ClassStatement, *ast.InterfaceDecl, *ast.RecordStatement, *ast.EnumStatement, *ast.ModuleStatement:
		// Declarations: check method bodies for classes; validate
		// interface method annotations (unknown names reported once here).
		// Wave 19: `module std.x;` is recorded, nothing to check.
		if cs, ok := s.(*ast.ClassStatement); ok {
			ci := c.classes[cs.Name.Value]
			// Wave 19, rule 1: class type params are in scope for every
			// method signature and body.
			c.pushTypeParams(ci.typeParams)
			c.checkTypeParamDecls(ci.typeParams, lineOf(cs))
			for _, m := range cs.Methods {
				fl := &ast.FunctionLiteral{Token: m.Token, Parameters: m.Parameters,
					ParamTypes: m.ParamTypes, ReturnType: m.ReturnType,
					TypeParams: m.TypeParams, Body: m.Body}
				c.checkFunctionBody(m.Name.Value, fl)
			}
			c.popTypeParams()
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
// Wave 19: the literal's type parameters are in scope for the body; rules
// 2 (shadowing warning), 3 (duplicates), 4 (bound names) are enforced.
func (c *Checker) checkFunctionBody(name string, fl *ast.FunctionLiteral) {
	sig := c.funcSigOf(fl)
	// Rule 2: warn when a type parameter shadows one from an outer scope
	// (e.g. a method's `T` shadowing the class's `T`).
	outer := map[string]bool{}
	for i := 0; i < len(c.tparamStack); i++ {
		for n := range c.tparamStack[i] {
			outer[n] = true
		}
	}
	for _, tp := range sig.TypeParams {
		if outer[tp.Name] {
			c.warnf(lineOf(fl), "type parameter '%s' shadows an outer type parameter", tp.Name)
		}
	}
	c.pushTypeParams(sig.TypeParams)
	defer c.popTypeParams()
	// Rules 3 and 4.
	c.checkTypeParamDecls(sig.TypeParams, lineOf(fl))
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
		// Wave 19: `new Box<int, string>` for 1-param Box is an error.
		if len(n.TypeArgs) > 0 {
			if ci, ok := c.classes[n.ClassName.Value]; ok &&
				len(ci.typeParams) != len(n.TypeArgs) {
				c.errorf(lineOf(n), "wrong number of type arguments for class '%s': got %d, want %d",
					n.ClassName.Value, len(n.TypeArgs), len(ci.typeParams))
			}
		}
		// Wave 19: inferred constructor type arguments must satisfy bounds
		// (rule 5); uninferred params are `any` (gradual, not an error).
		if ci, ok := c.classes[n.ClassName.Value]; ok && len(n.TypeArgs) == 0 {
			targs := c.newTypeArgs(n)
			for i, tp := range ci.typeParams {
				if i < len(targs) {
					if err := c.checkBound(targs[i], tp, map[string]Type{}); err != "" {
						c.errorf(lineOf(n), "%s", err)
					}
				}
			}
		}
	case *ast.PostfixExpression:
		// Wave 19: `x++` — just check the operand.
		c.checkExpression(n.Operand)
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

// assignErrorf reports a failed assignment, sharpening interface mismatches
// with the specific reason (wave 19, rule 8).
func (c *Checker) assignErrorf(line int, vt, t Type, what string) {
	msg := fmt.Sprintf("cannot assign %s to %s of type %s", vt, what, t)
	if t.Name != "" && vt.Name == "instance" {
		if _, ok := c.ifaces[t.Name]; ok {
			if r := c.ifaceMismatchReason(vt, t.Name); r != "" {
				msg += ": " + r
			}
		}
	}
	c.errorf(line, "%s", msg)
}
func (c *Checker) checkAssign(name string, value ast.Expression, line int) {
	vt := c.infer(value)
	if v, ok := c.lookup(name); ok && v.declared != nil {
		if !c.assignableTo(vt, *v.declared) {
			c.assignErrorf(line, vt, *v.declared, fmt.Sprintf("variable '%s'", name))
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
	// Spread in the argument list: the callee's arity can't be known
	// statically (e.g. f(...a, 10)), so skip arity and type checks.
	for _, a := range n.Arguments {
		if _, ok := a.(*ast.SpreadExpression); ok {
			return
		}
	}
	sig := c.resolveCall(n)
	if sig == nil {
		return // unknown callee (gradual), or resolution already errored.
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

// ---- Wave 19: generic call resolution (checker rules 5, 6) ----

// resolveCall returns the instantiated signature for a call: explicit type
// arguments are validated, otherwise type parameters are inferred from the
// value arguments. Errors are reported exactly once per call node (results
// are cached; infer() shares this path quietly).
func (c *Checker) resolveCall(n *ast.CallExpression) *FuncSig {
	if sig, ok := c.resolvedCalls[n]; ok {
		return sig
	}
	if c.resolveFailed[n] {
		return nil
	}
	base := c.calleeSig(n.Function)
	if base == nil {
		return nil // unknown callee: gradual, no checks.
	}
	if len(n.TypeArgs) > 0 && len(base.TypeParams) == 0 {
		c.errorf(lineOf(n), "not a generic function: got %d type arguments", len(n.TypeArgs))
		c.resolveFailed[n] = true
		return nil
	}
	// Wave 19: for method calls, the receiver's type arguments substitute
	// the class's type parameters (rule 1).
	classSubst := map[string]Type{}
	if mem, ok := n.Function.(*ast.MemberExpression); ok {
		ot := c.infer(mem.Object)
		if ot.Name == "instance" && len(ot.TypeArgs) > 0 {
			if ci, ok := c.classes[ot.ClassName]; ok && len(ci.typeParams) == len(ot.TypeArgs) {
				for i, tp := range ci.typeParams {
					classSubst[tp.Name] = ot.TypeArgs[i]
				}
			}
		}
	}
	sig := base
	if len(base.TypeParams) > 0 || len(classSubst) > 0 {
		var ok bool
		sig, ok = c.instantiateCall(n, base, classSubst)
		if !ok {
			c.resolveFailed[n] = true
			return nil
		}
	}
	c.resolvedCalls[n] = sig
	return sig
}

// instantiateCall substitutes type parameters in sig: classSubst first,
// then the function's own params via explicit args or inference.
func (c *Checker) instantiateCall(n *ast.CallExpression, sig *FuncSig, classSubst map[string]Type) (*FuncSig, bool) {
	subst := map[string]Type{}
	for k, v := range classSubst {
		subst[k] = v
	}
	if len(sig.TypeParams) > 0 {
		var targs []Type
		if len(n.TypeArgs) > 0 {
			if len(n.TypeArgs) != len(sig.TypeParams) {
				c.errorf(lineOf(n), "wrong number of type arguments: got %d, want %d",
					len(n.TypeArgs), len(sig.TypeParams))
				return nil, false
			}
			for i, ta := range n.TypeArgs {
				t, unk := annotationType(ta, c)
				if unk != "" {
					c.errorf(lineOf(ta), "%s", unk)
					return nil, false
				}
				if err := c.checkBound(t, sig.TypeParams[i], subst); err != "" {
					c.errorf(lineOf(ta), "%s", err)
					return nil, false
				}
				targs = append(targs, t)
			}
		} else {
			var ok bool
			targs, ok = c.inferTypeArgs(n, sig, subst)
			if !ok {
				return nil, false
			}
		}
		for i, tp := range sig.TypeParams {
			subst[tp.Name] = targs[i]
		}
	}
	out := c.substFuncSig(sig, subst)
	out.TypeParams = nil // instantiated: no longer generic
	return out, true
}

// inferTypeArgs infers a generic signature's type parameters from the value
// arguments (rule 6). subst carries already-known substitutions (class type
// params) so inference sees through them.
func (c *Checker) inferTypeArgs(n *ast.CallExpression, sig *FuncSig, subst map[string]Type) ([]Type, bool) {
	targs := make([]Type, len(sig.TypeParams))
	inferred := make([]bool, len(sig.TypeParams))
	indexOf := map[string]int{}
	for i, tp := range sig.TypeParams {
		indexOf[tp.Name] = i
	}
	psig := c.substFuncSig(sig, subst)
	for i, a := range n.Arguments {
		if i >= len(psig.Params) {
			break
		}
		pt := psig.Params[i].Type
		if pt == nil {
			continue
		}
		at := c.infer(a)
		c.unifyTypeParam(*pt, at, indexOf, targs, inferred)
	}
	for i, tp := range sig.TypeParams {
		if !inferred[i] {
			c.errorf(lineOf(n), "cannot infer type parameter '%s'; provide it explicitly", tp.Name)
			return nil, false
		}
		if err := c.checkBound(targs[i], tp, subst); err != "" {
			c.errorf(lineOf(n), "%s", err)
			return nil, false
		}
	}
	return targs, true
}

// unifyTypeParam matches a (possibly generic) parameter type against an
// argument type, recording inferred type arguments. First inference wins;
// inconsistencies surface later as ordinary argument-type errors.
func (c *Checker) unifyTypeParam(pt, at Type, indexOf map[string]int, targs []Type, inferred []bool) {
	if pt.TypeParam != "" {
		if i, ok := indexOf[pt.TypeParam]; ok && !inferred[i] {
			targs[i] = at
			inferred[i] = true
		}
		return
	}
	// Through function types: fn(T) -> U against fn(int) -> string.
	if pt.Name == "function" && at.Name == "function" && pt.Sig != nil && at.Sig != nil {
		for j := range pt.Sig.Params {
			if j < len(at.Sig.Params) {
				var ppt, aat Type = anyType(), anyType()
				if pt.Sig.Params[j].Type != nil {
					ppt = *pt.Sig.Params[j].Type
				}
				if at.Sig.Params[j].Type != nil {
					aat = *at.Sig.Params[j].Type
				}
				c.unifyTypeParam(ppt, aat, indexOf, targs, inferred)
			}
		}
		if pt.Sig.Return != nil && at.Sig.Return != nil {
			c.unifyTypeParam(*pt.Sig.Return, *at.Sig.Return, indexOf, targs, inferred)
		}
		return
	}
	// Through parameterized instances: Vec<T> against Vec<int>.
	if pt.Name == "instance" && at.Name == "instance" &&
		pt.ClassName == at.ClassName &&
		len(pt.TypeArgs) > 0 && len(pt.TypeArgs) == len(at.TypeArgs) {
		for j := range pt.TypeArgs {
			c.unifyTypeParam(pt.TypeArgs[j], at.TypeArgs[j], indexOf, targs, inferred)
		}
		return
	}
	// Through unions: try each member.
	if len(pt.Union) > 0 {
		for _, m := range pt.Union {
			c.unifyTypeParam(m, at, indexOf, targs, inferred)
		}
	}
}

// newTypeArgs resolves a NewExpression's class type arguments: explicit
// `<...>` first, otherwise inferred from the init signature's use of the
// class's type parameters (uninferred → any; gradual, never an error here).
// Bounds are checked by the caller (checkExpression).
func (c *Checker) newTypeArgs(n *ast.NewExpression) []Type {
	if len(n.TypeArgs) > 0 {
		var targs []Type
		for _, ta := range n.TypeArgs {
			at, unk := annotationType(ta, c)
			if unk != "" {
				at = anyType()
			}
			targs = append(targs, at)
		}
		return targs
	}
	ci, ok := c.classes[n.ClassName.Value]
	if !ok || len(ci.typeParams) == 0 {
		return nil
	}
	initSig := c.lookupMethod(ci, "init")
	if initSig == nil {
		return nil
	}
	targs := make([]Type, len(ci.typeParams))
	inferred := make([]bool, len(ci.typeParams))
	indexOf := map[string]int{}
	for i, tp := range ci.typeParams {
		indexOf[tp.Name] = i
	}
	for i, a := range n.Arguments {
		if i >= len(initSig.Params) {
			break
		}
		pt := initSig.Params[i].Type
		if pt == nil {
			continue
		}
		c.unifyTypeParam(*pt, c.infer(a), indexOf, targs, inferred)
	}
	for i := range targs {
		if !inferred[i] {
			targs[i] = anyType()
		}
	}
	return targs
}

// checkBound enforces rule 5: t must satisfy tp's bound ("" = ok).
// subst lets bounds reference outer type params (e.g. class params).
func (c *Checker) checkBound(t Type, tp *GenericParam, subst map[string]Type) string {
	if tp.Bound == nil || isAny(t) {
		return "" // unbounded, or gradual: anything goes (rule 9)
	}
	bound := c.substType(*tp.Bound, subst)
	if !c.assignableTo(t, bound) {
		return fmt.Sprintf("type %s does not satisfy bound %s of type parameter '%s'", t, bound, tp.Name)
	}
	return ""
}

// substType replaces type-parameter references using subst.
func (c *Checker) substType(t Type, subst map[string]Type) Type {
	if t.TypeParam != "" {
		if s, ok := subst[t.TypeParam]; ok {
			return s
		}
		return t
	}
	if len(t.Union) > 0 {
		members := make([]Type, len(t.Union))
		for i, m := range t.Union {
			members[i] = c.substType(m, subst)
		}
		t.Union = members
		return t
	}
	if t.Sig != nil {
		t.Sig = c.substFuncSig(t.Sig, subst)
		return t
	}
	if len(t.TypeArgs) > 0 {
		args := make([]Type, len(t.TypeArgs))
		for i, a := range t.TypeArgs {
			args[i] = c.substType(a, subst)
		}
		t.TypeArgs = args
		return t
	}
	return t
}

// substFuncSig substitutes type parameters throughout a signature.
func (c *Checker) substFuncSig(sig *FuncSig, subst map[string]Type) *FuncSig {
	if len(subst) == 0 {
		return sig
	}
	out := &FuncSig{TypeParams: sig.TypeParams}
	for _, p := range sig.Params {
		np := p
		if p.Type != nil {
			t := c.substType(*p.Type, subst)
			np.Type = &t
		}
		out.Params = append(out.Params, np)
	}
	if sig.Return != nil {
		t := c.substType(*sig.Return, subst)
		out.Return = &t
	}
	return out
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
		if sig := c.resolveCall(n); sig != nil && sig.Return != nil {
			return *sig.Return
		}
		return anyType()
	case *ast.FunctionLiteral:
		return Type{Name: "function", Sig: c.funcSigOf(n)}
	case *ast.NewExpression:
		if c.isDeclaredType(n.ClassName.Value) {
			t := Type{Name: "instance", ClassName: n.ClassName.Value}
			// Wave 19: explicit `new Box<int>(...)`, else infer from init.
			t.TypeArgs = c.newTypeArgs(n)
			return t
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
	case "-", "*", "%", "**":
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
	case *ast.TypeAnnotation:
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
	errs, _ := CheckWithWarnings(file, src)
	return errs
}

// CheckWithWarnings parses src and returns static type errors and advisory
// warnings (wave 19: e.g. type-parameter shadowing). Warnings never fail
// `nvs check`; they are reported for tooling and `--strict` futures.
func CheckWithWarnings(file, src string) ([]CheckError, []CheckError) {
	p := parser.New(lexer.New(src))
	prog := p.ParseProgram()
	var errs []CheckError
	for _, pe := range p.Errors() {
		errs = append(errs, CheckError{File: file, Msg: "parse error: " + pe})
	}
	if len(errs) > 0 {
		return errs, nil
	}
	c := newChecker(file)
	c.collectDecls(prog)
	c.checkProgram(prog)
	return c.errs, c.warns
}
