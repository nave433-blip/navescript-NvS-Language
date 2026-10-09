package parser

import (
	"testing"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

// ---- Wave 19: generics + std-dialect parser features ----

func parseOne19(t *testing.T, src string) *ast.Program {
	t.Helper()
	p := New(lexer.New(src))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors for %q: %v", src, p.Errors())
	}
	return prog
}

func expectParseError(t *testing.T, src, substr string) {
	t.Helper()
	p := New(lexer.New(src))
	p.ParseProgram()
	found := false
	for _, e := range p.Errors() {
		if contains(e, substr) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected parse error containing %q for %q, got %v", substr, src, p.Errors())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWave19ClassTypeParams(t *testing.T) {
	prog := parseOne19(t, `class Box<T> { fn get() -> T { return this.v; } }`)
	cs := prog.Statements[0].(*ast.ClassStatement)
	if len(cs.TypeParams) != 1 || cs.TypeParams[0].Name.Value != "T" {
		t.Fatalf("class type params = %v, want [T]", cs.TypeParams)
	}
	if cs.TypeParams[0].Bound != nil {
		t.Fatalf("expected unbounded T")
	}
}

func TestWave19ClassTypeParamsBound(t *testing.T) {
	prog := parseOne19(t, `class Box<T: Numeric, U> {}`)
	cs := prog.Statements[0].(*ast.ClassStatement)
	if len(cs.TypeParams) != 2 {
		t.Fatalf("want 2 type params, got %d", len(cs.TypeParams))
	}
	if cs.TypeParams[0].Bound == nil || cs.TypeParams[0].Bound.Name != "Numeric" {
		t.Fatalf("T bound = %v, want Numeric", cs.TypeParams[0].Bound)
	}
	if cs.TypeParams[1].Bound != nil {
		t.Fatalf("U should be unbounded")
	}
}

func TestWave19MethodTypeParams(t *testing.T) {
	prog := parseOne19(t, `class Vec<T> { fn map<U>(f) -> Vec<U> { return this; } }`)
	cs := prog.Statements[0].(*ast.ClassStatement)
	m := cs.Methods[0]
	if len(m.TypeParams) != 1 || m.TypeParams[0].Name.Value != "U" {
		t.Fatalf("method type params = %v, want [U]", m.TypeParams)
	}
	if m.ReturnType == nil || m.ReturnType.Name != "Vec" || len(m.ReturnType.TypeArgs) != 1 {
		t.Fatalf("return type = %v, want Vec<U>", m.ReturnType)
	}
}

func TestWave19FnTypeParams(t *testing.T) {
	prog := parseOne19(t, `fn identity<T>(x: T) -> T { return x; }`)
	ls := prog.Statements[0].(*ast.LetStatement)
	fl := ls.Value.(*ast.FunctionLiteral)
	if len(fl.TypeParams) != 1 || fl.TypeParams[0].Name.Value != "T" {
		t.Fatalf("fn type params = %v, want [T]", fl.TypeParams)
	}
	if fl.ParamTypes[0].Name != "T" || fl.ReturnType.Name != "T" {
		t.Fatalf("param/return types not preserved")
	}
}

func TestWave19ArrowReturn(t *testing.T) {
	prog := parseOne19(t, `fn f() -> int { return 1; }`)
	fl := prog.Statements[0].(*ast.LetStatement).Value.(*ast.FunctionLiteral)
	if fl.ReturnType == nil || fl.ReturnType.Name != "int" {
		t.Fatalf("arrow return type = %v", fl.ReturnType)
	}
	// Colon form still works.
	prog2 := parseOne19(t, `fn f(): int { return 1; }`)
	fl2 := prog2.Statements[0].(*ast.LetStatement).Value.(*ast.FunctionLiteral)
	if fl2.ReturnType == nil || fl2.ReturnType.Name != "int" {
		t.Fatalf("colon return type = %v", fl2.ReturnType)
	}
}

func TestWave19AnnotationTypeArgs(t *testing.T) {
	prog := parseOne19(t, `let x: Vec<int> = v;`)
	tl := prog.Statements[0].(*ast.TypedLetStatement)
	if tl.Type.Name != "Vec" || len(tl.Type.TypeArgs) != 1 || tl.Type.TypeArgs[0].Name != "int" {
		t.Fatalf("type = %v, want Vec<int>", tl.Type)
	}
}

func TestWave19NestedTypeArgs(t *testing.T) {
	prog := parseOne19(t, `let x: Map<string, Vec<int>> = v;`)
	tl := prog.Statements[0].(*ast.TypedLetStatement)
	if tl.Type.Name != "Map" || len(tl.Type.TypeArgs) != 2 {
		t.Fatalf("type = %v", tl.Type)
	}
	inner := tl.Type.TypeArgs[1]
	if inner.Name != "Vec" || len(inner.TypeArgs) != 1 || inner.TypeArgs[0].Name != "int" {
		t.Fatalf("nested = %v", inner)
	}
	// `>>` without space must close two levels.
	prog2 := parseOne19(t, `let x: Vec<Vec<int>> = v;`)
	tl2 := prog2.Statements[0].(*ast.TypedLetStatement)
	if len(tl2.Type.TypeArgs) != 1 || len(tl2.Type.TypeArgs[0].TypeArgs) != 1 {
		t.Fatalf("double-nested = %v", tl2.Type)
	}
}

func TestWave19ArraySugar(t *testing.T) {
	for _, src := range []string{`let x: string[] = v;`, `let x: [string] = v;`} {
		prog := parseOne19(t, src)
		tl := prog.Statements[0].(*ast.TypedLetStatement)
		if !tl.Type.IsArray {
			t.Fatalf("%s: IsArray not set (%v)", src, tl.Type)
		}
	}
}

func TestWave19FnTypeAnnotation(t *testing.T) {
	prog := parseOne19(t, `fn apply(f: fn(int) -> string) -> string { return f(1); }`)
	ls := prog.Statements[0].(*ast.LetStatement)
	fl := ls.Value.(*ast.FunctionLiteral)
	pt := fl.ParamTypes[0]
	if pt.FnSig == nil || len(pt.FnSig.Params) != 1 || pt.FnSig.Params[0].Name != "int" {
		t.Fatalf("fn type param = %v", pt)
	}
	if pt.FnSig.Return == nil || pt.FnSig.Return.Name != "string" {
		t.Fatalf("fn type return = %v", pt.FnSig.Return)
	}
}

func TestWave19Turbofish(t *testing.T) {
	prog := parseOne19(t, `let m = v.map::<string>(f);`)
	ls := prog.Statements[0].(*ast.LetStatement)
	call := ls.Value.(*ast.CallExpression)
	if len(call.TypeArgs) != 1 || call.TypeArgs[0].Name != "string" {
		t.Fatalf("turbofish type args = %v", call.TypeArgs)
	}
	if _, ok := call.Function.(*ast.MemberExpression); !ok {
		t.Fatalf("turbofish function should stay a member expression")
	}
}

func TestWave19BareTypeArgsCall(t *testing.T) {
	prog := parseOne19(t, `let x = identity<string>("hi");`)
	call := prog.Statements[0].(*ast.LetStatement).Value.(*ast.CallExpression)
	if len(call.TypeArgs) != 1 || call.TypeArgs[0].Name != "string" {
		t.Fatalf("type args = %v", call.TypeArgs)
	}
}

func TestWave19BareTypeArgsOnMethodRejected(t *testing.T) {
	expectParseError(t, `v.map<string>(f);`, "turbofish")
}

func TestWave19NewTypeArgs(t *testing.T) {
	prog := parseOne19(t, `let b = new Box<int>(42);`)
	ne := prog.Statements[0].(*ast.LetStatement).Value.(*ast.NewExpression)
	if len(ne.TypeArgs) != 1 || ne.TypeArgs[0].Name != "int" {
		t.Fatalf("new type args = %v", ne.TypeArgs)
	}
}

func TestWave19LessThanStillWorks(t *testing.T) {
	// The probe must not steal genuine comparisons.
	prog := parseOne19(t, `if (a < b) { print(1); }`)
	ife := prog.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.IfExpression)
	if _, ok := ife.Condition.(*ast.InfixExpression); !ok {
		t.Fatalf("comparison misparsed as type args: %T", ife.Condition)
	}
	prog2 := parseOne19(t, `let x = a < b(c);`)
	_ = prog2 // parses as comparison: a < (b(c))
}

