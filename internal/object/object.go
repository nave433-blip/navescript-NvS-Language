package object

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/navescript/nvs/internal/ast"
)

type ObjectType string

const (
	INTEGER_OBJ      = "INTEGER"
	FLOAT_OBJ        = "FLOAT"
	BOOLEAN_OBJ      = "BOOLEAN"
	NULL_OBJ         = "NULL"
	RETURN_VALUE_OBJ = "RETURN_VALUE"
	ERROR_OBJ        = "ERROR"
	FUNCTION_OBJ     = "FUNCTION"
	STRING_OBJ       = "STRING"
	BUILTIN_OBJ      = "BUILTIN"
	ARRAY_OBJ        = "ARRAY"
	HASH_OBJ         = "HASH"
	TUPLE_OBJ        = "TUPLE"
	RECORD_OBJ       = "RECORD"
	RECORD_DEF_OBJ   = "RECORD_DEF"
	BREAK_OBJ        = "BREAK"
	CONTINUE_OBJ     = "CONTINUE"
	CLASS_OBJ        = "CLASS"
	INSTANCE_OBJ     = "INSTANCE"
	// Wave 5: structural interface declarations.
	INTERFACE_OBJ = "INTERFACE"
	GENERATOR_OBJ = "GENERATOR"
	YIELD_OBJ     = "YIELD"
	NAMED_ARG_OBJ = "NAMED_ARG"
)

type Object interface {
	Type() ObjectType
	Inspect() string
}

type Integer struct {
	Value int64
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

type Float struct {
	Value float64
}

func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string  { return fmt.Sprintf("%g", f.Value) }

type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }

type Error struct {
	Message string
	Line    int
	Column  int
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	if e.Line > 0 {
		return fmt.Sprintf("ERROR (line %d): %s", e.Line, e.Message)
	}
	return "ERROR: " + e.Message
}

type Function struct {
	// Name is the binding name when known (set for `let f = fn...`,
	// `fn f...`, and class methods); "" for anonymous literals. Used only
	// for error messages.
	Name       string
	Parameters []*ast.Identifier
	Defaults   []ast.Expression // optional default expressions (parallel to Parameters)
	// Wave 5: runtime type contracts. ParamTypes is parallel to Parameters
	// (nil entry = unannotated); ReturnType nil = unannotated. Enforced at
	// call time / on return — NvS has no static checker.
	ParamTypes []*ast.TypeAnnotation
	ReturnType *ast.TypeAnnotation
	Body       *ast.BlockStatement
	Env        *Environment
}

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out bytes.Buffer
	params := []string{}
	for _, p := range f.Parameters {
		params = append(params, p.String())
	}
	out.WriteString("fn")
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") {\n")
	out.WriteString(f.Body.String())
	out.WriteString("\n}")
	return out.String()
}

type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

type BuiltinFunction func(args ...Object) Object

type Builtin struct {
	Fn BuiltinFunction
	// Name is an optional display label (e.g. "partial(add)"). Inspect()
	// stays honest: it is still a builtin function object either way.
	Name string
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string {
	if b.Name != "" {
		return "builtin function " + b.Name
	}
	return "builtin function"
}

// ---- Wave 2: named arguments ----

// NamedArg is the evaluated form of ast.NamedArgument: one `name: value`
// call argument. It only ever exists transiently inside evaluated argument
// lists; call application (applyFunctionNamed / applyMethod / applyCallArgs)
// splits it back out before invoking anything.
type NamedArg struct {
	Name  string
	Value Object
}

func (na *NamedArg) Type() ObjectType { return NAMED_ARG_OBJ }
func (na *NamedArg) Inspect() string  { return na.Name + ": " + na.Value.Inspect() }

type Array struct {
	Elements []Object
	// Frozen marks a deep-frozen (immutable) array. Set by the freeze()
	// builtin; every mutation path in eval must refuse a frozen array.
	Frozen bool
}

func (a *Array) Type() ObjectType { return ARRAY_OBJ }
func (a *Array) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range a.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}

