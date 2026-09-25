# NvS (Navescript) 2.1.0 — Language Overview

NvS is a custom multi-paradigm scripting language drawing features from
Python, JavaScript, Go, Ruby, Rust, Java, C#, and others — implemented
where a tree-walking interpreter can support them honestly.

## Quick start
```bash
nvs                  # REPL
nvs run file.ns
nvs init
nvs info
```

## Feature map (established-language inspired)

| Area | Features | Inspired by |
|------|----------|-------------|
| Bindings | `let`, `const`, optional type annotations | JS, TS, Rust |
| Null | `null`, `??` | JS, C# |
| Logic | `and`/`or`, `&&`/`||`, comparisons | Python, C |
| Numbers | int, float, `0x` hex, `0b` binary | Python, C, JS |
| Strings | index, slice `[a:b]`, split/join/… | Python, Go |
| Arrays | literals, push/pop, slice, map/filter/reduce | JS, Python |
| Maps | `{k:v}`, keys, membership, `map_merge`, `map_pick`/`map_omit`, `invert` | Python, JS |
| Sets | `set()`, `set_add`, `set_has`, `set_remove`, `set_union`/`set_intersect`/`set_diff`, `set_len`, `set_to_array` | Python |
| Control | if/else, while, for, for-in, break/continue | C family |
| Pattern | `match` / `switch` / case / default, guards, destructuring patterns, expression form | Rust, C#, Elixir |
| Errors | try / catch / **finally** / throw | Java, Python |
| Functions | closures, defaults, generators, decorators | JS, Python |
| Defer | `defer expr` (LIFO at function exit) | Go |
| OOP | class, new, this, extends | JS, Java |
| Enums | `enum Name { A, B }` | Rust, TS, Java |
| Modules | `import "file.ns"` | Python, Go |
| Types | `type()`, `typeof()`, `isinstance()`, `type_of()`; `is_int`/`is_float`/`is_number`/`is_string`/`is_bool`/`is_null`/`is_array`/`is_hash`/`is_tuple`/`is_function` | Python, JS |
| Equality | `==`, `deep_equal()` | Python |
| Math | abs, floor, ceil, sqrt, sin, cos, tan, exp, round, pow | C, Python |
| Random | random, rand_int | Python |
| Regex | regex_match/find/replace | Perl, Python |
| FS / JSON | read/write, json_*, csv_parse | Node, Python |
| HTTP | http_get/post/serve | JS |
| Polyglot | python, js, ruby, rust, go, c, cpp, java, css | FFI |
| Interop | detect_lang, to_nvs, from_nvs, translate, corrections DB | — |
| DX | highlight, fuzzy_*, nvs_info | editors / shells |
| Meta | nvs_version, nvs_language, plugins, applets | — |

## Wave 1 — ergonomics robbery (2.2.0-dev)

| Feature | Syntax | Inspired by |
|---------|--------|-------------|
| Destructuring | `let [a, b] = [1,2,3]`, `let [a, ...rest] = arr`, `let {x, y} = {x:1,y:2}`, `let {k: renamed} = m` (also `const`); missing → `null` | JS, Python, Rust |
| Spread / rest | `[...a, 0]`, `f(...args)`, `{...m, k: 1}` (later keys win); non-array/map spread is a runtime error | JS, Python |
| Optional chaining | `a?.b`, `a?.b()`, `a?.[i]`, `a?.b.c?.d` — null short-circuits the whole chain, no calls made | JS, C#, Swift |
| String interpolation | `"hello ${name}, ${age + 1}"` — balanced `${...}`, nesting works | JS, Ruby, Kotlin |
| Pipeline | `x \|> f \|> g` → `g(f(x))`; `x \|> f(a, b)` → `f(x, a, b)` | Elixir, F# |
| Ranges | `1..10` → `[1..10]` inclusive; `1...5` → `[1..4]` exclusive; int endpoints only | Ruby, Rust, Kotlin |
| Loop else | `for (x in xs) {...} else {...}`, `while (c) {...} else {...}` — else runs only if no `break` | Python |
| Labeled break/continue | `outer: for (...) {...}` + `break outer` / `continue outer` | Java, JS, Rust |

Limitations (honest): destructuring patterns are one level (identifiers + `...rest` /
`{x}` / `{k: v}` — no nested patterns); no escape for a literal `${` in strings
(use `"$" + "{x}"`); `break <label>` needs the label on the same line; in
`{k: 0, ...m}` an explicit key beats a later spread (spreads apply first, then
explicit pairs).

## Wave 3 — data robbery (2.2.0-dev)

