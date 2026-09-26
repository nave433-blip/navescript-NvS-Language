package checker

import (
	"strings"
	"testing"
)

// check is a test helper: returns the error messages for src.
func check(t *testing.T, src string) []string {
	t.Helper()
	errs := CheckSource("t.ns", src)
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
		if !strings.HasPrefix(msgs[i], "t.ns:") {
			t.Errorf("error %q missing file:line prefix", msgs[i])
		}
	}
	return msgs
}

func expectClean(t *testing.T, src string) {
	t.Helper()
	if msgs := check(t, src); len(msgs) > 0 {
		t.Fatalf("expected no errors, got:\n%s", strings.Join(msgs, "\n"))
	}
}

func expectErrors(t *testing.T, src string, substrs ...string) {
	t.Helper()
	msgs := check(t, src)
	if len(msgs) != len(substrs) {
		t.Fatalf("expected %d errors, got %d:\n%s", len(substrs), len(msgs), strings.Join(msgs, "\n"))
	}
	for i, sub := range substrs {
		if !strings.Contains(msgs[i], sub) {
			t.Errorf("error %d = %q, want substring %q", i, msgs[i], sub)
		}
	}
}

func TestCheckCleanUnannotated(t *testing.T) {
	expectClean(t, `
let x = 1
x = "hello"
fn add(a, b) { return a + b }
print add(1, 2)
for (i = 0; i < 10; i = i + 1) { print i }
`)
}

func TestCheckTypedLet(t *testing.T) {
	expectClean(t, `let x: int = 42`)
	expectClean(t, `let x: string = "hi"`)
	expectClean(t, `let x: int | string = "hi"`)
	expectClean(t, `let x: number = 1.5`)
	expectClean(t, `let x: number = 1`)
	expectClean(t, `let x: any = true`)
	expectErrors(t, `let x: int = "hi"`,
		"cannot assign string to variable 'x' of type int")
	expectErrors(t, `let x: int | string = true`,
		"cannot assign bool to variable 'x' of type int | string")
	expectErrors(t, `let x: string = 42`,
		"t.ns:1: cannot assign int to variable 'x' of type string")
}

func TestCheckReassignment(t *testing.T) {
	expectClean(t, "let x: int = 1\nx = 2\n")
	expectErrors(t, "let x: int = 1\nx = \"s\"\n",
		"cannot assign string to variable 'x' of type int")
	// Unannotated variables accept anything (gradual).
	expectClean(t, "let x = 1\nx = \"s\"\n")
}

func TestCheckFnAnnotations(t *testing.T) {
	expectClean(t, `fn add(a: int, b: int): int { return a + b }`)
	expectErrors(t, `fn add(a: int, b: int): int { return "x" }`,
		"return type mismatch: got string, want int")
	expectErrors(t, `fn f(): string { return 42 }`,
		"return type mismatch: got int, want string")
	// Explicit null return against a non-null return type.
	expectErrors(t, `fn f(): int { return null }`,
		"return type mismatch: got null, want int")
	expectClean(t, `fn f(): int | null { return null }`)
}

func TestCheckCallArity(t *testing.T) {
	expectClean(t, `fn add(a, b) { return a + b }`+"\n"+`print add(1, 2)`)
	expectErrors(t, `fn add(a, b) { return a + b }`+"\n"+`print add(1)`,
		"wrong number of arguments: got 1, want 2..2")
	expectErrors(t, `fn add(a, b) { return a + b }`+"\n"+`print add(1, 2, 3)`,
		"wrong number of arguments: got 3, want 2..2")
	// Defaults widen the accepted range.
	expectClean(t, `fn g(a, b = 2) { return a + b }`+"\n"+`print g(1)`)
	expectErrors(t, `fn g(a, b = 2) { return a + b }`+"\n"+`print g()`,
		"wrong number of arguments: got 0, want 1..2")
	// Unknown callees are not checked (gradual).
	expectClean(t, `print unknown_fn(1, 2, 3)`)
}

func TestCheckCallArgTypes(t *testing.T) {
	expectClean(t, `fn add(a: int, b: int): int { return a + b }`+"\n"+`print add(1, 2)`)
	expectErrors(t, `fn add(a: int, b: int): int { return a + b }`+"\n"+`print add(1, "x")`,
		"argument 2: got string, want int")
	// Unannotated params accept anything.
	expectClean(t, `fn f(a) { return a }`+"\n"+`print f("x")`)
	// Builtin len: arity + return type.
	expectClean(t, `let n: int = len("abc")`)
	expectErrors(t, `print len("a", "b")`,
		"wrong number of arguments: got 2, want 1..1")
	expectErrors(t, `let s: string = len("abc")`,
		"cannot assign int to variable 's' of type string")
}

func TestCheckUnknownType(t *testing.T) {
	expectErrors(t, `let x: Nope = 1`, "unknown type 'Nope'")
	expectErrors(t, `fn f(a: Nope) { return a }`, "unknown type 'Nope' in parameter 'a'")
	expectErrors(t, `fn f(): Nope { return 1 }`, "unknown type 'Nope' in return type")
	// Builtin annotation names are case-insensitive, exactly like the
	// runtime (checkAnnotation lowercases before the builtin table).
	expectClean(t, `let x: Int = 1`)
	expectClean(t, `let x: STRING = "a"`)
}

