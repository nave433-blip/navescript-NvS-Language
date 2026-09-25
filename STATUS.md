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
