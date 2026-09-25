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
| Pattern | `match` / `switch` / case / default | Rust, C# |
| Errors | try / catch / **finally** / throw | Java, Python |
| Functions | closures, defaults, generators, decorators | JS, Python |
| Defer | `defer expr` (LIFO at function exit) | Go |
| OOP | class, new, this, extends | JS, Java |
| Enums | `enum Name { A, B }` | Rust, TS, Java |
| Modules | `import "file.ns"` | Python, Go |
| Types | `type()`, `typeof()`, `isinstance()` | Python, JS |
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

## Not full ports (by design)
- Static Hindley–Milner type inference
- True OS threads / async runtime
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
