package parser

import (
	"testing"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

// ---- Wave 3: tuple syntax ----

func tupleLit(t *testing.T, input string) *ast.TupleLiteral {
	t.Helper()
	e := parseExpr(t, input)
	tl, ok := e.(*ast.TupleLiteral)
	if !ok {
		t.Fatalf("expected *ast.TupleLiteral for %q, got %T", input, e)
	}
	return tl
}

func TestWave3TupleBasic(t *testing.T) {
	tl := tupleLit(t, "(1, 2)")
	if len(tl.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tl.Elements))
	}
	if tl.Elements[0].String() != "1" || tl.Elements[1].String() != "2" {
		t.Fatalf("unexpected elements: %s", tl.String())
	}
}

func TestWave3TupleSingleTrailingComma(t *testing.T) {
	tl := tupleLit(t, "(x,)")
	if len(tl.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(tl.Elements))
	}
	if tl.String() != "(x,)" {
		t.Fatalf("expected (x,), got %s", tl.String())
	}
}

func TestWave3TupleEmpty(t *testing.T) {
	tl := tupleLit(t, "()")
	if len(tl.Elements) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(tl.Elements))
	}
}

func TestWave3GroupingPreserved(t *testing.T) {
	// (x) is still grouping, not a tuple.
	e := parseExpr(t, "(x)")
	if _, ok := e.(*ast.TupleLiteral); ok {
		t.Fatalf("(x) parsed as tuple, want grouping")
	}
	if id, ok := e.(*ast.Identifier); !ok || id.Value != "x" {
		t.Fatalf("expected identifier x, got %T (%s)", e, e.String())
	}
	// (1 + 2) * 3 still groups.
	e2 := parseExpr(t, "(1 + 2) * 3")
	if _, ok := e2.(*ast.InfixExpression); !ok {
		t.Fatalf("expected infix, got %T", e2)
	}
}

func TestWave3TupleTrailingCommaMulti(t *testing.T) {
	tl := tupleLit(t, "(1, 2,)")
	if len(tl.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tl.Elements))
	}
}

func TestWave3TupleNested(t *testing.T) {
	tl := tupleLit(t, "(1, (2, 3))")
	if len(tl.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tl.Elements))
	}
	if _, ok := tl.Elements[1].(*ast.TupleLiteral); !ok {
		t.Fatalf("expected nested tuple, got %T", tl.Elements[1])
	}
}

// ---- Wave 3: record syntax ----

func recordStmt(t *testing.T, input string) *ast.RecordStatement {
	t.Helper()
	l := lexer.New(input)
	p := New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors for %q: %v", input, p.Errors())
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement for %q, got %d", input, len(prog.Statements))
	}
	rs, ok := prog.Statements[0].(*ast.RecordStatement)
	if !ok {
		t.Fatalf("expected *ast.RecordStatement for %q, got %T", input, prog.Statements[0])
	}
	return rs
}

func TestWave3RecordBasic(t *testing.T) {
	rs := recordStmt(t, "record Point(x, y)")
	if rs.Name.Value != "Point" {
		t.Fatalf("expected name Point, got %s", rs.Name.Value)
	}
	if len(rs.Fields) != 2 || rs.Fields[0].Value != "x" || rs.Fields[1].Value != "y" {
		t.Fatalf("unexpected fields: %s", rs.String())
	}
}

func TestWave3RecordEmpty(t *testing.T) {
	rs := recordStmt(t, "record Unit()")
	if len(rs.Fields) != 0 {
		t.Fatalf("expected 0 fields, got %d", len(rs.Fields))
	}
}

func TestWave3RecordDuplicateField(t *testing.T) {
	l := lexer.New("record P(x, x)")
	p := New(l)
	p.ParseProgram()
	if len(p.Errors()) == 0 {
		t.Fatalf("expected parse error for duplicate field, got none")
	}
}

func TestWave3RecordMissingParens(t *testing.T) {
	l := lexer.New("record Point")
	p := New(l)
	p.ParseProgram()
	if len(p.Errors()) == 0 {
		t.Fatalf("expected parse error for missing parens, got none")
	}
}
