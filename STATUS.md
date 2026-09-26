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
- [x] Honest gaps documented in LANGUAGE.md: `nvs check` is advisory only
      (no dataflow/narrowing, no generics, no cross-file analysis), no
      signature-variance analysis, generator return annotations unchecked,
      missing-arg `null` binding unchanged, `obj.method()` still needs
      instances (maps use `m["name"]()`), no `const` annotations, no
      interface bodies/`extends`

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

### Wave 7 — stdlib robbery (in progress)
- [x] **Additive only**: no existing builtin changed shape; `env` gained an optional default (`env(name, default)` — 1-arg form keeps `""`-when-unset); everything else is new
- [x] Datetime gaps: `now_iso()` (UTC RFC3339), `unixtime_ms()`, `date_format(ts, layout)`, `parse_date(str)` (RFC3339 / `2006-01-02[ 15:04:05]` / RFC1123/822 / Kitchen; zoneless = local time), `date_add(ts, n, unit)` (`s`/`m`/`h`/`d`/`w` + long names)
- [x] Fuller HTTP: `http_request(method, url, opts?)` → `{status, headers, body}` (headers name → array of values; unknown opts are errors); `http_get`/`http_post` return shapes untouched
- [x] Crypto gaps: `sha1`, `hmac_sha256` (md5/sha1 documented as fingerprinting hashes, not security primitives); `base64url_encode`/`base64url_decode` (raw URL-safe, no padding)
- [x] Honest subprocess: `exec(cmd, args...)` (argv, no shell) and `sh(cmd)` (`sh -c`) → `{code, stdout, stderr}`; non-zero exit is data, only failure-to-start is an error
- [x] Path gaps: `extname`, `abs_path` (`basename`/`dirname`/`join_path`/`exists` already existed)
- [x] `toml_parse`: documented pure-Go TOML *subset* (tables, dotted keys, strings/ints/floats/bools/single-line arrays, comments); multi-line strings/arrays, inline tables, datetimes, `[[array-of-tables]]` are loud errors
- [x] `gzip_compress`/`gzip_decompress`: lossless string round-trip (Go strings are byte-safe)
- [x] Deliberately skipped: YAML (genuinely hard — no lying subset), `uuid4()` (`uuid()` already v4), `file_exists` (`exists()` already covers it)
- [x] 44 new Go tests (httptest-backed for HTTP, known vectors for hashes); examples for every offline area

```bash
nvs run examples/wave7_datetime.ns
nvs run examples/wave7_crypto.ns
nvs run examples/wave7_subprocess.ns
nvs run examples/wave7_env_path.ns
nvs run examples/wave7_toml.ns
nvs run examples/wave7_gzip.ns
```

### Wave 8 — quantum robbery (in progress)
- [x] **LOCAL state-vector simulator, NOT quantum hardware** — the disclaimer is in LANGUAGE.md, the code comments, `QRegister.Inspect()`, and the circuit diagram header. Ideal/noise-free; gates are real matrix math on 2^n complex128 amplitudes (Qiskit-Aer-style); measurement collapse uses `crypto/rand`
- [x] `qalloc(n)` → register in |0…0⟩; **cap 24 qubits** (2^24 = 256MB state, O(2^n) gates) with a loud `simulator limit` error beyond it; `q_nqubits`
- [x] Gates (all return the register, chainable): `q_h`/`q_x`/`q_y`/`q_z`/`q_s`/`q_t`, `q_rx`/`q_ry`/`q_rz` (radians), `q_cnot`/`q_cz`/`q_swap`; out-of-range qubit index and same-qubit two-qubit gates are honest errors
- [x] **Little-endian convention** (qubit 0 = LSB), documented in LANGUAGE.md and wave8.go; `q_measure` → 0/1, `q_measure_all` → bit array in qubit order
- [x] Inspection: `q_probs` (deterministic 2^n probabilities), `q_state` (`[re, im]` pairs — NvS has no complex type), `q_circuit` (ASCII diagram + op legend), `q_reset` (state and diagram cleared)
- [x] `QuantumBackend` Go interface (ApplyGate/Measure/MeasureAll/Probabilities/Amplitudes/Reset/CircuitDiagram) with the local simulator as its implementation; `q_backend("local")` → `"local-simulator"`, anything else (e.g. `"azure"`) is a loud "not connected in this build" error — hardware is never faked
- [x] 24 new Go tests (gate matrices vs known states via `q_probs`/`q_state`, Bell state, error paths, circuit string, backend registry); measurement tests only assert forced outcomes / post-collapse shape — no flaky statistics
- [x] 5 examples, each run 5× with no flakes: Bell-pair correlation (asserts 01/10 never occur, not exact ratios), superposition probs, rotations, circuit display, measure collapse