func TestWave19DColonNamespace(t *testing.T) {
	prog := parseOne19(t, `ffi::call("x", []);`)
	call := prog.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
	mem := call.Function.(*ast.MemberExpression)
	if mem.Object.(*ast.Identifier).Value != "ffi" || mem.Property.Value != "call" {
		t.Fatalf("namespace call = %v", call.Function)
	}
}

func TestWave19Pub(t *testing.T) {
	prog := parseOne19(t, `pub fn f() {} pub class C {} pub let x = 1;`)
	if !prog.Statements[0].(*ast.LetStatement).Public {
		t.Fatalf("pub fn not recorded")
	}
	if !prog.Statements[1].(*ast.ClassStatement).Public {
		t.Fatalf("pub class not recorded")
	}
	if !prog.Statements[2].(*ast.LetStatement).Public {
		t.Fatalf("pub let not recorded")
	}
	prog2 := parseOne19(t, `class C { pub fn m() {} pub let f: int = 0; }`)
	cs := prog2.Statements[0].(*ast.ClassStatement)
	if !cs.Methods[0].Public {
		t.Fatalf("pub method not recorded")
	}
	if len(cs.Fields) != 1 || !cs.Fields[0].Public || cs.Fields[0].Name.Value != "f" {
		t.Fatalf("pub field not recorded: %+v", cs.Fields)
	}
}

