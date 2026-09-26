// Package transpile implements an honest-subset source-to-source transpiler
// from NvS to JavaScript and Python.
//
// Design contract:
//
//   - The transpiler covers a DOCUMENTED safe subset (see the subset table
//     in LANGUAGE.md). The subset was chosen so that every construct in it
//     has identical semantics in NvS, JS, and Python — with a small runtime
//     prelude of helpers (__div, __str, __truthy, ...) emitted alongside the
//     code wherever the targets would otherwise differ from NvS (integer
//     division, NvS truthiness where only null/false are falsy, Inspect-based
//     string coercion, Go %g float formatting).
//   - ANYTHING outside the subset is rejected with an *UnsupportedError that
//     names the construct, e.g.
//
//       transpile error: unsupported construct: MatchExpression (pattern matching is outside the transpilable subset)
//
//     The transpiler never emits silently-wrong code for an unsupported
//     construct.
//   - The transpiler assumes the input program runs without errors in NvS.
//     Error behavior (const violations, type errors, arity errors, ...) is
//     not replicated; erroneous programs are outside the contract.
//   - Comments are dropped. Type annotations are erased (no runtime
//     enforcement in the emitted code).
package transpile

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// Targets returns the supported output targets.
func Targets() []string { return []string{"js", "python"} }

// UnsupportedError is returned when the input uses a construct outside the
// transpilable subset. It names the construct so the user knows exactly
// what to rewrite.
type UnsupportedError struct {
	Construct string // e.g. "MatchExpression"
	Detail    string // e.g. "pattern matching is outside the transpilable subset"
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("transpile error: unsupported construct: %s (%s)", e.Construct, e.Detail)
}

// unsupported builds an *UnsupportedError for an AST node, using the hint
// table below where one exists.
func unsupported(node ast.Node, fallback string) *UnsupportedError {
	name := fmt.Sprintf("%T", node)
	name = strings.TrimPrefix(name, "*ast.")
	detail, ok := unsupportedHints[name]
	if !ok {
		detail = fallback
	}
	return &UnsupportedError{Construct: name, Detail: detail}
}

var unsupportedHints = map[string]string{
	"MatchExpression":          "pattern matching is outside the transpilable subset",
	"ClassStatement":           "classes are outside the transpilable subset",
	"InterfaceDecl":            "interfaces are outside the transpilable subset",
	"EnumStatement":            "enums are outside the transpilable subset",
	"RecordStatement":          "records are outside the transpilable subset",
	"TryStatement":             "exceptions (try/catch/throw) are outside the transpilable subset",
	"ThrowStatement":           "exceptions (try/catch/throw) are outside the transpilable subset",
	"DeferStatement":           "defer is outside the transpilable subset",
	"YieldStatement":           "generators are outside the transpilable subset",
	"DecoratorStatement":       "decorators are outside the transpilable subset",
	"DestructureLetStatement":  "destructuring bindings are outside the transpilable subset",
	"SpreadExpression":         "spread is outside the transpilable subset",
	"OptionalChainExpression":  "optional chaining is outside the transpilable subset",
	"RangeExpression":          "ranges (a..b) are outside the transpilable subset; use range(a, b)",
	"TupleLiteral":             "tuples are outside the transpilable subset",
	"NewExpression":            "classes are outside the transpilable subset",
	"ThisExpression":           "classes are outside the transpilable subset",
	"MemberExpression":         "member access is outside the transpilable subset",
	"MemberAssignExpression":   "member assignment is outside the transpilable subset",
	"ImportStatement":          "modules are outside the transpilable subset",
	"NamedArgument":            "named arguments are outside the transpilable subset",
	"ArrayPattern":             "patterns are outside the transpilable subset",
	"HashPattern":              "patterns are outside the transpilable subset",
	"ChainLink":                "pipeline/chains are outside the transpilable subset",
}

// TranspileSource parses NvS source and transpiles it to target
// ("js" or "python"). Parse errors are returned as plain errors.
func TranspileSource(src, targetName string) (string, error) {
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return "", fmt.Errorf("transpile error: parse error: %s", strings.Join(errs, "; "))
	}
	return TranspileProgram(prog, targetName)
}

type target int

const (
	targetJS target = iota
	targetPython
)

// TranspileProgram transpiles an already-parsed program.
func TranspileProgram(prog *ast.Program, targetName string) (string, error) {
	var t target
	switch targetName {
	case "js", "javascript":
		t = targetJS
	case "py", "python":
		t = targetPython
	default:
		return "", fmt.Errorf("transpile error: unknown target %q (want \"js\" or \"python\")", targetName)
	}
	e := &emitter{t: t, builtins: map[string]bool{}}
	for _, name := range eval.BuiltinNames() {
		e.builtins[name] = true
	}
	e.scopes = []*fnScope{newFnScope()}
	// Pre-analyze the top level so functions defined before the bindings
	// they assign still get correct nonlocal/global declarations in the
	// Python target regardless of definition order.
	for _, s := range prog.Statements {
		analyzeStmtNames(s, e.scopes[0])
	}
	e.writePrelude()
	for _, s := range prog.Statements {
		e.stmt(s)
		if e.err != nil {
			return "", e.err
		}
	}
	return e.sb.String(), nil
}

// ---------------------------------------------------------------------------
// Emitter state
// ---------------------------------------------------------------------------

type fnScope struct {
	params   map[string]bool
	declared map[string]bool // let/const bindings (NvS is function-scoped)
	assigned map[string]bool // plain `x = ...` not covered by params/declared
}

func newFnScope() *fnScope {
	return &fnScope{
		params:   map[string]bool{},
		declared: map[string]bool{},
		assigned: map[string]bool{},
	}
}

type loopKind int

const (
	loopCFor loopKind = iota
	loopForIn
	loopWhile
)

type loopInfo struct {
	kind loopKind
	post ast.Expression // C-style for post expression (nil if none)
}

type emitter struct {
	t        target
	sb       strings.Builder
	indent   int
	err      error
	builtins map[string]bool
	scopes   []*fnScope // scopes[0] is the top level; pushed per function
	loops    []loopInfo // innermost loop last; cleared on function entry
}

