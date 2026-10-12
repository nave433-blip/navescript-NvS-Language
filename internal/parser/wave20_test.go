package parser

import "testing"

// ---- Wave 20: parse errors must never panic ----

// A parse failure inside `pub class ...` used to arrive at
// parsePubStatement as a TYPED nil *ast.ClassStatement, and the
// `.Public = true` write panicked. Regression: errors, not panics.
func TestWave20PubClassParseErrorNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("parser panicked on pub class parse error: %v", r)
		}
	}()
	expectParseError(t, `pub class Bad implements X { fn m() {} }`, "expected '{'")
}

func TestWave20PubFnParseErrorNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("parser panicked on pub fn parse error: %v", r)
		}
	}()
	expectParseError(t, `pub fn broken( { }`, "expected ')'")
}

func TestWave20PubClassStillWorks(t *testing.T) {
	prog := parseOne19(t, `pub class Good { fn m() { return 1; } }`)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
}