func TestCheckUnionReturn(t *testing.T) {
	src := `
fn f(flag: bool): int | string {
  if (flag) { return 1 } else { return "x" }
}
let a: int | string = f(true)
`
	expectClean(t, src)
	expectErrors(t, `
fn f(flag: bool): int | string {
  if (flag) { return 1 } else { return true }
}
`, "return type mismatch: got bool, want int | string")
}

func TestCheckInterfaceStructural(t *testing.T) {
	iface := `interface Speaker { speak(volume: int): string }` + "\n"
	// Class instance satisfying the interface: clean.
	expectClean(t, iface+`
class Dog {
  fn speak(volume: int): string { return "woof" }
}
let s: Speaker = new Dog()
`)
	// Class missing the method: error.
	expectErrors(t, iface+`
class Cat {
  fn meow(): string { return "meow" }
}
let s: Speaker = new Cat()
`, "cannot assign Cat to variable 's' of type Speaker")
	// Hash literal with matching function: clean.
	expectClean(t, iface+`
let s: Speaker = { "speak": fn(volume: int): string { return "hi" } }
`)
	// Hash literal missing the method: error.
	expectErrors(t, iface+`
let s: Speaker = { "other": 1 }
`, "cannot assign hash to variable 's' of type Speaker")
	// Hash literal with wrong arity: error.
	expectErrors(t, iface+`
let s: Speaker = { "speak": fn(a: int, b: int): string { return "hi" } }
`, "cannot assign hash to variable 's' of type Speaker")
}

func TestCheckClassHierarchy(t *testing.T) {
	src := `
class Animal { fn move(): string { return "..." } }
class Dog extends Animal { fn bark(): string { return "woof" } }
let a: Animal = new Dog()
`
	expectClean(t, src)
	expectErrors(t, `
class Animal { }
class Dog extends Animal { }
let d: Dog = new Animal()
`, "cannot assign Animal to variable 'd' of type Dog")
}

func TestCheckMethodCalls(t *testing.T) {
	src := `
class Calc {
  fn add(a: int, b: int): int { return a + b }
}
let c = new Calc()
let n: int = c.add(20, 22)
`
	expectClean(t, src)
	expectErrors(t, `
class Calc {
  fn add(a: int, b: int): int { return a + b }
}
let c = new Calc()
print c.add(1)
`, "wrong number of arguments: got 1, want 2..2")
	expectErrors(t, `
class Calc {
  fn add(a: int, b: int): int { return a + b }
}
let c = new Calc()
print c.add(1, "x")
`, "argument 2: got string, want int")
}

func TestCheckForwardReference(t *testing.T) {
	// Types may be referenced before their declaration line.
	expectClean(t, `
let s: Speaker = new Dog()
interface Speaker { speak(): string }
class Dog { fn speak(): string { return "woof" } }
`)
	expectClean(t, `
fn make(): Widget { return new Widget() }
class Widget { }
`)
}

func TestCheckInference(t *testing.T) {
	expectClean(t, `let x: string = "a" + "b"`)
	expectClean(t, `let x: int = 1 + 2`)
	expectClean(t, `let x: bool = 1 < 2`)
	expectClean(t, `let x: string = "hi ${1 + 1}"`)
	expectClean(t, `let x: array = [1, 2, 3]`)
	expectClean(t, `let x: array = 1..3`)
	expectErrors(t, `let x: string = 1 + 2`,
		"cannot assign int to variable 'x' of type string")
	expectErrors(t, `let x: int = "a" + "b"`,
		"cannot assign string to variable 'x' of type int")
}

func TestCheckIfExpressionUnion(t *testing.T) {
	expectClean(t, `let x: int | string = if (true) { 1 } else { "s" }`)
	expectErrors(t, `let x: int = if (true) { 1 } else { "s" }`,
		"cannot assign int | string to variable 'x' of type int")
}

func TestCheckEnum(t *testing.T) {
	expectClean(t, `
enum Color { Red, Green, Blue }
let c: Color = Color.Red
`)
	expectErrors(t, `
enum Color { Red, Green, Blue }
let c: Color = "red"
`, "cannot assign string to variable 'c' of type Color")
}

func TestCheckNoFalsePositivesOnStdlib(t *testing.T) {
	// Common dynamic patterns must not error.
	expectClean(t, `
let m = { "a": 1 }
m["b"] = 2
let xs = [1, 2, 3]
for (x in xs) { print x }
let f = fn(a, b) { return a }
print f(1, 2)
`)
}

func TestCheckSpreadCallSkipped(t *testing.T) {
	// Spread argument counts are unknowable statically: no arity errors.
	expectClean(t, `
fn sum3(x, y, z) { return x + y + z }
let a = [1, 2]
print sum3(...a, 10)
print sum3(...[1, 2, 3])
`)
}

func TestCheckParseError(t *testing.T) {
	msgs := check(t, "fn = =\n")
	if len(msgs) == 0 || !strings.Contains(msgs[0], "parse error") {
		t.Fatalf("want parse error, got %v", msgs)
	}
}

func TestCheckMultipleErrors(t *testing.T) {
	msgs := check(t, "let a: int = \"x\"\nlet b: string = 42\n")
	if len(msgs) != 2 {
		t.Fatalf("want 2 errors, got %v", msgs)
	}
	if !strings.Contains(msgs[0], "t.ns:1:") || !strings.Contains(msgs[1], "t.ns:2:") {
		t.Fatalf("want line numbers, got %v", msgs)
	}
}