func (e *emitter) fail(err error) {
	if e.err == nil {
		e.err = err
	}
}

func (e *emitter) line(s string) {
	for i := 0; i < e.indent; i++ {
		e.sb.WriteString(e.t.indentUnit())
	}
	e.sb.WriteString(s)
	e.sb.WriteString("\n")
}

func (t target) indentUnit() string {
	if t == targetJS {
		return "  "
	}
	return "    "
}

func (e *emitter) endStmt(s string) string {
	if e.t == targetJS {
		return s + ";"
	}
	return s
}

// ---------------------------------------------------------------------------
// Runtime preludes — helpers that replicate NvS semantics where the targets
// would otherwise differ. Emitted at the top of every transpiled file.
// ---------------------------------------------------------------------------

const jsPrelude = `// Generated by ` + "`nvs transpile --to=js`" + ` — NvS subset transpiler.
// Runtime prelude: these helpers replicate NvS semantics where plain JS
// would differ. See LANGUAGE.md ("Transpiler subset") for the contract.
// NvS truthiness: only null and false are falsy (0, "", [] are truthy).
function __truthy(x) { return x !== null && x !== undefined && x !== false; }
// NvS integer division truncates toward zero; float division is exact.
function __div(a, b) {
  if (b === 0) throw new Error("division by zero");
  if (Number.isInteger(a) && Number.isInteger(b)) return Math.trunc(a / b);
  return a / b;
}
// NvS ** : integer power for ints (negative exponent -> 0), Math.pow otherwise.
function __pow(a, b) {
  if (Number.isInteger(a) && Number.isInteger(b)) {
    if (b < 0) return 0;
    let r = 1;
    for (let i = 0; i < b; i++) r *= a;
    return r;
  }
  return Math.pow(a, b);
}
// NvS % is the truncated remainder on integers only.
function __mod(a, b) {
  if (!Number.isInteger(a) || !Number.isInteger(b)) throw new Error("unknown operator: non-integer %");
  return a % b;
}
// Go %g float formatting, as NvS print uses it.
function __fmt_num(f) {
  if (Number.isNaN(f)) return "NaN";
  if (f === Infinity) return "+Inf";
  if (f === -Infinity) return "-Inf";
  if (f === 0) return 1 / f < 0 ? "-0" : "0";
  const neg = f < 0, af = neg ? -f : f;
  let s = String(af), mant = s, exp = 0;
  const ei = s.indexOf("e");
  if (ei >= 0) { mant = s.slice(0, ei); exp = parseInt(s.slice(ei + 1), 10); }
  let ip = mant.indexOf("."), digits, E, lz = 0;
  if (ip < 0) { digits = mant; E = exp + digits.length - 1; }
  else { digits = mant.replace(".", ""); E = exp + ip - 1; }
  while (lz < digits.length && digits[lz] === "0") lz++;
  digits = digits.slice(lz);
  E -= lz;
  let sig = digits.replace(/0+$/, "");
  if (sig === "") sig = "0";
  let out;
  if (E >= -4 && E < 6) {
    if (E >= 0) {
      out = sig.length <= E + 1 ? sig + "0".repeat(E + 1 - sig.length)
                               : sig.slice(0, E + 1) + "." + sig.slice(E + 1);
    } else {
      out = "0." + "0".repeat(-E - 1) + sig;
    }
  } else {
    const m = sig[0] + (sig.length > 1 ? "." + sig.slice(1) : "");
    const es = (E < 0 ? "-" : "+") + String(Math.abs(E)).padStart(2, "0");
    out = m + "e" + es;
  }
  return neg ? "-" + out : out;
}
// NvS Inspect-compatible string coercion (print / str() / interpolation).
function __str(x) {
  if (x === null || x === undefined) return "null";
  if (x === true) return "true";
  if (x === false) return "false";
  if (typeof x === "number") return Number.isInteger(x) ? String(x) : __fmt_num(x);
  if (typeof x === "string") return x;
  if (Array.isArray(x)) return "[" + x.map(__str).join(", ") + "]";
  if (typeof x === "object")
    return "{" + Object.entries(x).map(([k, v]) => __str(k) + ": " + __str(v)).join(", ") + "}";
  throw new Error("cannot convert value to string");
}
// NvS len() over strings, arrays, and hashes.
function __len(x) {
  if (typeof x === "string" || Array.isArray(x)) return x.length;
  if (x !== null && typeof x === "object") return Object.keys(x).length;
  throw new Error("argument to 'len' not supported");
}
// NvS push(a, x): appends AND returns the array (unlike Array.push).
function __push(a, x) { a.push(x); return a; }
// NvS range() builtin (integer semantics).
function __range(start, end, step) {
  if (end === undefined) { end = start; start = 0; }
  if (step === undefined) step = 1;
  if (step === 0) throw new Error("range: step cannot be 0");
  const out = [];
  if (step > 0) { for (let i = start; i < end; i += step) out.push(i); }
  else { for (let i = start; i > end; i += step) out.push(i); }
  return out;
}
// NvS for-in iterates array values, string chars (code points), hash keys.
function __iter(x) {
  if (typeof x === "string") return Array.from(x);
  if (Array.isArray(x)) return x;
  if (x !== null && typeof x === "object") return Object.keys(x);
  throw new Error("for-in not supported on this value");
}
// Clamped slice replicating NvS a[s:e] (negative end = to end).
function __slice(a, s, e) {
  const isStr = typeof a === "string";
  const cp = isStr ? Array.from(a) : a;
  const n = cp.length;
  if (e < 0) e = n;
  if (s < 0) s = 0;
  if (s > n) s = n;
  if (e > n) e = n;
  if (s > e) return isStr ? "" : [];
  const part = cp.slice(s, e);
  return isStr ? part.join("") : part;
}
`

