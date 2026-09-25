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
nvs fmt [--check] [files...]
nvs lint [--json] [files...]
nvs doc [files...]
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
| HTTP | http_get/post/serve, http_request | JS |
| Polyglot | python, js, ruby, rust, go, c, cpp, java, css | FFI |
| Interop | detect_lang, to_nvs, from_nvs, translate, corrections DB | — |
| DX | highlight, fuzzy_*, nvs_info | editors / shells |
| Meta | nvs_version, nvs_language, plugins, applets | — |
| Quantum | qalloc, q_h/x/y/z/s/t, q_rx/ry/rz, q_cnot/cz/swap, q_measure(_all), q_probs/state, q_circuit, q_reset, q_backend — **local state-vector simulator, not hardware** | Qiskit, Cirq |

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

## Wave 7 — stdlib robbery (2.2.0-dev)

Baked-in standard library, filling genuine gaps — stdlib-only Go packages,
no new mandatory dependencies. Everything is additive: no existing builtin
changed shape (the only edit is `env` gaining an optional default).

| Area | New builtins | Notes |
|------|--------------|-------|
| Datetime | `now_iso()`, `unixtime_ms()`, `date_format(ts, layout)`, `parse_date(str)`, `date_add(ts, n, unit)` | `now()` (unix seconds) and `date([ts[, layout]])` already existed. `parse_date` tries RFC3339, `2006-01-02[ 15:04:05]`, RFC1123/822, Kitchen; zoneless layouts use local time, like `date()`. Units: `s`/`m`/`h`/`d`/`w` plus long names (`"days"`, …). |
| HTTP | `http_request(method, url, opts?)` → `{status, headers, body}` | `http_get`/`http_post` keep their body-string return shape (backward compat). `opts`: `{"headers": {...}, "body": "...", "timeout": seconds}`. Response `headers` maps name → array of values (lossless — HTTP allows repeats). Unknown opts are an error, not silently ignored. |
| Hashing | `sha1(str)`, `hmac_sha256(key, msg)` | `md5`/`sha256` already existed. **md5/sha1 are fingerprinting hashes, not security primitives** — checksums and cache keys yes, passwords no. |
| Base64 | `base64url_encode`, `base64url_decode` | Raw URL-safe alphabet, no padding (`+`/`/` → `-`/`_`). `base64_encode`/`decode` already existed; `uuid()` already existed (v4, so no `uuid4()`). |
| Subprocess | `exec(cmd, args...)` → `{code, stdout, stderr}`, `sh(cmd)` → `{code, stdout, stderr}` | Real OS processes — the caller's responsibility. `exec` takes argv directly (no shell, no glob expansion); `sh` runs through `sh -c`. Non-zero exits are data (`r["code"]`), not NvS errors; only failure to *start* (e.g. command not found) is an error. `system()` already existed but merges stderr into stdout and hides the exit code. |
| Env/args | `env(name, default)` | 1-arg form unchanged (`""` when unset). `set_env` is process-local. `args()` already returned script argv (`nvs run f.ns a b c` → `["a","b","c"]`). |
| Path | `extname(path)`, `abs_path(path)` | `basename`/`dirname`/`join_path`/`exists` already existed (`file_exists` was never missing — it's `exists()`). |
| TOML | `toml_parse(str)` → hash | **Documented subset, not full TOML** — see below. |
| Compression | `gzip_compress(str)` → str, `gzip_decompress(str)` → str | Go strings are byte-safe, so raw gzip bytes round-trip through NvS strings untouched. |

### `toml_parse` subset (honest, by design)

Supported: `[table]` / `[table.sub]` headers, `key = value`, dotted keys,
bare + quoted keys, basic strings (with `\b \t \n \f \r \" \\ \uXXXX \UXXXXXXXX`
escapes), literal strings, ints (decimal, `0x`/`0o`/`0b`, underscores),
floats, bools, single-line arrays (nested ok, trailing comma ok), `#`
comments and blank lines. NOT supported — loud parse errors, never silent
wrong answers: multi-line strings/arrays, inline tables `{k = v}`, datetimes
(quote them as strings), `[[array-of-tables]]`, `inf`/`nan`. Duplicate keys
and redefined tables are errors.

### Deliberately not added

- **YAML**: a correct YAML parser is genuinely hard (anchors, aliases,
  multi-document streams, duplicate-key semantics). We will not ship a lying
  subset; `toml_parse` covers the config-file use case honestly.
- **`uuid4()`**: `uuid()` already exists and already returns a v4 UUID.
- **`file_exists`**: `exists()` already covers it.

```bash
nvs run examples/wave7_datetime.ns
nvs run examples/wave7_crypto.ns
nvs run examples/wave7_subprocess.ns
nvs run examples/wave7_env_path.ns
nvs run examples/wave7_toml.ns
nvs run examples/wave7_gzip.ns
```

## Wave 8 — quantum robbery (2.2.0-dev)

> **THIS IS A LOCAL SIMULATOR, NOT QUANTUM HARDWARE.** Every builtin below
> runs on your own CPU. NvS tracks the full state vector (2^n complex
> amplitudes for n qubits) and applies real gate matrices to it — the same
> approach as Qiskit Aer / Cirq's simulator. Nothing here talks to a quantum
> processor or to Azure Quantum / IBM Quantum / any cloud backend. Gate
> results are computed, not measured from nature; the only genuine
> randomness is classical (`crypto/rand`) collapsing the simulated state on
> measurement. Ideal and noise-free: no decoherence, no gate errors.

| Builtin | Meaning |
|---------|---------|
| `qalloc(n)` | Allocate an n-qubit register in \|0…0⟩. **Simulator limit: at most 24 qubits** (2^24 complex128 = 256MB of state; every gate is O(2^n)) — more is a loud error, not a hang. |
| `q_h`/`q_x`/`q_y`/`q_z`/`q_s`/`q_t(q, i)` | Single-qubit gates on qubit `i`. Real matrix application, not faked. |
| `q_rx`/`q_ry`/`q_rz(q, i, theta)` | Rotations, **theta in radians** (int or float). |
| `q_cnot(q, ctrl, tgt)` | Controlled-NOT. `q_cz(q, a, b)` phase flip, `q_swap(q, a, b)` exchange. |
| `q_measure(q, i)` → `0`/`1` | Real probabilistic collapse via `crypto/rand` (honest randomness, no fixed seed). |
| `q_measure_all(q)` | Array of bits **in qubit order** (element 0 = qubit 0). |
| `q_probs(q)` | Array of 2^n outcome probabilities — deterministic, the test-friendly surface. |
| `q_state(q)` | Array of `[re, im]` pairs (NvS has no complex type — documented representation). |
| `q_nqubits(q)` | Register width. |
| `q_circuit(q)` | ASCII circuit diagram of gates applied so far (●/⊕ = CNOT, ◉ = CZ, × = SWAP, M = measurement), plus an op legend. |
| `q_reset(q)` | Back to \|0…0⟩, recorded circuit cleared. |
| `q_backend(name)` | `"local"` → `"local-simulator"`. Anything else (e.g. `"azure"`) is an **honest error**: `hardware backend "azure" is not connected in this build — local simulator only`. A future hardware backend implements the Go `QuantumBackend` interface (`ApplyGate`, `Measure`, …); the local simulator is its first implementation. NvS will never fake a hardware connection. |

Gates return the register, so calls chain: `q_h(q_h(qalloc(1), 0), 0)`.
Out-of-range qubit indices are loud errors. Registers are mutable handles
(like wave-6 channels/tasks): they pass by reference and are not safe to
share across tasks.

**Conventions (one choice, documented): little-endian** — qubit 0 is the
least significant bit. State index `i` is \|q_{n-1}…q_1 q_0⟩ with bit `j` of
`i` = qubit `j`: `q_probs(q)[2]` on 2 qubits is P(\|10⟩) = P(qubit1=1,
qubit0=0), and the Bell state from `q_h(q,0); q_cnot(q,0,1)` has
probabilities `[0.5, 0, 0, 0.5]`.

```bash
nvs run examples/wave8_bell.ns
nvs run examples/wave8_superposition.ns
nvs run examples/wave8_rotations.ns
nvs run examples/wave8_circuit.ns
nvs run examples/wave8_measure.ns
```

## Wave 9 — tooling robbery (2.2.0-dev)

Developer tooling, gofmt/rustfmt/clippy/cargo-doc inspired — small and
honest about scope.

### `nvs fmt` — canonical formatter (lexical, not AST-based)

The AST `String()` methods are lossy (`try` prints as `try { ... }`,
`match` as `match (...) { ... }`), so an AST pretty-printer could not
round-trip. `nvs fmt` is therefore **lexical**: it never parses, it only
re-indents and tidies whitespace.

```bash
nvs fmt [files...]          # format in place (stdin → stdout if no files)
nvs fmt --check [files...]  # exit 1 + list files that would change (CI use)
```

Normalizes: 4-space indentation by `{ }`/`( )`/`[ ]` depth; tabs → 4 spaces
(outside strings); trailing-whitespace removal; blank lines collapsed to at
most one (leading blanks dropped); exactly one trailing newline.
Deliberately leaves alone: string contents (including `${...}`
interpolation — braces there never affect indentation), `//` and `/* */`
comment contents (lines inside a multi-line string/comment are not
re-indented), and all in-line spacing (`f (x)` stays `f (x)`).
Guarantees, tested: idempotent (`fmt(fmt(x)) == fmt(x)`) and
behavior-preserving (verified: every `examples/*.ns` runs byte-identical
before/after formatting, modulo pre-existing nondeterminism).

### `nvs lint` — a small set of sound static checks

```bash
nvs lint [files...]         # 0 = clean, 1 = findings; findings print as
                            # file:line: severity rule: message
nvs lint --json [files...]  # machine-readable output
```

| Rule | Severity | Meaning |
|------|----------|---------|
| `unused-binding` | warning | `let`/`const` bound but never referenced anywhere in the file. Excludes function params (too noisy) and named `fn` declarations (entry points / library APIs). Any mention — including a plain assignment — counts as a use, so no false positives on normal code (shadowing can hide a case: false negatives, never false positives). |
| `shadow-builtin` | warning | `let`/`const`/plain assignment to a Go-builtin name (`len`, `print`, …). Covers Go-registered builtins only, not `prelude.ns` functions. |
| `unreachable-code` | warning | Statements in a block directly after `return`/`break`/`continue`/`throw`. Only direct block statements — no analysis through conditions (code after `while (true) {}` is not flagged). |
| `null-comparison` | style | `x == null` / `x != null` → suggests `is_null(x)`. |
| `parse-error` | error | File doesn't parse; no further checks ran. |

What is NOT claimed: no dataflow analysis, no type inference, no
cross-file/module analysis, no dead-code detection beyond the direct
unreachable rule.

### `nvs doc` — minimal doc-comment extractor (stub, labeled as such)

```bash
nvs doc [files...]          # Markdown to stdout
```

Extracts `//` doc comments immediately preceding **top-level**
`fn`/`class`/`record`/`interface` declarations (a blank line stops the
comment block) and emits `## name`, a reconstructed signature
(`fn add(a, b)`, defaults and return annotation included when trivial),
and the doc text. Nested declarations, cross-references, and index pages
are out of scope — this is a stub, not rustdoc.

```bash
nvs run examples/wave9_fmt_bad.ns    # deliberately badly formatted
nvs lint examples/wave9_lint.ns      # exits 1, names the four rules
nvs doc examples/wave9_doc.ns
```

## Wave 10 — polyglot interop: write NvS once, use it from any language

Four mechanisms, one contract: NvS values cross language boundaries as
JSON (`internal/polyglot`), and anything that cannot convert fails loudly
— never silently mis-converted.

### C ABI (`cbridge/`)

```bash
go build -buildmode=c-shared -o libnvs.so ./cbridge   # → libnvs.so + libnvs.h
```

Exports:

```c
char* nvs_eval(const char* src);                 // eval NvS source
char* nvs_call(const char* func_name, const char* args_json);  // call fn by name
void  nvs_free(char* s);                         // free a returned string
```

Contract (also documented in `cbridge/cbridge.go`):

- **One persistent interpreter** for the process lifetime: functions and
  bindings defined by one `nvs_eval` are visible to later calls.
- **Serialized**: concurrent calls from multiple threads are safe (one
  global mutex); they never run in parallel.
- **Ownership**: every non-null return from `nvs_eval`/`nvs_call` is a
  freshly `malloc`'d C string you own — **you must call `nvs_free`** on
  it. Never `free()` it yourself, never leak it.
- Responses are the shared JSON envelope (below): `{"ok":true,"result":…}`
  or `{"ok":false,"error":"…"}`.

Convertible NvS values: int, float, string, bool, null, array, hash (keys:
string/int/bool, rendered `"1"`/`"true"`; a key collision after rendering
is an error). A function value, builtin, or anything else as a result is
`ok:false` naming the NvS type — e.g. `cannot convert NvS FUNCTION to
JSON`. Integer-syntax JSON numbers stay NvS integers across the boundary
(`UseNumber`, so `9007199254740993` never becomes a float).

Tested for real: `examples/wave10_ctypes.py` (Python `ctypes`, all
assertions pass — signatures use `c_void_p` so `nvs_free` gets the real
pointer, and every return is freed).

Other languages, same ABI — **sketches, not tested** (the ABI is plain C,
so these are the standard FFI spellings; verify against your toolchain):

| Language | FFI route | Sketch |
|----------|-----------|--------|
| Python | ctypes (tested) / cffi | `examples/wave10_ctypes.py` |
| Python | JSON bridge + `nvs bindgen` (tested) | `nvs bindgen --to=python calc.ns -o calc.py`, then `Calc().add(2, 3)` |
| Node.js | JSON bridge (tested) | `examples/wave11_node_bridge.mjs` — spawn `nvs bridge`, exchange line-delimited JSON |
| Node.js | ffi-napi (untested sketch) | `const lib = ffi.Library('./libnvs', {nvs_eval: ['string', ['string']], nvs_call: ['string', ['string', 'string']], nvs_free: ['void', ['string']]}); const p = lib.nvs_eval('1+2'); …` — note: route the raw pointer through `nvs_free`; do not let ffi-napi free it. Requires an npm build step; not covered by tests. |
| Ruby | fiddle / ffi gem | `Fiddle::Function.new(handle['nvs_eval'], [Fiddle::TYPE_VOIDP], Fiddle::TYPE_VOIDP)` — same ownership rule |
| Rust | `extern "C"` | `extern "C" { fn nvs_eval(src: *const c_char) -> *mut c_char; fn nvs_free(s: *mut c_char); }` + `CStr::from_ptr` + `nvs_free` |
| C# | P/Invoke | `[DllImport("libnvs")] static extern IntPtr nvs_eval(string src);` — marshal as `IntPtr`, call `nvs_free`, never `Marshal.FreeHGlobal` |
| Java | JNA | `Pointer nvs_eval(String src); void nvs_free(Pointer p);` — read with `getString(0)`, then `nvs_free` |

### JSON stdio bridge (`nvs bridge`)

For languages without (or above) FFI: a persistent interpreter behind a
JSON-line protocol.

```bash
nvs bridge            # reads requests on stdin, writes responses on stdout
```

Protocol — one JSON object per line in, one JSON envelope per line out:

```
{"eval": "<nvs source>"}            → {"ok":true,"result":<json>}
{"call": "<name>", "args": [...]}   → {"ok":true,"result":<json>}   (user fns + builtins)
anything malformed                  → {"ok":false,"error":"invalid request: …"}
```

One session = one interpreter: definitions persist across lines, all
requests serialized. `examples/wave10_bridge_client.py` drives it from
Python for real (persistence, builtins, nested values, honest parse/call
errors — see `wave10_bridge_client.expected`).

### Transpiler (`nvs transpile --to=js|python`)

An **honest-subset** source-to-source transpiler (`internal/transpile`):
it covers only the subset whose semantics are identical in NvS, JS, and
Python, emits a small runtime prelude (`__div`, `__str`, `__truthy`, …)
where the targets would otherwise differ, and **rejects everything else
with an error naming the construct** — it never emits silently-wrong code.

```bash
nvs transpile --to=js examples/wave10_transpile_demo.ns > demo.js
nvs transpile --to=python examples/wave10_transpile_demo.ns > demo.py
```

**In the subset:** `let`/`const` (typed bindings: annotations erased),
arithmetic with NvS integer division/modulo, string interpolation,
`if`/`else` statements, ternaries, `while`, C-style `for`, `for`-`in`
over arrays/strings/hash-keys, `break`/`continue` (incl. continue-with-post
in desugared Python `for`), named `fn` with implicit trailing-expression
returns and default literal params, recursion, closures assigning outer
bindings (Python gets correct `nonlocal`/`global`), arrays, string-keyed
hashes, indexing/slicing with NvS clamping, `??`, `and`/`or`/`!` with NvS
truthiness, `==` with NvS identity semantics for arrays/hashes. Builtin
mapping is explicit and small: `len`, `str`, `push`, `split`, `upper`,
`lower`, `abs`, `range`, `keys`.

**Out of the subset (each a named error):** classes, pattern matching,
exceptions, defer, generators, decorators, destructuring, spread, optional
chaining, ranges `a..b`, tuples, member access/calls, imports, named args,
anonymous functions (Python target), if/while/for as a function value,
loop labels, for/while-`else`.

**Preconditions / non-goals:** the input must run without errors in NvS —
error behavior (const violations, type errors, arity errors) is *not*
replicated; comments are dropped; type annotations are erased without
runtime enforcement; numeric range is the target's (JS bitwise ops are
32-bit); hash iteration order stays unspecified everywhere.