func TestWave19ClassFields(t *testing.T) {
	prog := parseOne19(t, `class C { let a: int; let b = 2; }`)
	cs := prog.Statements[0].(*ast.ClassStatement)
	if len(cs.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(cs.Fields))
	}
	if cs.Fields[0].Type == nil || cs.Fields[0].Type.Name != "int" {
		t.Fatalf("field type = %v", cs.Fields[0].Type)
	}
	if cs.Fields[0].Default != nil {
		t.Fatalf("field a should have no default")
	}
	if cs.Fields[1].Default == nil {
		t.Fatalf("field b should have a default")
	}
}

func TestWave19Module(t *testing.T) {
	prog := parseOne19(t, `module std.collections.vector;`)
	ms := prog.Statements[0].(*ast.ModuleStatement)
	if ms.Name != "std.collections.vector" {
		t.Fatalf("module name = %q", ms.Name)
	}
}

func TestWave19ImportModulePath(t *testing.T) {
	prog := parseOne19(t, `import std.core;`)
	is := prog.Statements[0].(*ast.ImportStatement)
	if is.ModulePath != "std.core" {
		t.Fatalf("module path = %q", is.ModulePath)
	}
	// Quoted form still works.
	prog2 := parseOne19(t, `import "lib.ns";`)
	if prog2.Statements[0].(*ast.ImportStatement).Path == nil {
		t.Fatalf("quoted import broken")
	}
}

func TestWave19Postfix(t *testing.T) {
	prog := parseOne19(t, `x++; y--;`)
	pe := prog.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.PostfixExpression)
	if pe.Operator != "++" {
		t.Fatalf("operator = %q", pe.Operator)
	}
	pe2 := prog.Statements[1].(*ast.ExpressionStatement).Expression.(*ast.PostfixExpression)
	if pe2.Operator != "--" {
		t.Fatalf("operator = %q", pe2.Operator)
	}
}

func TestWave19BracelessIf(t *testing.T) {
	prog := parseOne19(t, `if (a) print(1); else print(2);`)
	ife := prog.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.IfExpression)
	if len(ife.Consequence.Statements) != 1 || len(ife.Alternative.Statements) != 1 {
		t.Fatalf("braceless if misparsed")
	}
	prog2 := parseOne19(t, `while (a) x++;`)
	ws := prog2.Statements[0].(*ast.WhileStatement)
	if len(ws.Body.Statements) != 1 {
		t.Fatalf("braceless while misparsed")
	}
}

func TestWave19InterfaceNoTypeParams(t *testing.T) {
	expectParseError(t, `interface I { map<U>(f); }`, "interface methods cannot declare type parameters")
	expectParseError(t, `interface I { fn map<U>(f); }`, "interface methods cannot declare type parameters")
	// Non-generic interface methods still fine.
	prog := parseOne19(t, `interface I { map(f); fn get() -> int; }`)
	if len(prog.Statements[0].(*ast.InterfaceDecl).Methods) != 2 {
		t.Fatalf("interface methods misparsed")
	}
}
