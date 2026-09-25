package eval

// Regression tests for bugs found while reconciling the 2.2–2.9 track:
//  1. `and`/`or` bound too tightly (same level as ==), so
//     `c == "+" or c == "-"` evaluated as `c == ("+" or c) == "-"`.
//  2. String `<`, `>`, `<=`, `>=` were unsupported (needed by selfhost).
//  3. A `while`/`for` loop whose final iteration hit `continue` leaked the
//     Continue signal object as the loop's value (e.g. a function returning
//     the string "continue").

import (
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func TestReconcileAndOrPrecedence(t *testing.T) {
	cases := []struct{ src, want string }{
		{`1 == 1 or 2 == 3`, "true"},
		{`1 == 2 or 2 == 2`, "true"},
		{`1 == 2 or 2 == 3`, "false"},
		{`1 == 1 and 2 == 2`, "true"},
		{`1 == 1 and 2 == 3`, "false"},
		// and/or bind looser than comparison, tighter than assignment-like use
		{`let c = "+"; c == "+" or c == "-"`, "true"},
		{`let c = "x"; c == "+" or c == "-"`, "false"},
		{`!(1 == 2) and 3 == 3`, "true"},
	}
	for _, c := range cases {
		env := object.NewEnvironment()
		LoadPrelude(env)
		l := lexer.New(c.src)
		p := parser.New(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Fatalf("parse errors for %q: %v", c.src, p.Errors())
		}
		got := Eval(prog, env)
		if isError(got) {
			t.Fatalf("eval error for %q: %s", c.src, got.Inspect())
		}
		if got.Inspect() != c.want {
			t.Errorf("%q = %s, want %s", c.src, got.Inspect(), c.want)
		}
	}
}

func TestReconcileStringComparisons(t *testing.T) {
	cases := []struct{ src, want string }{
		{`"a" < "b"`, "true"},
		{`"b" > "a"`, "true"},
		{`"a" <= "a"`, "true"},
		{`"a" >= "b"`, "false"},
		{`"abc" < "abd"`, "true"},
		{`"+" < "0"`, "true"}, // "+" (0x2B) < "0" (0x30)
	}
	for _, c := range cases {
		env := object.NewEnvironment()
		LoadPrelude(env)
		l := lexer.New(c.src)
		p := parser.New(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Fatalf("parse errors for %q: %v", c.src, p.Errors())
		}
		got := Eval(prog, env)
		if isError(got) {
			t.Fatalf("eval error for %q: %s", c.src, got.Inspect())
		}
		if got.Inspect() != c.want {
			t.Errorf("%q = %s, want %s", c.src, got.Inspect(), c.want)
		}
	}
}

func TestReconcileContinueDoesNotLeak(t *testing.T) {
	src := `
fn f() {
  let lines = ["a", "", "b", ""]
  let li = 0
  let out = ""
  while (li < len(lines)) {
    let line = lines[li]
    if (line == "") { li = li + 1; continue }
    out = out + "[" + line + "]"
    li = li + 1
  }
  return out
}
f()
`
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	got := Eval(prog, env)
	if isError(got) {
		t.Fatalf("eval error: %s", got.Inspect())
	}
	if got.Inspect() != "[a][b]" {
		t.Errorf("got %q, want %q", got.Inspect(), "[a][b]")
	}

	// for-loop variant: trailing continue must not leak either
	src2 := `
fn g() {
  let out = ""
  for (let i = 0; i < 4; i = i + 1) {
    if (i == 3) { continue }
    out = out + "x"
  }
  return out
}
g()
`
	env2 := object.NewEnvironment()
	LoadPrelude(env2)
	l2 := lexer.New(src2)
	p2 := parser.New(l2)
	prog2 := p2.ParseProgram()
	if len(p2.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p2.Errors())
	}
	got2 := Eval(prog2, env2)
	if isError(got2) {
		t.Fatalf("eval error: %s", got2.Inspect())
	}
	if got2.Inspect() != "xxx" {
		t.Errorf("got %q, want %q", got2.Inspect(), "xxx")
	}
}
