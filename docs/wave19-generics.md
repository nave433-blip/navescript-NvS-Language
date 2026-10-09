# Wave 19 sketch — Generics: class-level + method-level type parameters

Status: **design sketch** (not implemented). Robs Go 1.27's generic methods
(shipped 2026-08-19) and finishes what NvS's generics story started.

## 0. Honest status quo (read this first)

NvS today does **not** have working generics, despite appearances:

- `internal/types/types.go` has a `GenericType` struct, but nothing in the
  codebase ever constructs one. It is a stub.
- The parser registers `<` **only** as the less-than infix operator.
  `class Box<T>` is a parse error today (verified 2026-10-09).
- `parseTypeAnnotation` handles `int`, `int | string`, `T?` — but not
  `Vector<int>`. There is no parameterized-annotation syntax.
- The AST (`ClassStatement`, `ClassMethod`, `TypeAnnotation`) has no
  type-parameter fields.
- `nvs check` explicitly disclaims generics: "What is NOT claimed:
  dataflow/narrowing, **generics**, cross-file analysis, macros."
- `std/collections/vector.ns` and friends use `class Vector<T>` syntax, but
  the main parser rejects those files. They are aspirational — written in a
  dialect the interpreter cannot read yet.

So Wave 19 is not "bolt method type-params onto working class generics."
It is: **design NvS generics end-to-end** — declarations, methods,
checker, runtime — in one coherent wave. The std files already show us the
dialect Evan wants; this wave makes the parser understand it.

## 1. What we're robbing (Go 1.27, Aug 2026)

Go 1.18 gave generic *types/functions*; Go 1.27 (Aug 2026) finally allowed
**generic methods**: a method declares its own type parameters, independent
of the receiver's:

```go
func (s Slice[T]) Map[U any](f func(T) U) Slice[U] { ... }
```

The killer use: `From(xs).Filter(...).Map(...).Collect()` — each step in the
chain introduces a *new* type the receiver never knew about.

Go's **honest boundary** (we copy it, §4 rule 7–8):

- Interface methods **cannot** declare type parameters.
- A generic method **cannot** implement / satisfy an interface method.
- Reason: interfaces dispatch on fixed method sets; a method that is really
  an infinite family of methods (`Map[int]`, `Map[string]`, …) has no single
  entry in a dispatch table. This is a real constraint, not a missing feature.

## 2. Syntax sketch

### 2a. Class-level type parameters (adopt the std dialect for real)

```nvs
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}

let b = new Box<int>(42);   // explicit
let b2 = new Box(42);       // inferred from args (preferred NvS style)
```

### 2b. Method-level type parameters (the actual robbery)

A method may declare its own `<...>` list after its name. Method params
**shadow** class params of the same name (Go rule — shadowing is a checker
warning, not silent):

```nvs
class Vec<T> {
    fn init(items: [T]) { this.items = items; }

    // U is new: the receiver never mentioned it.
    fn map<U>(f: fn(T) -> U) -> Vec<U> {
        let out = [];
        for x in this.items { out = out + [f(x)]; }
        return new Vec(out);
    }

    fn filter(f: fn(T) -> bool) -> Vec<T> {
        let out = [];
        for x in this.items { if f(x) { out = out + [x]; } }
        return out;
    }
}

let names = new Vec([1, 2, 3])
    .map(fn(x: int) -> string { return "n" + str(x); })
    .filter(fn(s: string) -> bool { return len(s) > 0; });
```

### 2c. Top-level generic functions

```nvs
fn identity<T>(x: T) -> T { return x; }
fn first<T>(xs: [T]) -> T? { return len(xs) > 0 ? xs[0] : null; }

let a = identity(42);        // T = int, inferred
let b = identity<string>("hi"); // explicit when inference can't see it
```

### 2d. Constraints (keep it NvS-simple)

Bounds use `:` inside the parameter list, reusing annotation syntax.
Unbounded `T` means `any` — always.

```nvs
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
fn show<T: Printable>(x: T) -> string { return x.to_string(); }
```

Constraint names resolve like annotations: builtin (`Numeric`, `Printable`,
`Comparable`), class/interface names, or unions (`T: int | string`).
An unsatisfiable bound is a **checker error**, never a runtime surprise.

### 2e. Explicit type arguments at call sites

Inference is the default (NvS is dynamic; types are advisory). When the
checker cannot infer — e.g. `fn make<T>() -> T` with no value params —
explicit arguments are required:

```nvs
let e = empty<Vec<int>>();          // top-level fn: name<args>(...)
let m = v.map::<string>(f);         // method: turbofish avoids `<` ambiguity
```

`::<>` (turbofish) for member calls keeps `<` unambiguous with less-than in
expression position. Bare `v.map<string>(f)` is reserved; the parser will
reject it and suggest `::<>`.

## 3. Checker rules (`nvs check` gains generics)

The checker stays advisory (NvS philosophy: unannotated code never errors),
but annotated generic code gets real rules:

1. **Scope.** Class type params are in scope in every method signature and
   body. Method type params are in scope in that method's signature and body
   only. Top-level fn params are scoped to that fn.
2. **Shadowing.** A method type param with the same name as a class type
   param is a **warning** (`W-shadowing`): inside the method, the name means
   the method's param. (Matches Go; silence here causes real bugs.)
3. **No duplicates.** `fn f<T, T>` is an error. `class C<T, T>` is an error.
4. **Bounds checked at declaration.** `T: Nope` (unknown name) is an error,
   reported once, like unknown annotations today.
