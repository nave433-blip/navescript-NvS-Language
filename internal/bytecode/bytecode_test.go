package bytecode

// Tests for the bytecode compiler + stack VM ported from the 2.8 track.
// Honest scope: arithmetic (with truncating int division), string concat
// and comparison, comparisons, let/const, if/else, while and C-style for
// loops with break/continue, print(...), len() over strings and arrays,
// array literals.
// Function calls (other than print/len), for-in loops, and data structures
// beyond arrays are NOT supported — the compiler returns a loud error.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
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
	_, err := compileRun(t, `print foo(1)`)
	if err == nil {
		t.Fatal("want compile error for foo(), got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error should say unsupported, got: %v", err)
	}
}

func TestBytecodeLen(t *testing.T) {
	out, err := compileRun(t, `print len("hello")`+"\n"+`print len([1, 2, 3])`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || strings.TrimSpace(lines[0]) != "5" || strings.TrimSpace(lines[1]) != "3" {
		t.Fatalf("got %q, want 5 and 3", out)
	}
}

func TestBytecodeLenArity(t *testing.T) {
	_, err := compileRun(t, `print len("a", "b")`)
	if err == nil || !strings.Contains(err.Error(), "exactly 1 argument") {
		t.Fatalf("want arity error, got: %v", err)
	}
}

func TestBytecodeLenTypeError(t *testing.T) {
	_, err := compileRun(t, `print len(42)`)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("want runtime len type error, got: %v", err)
	}
}

func TestBytecodeStringConcat(t *testing.T) {
	out, err := compileRun(t, `print "foo" + "bar"`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "foobar" {
		t.Fatalf("got %q, want foobar", out)
	}
}

func TestBytecodeStringConcatTypeError(t *testing.T) {
	l := lexer.New(`print "a" + 1`)
	p := parser.New(l)
	prog := p.ParseProgram()
	c := NewCompiler()
	if err := c.Compile(prog); err != nil {
		t.Fatal(err)
	}
	vm := NewVM(c.Bytecode())
	_, err := vm.Run()
	if err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("want type mismatch, got: %v", err)
	}
}

func TestBytecodeStringCompare(t *testing.T) {
	out, err := compileRun(t, `
print "b" > "a"
print "a" < "b"
print "a" <= "a"
print "b" >= "c"
`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := []string{"true", "true", "true", "false"}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Fatalf("line %d = %q, want %q (full: %q)", i, lines[i], w, out)
		}
	}
}

func TestBytecodeIntDivision(t *testing.T) {
	// Truncating, like the tree-walker: 7/2 == 3, not 3.5.
	out, err := compileRun(t, `print 7 / 2`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "3" {
		t.Fatalf("got %q, want 3", out)
	}
}

func TestBytecodeDivByZero(t *testing.T) {
	l := lexer.New(`print 1 / 0`)
	p := parser.New(l)
	c := NewCompiler()
	if err := c.Compile(p.ParseProgram()); err != nil {
		t.Fatal(err)
	}
	vm := NewVM(c.Bytecode())
	_, err := vm.Run()
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("want division by zero, got: %v", err)
	}
}

func TestBytecodeModByZero(t *testing.T) {
	l := lexer.New(`print 1 % 0`)
	p := parser.New(l)
	c := NewCompiler()
	if err := c.Compile(p.ParseProgram()); err != nil {
		t.Fatal(err)
	}
	vm := NewVM(c.Bytecode())
	_, err := vm.Run()
	if err == nil || !strings.Contains(err.Error(), "modulo by zero") {
		t.Fatalf("want modulo by zero, got: %v", err)
	}
}