const pythonPrelude = `# Generated by ` + "`nvs transpile --to=python`" + ` — NvS subset transpiler.
# Runtime prelude: these helpers replicate NvS semantics where plain Python
# would differ. See LANGUAGE.md ("Transpiler subset") for the contract.
import math


# NvS truthiness: only null and false are falsy (0, "", [] are truthy).
def __truthy(x):
    return x is not None and x is not False


# NvS integer division truncates toward zero; float division is exact.
def __div(a, b):
    if b == 0:
        raise ZeroDivisionError("division by zero")
    if type(a) is int and type(b) is int:
        q = abs(a) // abs(b)
        return q if (a < 0) == (b < 0) else -q
    return a / b


# NvS ** : integer power for ints (negative exponent -> 0), ** otherwise.
def __pow(a, b):
    if type(a) is int and type(b) is int:
        if b < 0:
            return 0
        r = 1
        for _ in range(b):
            r *= a
        return r
    return a ** b


# NvS % is the truncated remainder on integers only.
def __mod(a, b):
    if type(a) is int and type(b) is int:
        return a - __div(a, b) * b
    raise Exception("unknown operator: non-integer %")


# Go %g float formatting, as NvS print uses it.
def __fmt_float(f):
    if math.isnan(f):
        return "NaN"
    if math.isinf(f):
        return "+Inf" if f > 0 else "-Inf"
    if f == 0.0:
        return "-0" if math.copysign(1.0, f) < 0.0 else "0"
    neg = f < 0.0
    af = -f if neg else f
    s = repr(af)
    mant, _, exp = s.partition("e")
    exp = int(exp) if exp else 0
    ip = mant.find(".")
    if ip < 0:
        digits, E = mant, exp + len(mant) - 1
    else:
        digits, E = mant.replace(".", ""), exp + ip - 1
    lz = len(digits) - len(digits.lstrip("0"))
    digits = digits[lz:]
    E -= lz
    sig = digits.rstrip("0") or "0"
    if -4 <= E < 6:
        if E >= 0:
            if len(sig) <= E + 1:
                out = sig + "0" * (E + 1 - len(sig))
            else:
                out = sig[:E + 1] + "." + sig[E + 1:]
        else:
            out = "0." + "0" * (-E - 1) + sig
    else:
        m = sig[0] + ("." + sig[1:] if len(sig) > 1 else "")
        es = ("+" if E >= 0 else "-") + str(abs(E)).zfill(2)
        out = m + "e" + es
    return ("-" + out) if neg else out


# NvS Inspect-compatible string coercion (print / str() / interpolation).
def __str(x):
    if x is None:
        return "null"
    if x is True:
        return "true"
    if x is False:
        return "false"
    if isinstance(x, int):
        return str(x)
    if isinstance(x, float):
        return __fmt_float(x)
    if isinstance(x, str):
        return x
    if isinstance(x, list):
        return "[" + ", ".join(__str(e) for e in x) + "]"
    if isinstance(x, dict):
        return "{" + ", ".join(__str(k) + ": " + __str(v) for k, v in x.items()) + "}"
    raise Exception("cannot convert value to string")


# NvS == : numbers compare numerically, strings/bools by value,
# null by identity, arrays/hashes/functions by identity.
def __nvs_eq(a, b):
    if a is None or b is None:
        return a is None and b is None
    if isinstance(a, bool) or isinstance(b, bool):
        return type(a) is type(b) and a == b
    if isinstance(a, (int, float)) and isinstance(b, (int, float)):
        return a == b
    if isinstance(a, str) and isinstance(b, str):
        return a == b
    return a is b


# NvS push(a, x): appends AND returns the array.
def __push(a, x):
    a.append(x)
    return a


# NvS ?? — thunks keep the short-circuit (right side unevaluated).
def __coalesce(fa, fb):
    v = fa()
    return v if v is not None else fb()


# Clamped slice replicating NvS a[s:e] (negative end = to end).
def __slice(a, s, e):
    n = len(a)
    if e < 0:
        e = n
    if s < 0:
        s = 0
    if s > n:
        s = n
    if e > n:
        e = n
    if s > e:
        return "" if isinstance(a, str) else []
    return a[s:e]


`

func (e *emitter) writePrelude() {
	if e.t == targetJS {
		e.sb.WriteString(jsPrelude)
	} else {
		e.sb.WriteString(pythonPrelude)
	}
}

// ---------------------------------------------------------------------------
// Scope analysis — NvS lets closures assign to outer bindings
// (`let x = 1; fn f() { x = 2 }`). Python needs an explicit `nonlocal` /
// `global` declaration for that, which must precede the body. This pass
// collects, per function, the names it assigns that are bound outside it.
// NvS bindings are function-scoped (blocks share the function env), so the
// analysis treats blocks as transparent.
// ---------------------------------------------------------------------------

func analyzeFunction(fl *ast.FunctionLiteral) *fnScope {
	sc := newFnScope()
	for _, p := range fl.Parameters {
		sc.params[p.Value] = true
	}
	for _, d := range fl.Defaults {
		if d != nil {
			// Defaults evaluate in the ENCLOSING scope at call time.
			analyzeExprNames(d, sc)
		}
	}
	if fl.Body != nil {
		analyzeBlockNames(fl.Body, sc)
	}
	return sc
}

func analyzeBlockNames(b *ast.BlockStatement, sc *fnScope) {
	for _, s := range b.Statements {
		analyzeStmtNames(s, sc)
	}
}

func analyzeStmtNames(s ast.Statement, sc *fnScope) {
	switch n := s.(type) {
	case *ast.LetStatement:
		sc.declared[n.Name.Value] = true
		analyzeExprNames(n.Value, sc)
	case *ast.ConstStatement:
		sc.declared[n.Name.Value] = true
		analyzeExprNames(n.Value, sc)
	case *ast.TypedLetStatement:
		sc.declared[n.Name.Value] = true
		analyzeExprNames(n.Value, sc)
	case *ast.ExpressionStatement:
		analyzeExprNames(n.Expression, sc)
	case *ast.ReturnStatement:
		if n.ReturnValue != nil {
			analyzeExprNames(n.ReturnValue, sc)
		}
	case *ast.WhileStatement:
		analyzeExprNames(n.Condition, sc)
		analyzeBlockNames(n.Body, sc)
	case *ast.ForStatement:
		if n.Init != nil {
			analyzeStmtNames(n.Init, sc)
		}
		if n.Condition != nil {
			analyzeExprNames(n.Condition, sc)
		}
		if n.Post != nil {
			analyzeExprNames(n.Post, sc)
		}
		analyzeBlockNames(n.Body, sc)
	case *ast.ForInStatement:
		sc.declared[n.Name.Value] = true
		analyzeExprNames(n.Iterable, sc)
		analyzeBlockNames(n.Body, sc)
	case *ast.PrintStatement:
		analyzeExprNames(n.Value, sc)
	}
}