// HashKey for map keys
type HashKey struct {
	Type  ObjectType
	Value uint64
}

type Hashable interface {
	HashKey() HashKey
}

func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	}
	return HashKey{Type: b.Type(), Value: value}
}

func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
}

func (s *String) HashKey() HashKey {
	h := uint64(0)
	for _, c := range s.Value {
		h = h*31 + uint64(c)
	}
	return HashKey{Type: s.Type(), Value: h}
}

type HashPair struct {
	Key   Object
	Value Object
}

type Hash struct {
	Pairs map[HashKey]HashPair
	// Frozen marks a deep-frozen (immutable) hash. Set by the freeze()
	// builtin; every mutation path in eval must refuse a frozen hash.
	Frozen bool
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }
func (h *Hash) Inspect() string {
	var out bytes.Buffer
	pairs := []string{}
	for _, pair := range h.Pairs {
		pairs = append(pairs, pair.Key.Inspect()+": "+pair.Value.Inspect())
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// ---- Wave 3: tuples, records ----

// Tuple is an immutable ordered sequence: (1, 2), (x,), ().
// Tuples are immutable by construction — there is no mutation path that
// accepts one, so no Frozen flag is needed.
type Tuple struct {
	Elements []Object
}

func (t *Tuple) Type() ObjectType { return TUPLE_OBJ }
func (t *Tuple) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range t.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(elements, ", "))
	if len(t.Elements) == 1 {
		out.WriteString(",")
	}
	out.WriteString(")")
	return out.String()
}

// RecordDef is the type object created by `record Point(x, y)`. Calling it
// (Point(1, 2) or Point(x: 1, y: 2)) constructs an immutable Record.
type RecordDef struct {
	Name   string
	Fields []string
}

func (r *RecordDef) Type() ObjectType { return RECORD_DEF_OBJ }
func (r *RecordDef) Inspect() string {
	return "record " + r.Name + "(" + strings.Join(r.Fields, ", ") + ")"
}

// FieldIndex returns the positional index of a field name.
func (r *RecordDef) FieldIndex(name string) (int, bool) {
	for i, f := range r.Fields {
		if f == name {
			return i, true
		}
	}
	return 0, false
}

// Record is an immutable struct value: Point(x=1, y=2).
type Record struct {
	Def    *RecordDef
	Values []Object // parallel to Def.Fields
}

func (r *Record) Type() ObjectType { return RECORD_OBJ }
func (r *Record) Inspect() string {
	parts := []string{}
	for i, f := range r.Def.Fields {
		parts = append(parts, f+"="+r.Values[i].Inspect())
	}
	return r.Def.Name + "(" + strings.Join(parts, ", ") + ")"
}

// Field returns the value of a named field.
func (r *Record) Field(name string) (Object, bool) {
	if idx, ok := r.Def.FieldIndex(name); ok {
		return r.Values[idx], true
	}
	return nil, false
}

type Break struct{ Label string }

func (b *Break) Type() ObjectType { return BREAK_OBJ }
func (b *Break) Inspect() string {
	if b.Label != "" {
		return "break " + b.Label
	}
	return "break"
}

type Continue struct{ Label string }

func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }
func (c *Continue) Inspect() string {
	if c.Label != "" {
		return "continue " + c.Label
	}
	return "continue"
}

type Class struct {
	Name    string
	Parent  *Class
	Methods map[string]*Function
}

func (c *Class) Type() ObjectType { return CLASS_OBJ }
func (c *Class) Inspect() string  { return "class " + c.Name }

func (c *Class) GetMethod(name string) (*Function, bool) {
	if m, ok := c.Methods[name]; ok {
		return m, true
	}
	if c.Parent != nil {
		return c.Parent.GetMethod(name)
	}
	return nil, false
}

type Instance struct {
	Class  *Class
	Fields map[string]Object
}

func (i *Instance) Type() ObjectType { return INSTANCE_OBJ }
func (i *Instance) Inspect() string {
	return "instance of " + i.Class.Name
}