5. **Bounds checked at use.** `add<int>("a", "b")` with `T: Numeric` is an
   error: `string` does not satisfy `Numeric`. With inference, the inferred
   type must satisfy the bound or it's an error at the call.
6. **Inference.** Type args are inferred from value arguments left to right;
   an uninferrable param (appears only in return position) **must** be given
   explicitly — otherwise error "cannot infer T; provide it explicitly".
7. **Interfaces stay non-generic** (Go's honest boundary, copied).
   `interface I { fn map<U>(f: fn(T)->U); }` is a **parse-time error**:
   "interface methods cannot declare type parameters".
8. **Generic methods don't satisfy interfaces.** A class with only
   `fn map<U>(...)` does not satisfy `interface M { fn map(f); }`. The
   checker reports which method failed and why. (Structural check is
   arity-first today; generic methods are skipped in interface satisfaction
   with an explanatory note.)
9. **`any` escapes everything.** Unannotated params/returns are `any`;
   `any` satisfies every bound and unifies with every param. Existing
   dynamic programs keep checking clean — no new errors on old code.
10. **Variance: invariant.** `Box<int>` is not `Box<Numeric>`, even if `int`
    satisfies `Numeric`. (Simple, predictable, matches Go. Covariance is a
    future wave if ever.)

## 4. Runtime semantics: erasure

NvS is a dynamically-typed tree-walking interpreter. Generics are
**check-time only** — full type erasure at runtime:

- The evaluator never sees type parameters. `new Box<int>(42)` evaluates
  exactly like `new Box(42)`. No monomorphization, no runtime type tables,
  no performance cliff.
- This means: **zero** evaluator changes for the core feature. The work is
  parser + AST + checker. (The honest trade: no runtime specialization, so
  no speed gain — but also no complexity bomb in the tree-walker.)
- `implements()` / `assert_implements()` keep working on erased signatures.

## 5. Parser notes (the `<` problem)

`<` is currently less-than only. Disambiguation points:

- After `class Name`: `class Box<T>` — `<` starts a type-param list.
  (No valid program has `class Box < T >` as comparison, so this is safe.)
- After `fn name` / method name, **before** `(`: `fn map<U>(...)` — `<`
  starts a type-param list. In expression position (`x < y`), `<` stays
  less-than because no `fn`-name precedes it.
- In annotations: `-> Vec<U>`, `let x: Box<int>` — inside annotation
  parsing, `<...>` is a type-argument list. Annotations never contain
  comparisons, so no ambiguity.
- Call sites: `v.map::<string>(f)` — the `::` token sequence is new in
  expression position and unambiguous. Plain `v.map<string>(f)` stays a
  parse error with a helpful message.

Lexer needs no new tokens: `LT`, `GT`, `COLON` (for `::`) all exist.

## 6. Implementation phases

1. **AST**: `TypeParams []*TypeParam` on `ClassStatement`, `ClassMethod`,
   `FunctionLiteral`/`FnStatement`; `TypeArgs` on `TypeAnnotation` and a new
   `ExplicitTypeArgs` on call expressions. `TypeParam{Name, Bound}`.
2. **Parser**: `parseTypeParamList()` after class/fn/method names;
   `parseTypeArgs()` inside annotations; turbofish in member calls.
   Tests: parse-only, no eval.
3. **Checker**: scope handling for type params; rules 1–10; inference at
   call sites. `nvs check` learns generics (removing the old disclaimer).
4. **Docs**: LANGUAGE.md checker table updated; `docs/book` gets a
   generics chapter; std `vector.ns`/`set.ns` become parseable — the first
   real proof the dialect landed.
5. **Negative tests**: each checker rule gets a failing example that must
   produce the right error (never weaken tests to pass).

## 7. Open questions for Evan

1. **Constraint syntax**: `T: Numeric` (annotation-style, proposed above)
   vs `where T: Numeric` (trailing clause, Rust-style). Proposed default:
   `T: Numeric` — one syntax for bounds everywhere.
2. **The interface boundary**: copy Go exactly (no generic interface
   methods, §3 rules 7–8), or allow them with NvS-dynamic dispatch?
   Proposed default: copy Go. The dispatch-table argument is real, and
   "weaker than Go here" is not a robbery worth doing.
3. **Wave scope**: fold class-level generics into this wave (needed anyway —
   method params are useless without declarable class params), or split
   into 19a (classes) / 19b (methods)? Proposed: one wave, phased per §6.

---
*Sketch written 2026-10-09. §7 greenlit by Evan same day — built with the
proposed defaults (`T: Numeric`, Go's interface boundary copied exactly,
single phased wave). Implemented in commit `7ab7e27` (phases 1–4 + tests).
Deviations from the sketch:*
- *`[T]` array sugar also accepted (the sketch used `[T]` in §2b examples
  but only specified `T[]`).*
- *`Numeric`/`Comparable`/`Printable` added as real builtin constraint
  names (`Numeric` → the `number` tower; the other two honestly accept
  every value since NvS `==` never errors and everything stringifies).*
- *`new C(args)` infers class type args from `init` (the sketch left this
  dynamic); uninferred params become `any`, never an error.*
- *`pub` accepted on interface methods' `fn` prefix for consistent errors.*
- *Parser gaps closed beyond generics: `pub`, `module`, `import std.x`,
  `::`, class fields, `++`/`--`, braceless bodies, `->` returns —
  `std/*.ns` files now parse (module loading and `ffi` remain future work;
  see PLACEHOLDERS.md).*
