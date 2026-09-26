package parser

import (
	"testing"

	"github.com/navescript/nvs/internal/ast"
)

// ---- Wave 4: pattern matching ----

func parseMatch(t *testing.T, input string) *ast.MatchExpression {
	t.Helper()
	e := parseExpr(t, input)
	me, ok := e.(*ast.MatchExpression)
	if !ok {
		t.Fatalf("expected *ast.MatchExpression for %q, got %T", input, e)
	}
	return me
}

func TestWave4GuardAST(t *testing.T) {
	me := parseMatch(t, `match (x) { case n if n > 0: { 1 } }`)
	if len(me.Arms) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(me.Arms))
	}
	arm := me.Arms[0]
	if id, ok := arm.Pattern.(*ast.Identifier); !ok || id.Value != "n" {
		t.Fatalf("expected identifier pattern n, got %T (%s)", arm.Pattern, arm.Pattern.String())
	}
	if arm.Guard == nil {
		t.Fatal("expected guard, got nil")
	}
	if _, ok := arm.Guard.(*ast.InfixExpression); !ok {
		t.Fatalf("expected infix guard, got %T", arm.Guard)
	}
	if arm.Guard.String() != "(n > 0)" {
		t.Fatalf("unexpected guard: %s", arm.Guard.String())
	}
}

func TestWave4GuardAbsent(t *testing.T) {
	me := parseMatch(t, `match (x) { case 1: { 2 } }`)
	if me.Arms[0].Guard != nil {
		t.Fatalf("expected nil guard, got %s", me.Arms[0].Guard.String())
	}
}

func TestWave4GuardOnDestructurePattern(t *testing.T) {
	me := parseMatch(t, `match (p) { case [a, b] if a != b: { 1 } }`)
	arm := me.Arms[0]
	if _, ok := arm.Pattern.(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected array literal pattern, got %T", arm.Pattern)
	}
	if arm.Guard == nil {
		t.Fatal("expected guard, got nil")
	}
}

func TestWave4HashPatternAST(t *testing.T) {
	me := parseMatch(t, `match (m) { case {x, y}: { 1 } }`)
	arm := me.Arms[0]
	hp, ok := arm.Pattern.(*ast.HashPattern)
	if !ok {
		t.Fatalf("expected *ast.HashPattern, got %T", arm.Pattern)
	}
	if len(hp.Entries) != 2 || hp.Entries[0].Key.Value != "x" || hp.Entries[1].Value.Value != "y" {
		t.Fatalf("unexpected entries: %s", hp.String())
	}
}

func TestWave4HashLiteralPatternFallsBack(t *testing.T) {
	// {x: 1} is not wave-1-shaped (non-identifier value) → stays a hash
	// literal expression, compared by value as before.
	me := parseMatch(t, `match (m) { case {x: 1}: { 2 } }`)
	if _, ok := me.Arms[0].Pattern.(*ast.HashLiteral); !ok {
		t.Fatalf("expected *ast.HashLiteral, got %T", me.Arms[0].Pattern)
	}
}

func TestWave4TuplePatternAST(t *testing.T) {
	me := parseMatch(t, `match (t) { case (a, b): { 1 } }`)
	if _, ok := me.Arms[0].Pattern.(*ast.TupleLiteral); !ok {
		t.Fatalf("expected *ast.TupleLiteral, got %T", me.Arms[0].Pattern)
	}
}

func TestWave4RecordPatternAST(t *testing.T) {
	// Point(x, y) parses as a call; the evaluator sniffs the record shape.
	me := parseMatch(t, `match (p) { case Point(x, y): { 1 } }`)
	ce, ok := me.Arms[0].Pattern.(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected *ast.CallExpression, got %T", me.Arms[0].Pattern)
	}
	fn, ok := ce.Function.(*ast.Identifier)
	if !ok || fn.Value != "Point" {
		t.Fatalf("expected call of Point, got %s", ce.Function.String())
	}
	if len(ce.Arguments) != 2 {
		t.Fatalf("expected 2 args, got %d", len(ce.Arguments))
	}
}

func TestWave4OptionalParens(t *testing.T) {
	with := parseMatch(t, `match (x) { case 1: { 2 } }`)
	without := parseMatch(t, `match x { case 1: { 2 } }`)
	if with.Value.String() != without.Value.String() {
		t.Fatalf("scrutinee differs: %q vs %q", with.Value.String(), without.Value.String())
	}
	if len(without.Arms) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(without.Arms))
	}
}

func TestWave4SingleStatementArm(t *testing.T) {
	// case 1: "one" — single-expression arm without braces (was a parse
	// error before wave 4).
	me := parseMatch(t, `match (v) { case 1: "one"; default: "other" }`)
	if len(me.Arms) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(me.Arms))
	}
	body := me.Arms[0].Body
	if len(body.Statements) != 1 {
		t.Fatalf("expected 1 body statement, got %d", len(body.Statements))
	}
	if me.Default == nil || len(me.Default.Statements) != 1 {
		t.Fatal("expected single-statement default")
	}
}

func TestWave4SwitchAliasStillParses(t *testing.T) {
	me := parseMatch(t, `switch (x) { case n if n > 1: { 1 } default: { 2 } }`)
	if me.TokenLiteral() != "switch" {
		t.Fatalf("expected switch token, got %q", me.TokenLiteral())
	}
	if me.Arms[0].Guard == nil {
		t.Fatal("expected guard on switch arm")
	}
}
