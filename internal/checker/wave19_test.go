package checker

import (
	"strings"
	"testing"
)

// ---- Wave 19: generics — the 10 checker rules ----

// (rules 1-10 tests follow)

// ---- Wave 19b: Go 1.27 companion steals ----

// Array element types are preserved for inference (erased at runtime,
// shape-based in assignability).
func TestWave19bArrayElementInference(t *testing.T) {
	// `new Vector([1,2,3])` infers T = int from the literal's elements.
	expectCleanW(t, `
class Vector<T> {
    fn init(items: [T] = []) { this.items = items; }
    fn get(i: int) -> T { return this.items[i]; }
    fn push(x: T) { this.items = push(this.items, x); }
}
let v = new Vector([1, 2, 3]);
let n: int = v.get(0);
`)
	expectErrors(t, `
class Vector<T> {
    fn init(items: [T] = []) { this.items = items; }
    fn get(i: int) -> T { return this.items[i]; }
    fn push(x: T) { this.items = push(this.items, x); }
}
let v = new Vector([1, 2, 3]);
let s: string = v.get(0);
`, "cannot assign int to variable 's' of type string")
	expectErrors(t, `
class Vector<T> {
    fn init(items: [T] = []) { this.items = items; }
    fn push(x: T) { this.items = push(this.items, x); }
}
let v = new Vector([1, 2, 3]);
v.push("nope");
`, "argument 1: got string, want int")
	// Mixed/empty literals stay erased (gradual).
	expectCleanW(t, `
class Vector<T> {
    fn init(items: [T] = []) { this.items = items; }
    fn push(x: T) { this.items = push(this.items, x); }
}
let v = new Vector([]);
v.push("anything");
let w = new Vector([1, "mixed"]);
w.push(3.5);
`)
	// Invariance still holds through inference.
	expectErrors(t, `
class Vector<T> {
    fn init(items: [T] = []) { this.items = items; }
}
let v: Vector<string> = new Vector([1, 2, 3]);
`, "cannot assign Vector<int> to variable 'v' of type Vector<string>")
}

// Generalized function type inference: a generic function assigned to a
// declared fn type gets its type arguments from the target.
func TestWave19bGenericFnTargetInference(t *testing.T) {
	// Clean: T = int inferred from the target.
	expectCleanW(t, `
fn id<T>(x: T) -> T { return x; }
let g: fn(int) -> int = id;
`)
	// Error: T = int, but the target wants string back.
	expectErrors(t, `
fn id<T>(x: T) -> T { return x; }
let f: fn(int) -> string = id;
`, "inferred T = int, but return: got int, want string")
	// Arity mismatch still errors.
	expectErrors(t, `
fn id<T>(x: T) -> T { return x; }
let h: fn(int, int) -> int = id;
`, "arity mismatch")
	// Bounds are enforced on target-inferred arguments.
	expectErrors(t, `
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
let f: fn(string, string) -> string = add;
`, "does not satisfy bound")
	// Uninferrable params become any (gradual).
	expectCleanW(t, `
fn make<T>() -> T { return null; }
let f: fn() -> int = make;
`)
}

// Generic functions as call arguments: inference from the parameter type.
func TestWave19bGenericFnAsArgument(t *testing.T) {
	expectCleanW(t, `
fn id<T>(x: T) -> T { return x; }
fn apply(cb: fn(int) -> int) -> int { return cb(1); }
let n: int = apply(id);
`)
	expectErrors(t, `
fn id<T>(x: T) -> T { return x; }
fn apply(cb: fn(int) -> string) -> string { return cb(1); }
apply(id);
`, "return: got int, want string")
}

// Contextual typing: fn literals are body-checked against the target type.
func TestWave19bFnLiteralContextual(t *testing.T) {
	// Return mismatch caught.
	expectErrors(t, `
let f: fn(int) -> string = fn(x) { return 42; };
`, "return type mismatch: got int, want string")
	// Clean when the body agrees; the unannotated param takes the target type.
	expectCleanW(t, `
let f: fn(int) -> string = fn(x) { return str(x); };
`)
	// Argument position.
	expectErrors(t, `
fn apply(cb: fn(int) -> string) -> string { return cb(1); }
apply(fn(x) { return 42; });
`, "return type mismatch: got int, want string")
	// Explicit annotations on the literal win over the target.
	expectErrors(t, `
let f: fn(int) -> string = fn(x: string) -> string { return x; };
`, "parameter 1: got string, want int")
	// Non-fn targets are unaffected.
	expectCleanW(t, `
let g = fn(x) { return x; };
`)
}

// checkW is like check but also returns warnings.
func checkW(t *testing.T, src string) (errs []string, warns []string) {
	t.Helper()
	e, w := CheckWithWarnings("t.ns", src)
	for _, x := range e {
		errs = append(errs, x.Error())
	}
	for _, x := range w {
		warns = append(warns, x.Error())
	}
	return errs, warns
}