func analyzeExprNames(x ast.Expression, sc *fnScope) {
	if x == nil {
		return
	}
	switch n := x.(type) {
	case *ast.FunctionLiteral:
		// Nested function: its own scope, analyzed separately at emit
		// time. Its defaults belong to THIS scope (see analyzeFunction).
		for _, d := range n.Defaults {
			if d != nil {
				analyzeExprNames(d, sc)
			}
		}
	case *ast.AssignExpression:
		if !sc.declared[n.Name.Value] && !sc.params[n.Name.Value] {
			sc.assigned[n.Name.Value] = true
		}
		analyzeExprNames(n.Value, sc)
	case *ast.PrefixExpression:
		analyzeExprNames(n.Right, sc)
	case *ast.InfixExpression:
		analyzeExprNames(n.Left, sc)
		analyzeExprNames(n.Right, sc)
	case *ast.TernaryExpression:
		analyzeExprNames(n.Condition, sc)
		analyzeExprNames(n.Consequence, sc)
		analyzeExprNames(n.Alternative, sc)
	case *ast.IfExpression:
		analyzeExprNames(n.Condition, sc)
		analyzeBlockNames(n.Consequence, sc)
		if n.Alternative != nil {
			analyzeBlockNames(n.Alternative, sc)
		}
	case *ast.CallExpression:
		analyzeExprNames(n.Function, sc)
		for _, a := range n.Arguments {
			analyzeExprNames(a, sc)
		}
	case *ast.ArrayLiteral:
		for _, el := range n.Elements {
			analyzeExprNames(el, sc)
		}
	case *ast.HashLiteral:
		for k, v := range n.Pairs {
			analyzeExprNames(k, sc)
			analyzeExprNames(v, sc)
		}
		for _, sp := range n.Spreads {
			analyzeExprNames(sp, sc)
		}
	case *ast.IndexExpression:
		analyzeExprNames(n.Left, sc)
		analyzeExprNames(n.Index, sc)
		if n.End != nil {
			analyzeExprNames(n.End, sc)
		}
	case *ast.IndexAssignExpression:
		analyzeExprNames(n.Left, sc)
		analyzeExprNames(n.Value, sc)
	case *ast.InterpolatedString:
		for _, p := range n.Parts {
			analyzeExprNames(p, sc)
		}
	}
}

// outerBindings reports, for each name assigned in sc but not local to it,
// whether an enclosing function scope binds it ("nonlocal") or the top
// level binds it ("global"). Names bound nowhere are left alone: like NvS,
// Python then treats the assignment as function-local.
func (e *emitter) outerBindings(sc *fnScope) (nonlocal, global []string) {
	seen := map[string]bool{}
	for name := range sc.assigned {
		if sc.params[name] || sc.declared[name] || seen[name] {
			continue
		}
		seen[name] = true
		found := false
		// Search enclosing function scopes (innermost first), skipping
		// scopes[0] which is the top level.
		for i := len(e.scopes) - 1; i >= 1; i-- {
			outer := e.scopes[i]
			if outer.params[name] || outer.declared[name] {
				nonlocal = append(nonlocal, name)
				found = true
				break
			}
		}
		if !found && len(e.scopes) > 0 {
			top := e.scopes[0]
			if top.declared[name] {
				global = append(global, name)
			}
		}
	}
	return nonlocal, global
}

// ---------------------------------------------------------------------------
// Statement emission
// ---------------------------------------------------------------------------

func (e *emitter) stmt(s ast.Statement) {
	if e.err != nil {
		return
	}
	switch n := s.(type) {
	case *ast.LetStatement:
		e.emitBinding(n.Name.Value, n.Value)
	case *ast.ConstStatement:
		e.emitBinding(n.Name.Value, n.Value)
	case *ast.TypedLetStatement:
		// Type annotations are erased: the transpiler emits no runtime
		// enforcement (documented in the subset table).
		e.emitBinding(n.Name.Value, n.Value)
	case *ast.ExpressionStatement:
		if ie, ok := n.Expression.(*ast.IfExpression); ok {
			e.emitIf(ie)
		} else {
			e.line(e.endStmt(e.stmtExpr(n.Expression)))
		}
	case *ast.ReturnStatement:
		if n.ReturnValue != nil {
			e.line(e.endStmt("return " + e.expr(n.ReturnValue)))
		} else {
			e.line(e.endStmt("return"))
		}
	case *ast.PrintStatement:
		e.line(e.endStmt(e.printCall(e.expr(n.Value))))
	case *ast.WhileStatement:
		e.emitWhile(n)
	case *ast.ForStatement:
		e.emitFor(n)
	case *ast.ForInStatement:
		e.emitForIn(n)
	case *ast.BreakStatement:
		if n.Label != "" {
			e.fail(unsupported(n, "labeled break is outside the transpilable subset"))
			return
		}
		e.line(e.endStmt("break"))
	case *ast.ContinueStatement:
		if n.Label != "" {
			e.fail(unsupported(n, "labeled continue is outside the transpilable subset"))
			return
		}
		e.emitContinue()
	default:
		e.fail(unsupported(s, "this statement is outside the transpilable subset"))
	}
}

// emitBinding handles let/const/typed-let. A FunctionLiteral value becomes a
// named function declaration; anything else becomes a plain binding.
// NvS bindings are function-scoped, so JS emits `var` (not `let`: `let` is
// block-scoped and would diverge for bindings declared inside blocks).
func (e *emitter) emitBinding(name string, value ast.Expression) {
	if fl, ok := value.(*ast.FunctionLiteral); ok {
		e.emitFunction(name, fl)
		return
	}
	v := e.expr(value)
	if e.err != nil {
		return
	}
	if e.t == targetJS {
		e.line(e.endStmt("var " + name + " = " + v))
	} else {
		e.line(e.endStmt(name + " = " + v))
	}
}

