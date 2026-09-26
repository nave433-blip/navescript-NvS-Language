package parser

import (
	"testing"

	"github.com/navescript/nvs/internal/ast"
)

// ---- Wave 5: annotations, unions, interfaces (parse-level) ----

func parseFnStmt(t *testing.T, input string) *ast.LetStatement {
	t.Helper()
	// `fn name(...)...` desugars to a LetStatement holding a FunctionLiteral.
	stmt := parseOne(t, input)
	ls, ok := stmt.(*ast.LetStatement)
	if !ok {
		t.Fatalf("expected *ast.LetStatement for %q, got %T", input, stmt)
	}
	return ls
}

func fnLit(t *testing.T, input string) *ast.FunctionLiteral {
	t.Helper()
	ls := parseFnStmt(t, input)
	fl, ok := ls.Value.(*ast.FunctionLiteral)
	if !ok {
		t.Fatalf("expected *ast.FunctionLiteral for %q, got %T", input, ls.Value)
	}
	return fl
}

func TestWave5ParamAnnotationsAST(t *testing.T) {
	fl := fnLit(t, `fn add(a: int, b: string) { return a }`)
	if len(fl.ParamTypes) != 2 {
		t.Fatalf("expected 2 param types, got %d", len(fl.ParamTypes))
	}
	if fl.ParamTypes[0] == nil || fl.ParamTypes[0].String() != "int" {
		t.Fatalf("expected param 0 type int, got %v", fl.ParamTypes[0])
	}
	if fl.ParamTypes[1] == nil || fl.ParamTypes[1].String() != "string" {
		t.Fatalf("expected param 1 type string, got %v", fl.ParamTypes[1])
	}
	if fl.ReturnType != nil {
		t.Fatalf("expected nil return type, got %v", fl.ReturnType)
	}
}

func TestWave5ReturnAnnotationAST(t *testing.T) {
	fl := fnLit(t, `fn add(a: int, b: int): int { return a }`)
	if fl.ReturnType == nil || fl.ReturnType.String() != "int" {
		t.Fatalf("expected return type int, got %v", fl.ReturnType)
	}
}

func TestWave5UnannotatedParamsNil(t *testing.T) {
	fl := fnLit(t, `fn f(a, b: int) { return a }`)
	if fl.ParamTypes[0] != nil {
		t.Fatalf("expected nil type for unannotated param, got %v", fl.ParamTypes[0])
	}
	if fl.ParamTypes[1] == nil || fl.ParamTypes[1].String() != "int" {
		t.Fatalf("expected int for param 1, got %v", fl.ParamTypes[1])
	}
}

func TestWave5UnionAnnotationAST(t *testing.T) {
	fl := fnLit(t, `fn f(x: int | string) { return x }`)
	ann := fl.ParamTypes[0]
	if ann == nil || len(ann.Union) != 2 {
		t.Fatalf("expected 2-member union, got %v", ann)
	}
	if ann.String() != "int | string" {
		t.Fatalf("unexpected union rendering: %s", ann.String())
	}
}

func TestWave5NullableSugarAST(t *testing.T) {
	fl := fnLit(t, `fn f(x: int?) { return x }`)
	if got := fl.ParamTypes[0].String(); got != "int | null" {
		t.Fatalf("expected int? to desugar to 'int | null', got %q", got)
	}
}

func TestWave5NullInUnion(t *testing.T) {
	// `null` is a keyword token; it must still parse as a type name.
	fl := fnLit(t, `fn f(x: string | null) { return x }`)
	if got := fl.ParamTypes[0].String(); got != "string | null" {
		t.Fatalf("unexpected annotation: %q", got)
	}
}

func TestWave5LetAnnotationAST(t *testing.T) {
	stmt := parseOne(t, `let x: int | string = "s"`)
	tl, ok := stmt.(*ast.TypedLetStatement)
	if !ok {
		t.Fatalf("expected *ast.TypedLetStatement, got %T", stmt)
	}
	if tl.Type.String() != "int | string" {
		t.Fatalf("unexpected let annotation: %s", tl.Type.String())
	}
	// Plain `let x = 1` still produces a LetStatement.
	if _, ok := parseOne(t, `let x = 1`).(*ast.LetStatement); !ok {
		t.Fatal("plain let should stay a LetStatement")
	}
}

func TestWave5InterfaceAST(t *testing.T) {
	stmt := parseOne(t, "interface Shape {\n  area(): float\n  greet(name: string): string\n}")
	id, ok := stmt.(*ast.InterfaceDecl)
	if !ok {
		t.Fatalf("expected *ast.InterfaceDecl, got %T", stmt)
	}
	if id.Name.Value != "Shape" {
		t.Fatalf("expected interface name Shape, got %s", id.Name.Value)
	}
	if len(id.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(id.Methods))
	}
	if id.Methods[0].Name.Value != "area" || len(id.Methods[0].ParamNames) != 0 {
		t.Fatalf("unexpected first method: %+v", id.Methods[0])
	}
	if id.Methods[0].ReturnType.String() != "float" {
		t.Fatalf("expected float return, got %v", id.Methods[0].ReturnType)
	}
	greet := id.Methods[1]
	if len(greet.ParamNames) != 1 || greet.ParamNames[0].Value != "name" {
		t.Fatalf("unexpected greet params: %+v", greet.ParamNames)
	}
	if greet.ParamTypes[0].String() != "string" {
		t.Fatalf("expected string param type, got %v", greet.ParamTypes[0])
	}
	// Empty interface is legal.
	empty := parseOne(t, `interface Empty {}`)
	if _, ok := empty.(*ast.InterfaceDecl); !ok {
		t.Fatalf("expected *ast.InterfaceDecl, got %T", empty)
	}
}

func TestWave5MethodAnnotationAST(t *testing.T) {
	stmt := parseOne(t, "class C {\n  fn area(): float { return 1.0 }\n}")
	cs, ok := stmt.(*ast.ClassStatement)
	if !ok {
		t.Fatalf("expected *ast.ClassStatement, got %T", stmt)
	}
	m := cs.Methods[0]
	if m.ReturnType == nil || m.ReturnType.String() != "float" {
		t.Fatalf("expected method return type float, got %v", m.ReturnType)
	}
}

func TestWave5AnnotationDefaultCombo(t *testing.T) {
	fl := fnLit(t, `fn f(x: int = 3, y: string) { return x }`)
	if fl.ParamTypes[0].String() != "int" || fl.ParamTypes[1].String() != "string" {
		t.Fatalf("unexpected param types: %v %v", fl.ParamTypes[0], fl.ParamTypes[1])
	}
	if fl.Defaults[0] == nil || fl.Defaults[1] != nil {
		t.Fatal("expected default only on first param")
	}
}