func expectCleanW(t *testing.T, src string) {
	t.Helper()
	if errs, warns := checkW(t, src); len(errs) > 0 {
		t.Fatalf("expected no errors, got:\n%s", strings.Join(errs, "\n"))
	} else if len(warns) > 0 {
		t.Fatalf("expected no warnings, got:\n%s", strings.Join(warns, "\n"))
	}
}

// Rule 1: type parameters are in scope in signatures and bodies.
func TestWave19Rule1Scope(t *testing.T) {
	expectCleanW(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
    fn set(v: T) { this.v = v; }
}
fn identity<T>(x: T) -> T { return x; }
let b = new Box<int>(42);
let n: int = b.get();
let m: int = identity(7);
`)
	// Method type params are scoped to the method.
	expectCleanW(t, `
class Vec<T> {
    fn map<U>(f: fn(T) -> U) -> Vec<U> { return new Vec([]); }
}
`)
}

// Rule 2: shadowing a class type param with a method type param warns.
func TestWave19Rule2ShadowingWarns(t *testing.T) {
	errs, warns := checkW(t, `
class Box<T> {
    fn map<T>(f) -> T { return f(this.v); }
}
`)
	if len(errs) > 0 {
		t.Fatalf("shadowing should warn, not error: %v", errs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "shadows an outer type parameter") {
		t.Fatalf("expected shadowing warning, got %v", warns)
	}
}

// Rule 3: duplicate type parameters are an error.
func TestWave19Rule3Duplicates(t *testing.T) {
	expectErrors(t, `fn f<T, T>(x: T) -> T { return x; }`, "duplicate type parameter 'T'")
	expectErrors(t, `class C<T, T> {}`, "duplicate type parameter 'T'")
}

// Rule 4: bounds must name known types.
func TestWave19Rule4BoundNames(t *testing.T) {
	expectErrors(t, `fn f<T: Nope>(x: T) -> T { return x; }`,
		"unknown type 'Nope' in bound of type parameter 'T'")
	expectCleanW(t, `fn f<T: Numeric>(x: T) -> T { return x; }`)
	expectCleanW(t, `fn f<T: int | string>(x: T) -> T { return x; }`)
}

// Rule 5: bounds are checked at use — explicit and inferred.
func TestWave19Rule5Bounds(t *testing.T) {
	// Explicit type argument violating the bound.
	expectErrors(t, `
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
add<string>("a", "b");
`, "type string does not satisfy bound number of type parameter 'T'")
	// Inferred type violating the bound.
	expectErrors(t, `
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
add("a", "b");
`, "type string does not satisfy bound number of type parameter 'T'")
	// Satisfying uses are clean.
	expectCleanW(t, `
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
add(1, 2);
add<int>(1, 2);
add(1.5, 2.5);
`)
}

// Rule 6: inference from value arguments; uninferrable is an error.
func TestWave19Rule6Inference(t *testing.T) {
	// Inferred return type flows through.
	expectCleanW(t, `
fn identity<T>(x: T) -> T { return x; }
let s: string = identity("hi");
let n: int = identity(42);
`)
	expectErrors(t, `
fn identity<T>(x: T) -> T { return x; }
let s: string = identity(42);
`, "cannot assign int to variable 's' of type string")
	// Uninferrable: T appears only in the return.
	expectErrors(t, `
fn make<T>() -> T { return null; }
let x = make();
`, "cannot infer type parameter 'T'; provide it explicitly")
	// Explicit saves it.
	expectCleanW(t, `
fn make<T>() -> T { return null; }
let x = make::<int>();
let y = make<string>();
`)
	// Inference through fn-type parameters: U from the callback's return.
	expectCleanW(t, `
class Vec<T> {
    fn map<U>(f: fn(T) -> U) -> Vec<U> { return new Vec([]); }
}
let v = new Vec([1, 2, 3]);
let strs: Vec<string> = v.map(fn(x: int) -> string { return str(x); });
`)
	expectErrors(t, `
class Vec<T> {
    fn map<U>(f: fn(T) -> U) -> Vec<U> { return new Vec([]); }
}
let v = new Vec([1, 2, 3]);
let nums: Vec<int> = v.map(fn(x: int) -> string { return str(x); });
`, "cannot assign Vec<string> to variable 'nums' of type Vec<int>")
}

// Rule 7: interface methods cannot declare type parameters (parse error).
func TestWave19Rule7InterfaceBoundary(t *testing.T) {
	expectErrors(t, `interface I { map<U>(f); }`,
		"parse error: interface methods cannot declare type parameters")
}

// Rule 8: generic methods don't satisfy interfaces (with reason).
func TestWave19Rule8GenericMethodNoSatisfy(t *testing.T) {
	expectErrors(t, `
class D<T> {
    fn transform<U>(x: U) -> U { return x; }
}
interface Tr { transform(x); }
let d: Tr = new D(1);
`, "method 'transform' is generic and cannot satisfy interface method")
	// A non-generic method still satisfies.
	expectCleanW(t, `
class D<T> {
    fn transform(x) { return x; }
}
interface Tr { transform(x); }
let d: Tr = new D(1);
`)
}

// Rule 9: `any` escapes everything; old dynamic code stays clean.
func TestWave19Rule9AnyEscapes(t *testing.T) {
	expectCleanW(t, `
class Box<T> {
    fn get() -> T { return this.v; }
}
let b = new Box(42);
let x = b.get();
b.foo(1, 2, 3);
`)
}

// Rule 10: invariance — Box<int> is not Box<string>.
func TestWave19Rule10Invariance(t *testing.T) {
	expectErrors(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let b: Box<string> = new Box<int>(1);
`, "cannot assign Box<int> to variable 'b' of type Box<string>")
	// Same args: fine.
	expectCleanW(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let b: Box<int> = new Box<int>(1);
let c: Box<int> = new Box(2);
`)
	// Unparameterized Box accepts any Box<...>.
	expectCleanW(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let b: Box = new Box<int>(1);
`)
}