func TestBytecodeWhile(t *testing.T) {
	out, err := compileRun(t, `
let i = 0
while (i < 3) {
  print i
  i = i + 1
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "0\n1\n2" {
		t.Fatalf("got %q", out)
	}
}

func TestBytecodeFor(t *testing.T) {
	out, err := compileRun(t, `
for (let i = 0; i < 3; i = i + 1) {
  print i * 10
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "0\n10\n20" {
		t.Fatalf("got %q", out)
	}
}

func TestBytecodeBreakContinue(t *testing.T) {
	out, err := compileRun(t, `
let i = 0
while (true) {
  i = i + 1
  if (i % 2 == 0) { continue }
  if (i > 5) { break }
  print i
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "1\n3\n5" {
		t.Fatalf("got %q", out)
	}
}

func TestBytecodeForBreakContinue(t *testing.T) {
	out, err := compileRun(t, `
for (let i = 0; i < 10; i = i + 1) {
  if (i == 2) { continue }
  if (i == 4) { break }
  print i
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "0\n1\n3" {
		t.Fatalf("got %q", out)
	}
}

func TestBytecodeBreakOutsideLoop(t *testing.T) {
	_, err := compileRun(t, `break`)
	if err == nil || !strings.Contains(err.Error(), "break outside loop") {
		t.Fatalf("want break-outside-loop error, got: %v", err)
	}
}

func TestBytecodeArray(t *testing.T) {
	out, err := compileRun(t, `
let a = [10, 20, 30]
print len(a)
`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "3" {
		t.Fatalf("got %q, want 3", out)
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

// treeWalk runs src through the tree-walking interpreter, capturing what
// the print builtin writes to stdout.
func treeWalk(t *testing.T, src string) string {
	t.Helper()
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	// Capture os.Stdout while the tree-walker runs.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	env := object.NewEnvironment()
	eval.LoadPrelude(env)
	result := eval.Eval(prog, env)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	if result != nil && result.Type() == object.ERROR_OBJ {
		t.Fatalf("tree-walker error: %s", result.Inspect())
	}
	return buf.String()
}

// TestBytecodeMatchesTreeWalker runs the same programs through the VM and
// the tree-walker and requires identical printed output.
func TestBytecodeMatchesTreeWalker(t *testing.T) {
	progs := []string{
		`print 1 + 2 * 3`,
		`print "foo" + "bar"`,
		`print 7 / 2`,
		`print 7 % 3`,
		`print "b" > "a"`,
		"print len(\"hello\")",
		"print len([1, 2, 3])",
		"let x = 10\nprint x * 2\nif (x > 5) { print \"big\" } else { print \"small\" }",
		"let i = 0\nwhile (i < 3) { print i\n i = i + 1 }",
		"for (let i = 0; i < 3; i = i + 1) { print i * 10 }",
		"let s = 0\nfor (let i = 1; i <= 5; i = i + 1) { if (i == 3) { continue }\n s = s + i }\nprint s",
	}
	for _, src := range progs {
		vmOut, err := compileRun(t, src)
		if err != nil {
			t.Fatalf("vm failed on %q: %v", src, err)
		}
		twOut := treeWalk(t, src)
		if vmOut != twOut {
			t.Fatalf("mismatch on %q:\nvm: %q\ntree-walker: %q", src, vmOut, twOut)
		}
	}
}

func TestBytecodeDisassembleLoop(t *testing.T) {
	l := lexer.New(`while (true) { break }`)
	p := parser.New(l)
	c := NewCompiler()
	if err := c.Compile(p.ParseProgram()); err != nil {
		t.Fatal(err)
	}
	d := Disassemble(c.Bytecode())
	for _, want := range []string{"Jump", "JumpNotTruthy"} {
		if !strings.Contains(d, want) {
			t.Fatalf("disassembly missing %s:\n%s", want, d)
		}
	}
}

func TestBytecodePower(t *testing.T) {
	out, err := compileRun(t, "print 2 ** 10")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "1024" {
		t.Fatalf("got %q, want 1024", out)
	}
	// VM/tree-walker agreement on negative int exponent (NvS semantics: 0).
	out, err = compileRun(t, "print 2 ** -1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "0" {
		t.Fatalf("got %q, want 0", out)
	}
}