| Feature | Syntax | Inspired by |
|---------|--------|-------------|
| Tuples | `(1, 2)`, `(x,)`, `()` — immutable; `(x)` stays grouping; index/slice/`in`/`len`/for-in; `==` compares by value; `tuple(arr)` converts | Python |
| Records | `record Point(x, y)` declares an immutable struct type; `Point(1, 2)` or `Point(x: 1, y: 2)` constructs; `.x` access; `==` is field-by-field; prints as `Point(x=1, y=2)`; destructurable via `let {x, y}` | Python namedtuple, C# records |
| Freeze | `freeze(obj)` deep-freezes arrays/hashes in place (JS `Object.freeze` semantics); `is_frozen(x)`; `thaw(x)` deep mutable copy (tuple→array, record→hash); any mutation of a frozen value is a loud runtime error naming the offense | Python frozenset, JS Object.freeze |
| Deep paths | `deep_get(obj, "a.b.0.c")` / `deep_get(obj, "a.b", default)`; `deep_set(obj, "a.b.c", v)` creates intermediate hashes lodash-style, returns the root | Lodash |
| Set/map builtins | `set_union`/`set_intersect`/`set_diff` (new sets), `set_len`, `set_to_array`, `set_remove`; `len()` now handles sets/hashes, tuples, records; `map_merge` (later-wins, new map), `map_pick`/`map_omit`, `invert` | Python, Lodash |