Verified: `internal/transpile` tests transpile a semantics-heavy program
and run the emitted JS under node and Python under python3, comparing
against NvS output — equal.

### WASM

The full interpreter cross-compiles to WebAssembly (pure Go, no cgo):

```bash
GOOS=js GOARCH=wasm go build -o nvs.wasm ./cmd/nvs/
```

Build-only: the binary builds (verified 2026-09-25); executing it in a
browser/Node WASM runtime has not been tested and is not claimed.

## Reconciliation — 2.2–2.9 track ports

The remote 2.2–2.9 track grew a parallel set of features (low-level quantum
API, self-hosting subset interpreter/emitter, bytecode VM, Nave workflow
runner). The genuinely working pieces are ported below onto the
tree-walking interpreter, which remains the primary execution engine.
Deliberately excluded: the malformed legacy `internal/vm`, the incomplete
`bindings/go` (illegal package name), and the cobra-based `nvm` (would add
an external dependency for no working feature).

### Low-level quantum API (2.2.0)

Alongside the wave-8 circuit API (`qalloc`, `q_h`, …), a lower-level raw
state-vector API exists. **Both are local CPU simulators** — the wave-8 API
uses `crypto/rand` for measurement; this one inherits the 2.9 package's
`math/rand` measurement. States are plain arrays of `[re, im]` pairs.

