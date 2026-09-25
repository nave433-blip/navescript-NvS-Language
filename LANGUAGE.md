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
| Maps | `{k:v}`, keys, membership | Python, JS |
| Sets | `set()`, `set_add`, `set_has` | Python |
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