Notes: tuple/record syntax is non-breaking — `(a, b)` was a parse error before.
Records are immutable: field assignment and index assignment are runtime errors,
and `new Point(...)` is not a thing (records aren't classes — just call them).
`freeze` is in-place, so aliases see the frozen value; `thaw` never mutates.
`deep_set` never auto-creates arrays (no sparse magic): missing intermediates
become hashes, and indexing into an array requires the index to already exist.
Numeric path segments on hashes mean string keys first, then integer keys.
`set_to_array` and `invert` sort by key rendering so results are deterministic
(Go map order is random). `record` is now a keyword. `print (a, b)` prints the
tuple `(a, b)` — previously that was a parse error.

## Wave 4 — pattern-matching robbery (2.2.0-dev)

| Feature | Syntax | Inspired by |
|---------|--------|-------------|
| Guards | `case x if x > 0: { ... }`, `case [a, b] if a != b: ...` — a falsy guard falls through to the next arm (never an error); guards see the arm's bindings, and a failed guard leaks nothing | Rust, Haskell |
| Destructuring patterns | `case [a, b]:`, `case [h, ...t]:`, `case {x, y}:`, `case {k: renamed}:`, `case (a, b):` (tuple), `case Point(x, y):` / `case Point(x: a, y: _)`: (record, positional or named) — refutable: shape mismatch falls through | Rust, Elixir |
| Expression form | `let r = match (v) { case 1: "one"; default: "other" }` — arm bodies yield their last value; single-statement arms (`case 1: "one"`) and `default: "other"` without braces now parse (previously a parse error); scrutinee parens optional: `match x { ... }` | Rust |

Pattern semantics (deliberate, documented):
- **Refutable** (Rust-like): `case [a, b]:` against a 3-element array, `case {x}:`
  against a non-hash or a hash missing `x`, `case Point(a):` against a 2-field
  record, all fall through to the next arm. This differs from wave-1 `let`
  destructuring, which binds missing → `null`.
- A **bare identifier always binds** (`case n:`); `_` is the wildcard and binds
  nothing. (Previously `case x:` on an undefined name was a runtime error, and
  `case _:` errored too — both now work as intended. To compare against an
  existing variable, use a guard: `case [x] if x == LIMIT:`.)
- Inside patterns, **identifiers bind, literals test**: `case [1, x]:` requires
  first element `== 1` and binds `x`; `case Point(0, y):` requires field `x == 0`.
  Any other expression (e.g. `case 1 + 2:`, `case f(x):`) is evaluated once and
  compared by value, as before.
- `{...}` keys in patterns are **field names** (like wave-1 destructuring):
  `case {x: 1}:` tests field `x`; `case {x, y}:` binds both. `{x: 1}` as a
  *value* still evaluates `x` (unchanged — quote keys in literals: `{"x": 1}`).
- Bindings live in the arm's scope only: arm bodies and guards run in a fresh
  enclosed environment, so bindings never leak into the enclosing scope (no
  example depended on the old leak, which only triggered for already-defined
  names anyway). Reads/writes of outer variables still work normally.
- `[a, b]` and `(a, b)` patterns destructure arrays, tuples, and records
  positionally (wave-1 `let` already treats them interchangeably); `{x, y}`
  matches hashes and records by field name. Record patterns match by type name
  and require every field (use `_` to skip).
- `case x` followed by an `if` now parses the `if` as a **guard**; write
  `case x: if ...` (with colon) for an if-statement body.

```bash
nvs run examples/wave4_guards.ns
nvs run examples/wave4_patterns.ns
nvs run examples/wave4_expr.ns
```

## Wave 2 — functions robbery (2.2.0-dev)

| Feature | Syntax | Inspired by |
|---------|--------|-------------|
| Named arguments | `f(x: 1, y: 2)` — positionals first, then named; unknown name / duplicate / missing required param are runtime errors; defaults fill the rest | Python, Kotlin, Swift |
| Partial application | `partial(f, a, b)` → callable with leading args pre-bound; positional-only; partial of a partial composes | JS, Python functools, Haskell |
| Currying | `curry(f)` → collects positional args one-or-more at a time until required arity is met; arity = params minus defaulted ones | Haskell, OCaml |
| Composition | `compose(f, g)` → `f(g(x))`; `compose(f, g, h)` → `f(g(h(x)))` — right-to-left; composes with partials and curried fns | Haskell, F# |
| `?.` call chains | `f?.(x: 1)`, `obj?.m(x: 1)` — named args work in chains too; null still short-circuits with no call | — |

Notes: `f(x=1)` still parses as an *assignment* expression (backward compat) —
named arguments use `name:` because `:` is free in call args and can't collide
with the ternary (which consumes its own `:`). Calling a builtin with named
args is an honest error (`builtin len does not accept named arguments`).
Class methods don't support default parameter values (pre-existing), so named
method calls must supply every parameter. `curry` refuses builtins and
non-functions instead of guessing an arity; extra args at the final curried
call pass through exactly like a normal call. The old two-arg NvS-level
`compose` in `stdlib/prelude.ns` was removed — the variadic builtin
supersedes it.

## Wave 5 — types robbery (2.2.0-dev)

**There is no static type checker in NvS, and wave 5 does not add one.**
Every feature below is a *runtime* contract: annotations are parsed, stored on
the function/binding, and enforced while the program runs — at call time, at
binding time, or on return. A violation is a runtime error, catchable with
`try`/`catch`. Unannotated code behaves exactly as before.

| Feature | Syntax | Inspired by |
|---------|--------|-------------|
| Param annotations | `fn add(a: int, b: int) { ... }` — wrong argument type is a runtime error naming the parameter, the expected type, and the got type | Python, TS |
| Return annotations | `fn add(a: int, b: int): int { ... }` — wrong return type errors on return; thrown errors propagate untouched (an error is not a return value) | Python, TS |
| Union annotations | `fn f(x: int | string)` — `\|` is union *only* in annotation position; elsewhere it stays bitwise-or | TS, Python |
| Nullable sugar | `fn f(x: int?)` — exactly `int \| null` | TS, C# |
| `let` annotations | `let x: int = 5` — initial binding checked; the declared type is remembered and later assignments (`x = ...`, incl. from closures) are checked | TS, Rust |
| Interfaces | `interface Shape { area(): number }` — structural contract | Go, TS, Python Protocol |
| `implements` / `assert_implements` | `implements(obj, Shape)` → bool; `assert_implements(obj, Shape)` → nil or a runtime error listing what's missing | Go |
| Interfaces as annotations | `fn draw(s: Shape)` — checks `implements` at call time | TS, Python |
| Class/record names as annotations | `fn t(a: Animal)`, `fn g(p: Point)` — instanceof incl. subclasses; record-def identity | Python, Java |
| Type guards | `is_int`, `is_float`, `is_number`, `is_string`, `is_bool`, `is_null` (pre-existing), `is_array`, `is_hash`, `is_tuple`, `is_function` | TS type predicates |
| `type_of` | `type_of(1)` → `"int"` — lowercase companion to `type()`/`typeof()` (which keep returning `"INTEGER"`) | — |

Annotation type vocabulary (case-insensitive): `int`/`integer`, `float`,
`number` (int or float), `string`/`str`, `bool`/`boolean`, `null`, `array`/`list`,
`hash`/`map`/`dict`, `tuple`, `function`/`fn`/`callable` (user functions *and*
builtins — anything callable), `record` (any record), `any` (no check).
Anything else resolves lexically at call time: an `interface` name checks
`implements`, a `class` name checks instanceof (subclasses count), a
`record` type name checks record identity. An unresolvable name is a loud
runtime error (`unknown type 'Nope' ...`), never a silent pass. Builtin names
win over same-named bindings.

What `implements()` verifies (and nothing more): every required member is
present on the value (class instances incl. inherited methods and fields,
hashes by string key, record fields), every required member is callable, and —
when the member is a user function with knowable arity — it can be invoked
with exactly the number of arguments the interface declares. It does **not**
verify parameter/return type compatibility (no signature-variance analysis);
an annotated implementation enforces its own contracts when called.

Deliberate semantics:
- Defaults compose: `fn f(x: int = 3)` — the default is checked *when used*;
  `f()` with `fn f(x: int = "bad")` errors, `f(5)` does not.
- Missing arguments keep the historical behavior (bound to `null`, no new
  arity error) — annotations only check values that are actually bound.
- Generator functions skip the return-annotation check (results flow through
  `yield`, not `return`).
- `partial`/`curry`/`compose` wrappers delegate to the underlying function,
  so its annotations still fire at the final call.
- `const` does not take annotations (`let` covers it); interface methods have
  no bodies and no `extends`; `obj.method()` call syntax still requires a
  class instance, so a map implementing an interface is invoked as
  `m["name"]()`.
- `is_function` is true for user functions and builtins alike (both callable).

```bash
nvs run examples/wave5_annotations.ns
nvs run examples/wave5_unions.ns
nvs run examples/wave5_interfaces.ns
nvs run examples/wave5_guards.ns
```

## Wave 6 — concurrency robbery (2.2.0-dev)

**The model is cooperative message-passing** (Lua coroutines / Erlang
processes / JS workers lineage). Under the hood the execution mechanism is
Go goroutines, but the documented contract — the only thing programs may
rely on — is cooperative:

- Tasks switch at **explicit yield points only**: `sleep()`,
  `task_yield()`, a blocking `send()`/`recv()`, or `join()`. User code can
  never observe a switch anywhere else.
- **NO SHARED MUTABLE STATE.** Values crossing a channel (`send`) or passed
  as `spawn` arguments are **deep-copied** — arrays, hashes, tuples, and
  records (frozen-ness preserved), so the sender can never observe or
  disturb what the receiver got. Channel and task handles pass by reference
  — they are the communication mechanism, never copied. Functions,
  builtins, class instances, and generators also pass by reference (they
  cannot be copied). **Mutating a value that is visible from two tasks is a
  data race and a bug in your program; communicate via channels.**
- The interpreter's variable store is mutex-guarded, so concurrent
  top-level `let`/assignment cannot corrupt the map itself. Racy programs
  get last-writer-wins semantics — and this paragraph telling them not to
  do that.
- `nvs run` waits for every spawned task to finish before exiting, so no
  task output is lost to an early exit.

| Builtin | Meaning |
|---------|---------|
| `spawn(fn, args...)` | Run `fn` concurrently; returns a task handle. Args are deep-copied (handles pass by reference). |
| `chan()` / `chan(n)` | Unbuffered channel / buffered with capacity `n`. |
| `send(ch, v)` | Blocking send (rendezvous if unbuffered). Errors on a closed channel. |
| `recv(ch)` | Blocking receive. Returns `null` once the channel is closed **and** drained. |
| `try_recv(ch)` | Non-blocking: `[true, v]` or `[false, null]` (empty and closed+drained both read `[false, null]`). |
| `try_send(ch, v)` | Non-blocking: `true` if accepted, `false` if the buffer is full. Errors on a closed channel. |
| `close(ch)` | Seal the channel: buffered values still drain, then `recv` gives `null`. Double-close is an error. |
| `sleep(x)` | Integer `x` = milliseconds (**unchanged** historical unit); float `x` = seconds. A yield point. |
| `task_yield()` | Explicit cooperative yield (named `task_yield` because `yield` is the generator keyword). |
| `join(task)` | Block until the task finishes; returns its return value. If the task raised, `join` re-raises the same error (catchable with `try`/`catch`). (The 2-argument `join(array, sep)` string form is unchanged.) |
| `task_status(task)` | `"running"` or `"done"`. |
| `pmap(fn, array)` | Parallel map: one task per element, results collected in order. A task error aborts the map and re-raises. |

```ns
let ch = chan()
fn worker(c) {
  let v = recv(c)
  return v * 2
}
let t = spawn(worker, ch)
send(ch, 21)
print join(t)   // 42
```

```bash
nvs run examples/wave6_pingpong.ns
nvs run examples/wave6_pmap.ns
nvs run examples/wave6_tryrecv.ns
nvs run examples/wave6_sleep_yield.ns
nvs run examples/wave6_join_values.ns
nvs run examples/wave6_closed_channel.ns
```

Honest edges, documented:
- A blocking `send` on an unbuffered channel with no receiver parks forever;
  pair every send with a receiver, or use `try_send` for deadlock-avoidance.
- Spawning a generator function (one containing `yield` statements) runs it
  to completion; `join` returns the generator.
- `pmap` over an empty array returns `[]` without spawning anything.

## Not full ports (by design)
- Static Hindley–Milner type inference
- Preemptive threading / shared-memory parallelism (the wave-6 model is
  cooperative message-passing; shared mutable state across tasks is a user
  bug, not a feature)
- Native GPU / USB drivers
- Full macro system / compiler backend

## Example
```ns
enum Status { Ok, Err }
let xs = [1, 2, 3, 4]
print xs[1:3]
try {
  throw "fail"
} catch (e) {
  print e
} finally {
  print "done"
}
fn f() {
  defer print "bye"
  print "hi"
}
f()
```