func (e *emitter) printCall(arg string) string {
	if e.t == targetJS {
		return "console.log(__str(" + arg + "))"
	}
	return "print(__str(" + arg + "))"
}

func (e *emitter) emitBlock(b *ast.BlockStatement) {
	e.indent++
	for _, s := range b.Statements {
		e.stmt(s)
		if e.err != nil {
			return
		}
	}
	e.indent--
}

func (e *emitter) emitIf(n *ast.IfExpression) {
	cond := e.expr(n.Condition)
	if e.err != nil {
		return
	}
	if e.t == targetJS {
		e.line("if (__truthy(" + cond + ")) {")
	} else {
		e.line("if __truthy(" + cond + "):")
	}
	e.emitBlock(n.Consequence)
	if n.Alternative != nil {
		if e.t == targetJS {
			e.line("} else {")
		} else {
			e.line("else:")
		}
		e.emitBlock(n.Alternative)
	}
	if e.t == targetJS {
		e.line("}")
	}
}

func (e *emitter) emitWhile(n *ast.WhileStatement) {
	if n.OrElse != nil {
		e.fail(unsupported(n, "while-else is outside the transpilable subset"))
		return
	}
	if n.Label != "" {
		e.fail(unsupported(n, "labeled loops are outside the transpilable subset"))
		return
	}
	cond := e.expr(n.Condition)
	if e.err != nil {
		return
	}
	e.loops = append(e.loops, loopInfo{kind: loopWhile})
	if e.t == targetJS {
		e.line("while (__truthy(" + cond + ")) {")
	} else {
		e.line("while __truthy(" + cond + "):")
	}
	e.emitBlock(n.Body)
	if e.t == targetJS {
		e.line("}")
	}
	e.loops = e.loops[:len(e.loops)-1]
}

// emitFor handles the C-style for. JS maps directly. Python desugars to a
// while loop; a `continue` that targets this loop runs the post-expression
// first (matching NvS, where continue still runs post), while `break`
// skips it. Only direct (non-nested) break/continue are affected.
func (e *emitter) emitFor(n *ast.ForStatement) {
	if n.OrElse != nil {
		e.fail(unsupported(n, "for-else is outside the transpilable subset"))
		return
	}
	if n.Label != "" {
		e.fail(unsupported(n, "labeled loops are outside the transpilable subset"))
		return
	}
	if e.t == targetJS {
		init := ""
		if n.Init != nil {
			init = e.forInitJS(n.Init)
		}
		cond := ""
		if n.Condition != nil {
			cond = "__truthy(" + e.expr(n.Condition) + ")"
		}
		post := ""
		if n.Post != nil {
			post = e.expr(n.Post)
		}
		if e.err != nil {
			return
		}
		e.loops = append(e.loops, loopInfo{kind: loopCFor})
		e.line("for (" + init + "; " + cond + "; " + post + ") {")
		e.emitBlock(n.Body)
		e.line("}")
		e.loops = e.loops[:len(e.loops)-1]
		return
	}
	// Python desugar.
	if n.Init != nil {
		e.forInitPy(n.Init)
		if e.err != nil {
			return
		}
	}
	if n.Condition != nil {
		cond := e.expr(n.Condition)
		if e.err != nil {
			return
		}
		e.line("while __truthy(" + cond + "):")
	} else {
		e.line("while True:")
	}
	e.loops = append(e.loops, loopInfo{kind: loopCFor, post: n.Post})
	e.emitBlock(n.Body)
	if n.Post != nil {
		e.indent++
		e.line(e.endStmt(e.stmtExpr(n.Post)))
		e.indent--
	}
	if e.t == targetJS {
		// unreachable
	}
	e.loops = e.loops[:len(e.loops)-1]
}

// forInitJS renders the for-init clause for the JS target.
func (e *emitter) forInitJS(init ast.Statement) string {
	switch n := init.(type) {
	case *ast.LetStatement:
		return "var " + n.Name.Value + " = " + e.expr(n.Value)
	case *ast.ConstStatement:
		return "var " + n.Name.Value + " = " + e.expr(n.Value)
	case *ast.TypedLetStatement:
		return "var " + n.Name.Value + " = " + e.expr(n.Value)
	case *ast.ExpressionStatement:
		return e.expr(n.Expression)
	default:
		e.fail(unsupported(init, "this for-init form is outside the transpilable subset"))
		return ""
	}
}

// forInitPy renders the for-init clause for the Python target as statements.
func (e *emitter) forInitPy(init ast.Statement) {
	switch n := init.(type) {
	case *ast.LetStatement:
		v := e.expr(n.Value)
		if e.err == nil {
			e.line(e.endStmt(n.Name.Value + " = " + v))
		}
	case *ast.ConstStatement:
		v := e.expr(n.Value)
		if e.err == nil {
			e.line(e.endStmt(n.Name.Value + " = " + v))
		}
	case *ast.TypedLetStatement:
		v := e.expr(n.Value)
		if e.err == nil {
			e.line(e.endStmt(n.Name.Value + " = " + v))
		}
	case *ast.ExpressionStatement:
		v := e.stmtExpr(n.Expression)
		if e.err == nil {
			e.line(e.endStmt(v))
		}
	default:
		e.fail(unsupported(init, "this for-init form is outside the transpilable subset"))
	}
}

