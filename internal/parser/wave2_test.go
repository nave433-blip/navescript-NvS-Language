package parser

import (
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

// parseCallArgs parses `f(<input>)` and returns the call's argument list.
func parseCallArgs(t *testing.T, input string) []ast.Expression {
	t.Helper()
	call, ok := parseExpr(t, "f("+input+")").(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected CallExpression for f(%s), got %T", input, parseExpr(t, "f("+input+")"))
	}
	return call.Arguments
}

func namedArg(t *testing.T, e ast.Expression, wantName string) *ast.NamedArgument {
	t.Helper()
	na, ok := e.(*ast.NamedArgument)
	if !ok {
		t.Fatalf("expected *ast.NamedArgument, got %T (%s)", e, e.String())
	}
	if na.Name.Value != wantName {
		t.Fatalf("expected named arg %q, got %q", wantName, na.Name.Value)
	}
	return na
}

func TestWave2NamedArgBasic(t *testing.T) {
	args := parseCallArgs(t, "x: 1")
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	na := namedArg(t, args[0], "x")
	if na.Value.String() != "1" {
		t.Fatalf("expected value 1, got %s", na.Value.String())
	}
}

func TestWave2NamedArgMultipleAndMixed(t *testing.T) {
	args := parseCallArgs(t, "1, y: 2, z: x + 1")
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if _, ok := args[0].(*ast.IntegerLiteral); !ok {
		t.Fatalf("expected positional IntegerLiteral first, got %T", args[0])
	}
	namedArg(t, args[1], "y")
	na := namedArg(t, args[2], "z")
	if na.Value.String() != "(x + 1)" {
		t.Fatalf("expected complex value (x + 1), got %s", na.Value.String())
	}
}

func TestWave2NamedArgDoesNotEatTernary(t *testing.T) {
	// f(a ? b : c): the ternary consumes its own ':' — not a named arg.
	args := parseCallArgs(t, "a ? b : c")
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if _, ok := args[0].(*ast.TernaryExpression); !ok {
		t.Fatalf("expected TernaryExpression, got %T", args[0])
	}
	// Ternary as a named-arg *value* is fine.
	args = parseCallArgs(t, "x: a ? b : c")
	na := namedArg(t, args[0], "x")
	if _, ok := na.Value.(*ast.TernaryExpression); !ok {
		t.Fatalf("expected ternary value, got %T", na.Value)
	}
}

func TestWave2AssignStillAssign(t *testing.T) {
	// Backward compat: f(x = 1) is an assignment expression, not named.
	args := parseCallArgs(t, "x = 1")
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if _, ok := args[0].(*ast.AssignExpression); !ok {
		t.Fatalf("expected AssignExpression, got %T", args[0])
	}
}

func TestWave2HashLiteralNotNamed(t *testing.T) {
	args := parseCallArgs(t, "{x: 1}")
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if _, ok := args[0].(*ast.HashLiteral); !ok {
		t.Fatalf("expected HashLiteral, got %T", args[0])
	}
}

func TestWave2PositionalAfterNamedIsError(t *testing.T) {
	l := lexer.New("f(y: 2, 1)")
	p := New(l)
	p.ParseProgram()
	errs := p.Errors()
	if len(errs) == 0 {
		t.Fatal("expected parse error for positional-after-named, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "positional argument follows named argument") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected positional-after-named error, got %v", errs)
	}
}

func TestWave2NamedArgInMethodAndChain(t *testing.T) {
	call, ok := parseExpr(t, "obj.m(x: 1)").(*ast.CallExpression)
	if !ok {
		t.Fatalf("expected CallExpression, got %T", parseExpr(t, "obj.m(x: 1)"))
	}
	if _, ok := call.Function.(*ast.MemberExpression); !ok {
		t.Fatalf("expected MemberExpression function, got %T", call.Function)
	}
	namedArg(t, call.Arguments[0], "x")

	oc, ok := parseExpr(t, "obj?.m(x: 1)").(*ast.OptionalChainExpression)
	if !ok {
		t.Fatalf("expected OptionalChainExpression, got %T", parseExpr(t, "obj?.m(x: 1)"))
	}
	if oc.Links[1].Kind != ast.ChainCall {
		t.Fatalf("expected ChainCall link, got %q", oc.Links[1].Kind)
	}
	namedArg(t, oc.Links[1].Arguments[0], "x")
}
