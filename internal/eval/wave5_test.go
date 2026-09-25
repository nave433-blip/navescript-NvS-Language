package eval

import (
	"testing"
)

// ---- Wave 5: runtime type contracts ----
// Annotations are RUNTIME contracts: no static checker exists. These tests
// pin the call-time / bind-time / return-time enforcement semantics.

// ---- parameter + return annotations ----

func TestWave5ParamAnnotationPositive(t *testing.T) {
	expectInspect(t, `fn add(a: int, b: int) { return a + b }; add(2, 3)`, "5")
	expectInspect(t, `fn id(x: string) { return x }; id("hi")`, "hi")
	expectInspect(t, `let f = fn(xs: array): int { return len(xs) }; f([1,2])`, "2")
}

func TestWave5ParamAnnotationViolation(t *testing.T) {
	expectErrorContains(t, `fn add(a: int, b: int) { return a + b }; add("x", 2)`,
		"type error: parameter 'a' of function 'add' expects int, got string")
	expectErrorContains(t, `fn add(a: int, b: int) { return a + b }; add(1, 2.5)`,
		"type error: parameter 'b' of function 'add' expects int, got float")
	// let-bound literals pick up their binding name for error messages.
	expectErrorContains(t, `let f = fn(x: bool) { return x }; f(1)`,
		"type error: parameter 'x' of function 'f' expects bool, got int")
	// Truly anonymous functions (called inline) report no name.
	expectErrorContains(t, `(fn(x: bool) { return x })(1)`,
		"type error: parameter 'x' expects bool, got int")
}

func TestWave5ReturnAnnotation(t *testing.T) {
	expectInspect(t, `fn f(): int { return 1 }; f()`, "1")
	expectErrorContains(t, `fn f(): int { return "nope" }; f()`,
		"type error: return value of function 'f' expects int, got string")
	// A thrown error is not a return value: it propagates untouched.
	expectInspect(t, `fn f(): int { throw "boom" }; try { f() } catch (e) { e }`, "boom")
}

func TestWave5DefaultsChecked(t *testing.T) {
	// Defaults are checked when they are USED...
	expectErrorContains(t, `fn d(x: int = "bad") { return x }; d()`,
		"type error: parameter 'x' of function 'd' expects int, got string")
	// ...but not when the caller supplies the argument.
	expectInspect(t, `fn d(x: int = "bad") { return x }; d(5)`, "5")
	expectInspect(t, `fn d(x: int = 3) { return x }; d()`, "3")
}

func TestWave5NamedArgsPathChecked(t *testing.T) {
	expectInspect(t, `fn f(a: int, b: string) { return b }; f(b: "hi", a: 1)`, "hi")
	expectErrorContains(t, `fn f(a: int, b: string) { return b }; f(a: "x", b: "hi")`,
		"type error: parameter 'a' of function 'f' expects int, got string")
}

func TestWave5MissingArgKeepsOldBehavior(t *testing.T) {
	// Backward compat: a missing argument still binds NULL (no new arity
	// error); annotations only check values that are actually bound.
	expectInspect(t, `fn f(x) { return x }; f()`, "null")
	expectInspect(t, `fn f(x: int) { return x }; f()`, "null")
}

func TestWave5MethodAnnotations(t *testing.T) {
	expectInspect(t, `
class Calc {
  fn add(a: int, b: int): int { return a + b }
}
new Calc().add(3, 4)`, "7")
	expectErrorContains(t, `
class Calc {
  fn add(a: int, b: int): int { return a + b }
}
new Calc().add(3, "4")`,
		"type error: parameter 'b' of function 'add' expects int, got string")
	expectErrorContains(t, `
class Calc {
  fn bad(): int { return "x" }
}
new Calc().bad()`,
		"type error: return value of function 'bad' expects int, got string")
}

func TestWave5PipeStillBitwiseOr(t *testing.T) {
	// `|` outside annotation position keeps its old meaning.
	expectInspect(t, `5 | 3`, "7")
	expectInspect(t, `fn f(x: int) { return x | 1 }; f(4)`, "5")
}

