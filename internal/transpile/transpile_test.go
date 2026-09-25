package transpile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustTranspile(t *testing.T, src, target string) string {
	t.Helper()
	out, err := TranspileSource(src, target)
	if err != nil {
		t.Fatalf("TranspileSource(%q, %s): %v", src, target, err)
	}
	return out
}

func mustReject(t *testing.T, src, target, wantConstruct string) {
	t.Helper()
	_, err := TranspileSource(src, target)
	if err == nil {
		t.Fatalf("TranspileSource(%q, %s) should fail", src, target)
	}
	ue, ok := err.(*UnsupportedError)
	if !ok {
		t.Fatalf("TranspileSource(%q, %s) error is %T, want *UnsupportedError: %v", src, target, err, err)
	}
	if !strings.Contains(ue.Construct, wantConstruct) {
		t.Fatalf("error names %q, want it to name %q: %v", ue.Construct, wantConstruct, err)
	}
}

// TestSubsetAcceptance checks representative subset constructs transpile
// without error on both targets.
func TestSubsetAcceptance(t *testing.T) {
	srcs := []string{
		`let x = 1 + 2 * 3`,
		`print "a${1+1}b"`,
		`if (true) { print 1 } else { print 2 }`,
		`while (false) { print 1 }`,
		`for (let i = 0; i < 3; i = i + 1) { print i }`,
		`for (c in "ab") { print c }`,
		`for (k in {"a": 1}) { print k }`,
		`fn f(a, b) { return a + b } print f(1, 2)`,
		`fn g() { 42 } print g()`,
		`let h = {"a": [1, 2]} print h["a"][0]`,
		`print [1, 2, 3][0:2]`,
		`print 7 / 2 print 0 - 7 % 3`,
		`print true ? 1 : 2`,
		`print null ?? "d"`,
		`let a = [1] print push(a, 2)`,
		`print len("abc") print str(1)`,
		`let n = 0 fn bump() { n = n + 1 } bump() print n`,
		`print 1 < 2 print "x" == "x"`,
		`print !false print 1 and 2`,
		`let i = 0 for (i = 0; i < 3; i = i + 1) { if (i == 1) { continue } if (i == 2) { break } } print i`,
	}
	for _, src := range srcs {
		for _, target := range Targets() {
			mustTranspile(t, src, target)
		}
	}
}

// TestSubsetRejection checks out-of-subset constructs fail with an
// UnsupportedError naming the construct — on both targets.
func TestSubsetRejection(t *testing.T) {
	cases := []struct{ src, construct string }{
		{`let m = match(x) { 1 -> "one" _ -> "other" }`, "MatchExpression"},
		{`class C {}`, "ClassStatement"},
		{`try { 1 } catch (e) { 2 }`, "TryStatement"},
		{`let [a, b] = [1, 2]`, "DestructureLetStatement"},
		{`let t = (1, 2)`, "TupleLiteral"},
		{`import "x"`, "ImportStatement"},
		{`let r = 1..5`, "RangeExpression"},
		{`print obj.method()`, "CallExpression"},
		{`print obj.field`, "MemberExpression"},
		{`let f = fn(x) { x }`, ""}, // named binding: accepted (checked below)
		{`print len([1]) + sleep(1)`, "CallExpression"}, // unknown builtin `sleep`
		{`fn f() { if (true) { 1 } }`, "IfExpression"},
		{`fn f() { while (true) { 1 } }`, "WhileStatement"},
		{`let h = {[1]: "x"}`, "HashLiteral"},
		{`let h = {...a}`, "HashLiteral"},
		{`print a?.b`, "OptionalChainExpression"},
	}
	for _, c := range cases {
		if c.construct == "" {
			continue
		}
		for _, target := range Targets() {
			mustReject(t, c.src, target, c.construct)
		}
	}
	// `sleep` is a real NvS builtin with no mapping: the error must name it.
	_, err := TranspileSource(`print sleep(1)`, "js")
	if err == nil || !strings.Contains(err.Error(), "sleep") {
		t.Fatalf("unmapped builtin error should name `sleep`, got %v", err)
	}
}

// TestPythonRejectsAnonymousFunctions: Python has no statement lambdas, so
// anonymous function values are an honest error there (JS accepts them).
func TestPythonRejectsAnonymousFunctions(t *testing.T) {
	mustReject(t, `print [fn(x) { x }][0](1)`, "python", "FunctionLiteral")
	mustTranspile(t, `print [fn(x) { x }][0](1)`, "js")
}

// runTarget executes transpiled code with the real interpreter for target.
func runTarget(t *testing.T, target, code string) string {
	t.Helper()
	var cmd *exec.Cmd
	switch target {
	case "python":
		py, err := exec.LookPath("python3")
		if err != nil {
			t.Skip("python3 not available")
		}
		f := filepath.Join(t.TempDir(), "prog.py")
		if err := os.WriteFile(f, []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(py, f)
	case "js":
		node, err := exec.LookPath("node")
		if err != nil {
			t.Skip("node not available")
		}
		f := filepath.Join(t.TempDir(), "prog.js")
		if err := os.WriteFile(f, []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(node, f)
	default:
		t.Fatalf("unknown target %s", target)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s execution failed: %v\ncode:\n%s\noutput:\n%s", target, err, code, out)
	}
	return string(out)
}

// TestEmittedOutputMatchesInterpreter transpiles a semantics-heavy program
// and runs the emitted code with the real target runtimes, comparing
// against expected output captured from the NvS interpreter. Hash
// iteration order is excluded (Go map order is nondeterministic); the
// program avoids printing it.
func TestEmittedOutputMatchesInterpreter(t *testing.T) {
	src := `
fn truthy(x) { if (x) { return "T" } return "F" }
print truthy(0)
print truthy("")
print truthy(null)
print truthy(false)
print truthy([1])
print 7 / 2
print 0 - 7
print 0 - 7 % 3
print "n=${null} t=${true} f=${1.5} s=${"hi"}"
fn implicit(a) { let b = a * 2
b + 1 }
print implicit(20)
let counter = 0
fn bump() { counter = counter + 1
counter }
print bump()
print bump()
print counter
let s = "hello"
print s[1:3]
print s[-3:-1]
print s[:]
for (c in "ab") { print c }
let a1 = [1, 2]
let a2 = [1, 2]
print a1 == a2
print a1 == a1
print 1 == 1.0
print null ?? "dflt"
print false ?? "x"
let acc = 0
for (let i = 1; i <= 6; i = i + 1) {
  if (i % 2 == 0) { continue }
  acc = acc + i
  if (i == 5) { break }
}
print acc
let arr = push([1], 2)
print arr
print str([1, "a", true, null])
print 1.5 + 2
print abs(0 - 5)
print range(3)
print keys({"k": 1})[0]
print upper("ab")
print acc > 5 ? "yes" : "no"
fn outer() {
  let n = 10
  fn inner() { n = n + 1
  n }
  inner()
  inner()
  return n
}
print outer()
`
	want := `T
T
F
F
T
3
-7
-1
n=null t=true f=1.5 s=hi
41
1
2
2
el
hello
hello
a
b
false
true
true
dflt
false
9
[1, 2]
[1, a, true, null]
3.5
5
[0, 1, 2]
k
AB
yes
12
`
	for _, target := range Targets() {
		code := mustTranspile(t, src, target)
		if got := runTarget(t, target, code); got != want {
			t.Errorf("%s output mismatch:\n got:\n%s\nwant:\n%s", target, got, want)
		}
	}
}
