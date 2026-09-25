package parser

import (
	"testing"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

func parseOne(t *testing.T, input string) ast.Statement {
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
	return prog.Statements[0]
}

func parseExpr(t *testing.T, input string) ast.Expression {
	t.Helper()
	stmt := parseOne(t, input)
	es, ok := stmt.(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement for %q, got %T", input, stmt)
	}
	return es.Expression
}

func TestWave1PipelineDesugar(t *testing.T) {
	// x |> f  desugars to f(x)
	call, ok := parseExpr(t, "x |> f").(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected CallExpression, got %T", parseExpr(t, "x |> f"))
	}
	if id, ok := call.Function.(*ast.Identifier); !ok || id.Value != "f" {
		t.Fatalf("expected function f, got %v", call.Function)
	}
	if len(call.Arguments) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Arguments))
	}

	// x |> f(a, b)  desugars to f(x, a, b)
	call = parseExpr(t, "x |> f(a, b)").(*ast.CallExpression)
	if len(call.Arguments) != 3 {
		t.Fatalf("expected 3 args, got %d", len(call.Arguments))
	}

	// left-assoc: x |> f |> g  ==  g(f(x))
	outer, ok := parseExpr(t, "x |> f |> g").(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected CallExpression, got %T", parseExpr(t, "x |> f |> g"))
	}
	if id, ok := outer.Function.(*ast.Identifier); !ok || id.Value != "g" {
		t.Fatalf("expected outer function g, got %v", outer.Function)
	}
	inner, ok := outer.Arguments[0].(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected nested CallExpression, got %T", outer.Arguments[0])
	}
	if id, ok := inner.Function.(*ast.Identifier); !ok || id.Value != "f" {
		t.Fatalf("expected inner function f, got %v", inner.Function)
	}
}

func TestWave1Ranges(t *testing.T) {
	r, ok := parseExpr(t, "1..10").(*ast.RangeExpression)
	if !ok {
		t.Fatalf("expected RangeExpression, got %T", parseExpr(t, "1..10"))
	}
	if !r.Inclusive {
		t.Fatal("1..10 should be inclusive")
	}
	r = parseExpr(t, "1...5").(*ast.RangeExpression)
	if r.Inclusive {
		t.Fatal("1...5 should be exclusive")
	}
	// member access still parses as member access
	m, ok := parseExpr(t, "a.b").(*ast.MemberExpression)
	if !ok {
		t.Fatalf("expected MemberExpression, got %T", parseExpr(t, "a.b"))
	}
	if m.Property.Value != "b" {
		t.Fatalf("expected property b, got %q", m.Property.Value)
	}
}

func TestWave1OptionalChain(t *testing.T) {
	oc, ok := parseExpr(t, "a?.b.c?.d").(*ast.OptionalChainExpression)
	if !ok {
		t.Fatalf("expected OptionalChainExpression, got %T", parseExpr(t, "a?.b.c?.d"))
	}
	if len(oc.Links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(oc.Links))
	}
	for _, l := range oc.Links {
		if l.Kind != ast.ChainMember {
			t.Fatalf("expected member link, got %q", l.Kind)
		}
	}
	oc = parseExpr(t, "a?.b(1)?.[2]").(*ast.OptionalChainExpression)
	if len(oc.Links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(oc.Links))
	}
	if oc.Links[1].Kind != ast.ChainCall || len(oc.Links[1].Arguments) != 1 {
		t.Fatalf("expected call link with 1 arg, got %+v", oc.Links[1])
	}
	if oc.Links[2].Kind != ast.ChainIndex {
		t.Fatalf("expected index link, got %q", oc.Links[2].Kind)
	}
	// ternary still parses
	te, ok := parseExpr(t, "a ? b : c").(*ast.TernaryExpression)
	if !ok {
		t.Fatalf("expected TernaryExpression, got %T", parseExpr(t, "a ? b : c"))
	}
	_ = te
}

func TestWave1Destructure(t *testing.T) {
	stmt, ok := parseOne(t, "let [a, b, ...rest] = arr").(*ast.DestructureLetStatement)
	if !ok {
		t.Fatalf("expected DestructureLetStatement, got %T", parseOne(t, "let [a, b, ...rest] = arr"))
	}
	if stmt.IsConst {
		t.Fatal("let destructure should not be const")
	}
	pat, ok := stmt.Pattern.(*ast.ArrayPattern)
	if !ok {
		t.Fatalf("expected ArrayPattern, got %T", stmt.Pattern)
	}
	if len(pat.Elements) != 2 || pat.Rest == nil || pat.Rest.Value != "rest" {
		t.Fatalf("bad array pattern: %+v", pat)
	}
	stmt = parseOne(t, "const {x, y: zed} = obj").(*ast.DestructureLetStatement)
	if !stmt.IsConst {
		t.Fatal("const destructure should be const")
	}
	hp, ok := stmt.Pattern.(*ast.HashPattern)
	if !ok {
		t.Fatalf("expected HashPattern, got %T", stmt.Pattern)
	}
	if len(hp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(hp.Entries))
	}
	if hp.Entries[0].Key.Value != "x" || hp.Entries[0].Value.Value != "x" {
		t.Fatalf("shorthand entry wrong: %+v", hp.Entries[0])
	}
	if hp.Entries[1].Key.Value != "y" || hp.Entries[1].Value.Value != "zed" {
		t.Fatalf("renamed entry wrong: %+v", hp.Entries[1])
	}
}

func TestWave1LoopElseAndLabels(t *testing.T) {
	stmt := parseOne(t, "for (x in xs) { print x } else { print 0 }")
	fi, ok := stmt.(*ast.ForInStatement)
	if !ok {
		t.Fatalf("expected ForInStatement, got %T", stmt)
	}
	if fi.OrElse == nil {
		t.Fatal("expected OrElse block")
	}
	stmt = parseOne(t, "outer: for (x in xs) { break outer }")
	fi = stmt.(*ast.ForInStatement)
	if fi.Label != "outer" {
		t.Fatalf("expected label outer, got %q", fi.Label)
	}
	bs, ok := fi.Body.Statements[0].(*ast.BreakStatement)
	if !ok || bs.Label != "outer" {
		t.Fatalf("expected labeled break, got %+v", fi.Body.Statements[0])
	}
	stmt = parseOne(t, "while (c) { continue } else { print 1 }")
	ws, ok := stmt.(*ast.WhileStatement)
	if !ok || ws.OrElse == nil {
		t.Fatalf("expected WhileStatement with else, got %T", stmt)
	}
}

func TestWave1Interpolation(t *testing.T) {
	is, ok := parseExpr(t, `"hi ${name}!"`).(*ast.InterpolatedString)
	if !ok {
		t.Fatalf("expected InterpolatedString, got %T", parseExpr(t, `"hi ${name}!"`))
	}
	if len(is.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(is.Parts))
	}
	// plain string stays a StringLiteral
	sl, ok := parseExpr(t, `"plain"`).(*ast.StringLiteral)
	if !ok || sl.Value != "plain" {
		t.Fatalf("expected plain StringLiteral, got %T", parseExpr(t, `"plain"`))
	}
}
