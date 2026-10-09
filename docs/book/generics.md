# Generics (Wave 19)

NvS has generic classes, methods, and functions — robbed from Go 1.27's
generic methods and rebuilt for a dynamically-typed, tree-walking
interpreter. The headline rule: **generics are check-time only**. The
interpreter erases every type parameter (`new Box<int>(42)` runs exactly
like `new Box(42)`); `nvs check` is where generics live.

## Declaring type parameters

```ns
class Box<T> {
    fn init(v: T) { this.v = v; }
    fn get() -> T { return this.v; }
}

fn identity<T>(x: T) -> T { return x; }
```

A method can declare its **own** type parameters, independent of the class:

```ns
class Vec<T> {
    fn init(items: [T]) { this.items = items; }

    // U is new — the receiver never mentioned it.
    fn map<U>(f: fn(T) -> U) -> Vec<U> {
        let out = [];
        for (x in this.items) { out = push(out, f(x)); }
        return new Vec(out);
    }

    fn filter(f: fn(T) -> bool) -> Vec<T> {
        let out = [];
        for (x in this.items) { if (f(x)) { out = push(out, x); } }
        return new Vec(out);
    }
}

let names = new Vec([1, 2, 3])
    .map(fn(x: int) -> string { return "n" + str(x); })
    .filter(fn(s: string) -> bool { return len(s) > 0; });
```

## Bounds

`T: Bound` constrains what a type parameter accepts:

```ns
fn add<T: Numeric>(a: T, b: T) -> T { return a + b; }
```

Constraint names resolve like annotations: builtins (`Numeric`,
`Comparable`, `Printable`), class/interface names, or unions
(`T: int | string`). Unbounded `T` means `any`. `nvs check` verifies bounds
at every call — explicit or inferred:

```ns
add<int>("a", "b");  // error: type string does not satisfy bound number
```

## Calling generic code

Type arguments are **inferred** from value arguments whenever possible:

```ns
let n: int = identity(42);      // T = int
let b = new Box(42);            // T = int, from init(v: T)
let v = new Vector([1, 2, 3]);  // T = int, from the literal's elements
```

Array literals carry their element type for inference (`[1, 2]` is
`array<int>` to the checker); mixed or empty literals stay erased. At
runtime every array is just an array — the element type never affects
execution.

When a parameter can't be inferred (it appears only in the return type),
say it explicitly — turbofish for methods, bare for functions:

```ns
fn make<T>() -> T { return null; }
let e = make::<int>();          // turbofish, any callable
let f = make<string>();         // bare <>, plain functions only
class Box<T> { fn get<U>() -> U { return this.v; } }
let g = new Box(1).get::<string>();
```

Bare `v.map<string>(f)` on a method call is a parse error suggesting
turbofish — it keeps `<` unambiguous with less-than.

## Function types flow both ways (Go 1.27)

A **generic function** assigned to a declared function type gets its type
arguments inferred from the target — in `let` bindings, arguments, and
returns:

```ns
fn id<T>(x: T) -> T { return x; }
let g: fn(int) -> int = id;      // T = int, checked
fn apply(cb: fn(int) -> int) -> int { return cb(1); }
apply(id);                        // also fine
let bad: fn(int) -> string = id;  // error: inferred T = int,
                                  // but return: got int, want string
```

A **function literal** is body-checked against the target type —
unannotated parameters take the target's types, and the return is verified:

```ns
let f: fn(int) -> string = fn(x) { return 42; };
// error: return type mismatch: got int, want string
```

(Go 1.27's third change — struct-literal keys as field selectors — has no
NvS analogue: NvS has no keyed struct literals and no embedding. Records
construct with named arguments, hashes take arbitrary key expressions.)

## The interface boundary

Copied honestly from Go, because the reason is real: a generic method is
an infinite family of methods, not one entry in a dispatch table.

- Interface methods **cannot** declare type parameters
  (`interface I { map<U>(f); }` is a parse error).
- A generic method **cannot** satisfy an interface method — `nvs check`
  tells you which method and why.

## Rules `nvs check` enforces

1. Class type params are in scope in every method; method type params only
   in that method; function type params only in that function.
2. Shadowing (a method's `T` over the class's `T`) is a **warning**.
3. `fn f<T, T>` / `class C<T, T>` — duplicate parameters are an error.
4. Bounds must name known types.
5. Inferred and explicit types must satisfy bounds.
6. Uninferrable parameters must be explicit.
7. Interface methods cannot declare type parameters.
8. A generic method cannot satisfy an interface method.
9. `any` (unannotated code) is compatible with everything — old programs
   keep checking clean.
10. Invariance: `Box<int>` is not `Box<string>`.

## Annotations that come with generics

| Syntax | Meaning |
|--------|---------|
| `Vec<int>` | parameterized type (nested `Map<string, Vec<int>>` works; `>>` needs no space) |
| `string[]`, `[string]` | array sugar (element types are erased) |
| `fn(int) -> string` | function type (parameters bivariant, return covariant) |
| `-> T` | return annotation arrow (the std-dialect spelling; `: T` still works) |

## What erasure means for you

- **No runtime cost, no runtime specialization.** Generic code runs at
  exactly the speed of the dynamic code it erases to.
- **No runtime bound checks.** If you need a value checked, annotate the
  parameter with a concrete type — bounds are a `check`-time contract.
- **Reflection sees the erasure.** `type_of` on a `Box<int>` reports the
  instance's class, not its arguments.

## The std dialect (same wave)

Wave 19 also taught the parser the dialect `std/*.ns` files are written
in, so those files now parse: `pub` visibility (recorded, advisory),
`module std.x;` declarations, `import std.x;` paths (resolution is a
future wave — clean error at runtime), `ffi::call(...)` namespaces
(parsed as member access; `ffi` itself is a future native-binding layer),
class fields (`let size: number = 0;` — defaults run per-instance before
`init`), `x++`/`x--` (postfix, returns the old value), braceless
`if`/`while`/`for` bodies, and `->` returns.
