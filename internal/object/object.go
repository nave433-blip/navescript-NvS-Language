package object

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"

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
	// Wave 6: cooperative concurrency — tasks and channels.
	TASK_OBJ    = "TASK"
	CHANNEL_OBJ = "CHANNEL"
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
	// Wave 17: file where the function was defined (eval.CurrentFile at
	// creation). The evaluator switches to it while running the body so
	// debugger/profiler hooks and nested imports attribute correctly.
	// "" when defined in a context without a file (REPL, -e).
	SourceFile string
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

// Environment maps names to values. Wave 6: the store is guarded by an
// RWMutex so that concurrent tasks cannot corrupt the map itself when they
// touch top-level bindings. This gives last-writer-wins semantics for racy
// programs — it does NOT make sharing mutable state between tasks safe.
// That is a data race and a bug in the user program; tasks must communicate
// via channels, whose values are deep-copied at send time.
type Environment struct {
	mu        sync.RWMutex
	store     map[string]Object
	constants map[string]bool
	// Wave 5: declared variable types from `let x: type = ...`. Checked on
	// the initial binding and on every later assignment (AssignStrict path
	// is checked by the evaluator before assigning).
	declaredTypes map[string]*ast.TypeAnnotation
	outer         *Environment
	// Wave 17: source file whose code this environment belongs to, for
	// debugger/profiler hook attribution. Inherited by enclosed scopes;
	// the evaluator sets it to a function's SourceFile on call and to
	// the imported file during nested imports. Access via GetSourceFile /
	// SetSourceFile: spawned tasks evaluate on other goroutines.
	sourceFile string
	// Wave 11: yield collection for generator functions. When a function
	// body is evaluated by evalCollectingYields, it installs a sink on the
	// call's own environment; evalYieldStatement appends to the NEAREST
	// sink up the scope chain, so yields inside loops/if/try blocks (which
	// evaluate in enclosed environments) are collected instead of being
	// swallowed. Each function call installs its own sink, so a yield in a
	// nested function call belongs to the inner function, never the outer.
	YieldSink *[]Object
}

// NearestYieldSink walks the scope chain outward and returns the closest
// installed yield sink, or nil if no enclosing function is collecting.
func (e *Environment) NearestYieldSink() *[]Object {
	for env := e; env != nil; env = env.outer {
		if env.YieldSink != nil {
			return env.YieldSink
		}
	}
	return nil
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
	if outer != nil {
		// Wave 17: scopes inherit their file for hook attribution.
		env.sourceFile = outer.GetSourceFile()
	}
	return env
}

// GetSourceFile reports the source file this environment's code belongs
// to ("" when unknown). Wave 17: debugger/profiler attribution.
func (e *Environment) GetSourceFile() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sourceFile
}

// SetSourceFile sets the source file for this environment's code.
// Wave 17: debugger/profiler attribution.
func (e *Environment) SetSourceFile(f string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sourceFile = f
}

func (e *Environment) SetConst(name string, val Object) Object {
	e.mu.Lock()
	e.store[name] = val
	e.constants[name] = true
	e.mu.Unlock()
	return val
}

func (e *Environment) IsConst(name string) bool {
	e.mu.RLock()
	if e.constants[name] {
		e.mu.RUnlock()
		return true
	}
	outer := e.outer
	e.mu.RUnlock()
	if outer != nil {
		return outer.IsConst(name)
	}
	return false
}