```bash
nvs run examples/wave8_bell.ns
nvs run examples/wave8_superposition.ns
nvs run examples/wave8_rotations.ns
nvs run examples/wave8_circuit.ns
nvs run examples/wave8_measure.ns
```

### Wave 9 — tooling robbery (in progress)
- [x] `nvs fmt`: LEXICAL formatter (AST `String()` is lossy — try/match/defer
      don't round-trip — so no AST pretty-printer). Normalizes: 4-space indent
      by brace/paren/bracket depth, tabs→spaces outside strings, trailing
      whitespace removed, blank lines collapsed (max 1, none leading), exactly
      one trailing newline. Leaves alone: string contents (incl. `${}`
      interpolation — braces there never indent), comment contents, in-line
      spacing. Tested idempotent + behavior-preserving (every `examples/*.ns`
      runs byte-identical before/after fmt, modulo pre-existing nondeterminism:
      map/set order, random/uuid/now/timeit, quantum measurement).
      `nvs fmt --check` for CI (exit 1 + lists files that would change).
- [x] `nvs lint`: four SOUND rules, no false positives on normal code —
      `unused-binding` (warning; fn params excluded, named fns exempt, any
      mention incl. assignment counts as a use), `shadow-builtin` (warning;
      Go builtins only, not prelude.ns), `unreachable-code` (warning; only
      direct statements after return/break/continue/throw in a block),
      `null-comparison` (style; suggests `is_null()`). `file:line: severity
      rule: message`; exit 0 clean / 1 findings; `--json` supported.
      NOT claimed: dataflow, cross-file analysis, type inference.
- [x] `nvs doc`: doc-comment extractor (honestly small, not rustdoc).
      Prefers `///` lines, falls back to contiguous `//`; typed signatures
      (`fn add(a: int, b: int): int`); covers top-level fn/class/record/
      interface/const/enum plus class and interface methods (`###`
      subsections); module-level `# <file>` heading; `nvs doc --out <dir>`
      writes one `.md` per input; exits nonzero on parse/read/write errors.
      No cross-links, no index page.
- [x] 24 new Go tests (formatter idempotency + behavior preservation incl.
      real examples, per-rule lint positive/negative cases, doc extraction);
      `eval.BuiltinNames()` exported for the shadow-builtin rule.
- [x] Examples: `wave9_fmt_bad.ns` (deliberately badly formatted),
      `wave9_lint.ns` (four findings, still `nvs run`-clean),
      `wave9_doc.ns` (documented decls, `nvs run`-clean).

```bash
nvs fmt --check examples/wave9_fmt_bad.ns   # exits 1, lists the file
nvs lint examples/wave9_lint.ns             # exits 1, names the four rules
nvs doc examples/wave9_doc.ns
```

### Wave 10 — polyglot interop (in progress)
- [x] **C ABI** (`cbridge/`, cgo): `nvs_eval`, `nvs_call`, `nvs_free` —
      one persistent interpreter, serialized calls, caller-owned return
      strings (must `nvs_free`). Shared JSON envelope
      `{"ok":true,"result":…}` / `{"ok":false,"error":"…"}`. Convertible:
      int/float/string/bool/null/array/hash (keys string/int/bool;
      collisions are errors); anything else is `ok:false` naming the NvS
      type. Verified: `go build -buildmode=c-shared` → `nm -D` shows all
      three symbols; `examples/wave10_ctypes.py` passes (ctypes,
      `c_void_p` signatures, every return freed).
- [x] **JSON stdio bridge** (`nvs bridge`): `{"eval":…}` /
      `{"call": name, "args": […]}` per line → one envelope per line;
      persistent session, malformed lines → honest `invalid request`
      envelopes, trailing data rejected. `examples/wave10_bridge_client.py`
      drives it for real (see `wave10_bridge_client.expected`).
- [x] **Transpiler** (`nvs transpile --to=js|python`, `internal/transpile`):
      HONEST SUBSET — runtime prelude (`__div`/`__str`/`__truthy`/…)
      preserves NvS integer division, NvS truthiness, Inspect coercion,
      Go %g floats, slice clamping; implicit trailing returns, closures
      with correct Python `nonlocal`/`global`, continue-with-post in
      desugared C-for. Anything outside the subset is an `UnsupportedError`
      naming the construct. Verified: emitted JS run under node and
      emitted Python under python3 match `nvs run` output
      (`examples/wave10_transpile_demo.ns` + `.expected`, plus Go tests
      comparing live).
- [x] **WASM**: `GOOS=js GOARCH=wasm go build -o nvs.wasm ./cmd/nvs/`
      succeeds (pure Go, no cgo). Build-only — browser/Node execution not
      tested, not claimed.
- [x] New Go tests: `internal/polyglot` (JSON round-trips, key collisions,
      honest type errors, trailing-data rejection), `internal/bridge`
      (persistence, builtins, malformed lines, integer preservation),
      `internal/transpile` (subset accept/reject, live-output comparison).
      `go build ./...`, `go vet ./...` clean; `go test ./...`: 248 pass,
      0 fail.
- [x] Backward compat: 64/69 pre-Wave-10 `examples/*.ns` byte-identical
      (stdout+stderr+exit) vs `b23b1fb` baseline binary; the 5 diffs are
      all excluded nondeterminism (random/uuid, timeit, quantum shots, Go
      map order, corrections-DB state). `claimed.ns`/`plugins.ns` still
      exit 1 (missing Ruby), `security_research.ns` still exits 1 —
      identical.

```bash
go build -buildmode=c-shared -o libnvs.so ./cbridge
python3 examples/wave10_ctypes.py ./libnvs.so
nvs transpile --to=js examples/wave10_transpile_demo.ns | node
GOOS=js GOARCH=wasm go build -o nvs.wasm ./cmd/nvs/
```

### Reconciliation — 2.2–2.9 track ports (2026-09-25)

Ported the genuinely working pieces of the diverged 2.2–2.9 track onto the
tree-walking interpreter (which remains the primary engine), in
`/tmp/nvs-reconcile` on branch `reconcile/robbery-onto-latest`:

- [x] **2.2.0**: low-level quantum API (`qubit`, `qzero`, `qgate`,
      `qmeasure`, `qprob`, `qtensor`, `qnormalize`, `qinner`),
      `physics_const`, `self_eval`, physics/Greek globals
      (`internal/quantum`, `internal/eval/port29.go`, 8 Go tests,
      `examples/port29_quantum_lowlevel.ns`).
- [x] **2.9.0**: Nave workflow runner (`internal/nave`, `nvs nave`,
      direct `.nave` execution, 6 Go tests, `examples/nave/hello.nave`
      now answers 42 via honest `native_op` arithmetic;
      `nasm_exec`/`component_call` remain explicit stubs).
- [x] **2.8.0**: experimental bytecode compiler + stack VM
      (`internal/bytecode`, `nvs bc [--disasm]`; loud errors outside the
      documented subset: arithmetic incl. truncating int division, string
      concat + lexicographic comparison, let/const, if/else, while and
      C-style for loops with break/continue, print(...), len(), arrays;
      explicit division/modulo-by-zero runtime errors).
- [x] **2.3–2.5**: NvS-written self-hosting subset (`stdlib/selfhost/`:
      `mini_eval.ns`, `emit_go.ns`, `bootstrap.ns`, `math_mini.ns`;
      `examples/selfhost_demo.ns`; emitted Go verified to compile/run).
- [x] Interpreter fixes: `and`/`or` precedence (new `ANDOR` level),
      string `<`/`>`/`<=`/`>=`, `continue`-signal leak in `while`/`for`
      (3 regression tests in `internal/eval/reconcile_test.go`).
- [x] Deliberately excluded: malformed legacy `internal/vm`, incomplete
      `bindings/go`, cobra-based `nvm` (nothing working to port).

```bash
nvs run examples/port29_quantum_lowlevel.ns
nvs run examples/selfhost_demo.ns
nvs nave examples/nave/hello.nave
nvs bc 'print 6 * 7' --disasm
```

### Wave 13 — types, docs, bytecode (in progress)
- [x] **Static checker** (`internal/checker`, `nvs check <files...>`):
      gradual typing — unannotated code never errors; checks annotated
      lets, reassignment, param/return annotations, call arity
      (defaults-aware) and argument types, unknown annotation names
      (forward references resolve), unions, class/interface structural
      satisfaction mirroring runtime rules, class hierarchy, enum members,
      `len` arity/return. `file:line` diagnostics on stderr; exit 0 clean,
      1 on errors. Advisory: the runtime still enforces types.
- [x] **Doc generator** (`nvs doc [--out <dir>]`): see Wave 9 entry above —
      stub label removed.
- [x] **Bytecode VM expansion** (`internal/bytecode`): while + C-style for
      with break/continue (loop-context patch stack), string `+` concat and
      lexicographic `<`/`>`/`<=`/`>=`, truncating int `/`, explicit
      division/modulo-by-zero errors, `len()` over strings/arrays, array
      literals, space-joined multi-arg `print`; `TestBytecodeMatchesTreeWalker`
      requires VM/tree-walker output identity on 11 programs. Still honest:
      no functions (beyond print/len), no for-in, no tuples/hashes/records/
      classes, everything else fails loudly at compile time.
- [x] Interpreter fix: `1 % 0` no longer panics (`ERROR: modulo by zero`).

```bash
nvs check examples/wave5_annotations.ns   # flags the intentional violations
nvs doc --out docs/ examples/wave9_doc.ns
nvs bc 'for (let i = 0; i < 3; i = i + 1) { print i }'
```

## Wave 16 — NT/Unix kernel compatibility

- [x] **Build matrix**: `go build ./...` clean on Linux; `GOOS=windows go build
      ./...` and `GOOS=darwin go build ./...` clean. A commit breaking any
      target is a bug.
- [x] **Platform shell**: `sh()` and `system()` went through a build-tagged
      `shellCommand` helper (`internal/eval/shell_unix.go`: `sh -c`;
      `internal/eval/shell_windows.go`: `cmd /c`). Same NvS code runs on
      both kernels. Tests: `TestShellCommandRuns`, `TestShellNameMatchesRuntime`.
- [x] **Polyglot runners**: `python3` → `python` fallback (`polyglot.PythonBinary()`,
      used by the `python` builtin, the polyglot runner, and the syntax
      validator); rustc temp binary gets `.exe` on Windows.
- [x] **OS introspection builtins**: `os_name()`, `os_kernel()` (`"nt"`/`"unix"`),
      `os_sep()`, `os_eol()`, `os_shell()` — tested by `TestOsBuiltins`.
- [x] **`stdlib/os.nvs`**: pure-NvS portability layer — `os_is_windows/unix`,
      `os_exe_suffix`, `os_path_norm[_with]`, `os_is_abs[_with]`,
      `os_path_join[_with]`; the `_with(sep, …)` forms take an explicit
      separator so Windows path behavior is unit-tested on Linux
      (`stdlib/tests/test_os.nvs`, all pass).
- [x] **CRLF audit**: `lines()` normalizes `\r\n`; the lexer skips `\r`;
      `@nvs` extract markers match after `TrimSpace`; `read_file`/`write_file`
      preserve bytes exactly (Go does no text-mode translation).
- [x] **Docs**: `docs/KERNELS.md` (support matrix, NT differences table, `os`
      module reference, honest unverified list, porting notes incl. HybridOS
      groundwork); `docs/STDLIB.md` catalog entry; `LANGUAGE.md` subprocess
      row notes the platform shell.
- [ ] **Honestly unverified**: no Windows/macOS hardware in this VM — NT code
      paths are compile-verified + unit-tested where platform-independent,
      but no NvS program has run on a real NT kernel yet.

```bash
GOOS=windows go build ./...   # must stay green
GOOS=darwin go build ./...    # must stay green
nvs -e 'print os_kernel() + " " + os_shell()'
```