func TestWave5UnknownTypeName(t *testing.T) {
	expectErrorContains(t, `fn f(x: Nope) { return x }; f(1)`,
		"unknown type 'Nope' in annotation for parameter 'x' of function 'f'")
}

// ---- unions ----

func TestWave5UnionAnnotation(t *testing.T) {
	expectInspect(t, `fn f(x: int | string) { return x }; f(1)`, "1")
	expectInspect(t, `fn f(x: int | string) { return x }; f("s")`, "s")
	expectErrorContains(t, `fn f(x: int | string) { return x }; f(1.5)`,
		"type error: parameter 'x' of function 'f' expects int | string, got float")
}

func TestWave5NullableSugar(t *testing.T) {
	expectInspect(t, `fn f(x: int?) { return x }; f(1)`, "1")
	expectInspect(t, `fn f(x: int?) { return x }; f(null)`, "null")
	expectErrorContains(t, `fn f(x: int?) { return x }; f("s")`,
		"type error: parameter 'x' of function 'f' expects int | null, got string")
}

func TestWave5NumberAnnotation(t *testing.T) {
	expectInspect(t, `fn f(x: number) { return x }; f(1)`, "1")
	expectInspect(t, `fn f(x: number) { return x }; f(1.5)`, "1.5")
	expectErrorContains(t, `fn f(x: number) { return x }; f("s")`,
		"type error: parameter 'x' of function 'f' expects number, got string")
}

// ---- let x: type (initial binding + reassignment) ----

func TestWave5LetAnnotation(t *testing.T) {
	expectInspect(t, `let x: int = 5; x`, "5")
	expectErrorContains(t, `let x: int = "s"`,
		"type error: variable 'x' expects int, got string")
	expectInspect(t, `let x: int | string = "s"; x = 42; x`, "42")
	expectErrorContains(t, `let x: int = 5; x = "s"`,
		"type error: cannot assign string to variable 'x' declared as int")
	// Unrelated variables are unaffected.
	expectInspect(t, `let x: int = 5; let y = "s"; y = 1; y`, "1")
	// Declarations are scoped: reassignment inside a closure is checked too.
	expectErrorContains(t, "let x: int = 5\nfn f() { x = \"s\" }\nf()",
		"type error: cannot assign string to variable 'x' declared as int")
}

// ---- interfaces ----

func TestWave5InterfaceImplements(t *testing.T) {
	expectInspect(t, `
interface Shape { area(): float }
class Circle {
  fn init(r) { this.r = r }
  fn area(): float { return 3.14 * this.r * this.r }
}
implements(new Circle(2), Shape)`, "true")
	// Maps of functions count (structural, not nominal).
	expectInspect(t, `
interface Shape { area(): float }
implements({"area": fn() { return 1.0 }}, Shape)`, "true")
	// Negative: missing method.
	expectInspect(t, `
interface Shape { area(): float }
implements({"nope": 1}, Shape)`, "false")
	// Negative: member present but not callable.
	expectInspect(t, `
interface Shape { area(): float }
implements({"area": 42}, Shape)`, "false")
	// Negative: arity incompatible with the declared signature.
	expectInspect(t, `
interface Shape { area(): float }
implements({"area": fn(a, b) { return 1.0 }}, Shape)`, "false")
	// assert_implements returns null on success...
	expectInspect(t, `
interface Shape { area(): float }
assert_implements({"area": fn() { return 1.0 }}, Shape)`, "null")
	// ...and a descriptive error on failure.
	expectErrorContains(t, `
interface Shape { area(): float }
assert_implements({"nope": 1}, Shape)`,
		"assert_implements: hash does not implement Shape: missing method 'area'")
	expectErrorContains(t, `
interface Shape { area(): float }
assert_implements({"area": fn(a, b) { return 1.0 }}, Shape)`,
		"method 'area' cannot be called with 0 argument(s)")
	// Second argument must be an interface.
	expectErrorContains(t, `implements(1, 2)`,
		"implements: second argument must be an interface, got int")
}

