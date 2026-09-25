# Status — NvS 2.1.0

Broad language-feature expansion based on established languages.

### Newly solidified
- [x] Hex / binary literals (`0x`, `0b`)
- [x] Array & string slices `a[i:j]`, `a[i:]`
- [x] `enum`
- [x] `try/catch/finally`
- [x] `defer` (function-scoped LIFO)
- [x] `typeof` / `isinstance` / `deep_equal`
- [x] Math: sin/cos/tan/exp/round
- [x] Sets: set / set_add / set_has
- [x] `print` as callable (for defer/HOF)

```bash
nvs run examples/language_features.ns
```

### Wave 1 — ergonomics robbery (in progress)
- [x] Destructuring: `let [a, b]`, `let [a, ...rest]`, `let {x, y}`, `let {k: v}` (+ `const`)
- [x] Spread/rest: `[...a]`, `f(...args)`, `{...m, k: 1}` (runtime error on non-array/map)
- [x] Optional chaining `?.` with null short-circuit (no calls made on null links)
- [x] String interpolation `"${expr}"` (balanced braces, nesting works)
- [x] Pipeline `|>` desugared to nested calls
- [x] Ranges `1..10` / `1...5` (int endpoints; `for (x in 1..5)` works)
- [x] `for...else` / `while...else` (else runs only when the loop didn't `break`)
- [x] Labeled `break`/`continue` (`outer: for ...` / `break outer`)

```bash
nvs run examples/wave1_destructure.ns
nvs run examples/wave1_spread.ns
nvs run examples/wave1_optional_chain.ns
nvs run examples/wave1_interpolation.ns
nvs run examples/wave1_pipeline.ns
nvs run examples/wave1_ranges.ns
nvs run examples/wave1_for_else.ns
nvs run examples/wave1_labels.ns
```

### Wave 2 — functions robbery (in progress)
- [x] Named arguments `f(x: 1, y: 2)` — positionals first, then named (parse error otherwise); unknown/duplicate/missing-required are runtime errors; defaults fill the rest; works for plain calls, `obj.m(x: 1)`, `new C(x: 1)`, and `?.` chains; builtins honestly reject named args
- [x] `partial(f, args...)` — real closure with leading args pre-bound, positional-only; partial-of-partial composes
- [x] `curry(f)` — collects positional args until required arity (params minus defaulted) is met; refuses builtins/non-functions rather than guessing arity; extras pass through like normal calls
- [x] `compose(f, g, ...)` — right-to-left `f(g(h(x)))`, variadic; composes with partials/curried functions (supersedes the old 2-arg prelude `compose`, removed from `stdlib/prelude.ns`)
- [x] `?.` call chains — extended coverage with args and named args (`examples/wave2_chains.ns`)

```bash
nvs run examples/wave2_named_args.ns
nvs run examples/wave2_partial_curry_compose.ns
nvs run examples/wave2_chains.ns
```

### Wave 3 — data robbery (in progress)
- [x] Tuples `(1, 2)`, `(x,)`, `()` — immutable; `(x)` stays grouping; index/slice/`in`/`len`/for-in; value `==`; `tuple(arr)`; destructurable; all mutations are loud runtime errors
- [x] Records `record Point(x, y)` — immutable struct types; `Point(1, 2)` / `Point(x: 1, y: 2)`; `.x` access; field-by-field `==`; `Point(x=1, y=2)` printing; `let {x, y}` destructuring; field assignment is a runtime error
- [x] `freeze(obj)` deep-freezes arrays/hashes in place; `is_frozen(x)`; `thaw(x)` deep mutable copy (tuple→array, record→hash); every mutation path (`push`/`pop`/`sort`/`reverse`/`delete`/`set_add`/`set_remove`, index/member assign, `deep_set`) refuses frozen values with a named error
- [x] `deep_get(obj, "a.b.0.c")` / `deep_get(obj, "a.b", default)`; `deep_set(obj, "a.b.c", v)` creates intermediate hashes lodash-style, works through existing array indices (no sparse auto-vivification), returns the root
- [x] Set/map builtins: `set_union`/`set_intersect`/`set_diff`/`set_len`/`set_to_array`/`set_remove`; `len()` handles sets, tuples, records; `map_merge` (later-wins), `map_pick`/`map_omit`, `invert`

```bash
nvs run examples/wave3_tuples.ns
nvs run examples/wave3_records.ns
nvs run examples/wave3_freeze.ns
nvs run examples/wave3_deep.ns
nvs run examples/wave3_setmap.ns
```

### Wave 4 — pattern-matching robbery (in progress)
- [x] Guards: `case x if x > 0:` / `case [a, b] if a != b:` — falsy guard falls through to next arm (not an error); guard sees arm bindings; failed guards leak nothing
- [x] Destructuring patterns (refutable, Rust-like): `case [a, b]:`, `case [h, ...t]:`, `case {x, y}:`, `case {k: v}:`, `case (a, b):` (tuple), `case Point(x, y):` / `case Point(x: a):` (record, positional/named/literal fields); shape mismatch falls through (differs from wave-1 `let`, which binds missing→null)
- [x] Expression form: `let r = match (v) { case 1: "one"; default: "other" }` — arm bodies yield last value; single-statement arms/defaults without braces fixed (were a parse error); scrutinee parens optional (`match x { ... }`); `switch` alias keeps working
- [x] Bare identifier always binds (`case n:`), `_` is a true wildcard (both previously errored on undefined names); bindings scoped to the arm, no leak into enclosing env

```bash
nvs run examples/wave4_guards.ns
nvs run examples/wave4_patterns.ns
nvs run examples/wave4_expr.ns
```

### Wave 5 — types robbery (in progress)
- [x] **Runtime contracts, not static analysis** — every annotation is enforced while the program runs (call/bind/return); there is no static type checker, by design
- [x] Param annotations `fn add(a: int, b: int) { ... }` (positional + named args), return annotations `fn f(): int { ... }` (thrown errors bypass the check — an error is not a return value), defaults checked when used (`fn f(x: int = "bad")` errors only on `f()`)
- [x] Unions `int | string` (union `|` *only* in annotation position; elsewhere still bitwise-or), `T?` = `T | null`, `null` accepted in unions
- [x] `let x: int = 5` — initial binding checked, type remembered and enforced on later reassignment (incl. from closures); `let x: int | string`; `any` skips checking
- [x] Vocabulary: `int`/`integer`, `float`, `number`, `string`/`str`, `bool`/`boolean`, `null`, `array`/`list`, `hash`/`map`/`dict`, `tuple`, `function`/`fn`/`callable`, `record`, `any`; class names (subclasses count), interface names (`implements`), record names (identity) resolve lexically at call time; unknown names are loud runtime errors
- [x] `interface Shape { area(): number }` + `implements()`/`assert_implements()` — structural, presence + callable + knowable-arity; works on instances (incl. inherited methods), hashes of functions, record fields; NOT verified: parameter/return signature variance
- [x] Interfaces as parameter annotations (`fn draw(s: Shape)`); class/record names as annotations
- [x] Guards: `is_int`/`is_float`/`is_number`/`is_string`/`is_bool`/`is_array`/`is_hash`/`is_tuple`/`is_function` (`is_null` pre-existed); `type_of()` → lowercase names, `type()`/`typeof()` unchanged
- [x] Honest gaps documented in LANGUAGE.md: no static checker, no signature-variance analysis, generator return annotations unchecked, missing-arg `null` binding unchanged, `obj.method()` still needs instances (maps use `m["name"]()`), no `const` annotations, no interface bodies/`extends`

```bash
nvs run examples/wave5_annotations.ns
nvs run examples/wave5_unions.ns
nvs run examples/wave5_interfaces.ns
nvs run examples/wave5_guards.ns
```

### Wave 6 — concurrency robbery (in progress)
- [x] **Cooperative message-passing model** (Lua/Erlang/JS-workers lineage; Go goroutines underneath): tasks switch only at yield points (`sleep`, `task_yield`, blocking `send`/`recv`, `join`) — never preemptively in any way user code can observe
- [x] **No shared mutable state**: channel sends and `spawn` args are deep-copied (arrays/hashes/tuples/records, frozen-ness preserved); channel/task handles pass by reference, never copied; functions/builtins/instances/generators pass by reference (documented in LANGUAGE.md, in bold terms)
- [x] `Environment` store mutex-guarded (`sync.RWMutex`): concurrent top-level `let`/assignment can't corrupt the map (last-writer-wins, documented as "don't do that"); `go test -race` clean
- [x] `spawn(fn, args...)` → task handle; `join(task)` returns the value or re-raises the task's error (catchable); `task_status(task)` → `"running"`/`"done"`; `task_yield()` (named so because `yield` is the generator keyword)
- [x] `chan()` / `chan(n)`; blocking `send`/`recv` (rendezvous if unbuffered); non-blocking `try_send`→bool / `try_recv`→`[true,v]`/`[false,null]`; `close(ch)`; `recv`→`null` once closed+drained; `send` on closed / double-close are loud errors
- [x] `sleep` extended honestly: integer = milliseconds (**unchanged**), float = seconds (new); `join(array, sep)` string form preserved exactly (1-arg task form dispatches on type)
- [x] `pmap(fn, array)` — one task per element, order-preserving, errors re-raise
- [x] `nvs run` drains all spawned tasks before exiting (no orphaned output); a program that raises still exits 1 immediately

```bash
nvs run examples/wave6_pingpong.ns
nvs run examples/wave6_pmap.ns
nvs run examples/wave6_tryrecv.ns
nvs run examples/wave6_sleep_yield.ns
nvs run examples/wave6_join_values.ns
nvs run examples/wave6_closed_channel.ns
```