func (e *Environment) Get(name string) (Object, bool) {
	e.mu.RLock()
	obj, ok := e.store[name]
	outer := e.outer
	e.mu.RUnlock()
	if !ok && outer != nil {
		return outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.mu.Lock()
	e.store[name] = val
	e.mu.Unlock()
	return val
}

// Names returns the sorted names bound in this environment frame only
// (not outer scopes). Wave 17: used by the terminal debugger's `locals`.
// The returned slice is a copy; the environment is not retained.
func (e *Environment) Names() []string {
	e.mu.RLock()
	names := make([]string, 0, len(e.store))
	for n := range e.store {
		names = append(names, n)
	}
	e.mu.RUnlock()
	sort.Strings(names)
	return names
}

// DeclareType remembers the annotated type of a `let name: type = ...`
// binding in this environment. Re-declaring a name replaces the type.
func (e *Environment) DeclareType(name string, ann *ast.TypeAnnotation) {
	e.mu.Lock()
	if e.declaredTypes == nil {
		e.declaredTypes = make(map[string]*ast.TypeAnnotation)
	}
	e.declaredTypes[name] = ann
	e.mu.Unlock()
}

// LookupDeclaredType finds the annotated type declared for name, walking
// outward through enclosing environments. The second return value reports
// whether any declaration was found.
func (e *Environment) LookupDeclaredType(name string) (*ast.TypeAnnotation, bool) {
	e.mu.RLock()
	ann, ok := e.declaredTypes[name]
	outer := e.outer
	e.mu.RUnlock()
	if ok {
		return ann, true
	}
	if outer != nil {
		return outer.LookupDeclaredType(name)
	}
	return nil, false
}

// Assign updates name in the environment where it was defined.
// Returns false if name is not found in the chain.
func (e *Environment) Assign(name string, val Object) bool {
	e.mu.Lock()
	if _, ok := e.store[name]; ok {
		if e.constants[name] {
			e.mu.Unlock()
			return false // signal const violation via caller
		}
		e.store[name] = val
		e.mu.Unlock()
		return true
	}
	outer := e.outer
	e.mu.Unlock()
	if outer != nil {
		return outer.Assign(name, val)
	}
	return false
}

func (e *Environment) AssignStrict(name string, val Object) (bool, string) {
	e.mu.Lock()
	if _, ok := e.store[name]; ok {
		if e.constants[name] {
			e.mu.Unlock()
			return false, "cannot assign to const: " + name
		}
		e.store[name] = val
		e.mu.Unlock()
		return true, ""
	}
	outer := e.outer
	e.mu.Unlock()
	if outer != nil {
		return outer.AssignStrict(name, val)
	}
	return false, ""
}

// ---- Wave 6: cooperative concurrency — tasks and channels ----

// Task is a handle to a concurrently running function invocation. It is a
// communication handle, never a value: it passes by reference and is never
// deep-copied.
type Task struct {
	ID   int64
	done chan struct{} // closed exactly once, when the task finishes
	mu   sync.Mutex
	res  Object // set before done is closed
}

func NewTask(id int64) *Task {
	return &Task{ID: id, done: make(chan struct{})}
}

func (t *Task) Type() ObjectType { return TASK_OBJ }
func (t *Task) Inspect() string {
	if t.IsDone() {
		return fmt.Sprintf("task #%d (done)", t.ID)
	}
	return fmt.Sprintf("task #%d (running)", t.ID)
}

// Finish records the task result (a return value, or an *Error if the task
// raised) and wakes all joiners. It must be called exactly once.
func (t *Task) Finish(res Object) {
	t.mu.Lock()
	t.res = res
	t.mu.Unlock()
	close(t.done)
}

// Wait blocks until the task finishes and returns its result.
func (t *Task) Wait() Object {
	<-t.done
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.res
}

// IsDone reports whether the task has finished, without blocking.
func (t *Task) IsDone() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

// Channel is a typed message queue between tasks. Values sent through it
// are deep-copied by the sender (see eval.deepCopyMessage); the Channel
// handle itself passes by reference and is never copied.
type Channel struct {
	ch     chan Object
	cap    int
	mu     sync.Mutex
	closed bool
}

func NewChannel(capacity int) *Channel {
	if capacity < 0 {
		capacity = 0
	}
	return &Channel{ch: make(chan Object, capacity), cap: capacity}
}

func (c *Channel) Type() ObjectType { return CHANNEL_OBJ }
func (c *Channel) Inspect() string {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return "channel (closed)"
	}
	if c.cap == 0 {
		return "channel (unbuffered)"
	}
	return fmt.Sprintf("channel (buffered, cap %d)", c.cap)
}

// Capacity returns the buffer capacity (0 = unbuffered rendezvous channel).
func (c *Channel) Capacity() int { return c.cap }

func (c *Channel) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// Close marks the channel closed. Receivers drain remaining values, then
// see null. Reports false if the channel was already closed.
func (c *Channel) Close() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	c.closed = true
	close(c.ch)
	return true
}

// Send blocks until a receiver takes the value (rendezvous when
// unbuffered). It panics if the channel is closed — callers recover and
// report an NvS error instead.
func (c *Channel) Send(v Object) { c.ch <- v }

// Recv blocks until a value arrives. ok is false once the channel is
// closed AND drained.
func (c *Channel) Recv() (v Object, ok bool) {
	v, ok = <-c.ch
	return v, ok
}

// TrySend never blocks: it reports whether the value was accepted. It
// panics if the channel is closed — callers check IsClosed first and
// recover as a backstop.
func (c *Channel) TrySend(v Object) bool {
	select {
	case c.ch <- v:
		return true
	default:
		return false
	}
}

// TryRecv never blocks. received is false when the channel is momentarily
// empty OR closed-and-drained; both surface as [false, null] in NvS.
func (c *Channel) TryRecv() (v Object, received bool) {
	select {
	case v, ok := <-c.ch:
		if !ok {
			return nil, false
		}
		return v, true
	default:
		return nil, false
	}
}