| Builtin | Meaning |
|---------|---------|
| `qzero(n)` | Zero state for n qubits: array of 2^n `[0,0]` pairs with `[1,0]` first. |
| `qubit(a, b)` | Single-qubit state from amplitudes `a`, `b` (ints/floats). |
| `qgate(state, matrix, targets...)` | Apply a gate matrix (array of `[re,im]` rows) to target qubit(s). |
| `qmeasure(state)` | Collapse; returns `{"outcome": bits, "state": new_state}`. |
| `qprob(state)` | Array of outcome probabilities. |
| `qtensor(a, b)` | Kronecker product of two states. |
| `qnormalize(state)` | Normalize; errors on zero vector. |
| `qinner(a, b)` | Inner product `[re, im]`. |
| `physics_const(name)` | SI constants: `"hbar"`, `"c"`, `"G"`, `"kB"`, `"e"`, `"me"`, `"mp"`, `"NA"`, `"h"`. |
| `self_eval(src)` | Evaluate NvS source in the current environment (the 2.3 track's reflective hook). |

Physics/Greek globals are also injected: `pi`, `tau`, `phi`, `hbar`,
`h_planck`, `c_light`, `G_grav`, `k_B`, `e_charge`, `m_e`, `m_p`, `N_A`,
plus `π`, `τ`, `φ`, `ℏ`, `α`.

```bash
nvs run examples/port29_quantum_lowlevel.ns
```

### Self-hosting subset (2.3–2.5)

`stdlib/selfhost/` contains NvS-written tools for a documented
**mini-language subset** (integers, `+ - * / %`, comparisons, parentheses,
`let`/`print`/`if`/`while`):

- `mini_eval.ns` — `mini_eval(src)` interprets the subset (a real
  tokenizer/parser/evaluator in ~450 lines of NvS, not a stub).
- `emit_go.ns` — `emit_go_program(src)` emits equivalent Go source
  (verified: the output compiles and runs under the Go toolchain).
- `bootstrap.ns`, `math_mini.ns` — supporting modules.

Subset means subset: full NvS (functions-as-values, records, pattern
matching, concurrency, …) is out of scope and rejected or left as a
comment, never faked.

```bash
nvs run examples/selfhost_demo.ns
```

### Experimental bytecode VM (2.8.0)

`nvs bc <file>` compiles a **small subset** (numeric/string literals,
arithmetic, comparisons, `let`/`const`, assignment, `if`/`else`, `print`)
to bytecode and runs it on a real stack VM (`internal/bytecode`).
`--disasm` prints the opcodes. Anything outside the subset is a loud
compile error naming the construct. This is an experiment, not a
replacement for the interpreter.

```bash
nvs bc 'print 6 * 7'
nvs bc examples/port29_quantum_lowlevel.ns   # fails honestly: subset only
```

### Nave workflow runner (2.9.0)

`nvs nave <file.nave` (or `nvs <file>.nave` directly) executes JSON
workflow documents: `log`, `set`, `answer`, non-interactive `input`,
`polyglot_eval` (Python/JS), `http_get`, `file_read`/`file_write`, minimal
`if`/`try`, `assert_eq`, and basic `native_op` arithmetic (`add`, `sub`,
`mul`, `div`, `eq`). `nasm_exec` and `component_call` are logged as
explicit stubs, not faked.

```bash
nvs nave examples/nave/hello.nave
```

### Interpreter fixes made during reconciliation

- `and`/`or` now bind looser than `==` (new `ANDOR` precedence level):
  `c == "+" or c == "-"` is `(c == "+") or (c == "-")`.
- String `<`, `>`, `<=`, `>=` comparisons (lexicographic).
- A `while`/`for` loop whose final iteration hit `continue` no longer leaks
  the `Continue` signal object as the loop's value (it yields `NULL`).

## Not full ports (by design)
- Static Hindley–Milner type inference
- Preemptive threading / shared-memory parallelism (the wave-6 model is
  cooperative message-passing; shared mutable state across tasks is a user
  bug, not a feature)
- Native GPU / USB drivers
- Full macro system / compiler backend
- Full YAML parser (`toml_parse` covers configs honestly; YAML's edge cases
  aren't worth a lying subset)

## Wave 11 — polyglot tooling + generator/map

**Polyglot** (see `docs/POLYGLOT.md` for the full protocol reference):

- `nvs exports <file.ns>` — static description of a file's public surface
  (functions with arity/params/defaults/return types, classes with methods,
  enums, constants) as JSON, without executing it.
- `nvs bindgen --to=python <file.ns> -o <module>.py` — generates a Python
  module exposing every top-level function as a callable, routed through
  the JSON bridge; NvS errors raise `NvSError`.
- `nvs bridge --once` — process a single request line and exit.
- Bridge requests may carry an `"id"`, echoed verbatim in the response.
- Tested live: Python ctypes (C ABI), Python bindgen round-trip, Python and
  Node.js bridge clients (`examples/wave11_node_bridge.mjs`).

**Language:**

- `for (x in gen())` — for-in now iterates generators (remaining values if
  partially consumed via `next()`); `break`/`continue`/labels work.
- `map(array, fn)` — the missing third of the map/filter/reduce trio.
- **Bug fix:** `yield` inside `while`/`for`/`if`/`try` blocks was silently
  swallowed — the function returned a non-generator value instead of
  collecting the yields. Yields are now collected through a per-call sink
  on the scope chain (nearest-sink rule keeps nested function calls
  collecting into their own generator).

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