func TestWave5InterfaceInheritance(t *testing.T) {
	// Inherited methods satisfy the interface (structural via GetMethod).
	expectInspect(t, `
interface Speaker { speak(): string }
class Animal { fn speak() { return "..." } }
class Dog extends Animal { }
implements(new Dog(), Speaker)`, "true")
}

func TestWave5InterfaceAsAnnotation(t *testing.T) {
	expectInspect(t, `
interface Shape { area(): float }
fn draw(s: Shape) { return s.area() }
class Circle {
  fn init(r) { this.r = r }
  fn area(): float { return 2.0 * this.r }
}
draw(new Circle(3))`, "6")
	expectErrorContains(t, `
interface Shape { area(): float }
fn draw(s: Shape) { return s.area() }
draw(42)`,
		"type error: parameter 's' of function 'draw' expects Shape, got int")
}

// ---- class / record names as annotations ----

func TestWave5ClassNameAnnotation(t *testing.T) {
	expectInspect(t, `
class Animal { fn speak() { return "..." } }
class Dog extends Animal { fn speak() { return "woof" } }
fn talk(a: Animal) { return a.speak() }
talk(new Dog())`, "woof")
	expectErrorContains(t, `
class Animal { }
fn talk(a: Animal) { return 1 }
talk("cat")`,
		"type error: parameter 'a' of function 'talk' expects Animal, got string")
}

func TestWave5RecordNameAnnotation(t *testing.T) {
	expectInspect(t, `
record Point(x, y)
fn getx(p: Point) { return p.x }
getx(Point(1, 2))`, "1")
	expectErrorContains(t, `
record Point(x, y)
fn getx(p: Point) { return p.x }
getx([1, 2])`,
		"type error: parameter 'p' of function 'getx' expects Point, got array")
}

// ---- type guards ----

func TestWave5TypeGuards(t *testing.T) {
	expectInspect(t, `is_int(1)`, "true")
	expectInspect(t, `is_int(1.5)`, "false")
	expectInspect(t, `is_float(1.5)`, "true")
	expectInspect(t, `is_float(1)`, "false")
	expectInspect(t, `is_number(1)`, "true")
	expectInspect(t, `is_number(1.5)`, "true")
	expectInspect(t, `is_number("s")`, "false")
	expectInspect(t, `is_string("s")`, "true")
	expectInspect(t, `is_bool(true)`, "true")
	expectInspect(t, `is_null(null)`, "true")
	expectInspect(t, `is_null(0)`, "false")
	expectInspect(t, `is_array([1])`, "true")
	expectInspect(t, `is_hash({"a": 1})`, "true")
	expectInspect(t, `is_tuple((1, 2))`, "true")
	expectInspect(t, `is_function(fn(x) { return x })`, "true")
	expectInspect(t, `is_function(len)`, "true")
	expectInspect(t, `is_function(42)`, "false")
}

func TestWave5TypeOf(t *testing.T) {
	expectInspect(t, `type_of(1)`, "int")
	expectInspect(t, `type_of(1.5)`, "float")
	expectInspect(t, `type_of("s")`, "string")
	expectInspect(t, `type_of(true)`, "bool")
	expectInspect(t, `type_of(null)`, "null")
	expectInspect(t, `type_of([1])`, "array")
	expectInspect(t, `type_of({"a": 1})`, "hash")
	expectInspect(t, `type_of((1, 2))`, "tuple")
	expectInspect(t, `type_of(fn(x) { return x })`, "function")
	expectInspect(t, `type_of(len)`, "builtin")
	// type()/typeof() keep returning the uppercase internal names.
	expectInspect(t, `type(1)`, "INTEGER")
	expectInspect(t, `typeof(1)`, "INTEGER")
}

// ---- partial/curry still enforce through the wrapper ----

func TestWave5PartialEnforcesAnnotations(t *testing.T) {
	expectInspect(t, `fn add(a: int, b: int) { return a + b }; partial(add, 1)(2)`, "3")
	expectErrorContains(t, `fn add(a: int, b: int) { return a + b }; partial(add, 1)("x")`,
		"type error: parameter 'b' of function 'add' expects int, got string")
}