func (i *Instance) Get(name string) (Object, bool) {
	if v, ok := i.Fields[name]; ok {
		return v, true
	}
	// method lookup
	if m, ok := i.Class.GetMethod(name); ok {
		return m, true
	}
	return nil, false
}

func (i *Instance) Set(name string, val Object) {
	i.Fields[name] = val
}

// ---- Wave 5: interfaces (structural contracts, checked at runtime) ----

type Interface struct {
	Name    string
	Methods map[string]*InterfaceMethod // by method name
	Order   []string                    // declaration order (deterministic errors)
}

type InterfaceMethod struct {
	Name       string
	Arity      int // number of declared parameters (signatures have no defaults)
	ParamTypes []*ast.TypeAnnotation
	ReturnType *ast.TypeAnnotation
}

func (i *Interface) Type() ObjectType { return INTERFACE_OBJ }
func (i *Interface) Inspect() string  { return "interface " + i.Name }

type YieldValue struct {
	Value Object
}

func (y *YieldValue) Type() ObjectType { return YIELD_OBJ }
func (y *YieldValue) Inspect() string {
	if y.Value != nil {
		return y.Value.Inspect()
	}
	return "yield"
}

type Generator struct {
	Values    []Object
	Index     int
	Exhausted bool
}

func (g *Generator) Type() ObjectType { return GENERATOR_OBJ }
func (g *Generator) Inspect() string  { return "generator" }

func (g *Generator) Next() Object {
	if g.Index >= len(g.Values) {
		g.Exhausted = true
		return &Null{}
	}
	v := g.Values[g.Index]
	g.Index++
	return v
}

// Environment

type Environment struct {
	store     map[string]Object
	constants map[string]bool
	// Wave 5: declared variable types from `let x: type = ...`. Checked on
	// the initial binding and on every later assignment (AssignStrict path
	// is checked by the evaluator before assigning).
	declaredTypes map[string]*ast.TypeAnnotation
	outer         *Environment
}

func NewEnvironment() *Environment {
	return &Environment{
		store:         make(map[string]Object),
		constants:     make(map[string]bool),
		declaredTypes: make(map[string]*ast.TypeAnnotation),
		outer:         nil,
	}
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

func (e *Environment) SetConst(name string, val Object) Object {
	e.store[name] = val
	e.constants[name] = true
	return val
}

func (e *Environment) IsConst(name string) bool {
	if e.constants[name] {
		return true
	}
	if e.outer != nil {
		return e.outer.IsConst(name)
	}
	return false
}

func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil {
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}

// DeclareType remembers the annotated type of a `let name: type = ...`
// binding in this environment. Re-declaring a name replaces the type.
func (e *Environment) DeclareType(name string, ann *ast.TypeAnnotation) {
	if e.declaredTypes == nil {
		e.declaredTypes = make(map[string]*ast.TypeAnnotation)
	}
	e.declaredTypes[name] = ann
}

// LookupDeclaredType finds the annotated type declared for name, walking
// outward through enclosing environments. The second return value reports
// whether any declaration was found.
func (e *Environment) LookupDeclaredType(name string) (*ast.TypeAnnotation, bool) {
	if ann, ok := e.declaredTypes[name]; ok {
		return ann, true
	}
	if e.outer != nil {
		return e.outer.LookupDeclaredType(name)
	}
	return nil, false
}

// Assign updates name in the environment where it was defined.
// Returns false if name is not found in the chain.
func (e *Environment) Assign(name string, val Object) bool {
	if _, ok := e.store[name]; ok {
		if e.constants[name] {
			return false // signal const violation via caller
		}
		e.store[name] = val
		return true
	}
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}
	return false
}

func (e *Environment) AssignStrict(name string, val Object) (bool, string) {
	if _, ok := e.store[name]; ok {
		if e.constants[name] {
			return false, "cannot assign to const: " + name
		}
		e.store[name] = val
		return true, ""
	}
	if e.outer != nil {
		return e.outer.AssignStrict(name, val)
	}
	return false, ""
}