// emitForIn handles for (x in iterable). NvS iterates array values, string
// chars (code points), and hash keys. Python's `for x in it` already does
// exactly that (dicts iterate keys); JS needs the __iter helper.
func (e *emitter) emitForIn(n *ast.ForInStatement) {
	if n.OrElse != nil {
		e.fail(unsupported(n, "for-else is outside the transpilable subset"))
		return
	}
	if n.Label != "" {
		e.fail(unsupported(n, "labeled loops are outside the transpilable subset"))
		return
	}
	it := e.expr(n.Iterable)
	if e.err != nil {
		return
	}
	e.loops = append(e.loops, loopInfo{kind: loopForIn})
	if e.t == targetJS {
		// `var` (not const/let): NvS binds the loop variable in the
		// function scope, visible after the loop.
		e.line("for (var " + n.Name.Value + " of __iter(" + it + ")) {")
	} else {
		e.line("for " + n.Name.Value + " in " + it + ":")
	}
	e.emitBlock(n.Body)
	if e.t == targetJS {
		e.line("}")
	}
	e.loops = e.loops[:len(e.loops)-1]
}

// emitContinue emits a continue statement. In a Python-desugared C-style
// for with a post-expression, a continue targeting that loop must run the
// post first (NvS semantics); break always skips it.
func (e *emitter) emitContinue() {
	if e.t == targetPython && len(e.loops) > 0 {
		top := e.loops[len(e.loops)-1]
		if top.kind == loopCFor && top.post != nil {
			e.line(e.endStmt(e.stmtExpr(top.post)))
			if e.err != nil {
				return
			}
		}
	}
	e.line(e.endStmt("continue"))
}

// ---------------------------------------------------------------------------
// Function emission
// ---------------------------------------------------------------------------

// emitFunction emits a named function. NvS functions return their body's
// last expression statement implicitly; the transpiler replicates that by
// turning a trailing expression statement into `return <expr>`. A trailing
// if/while/for (whose value NvS would propagate) is rejected with a
// targeted error — add an explicit return or a trailing expression.
func (e *emitter) emitFunction(name string, fl *ast.FunctionLiteral) {
	params := make([]string, len(fl.Parameters))
	for i, p := range fl.Parameters {
		def := ""
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			d := fl.Defaults[i]
			if !isLiteral(d) {
				e.fail(unsupported(fl, fmt.Sprintf("default value for parameter %q must be a literal in the transpilable subset", p.Value)))
				return
			}
			dv := e.expr(d)
			if e.err != nil {
				return
			}
			def = " = " + dv
		}
		// Parameter type annotations are erased (documented).
		params[i] = p.Value + def
	}

	sc := analyzeFunction(fl)
	e.scopes = append(e.scopes, sc)
	savedLoops := e.loops
	e.loops = nil

	if e.t == targetJS {
		e.line("function " + name + "(" + strings.Join(params, ", ") + ") {")
	} else {
		e.line("def " + name + "(" + strings.Join(params, ", ") + "):")
	}
	e.indent++
	if e.t == targetPython {
		nonlocal, global := e.outerBindings(sc)
		if len(nonlocal) > 0 {
			e.line("nonlocal " + strings.Join(nonlocal, ", "))
		}
		if len(global) > 0 {
			e.line("global " + strings.Join(global, ", "))
		}
	}

	body := fl.Body
	if body == nil || len(body.Statements) == 0 {
		// `fn f() {}` returns null in NvS.
		e.line(e.endStmt("return " + e.nullLit()))
	} else {
		last := body.Statements[len(body.Statements)-1]
		if es, ok := last.(*ast.ExpressionStatement); ok && !isIfExprStmt(es) {
			// Implicit return: the body's value is the last expression.
			for _, s := range body.Statements[:len(body.Statements)-1] {
				e.stmt(s)
				if e.err != nil {
					break
				}
			}
			if e.err == nil {
				e.line(e.endStmt("return " + e.expr(es.Expression)))
			}
		} else {
			switch last := last.(type) {
			case *ast.ReturnStatement:
				for _, s := range body.Statements {
					e.stmt(s)
					if e.err != nil {
						break
					}
				}
			case *ast.ExpressionStatement:
				// Only reached for a trailing if-as-value (non-if
				// trailing expressions are handled above): name the
				// IfExpression, not the wrapper statement.
				e.fail(&UnsupportedError{Construct: "IfExpression", Detail: "if used as a function value is outside the transpilable subset; end the body with an expression or an explicit return"})
			case *ast.WhileStatement, *ast.ForStatement, *ast.ForInStatement:
				// A trailing loop is a value in NvS; replicating that
				// needs a value-form transform, so it is rejected with a
				// targeted message instead.
				e.fail(unsupported(last, fmt.Sprintf("a trailing %s is a value in NvS and is outside the subset; end the body with an expression or an explicit return", shortNodeName(last))))
			default:
				// let/const/print/... evaluate to null as a trailing value.
				for _, s := range body.Statements {
					e.stmt(s)
					if e.err != nil {
						break
					}
				}
				if e.err == nil {
					e.line(e.endStmt("return " + e.nullLit()))
				}
			}
		}
	}
	e.indent--
	if e.t == targetJS {
		e.line("}")
	}

	e.loops = savedLoops
	e.scopes = e.scopes[:len(e.scopes)-1]
}

func (e *emitter) nullLit() string {
	if e.t == targetPython {
		return "None"
	}
	return "null"
}

func shortNodeName(n ast.Node) string {
	name := fmt.Sprintf("%T", n)
	return strings.TrimPrefix(name, "*ast.")
}

func isIfExprStmt(es *ast.ExpressionStatement) bool {
	_, ok := es.Expression.(*ast.IfExpression)
	return ok
}