// Constructor inference: `new Box(5)` infers T=int from init.
func TestWave19ConstructorInference(t *testing.T) {
	expectCleanW(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}
let b = new Box(42);
let n: int = b.get();
`)
	expectErrors(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}
let b = new Box(42);
let s: string = b.get();
`, "cannot assign int to variable 's' of type string")
	// Bound violations at construction.
	expectErrors(t, `
class Box<T: Numeric> {
    fn init(v: T) { this.v = v; }
}
let b = new Box("hi");
`, "type string does not satisfy bound number of type parameter 'T'")
}

// new with wrong number of type arguments.
func TestWave19NewArity(t *testing.T) {
	expectErrors(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let b = new Box<int, string>(1);
`, "wrong number of type arguments for class 'Box': got 2, want 1")
}

// Explicit type args on a non-generic function is an error.
func TestWave19ExplicitOnNonGeneric(t *testing.T) {
	expectErrors(t, `
fn f(x) { return x; }
f<int>(1);
`, "not a generic function")
}

// Chained generic methods (the Go 1.27 killer use).
func TestWave19Chain(t *testing.T) {
	expectCleanW(t, `
class Vec<T> {
    fn init(items: [T]) { this.items = items; }
    fn map<U>(f: fn(T) -> U) -> Vec<U> { return new Vec([]); }
    fn filter(f: fn(T) -> bool) -> Vec<T> { return new Vec([]); }
}
let out: Vec<string> = new Vec([1, 2, 3])
    .map(fn(x: int) -> string { return str(x); })
    .filter(fn(s: string) -> bool { return true; });
`)
}

// Turbofish explicit type arguments.
func TestWave19TurbofishCheck(t *testing.T) {
	expectCleanW(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}
fn make<T>() -> T { return null; }
let e = make::<string>();
let s: string = e;
`)
	expectErrors(t, `
fn make<T>() -> T { return null; }
let s: int = make::<string>();
`, "cannot assign string to variable 's' of type int")
}

// Parameterized annotations in lets and returns.
func TestWave19ParameterizedAnnotations(t *testing.T) {
	expectCleanW(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
class Pair<K, V> {}
fn wrap(x: int) -> Box<int> { return new Box(x); }
let b: Box<int> = wrap(1);
let d: Pair<string, int> = new Pair();
`)
	expectErrors(t, `
class Box<T> { fn init(v: T) { this.v = v; } }
let b: Box<Nope> = new Box(1);
`, "unknown type 'Nope'")
}

// Type args on non-class types are rejected.
func TestWave19TypeArgsOnlyOnClasses(t *testing.T) {
	expectErrors(t, `let x: int<string> = 1;`,
		"only classes can take type arguments, not 'int'")
}

// Array sugar annotations.
func TestWave19ArrayAnnotations(t *testing.T) {
	expectCleanW(t, `
let a: string[] = ["x"];
let b: [int] = [1];
fn f(xs: [string]) -> int { return 1; }
`)
}

// Shadowing: the method's param wins; like Go, an uninferrable shadowed
// param must be given explicitly.
func TestWave19ShadowResolution(t *testing.T) {
	// Bare call: the method's T is fresh and uninferrable → error (rule 6),
	// plus the shadowing warning (rule 2).
	errs, warns := checkW(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get<T>() -> T { return this.v; }
}
let b = new Box<int>(1);
let s: string = b.get();
`)
	if len(errs) != 1 || !strings.Contains(errs[0], "cannot infer type parameter 'T'") {
		t.Fatalf("expected cannot-infer error, got %v", errs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "shadows an outer type parameter") {
		t.Fatalf("expected shadowing warning, got %v", warns)
	}
	// Explicit type argument: clean (the shadowing warning still fires).
	errs2, warns2 := checkW(t, `
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get<T>() -> T { return this.v; }
}
let b = new Box<int>(1);
let s: string = b.get::<string>();
`)
	if len(errs2) > 0 {
		t.Fatalf("expected no errors, got %v", errs2)
	}
	if len(warns2) != 1 || !strings.Contains(warns2[0], "shadows an outer type parameter") {
		t.Fatalf("expected shadowing warning, got %v", warns2)
	}
}
