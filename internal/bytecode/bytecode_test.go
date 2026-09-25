package bytecode

// Tests for the bytecode compiler + stack VM ported from the 2.8 track.
// Honest scope: arithmetic, comparisons, let/const, if/else, print.
// Function calls (other than print), loops, and data structures are NOT
// supported — the compiler returns a loud error for those.

import (
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

func compileRun(t *testing.T, src string) (string, error) {
	t.Helper()
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	c := NewCompiler()
	if err := c.Compile(prog); err != nil {
		return "", err
	}
	vm := NewVM(c.Bytecode())
	if _, err := vm.Run(); err != nil {
		return "", err
	}
	return vm.Output.String(), nil
}

func TestBytecodeArithmetic(t *testing.T) {
	out, err := compileRun(t, `print 1 + 2 * 3`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "7" {
		t.Fatalf("got %q, want 7", out)
	}
}

func TestBytecodeLetAndCompare(t *testing.T) {
	out, err := compileRun(t, `
let x = 10
print x * 2
if (x > 5) { print "big" } else { print "small" }
print 7 % 3
`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"20", "big", "1"}
	if len(lines) != len(want) {
		t.Fatalf("got %q", out)
	}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestBytecodeRejectsFunctions(t *testing.T) {
	_, err := compileRun(t, `fn f(x) { return x + 1 } print f(2)`)
	if err == nil {
		t.Fatal("want compile error for fn, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error should say unsupported, got: %v", err)
	}
}

func TestBytecodeRejectsNonPrintCalls(t *testing.T) {
	_, err := compileRun(t, `print len("abc")`)
	if err == nil {
		t.Fatal("want compile error for len(), got nil")
	}
}

func TestBytecodeDisassemble(t *testing.T) {
	l := lexer.New(`print 1 + 2`)
	p := parser.New(l)
	c := NewCompiler()
	if err := c.Compile(p.ParseProgram()); err != nil {
		t.Fatal(err)
	}
	d := Disassemble(c.Bytecode())
	for _, want := range []string{"Constant", "Add", "Print", "Halt"} {
		if !strings.Contains(d, want) {
			t.Fatalf("disassembly missing %s:\n%s", want, d)
		}
	}
}