func isLiteral(x ast.Expression) bool {
	switch x.(type) {
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.Boolean, *ast.NullLiteral:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Expression emission
// ---------------------------------------------------------------------------

// expr renders an expression. Binary operators are always parenthesized so
// the output never depends on target precedence tables matching NvS's.
func (e *emitter) expr(x ast.Expression) string {
	if e.err != nil {
		return ""
	}
	switch n := x.(type) {
	case *ast.IntegerLiteral:
		return strconv.FormatInt(n.Value, 10)
	case *ast.FloatLiteral:
		return strconv.FormatFloat(n.Value, 'g', -1, 64)
	case *ast.StringLiteral:
		return quoteString(n.Value)
	case *ast.Boolean:
		if e.t == targetJS {
			return strconv.FormatBool(n.Value)
		}
		if n.Value {
			return "True"
		}
		return "False"
	case *ast.NullLiteral:
		if e.t == targetJS {
			return "null"
		}
		return "None"
	case *ast.Identifier:
		return n.Value
	case *ast.InterpolatedString:
		return e.emitInterpolation(n)
	case *ast.PrefixExpression:
		return e.emitPrefix(n)
	case *ast.InfixExpression:
		return e.emitInfix(n)
	case *ast.TernaryExpression:
		cond := e.expr(n.Condition)
		a := e.expr(n.Consequence)
		b := e.expr(n.Alternative)
		if e.err != nil {
			return ""
		}
		if e.t == targetJS {
			return "(__truthy(" + cond + ") ? " + a + " : " + b + ")"
		}
		return "(" + a + " if __truthy(" + cond + ") else " + b + ")"
	case *ast.IfExpression:
		e.fail(unsupported(n, "if as an expression value is outside the subset; use an if statement or a ternary"))
		return ""
	case *ast.ArrayLiteral:
		els := make([]string, len(n.Elements))
		for i, el := range n.Elements {
			els[i] = e.expr(el)
			if e.err != nil {
				return ""
			}
		}
		return "[" + strings.Join(els, ", ") + "]"
	case *ast.HashLiteral:
		return e.emitHash(n)
	case *ast.IndexExpression:
		return e.emitIndex(n)
	case *ast.AssignExpression:
		return e.emitAssign(n)
	case *ast.IndexAssignExpression:
		left := e.expr(n.Left)
		v := e.expr(n.Value)
		if e.err != nil {
			return ""
		}
		return "(" + left + " = " + v + ")"
	case *ast.CallExpression:
		return e.emitCall(n)
	case *ast.FunctionLiteral:
		// Anonymous function value.
		if e.t == targetPython {
			e.fail(unsupported(n, "anonymous functions are outside the Python subset; bind the function with let (which emits def) instead"))
			return ""
		}
		// JS: function expression. Give it no name; recursion through it
		// is outside the subset (bind with let for a named function).
		saved := e.sb
		e.sb = strings.Builder{}
		e.emitFunction("", n)
		out := e.sb.String()
		e.sb = saved
		if e.err != nil {
			return ""
		}
		fn := strings.TrimSpace(out)
		// "function (x) {\n...\n}" -> "(function (x) {...})" — the parens
		// keep it an expression in statement position.
		return "(" + fn + ")"
	default:
		e.fail(unsupported(x, "this expression is outside the transpilable subset"))
		return ""
	}
}

// stmtExpr renders an expression in statement position. In Python,
// assignments cannot be parenthesized (no assignment expressions), so
// the parens that expr() adds around them are unwrapped here —
// recursively, so `x = (y = 5)` becomes the equivalent `x = y = 5`.
func (e *emitter) stmtExpr(x ast.Expression) string {
	switch n := x.(type) {
	case *ast.AssignExpression:
		return n.Name.Value + " = " + e.stmtExpr(n.Value)
	case *ast.IndexAssignExpression:
		return e.expr(n.Left) + " = " + e.stmtExpr(n.Value)
	default:
		return e.expr(x)
	}
}

func (e *emitter) emitAssign(n *ast.AssignExpression) string {
	v := e.expr(n.Value)
	if e.err != nil {
		return ""
	}
	return "(" + n.Name.Value + " = " + v + ")"
}

// emitInterpolation desugars "a${x}b" into "a" + __str(x) + "b".
// NvS coerces interpolated parts with Inspect; __str replicates Inspect
// exactly (raw strings, true/false, null, Go %g floats, [1, 2], {a: 1}).
func (e *emitter) emitInterpolation(n *ast.InterpolatedString) string {
	parts := make([]string, len(n.Parts))
	for i, p := range n.Parts {
		if sl, ok := p.(*ast.StringLiteral); ok {
			parts[i] = quoteString(sl.Value)
		} else {
			parts[i] = "__str(" + e.expr(p) + ")"
		}
		if e.err != nil {
			return ""
		}
	}
	if len(parts) == 0 {
		return quoteString("")
	}
	return "(" + strings.Join(parts, " + ") + ")"
}

func (e *emitter) emitPrefix(n *ast.PrefixExpression) string {
	r := e.expr(n.Right)
	if e.err != nil {
		return ""
	}
	switch n.Operator {
	case "-":
		return "(-" + r + ")"
	case "!":
		// NvS ! uses NvS truthiness (only null/false are falsy).
		if e.t == targetJS {
			return "(!__truthy(" + r + "))"
		}
		return "(not __truthy(" + r + "))"
	default:
		e.fail(unsupported(n, fmt.Sprintf("prefix operator %q is outside the transpilable subset", n.Operator)))
		return ""
	}
}

func (e *emitter) emitInfix(n *ast.InfixExpression) string {
	l := e.expr(n.Left)
	r := e.expr(n.Right)
	if e.err != nil {
		return ""
	}
	js := e.t == targetJS
	switch n.Operator {
	case "+":
		return "(" + l + " + " + r + ")"
	case "-":
		return "(" + l + " - " + r + ")"
	case "*":
		return "(" + l + " * " + r + ")"
	case "**":
		return "__pow(" + l + ", " + r + ")"
	case "/":
		// NvS int/int truncates toward zero; the targets do float division.
		return "__div(" + l + ", " + r + ")"
	case "%":
		if js {
			return "__mod(" + l + ", " + r + ")"
		}
		return "__mod(" + l + ", " + r + ")"
	case "<", ">", "<=", ">=":
		return "(" + l + " " + n.Operator + " " + r + ")"
	case "==":
		// NvS == is value equality for numbers/strings/bools, identity
		// for arrays/hashes/functions. JS === matches that matrix;
		// Python == does not (list == list is deep), hence __nvs_eq.
		if js {
			return "(" + l + " === " + r + ")"
		}
		return "__nvs_eq(" + l + ", " + r + ")"
	case "!=":
		if js {
			return "(" + l + " !== " + r + ")"
		}
		return "(not __nvs_eq(" + l + ", " + r + "))"
	case "and", "&&":
		// NvS and/or return booleans using NvS truthiness.
		if js {
			return "(__truthy(" + l + ") && __truthy(" + r + "))"
		}
		return "(__truthy(" + l + ") and __truthy(" + r + "))"
	case "or", "||":
		if js {
			return "(__truthy(" + l + ") || __truthy(" + r + "))"
		}
		return "(__truthy(" + l + ") or __truthy(" + r + "))"
	case "??":
		// NvS ?? returns the left side unless it is null. JS ?? is
		// identical (transpiled values are never undefined). Python
		// needs thunks to keep the short-circuit.
		if js {
			return "(" + l + " ?? " + r + ")"
		}
		return "__coalesce(lambda: (" + l + "), lambda: (" + r + "))"
	case "&", "|", "^":
		// Bitwise ops. Documented limitation: the JS target uses 32-bit
		// semantics, so operands must fit in a signed 32-bit int there.
		return "(" + l + " " + n.Operator + " " + r + ")"
	default:
		e.fail(unsupported(n, fmt.Sprintf("operator %q is outside the transpilable subset", n.Operator)))
		return ""
	}
}

// emitHash emits a hash literal. Only string keys are in the subset: NvS
// allows int/bool keys, but JS object keys are always strings, so key
// types would not survive there. Spreads are rejected.
func (e *emitter) emitHash(n *ast.HashLiteral) string {
	if len(n.Spreads) > 0 {
		e.fail(unsupported(n, "hash spread is outside the transpilable subset"))
		return ""
	}
	pairs := make([]string, 0, len(n.Pairs))
	for k, v := range n.Pairs {
		ks, ok := k.(*ast.StringLiteral)
		if !ok {
			e.fail(unsupported(n, "only string hash keys are in the transpilable subset"))
			return ""
		}
		vv := e.expr(v)
		if e.err != nil {
			return ""
		}
		pairs = append(pairs, quoteString(ks.Value)+": "+vv)
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

// emitIndex emits a[i] and the slice form a[s:e] (End != nil).
func (e *emitter) emitIndex(n *ast.IndexExpression) string {
	a := e.expr(n.Left)
	i := e.expr(n.Index)
	if e.err != nil {
		return ""
	}
	if n.End == nil {
		return "(" + a + "[" + i + "])"
	}
	end := e.expr(n.End)
	if e.err != nil {
		return ""
	}
	return "__slice(" + a + ", " + i + ", " + end + ")"
}

// quoteString renders a double-quoted string literal with escapes valid in
// both JS and Python.
func quoteString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\t':
			sb.WriteString("\\t")
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&sb, "\\u%04x", r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// ---------------------------------------------------------------------------
// Call emission + the explicit builtin mapping table.
//
// Only builtins whose semantics are IDENTICAL in NvS and the target are
// mapped. Calling any other NvS builtin is an honest error (it names the
// builtin); calling a non-builtin identifier emits a plain call.
// ---------------------------------------------------------------------------

func (e *emitter) emitCall(n *ast.CallExpression) string {
	for _, a := range n.Arguments {
		if _, ok := a.(*ast.NamedArgument); ok {
			e.fail(unsupported(n, "named arguments are outside the transpilable subset"))
			return ""
		}
	}
	args := make([]string, len(n.Arguments))
	for i, a := range n.Arguments {
		args[i] = e.expr(a)
		if e.err != nil {
			return ""
		}
	}
	switch fn := n.Function.(type) {
	case *ast.Identifier:
		if e.builtins[fn.Value] {
			mapped, isMapped := e.emitBuiltin(fn.Value, args)
			if e.err != nil {
				return ""
			}
			if isMapped {
				return mapped
			}
			e.fail(unsupported(n, fmt.Sprintf("builtin `%s` has no mapping in the transpilable subset", fn.Value)))
			return ""
		}
		return fn.Value + "(" + strings.Join(args, ", ") + ")"
	case *ast.IndexExpression:
		// Calling through an index (fns[0](x), h["f"](x)): NvS evaluates
		// the index then calls it — identical in both targets.
		return e.emitIndex(fn) + "(" + strings.Join(args, ", ") + ")"
	default:
		e.fail(unsupported(n, "only direct calls (name or builtin) and calls through an index are in the transpilable subset; method calls are not"))
		return ""
	}
}

// emitBuiltin maps one builtin call. It returns (code, true) when the
// builtin is in the mapping table, ("", false) otherwise.
func (e *emitter) emitBuiltin(name string, args []string) (string, bool) {
	js := e.t == targetJS
	parens := func(a string) string { return "(" + a + ")" }
	switch name {
	case "len":
		// Python len() is identical for str/list/dict; JS needs __len.
		if js {
			return "__len" + parens(args[0]), true
		}
		return "len" + parens(args[0]), true
	case "str":
		// NvS str() is Inspect; both targets need the __str helper.
		return "__str" + parens(args[0]), true
	case "push":
		// NvS push(a, x) appends AND returns the array.
		return "__push" + parens(strings.Join(args, ", ")), true
	case "split":
		// Identical for non-empty string separators.
		return parens(args[0]) + ".split(" + args[1] + ")", true
	case "upper":
		if js {
			return parens(args[0]) + ".toUpperCase()", true
		}
		return parens(args[0]) + ".upper()", true
	case "lower":
		if js {
			return parens(args[0]) + ".toLowerCase()", true
		}
		return parens(args[0]) + ".lower()", true
	case "abs":
		if js {
			return "Math.abs" + parens(args[0]), true
		}
		return "abs" + parens(args[0]), true
	case "range":
		// NvS range() has Python's integer semantics and returns an
		// array; Python needs list(...) around it, JS a helper.
		if js {
			return "__range" + parens(strings.Join(args, ", ")), true
		}
		return "list(range" + parens(strings.Join(args, ", ")) + ")", true
	case "keys":
		// NvS keys(h) -> array of keys. Order is unspecified in NvS
		// (Go map), so target ordering differences are in-contract.
		if js {
			return "Object.keys" + parens(args[0]), true
		}
		return "list" + parens(parens(args[0])+".keys()"), true
	}
	return "", false
}
