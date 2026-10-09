package eval

import (
	"testing"
)

// ---- Wave 19: generics erasure + std-dialect runtime features ----

func TestWave19GenericClassRuns(t *testing.T) {
	// Erasure: `new Box<int>(42)` runs exactly like `new Box(42)`.
	expectInspect(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}
let b = new Box<int>(42);
b.get();
`, "42")
	expectInspect(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}
let b = new Box(42);
b.get();
`, "42")
}

func TestWave19GenericMethodRuns(t *testing.T) {
	expectInspect(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn map<U>(f) { return new Box(f(this.v)); }
}
let b = new Box(21);
b.map(fn(x) { return x * 2; }).v;
`, "42")
}

func TestWave19GenericFnRuns(t *testing.T) {
	expectInspect(t, `fn identity<T>(x: T) -> T { return x; }; identity(99);`, "99")
	expectInspect(t, `fn identity<T>(x: T) -> T { return x; }; identity("hi");`, "hi")
	// Explicit type args are erased at runtime.
	expectInspect(t, `fn identity<T>(x: T) -> T { return x; }; identity::<string>("hi");`, "hi")
	expectInspect(t, `fn make<T>() -> T { return 7; }; make::<int>();`, "7")
}

func TestWave19TypeParamsAreAnyAtRuntime(t *testing.T) {
	// A type parameter never triggers "unknown type" at runtime.
	expectInspect(t, `
fn f<T>(x: T) -> T { return x; }
f([1, 2]);
`, "[1, 2]")
	// Concrete annotations still enforced alongside generics.
	expectErrorContains(t, `
fn f<T>(x: T, n: int) -> T { return x; }
f("hi", "notanint");
`, "type error: parameter 'n' of function 'f' expects int, got string")
}

func TestWave19VecChainRuns(t *testing.T) {
	// The Go-1.27-style chain, interpreted with full erasure.
	expectInspect(t, `
class Vec<T> {
    fn init(items) { this.items = items; }
    fn map<U>(f) {
        let out = [];
        for (x in this.items) { out = push(out, f(x)); }
        return new Vec(out);
    }
    fn filter(f) {
        let out = [];
        for (x in this.items) { if (f(x)) { out = push(out, x); } }
        return new Vec(out);
    }
}
let v = new Vec([1, 2, 3, 4]);
let r = v.map(fn(x) { return x * 10; }).filter(fn(x) { return x > 20; });
r.items;
`, "[30, 40]")
}

func TestWave19ClassFields(t *testing.T) {
	// Field defaults initialize before init runs.
	expectInspect(t, `
class C {
    let a: int;
    let b: int = 10;
    fn init() { this.a = this.b + 1; }
}
let c = new C();
[c.a, c.b];
`, "[11, 10]")
	// Defaults evaluate per-instance.
	expectInspect(t, `
class C {
    let items = [];
}
let a = new C();
let b = new C();
a.items = push(a.items, 1);
[a.items, b.items];
`, "[[1], []]")
	// Fields without defaults are just declared.
	expectInspect(t, `
class C {
    let x: int;
    fn init(v) { this.x = v; }
}
new C(5).x;
`, "5")
}

func TestWave19Postfix(t *testing.T) {
	// Postfix returns the OLD value.
	expectInspect(t, `let x = 1; let y = x++; [y, x];`, "[1, 2]")
	expectInspect(t, `let x = 5; let y = x--; [y, x];`, "[5, 4]")
	expectInspect(t, `let x = 1.5; x++; x;`, "2.5")
	// Members and indexes.
	expectInspect(t, `
class C { fn init() { this.n = 0; } }
let c = new C();
c.n++;
c.n;
`, "1")
	expectInspect(t, `let a = [10, 20]; a[0]++; [a[0], a[1]];`, "[11, 20]")
	// Errors.
	expectErrorContains(t, `let x = "s"; x++;`, "++ requires a number, got string")
	expectErrorContains(t, `const x = 1; x++;`, "cannot assign to constant: x")
	expectErrorContains(t, `(1)++;`, "requires an assignable target")
}

func TestWave19BracelessBodiesRun(t *testing.T) {
	expectInspect(t, `let x = 0; if (true) x = 42; x;`, "42")
	expectInspect(t, `let x = 0; if (false) x = 1; else x = 2; x;`, "2")
	expectInspect(t, `let i = 0; while (i < 3) i++; i;`, "3")
}

func TestWave19ModuleNoop(t *testing.T) {
	expectInspect(t, `module std.example; 42;`, "42")
}

func TestWave19ImportModulePathError(t *testing.T) {
	expectErrorContains(t, `import std.core;`, "module path 'std.core' is not yet supported")
	// Quoted imports are unaffected (they fail on missing files, not on syntax).
	expectErrorContains(t, `import "nonexistent_xyz.ns";`, "import failed")
}

func TestWave19PubNoop(t *testing.T) {
	expectInspect(t, `pub let x = 1; pub fn f() { return 2; } f() + x;`, "3")
	expectInspect(t, `pub class C { pub fn m() { return 7; } } new C().m();`, "7")
}

func TestWave19DColonParsesLikeMember(t *testing.T) {
	// `ffi` isn't defined: honest runtime error, but it parses and
	// dispatches like a member call.
	expectErrorContains(t, `ffi::call("x", []);`, "identifier not found: ffi")
}

func TestWave19ArrayAnnotationRuntime(t *testing.T) {
	// `T[]` checks the shape (array); element types are erased.
	expectInspect(t, `let x: string[] = ["a"]; x;`, `[a]`)
	expectErrorContains(t, `let x: string[] = 5;`, "type error: variable 'x' expects array, got int")
	// Parameterized types check the base class; args erased.
	expectInspect(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let x: Box<int> = new Box(1);
x.v;
`, "1")
	expectErrorContains(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let x: Box<int> = 5;
`, "type error: variable 'x' expects Box, got int")
}

func TestWave19ArrowReturnRuntime(t *testing.T) {
	expectInspect(t, `fn f() -> int { return 1; } f();`, "1")
	expectErrorContains(t, `fn f() -> int { return "x"; } f();`,
		"type error: return value of function 'f' expects int, got string")
}
