# NvS Polyglot Guide — calling and reading NvS from other languages

NvS is designed to be *used* from other languages, not just written in.
There are four integration paths, in order of recommendation:

| Path | How | Best for |
|------|-----|----------|
| **JSON stdio bridge** (`nvs bridge`) | Spawn `nvs` as a subprocess; exchange line-delimited JSON | Any language with subprocess + JSON (Python, Node, Ruby, …). Tested: Python, Node.js |
| **`nvs bindgen`** | Generates a native client module from an `.ns` file, wired through the bridge | Python today (`--to=python`); the module embeds the NvS source and exposes each function as a callable |
| **C ABI** (`libnvs.so`) | `go build -buildmode=c-shared -o libnvs.so ./cbridge` → `nvs_eval` / `nvs_call` / `nvs_free` | Languages with C FFI. Tested: Python ctypes |
| **Transpiler** (`nvs transpile`) | Honest-subset NvS → JS/Python source | Shipping NvS logic as native target code where it fits the subset |

---

## 1. JSON bridge protocol ( normative )

Transport: the client's stdin/stdout connected to `nvs bridge`'s
stdin/stdout. **One JSON object per line in, one JSON envelope per line
out.** Responses are flushed after every line; the client must read
line-by-line.

### Requests

```jsonc
{"eval": "<nvs source>", "id": <any>}             // run code in the session env
{"call": "<name>", "args": [<json>...], "id": <any>}  // call a function by name
```

- `id` is optional and echoed verbatim in the response (`"id"` key is
  absent from the response when the request had none). Use it to match
  replies to requests.
- `args` may be omitted for zero-argument calls (defaults to `[]`).
- `call` resolves user-defined functions **and builtins** (`len`, `str`, …).
  Anything else is an error. Arguments are positional only.
- One session = one persistent interpreter: bindings defined by one
  `eval` are visible to later requests. Requests are serialized.

### Responses

```jsonc
{"ok": true, "result": <json>, "id": <any>}     // success
{"ok": false, "error": "<message>", "id": <any>} // failure
```

- `result` is the JSON encoding of the NvS value (see §2). `eval` of a
  program whose value is a bare statement yields `null`.
- **No silent nulls.** If the result value cannot be encoded (a function,
  class instance, channel, …), the response is `ok:false` with an error
  naming the NvS type, e.g.
  `cannot convert NvS FUNCTION to JSON (supported: int, float, string, bool, null, array, hash)`.
- Malformed lines (not a JSON object, trailing data after the object, or no
  known op) yield `{"ok":false,"error":"invalid request: …"}` — the session
  stays alive for the next line.

### Limits

- Maximum request line: 4 MiB (larger lines are a read error).
- A non-terminating `eval` blocks the session exactly like `nvs run` would.
- `nvs bridge --once` processes exactly one request line and exits (useful
  for one-shot scripting without managing a subprocess lifetime).

---

## 2. Value encoding

| NvS | JSON | Notes |
|-----|------|-------|
| int | number | decoded with `UseNumber`, so `42` stays an integer |
| float | number | |
| string | string | UTF-8 |
| bool | boolean | |
| null | null | |
| array | array | recursive |
| hash | object | keys must be string/int/bool; int/bool keys are rendered as `"42"` / `"true"`; a collision after rendering is an error |
| everything else | — | `ok:false` error naming the type |

Inbound (`args`, and `eval` has none): JSON `null`/bool/string/array/object
map to NvS null/bool/string/array/hash. Numbers arrive as integers when
written without fraction/exponent, floats otherwise. JSON object keys are
always NvS strings (one-way rendering).

---

## 3. C ABI (`cbridge/`)

```c
char* nvs_eval(const char* src);                 // eval in the global env
char* nvs_call(const char* func_name, const char* args_json);
void  nvs_free(char* s);                          // free a returned string
```

- One global interpreter per process; calls serialized on a mutex.
- Both `nvs_eval` and `nvs_call` return a freshly allocated JSON envelope
  string in the §1 response format. **The caller must pass it to `nvs_free`;
  never free it with the host allocator.**
- Tested live: Python ctypes (`examples/wave10_ctypes.py`) — persistence,
  error envelopes, `nvs_free` discipline. Other languages' FFI sketches in
  LANGUAGE.md are labeled by test status; treat untested ones as sketches.

---

## 4. `nvs exports` — machine-readable public surface

```bash
nvs exports calc.ns
```

Parses (without executing) and prints JSON describing the file's top-level
bindings — the contract foreign tooling builds on:

```jsonc
{
  "functions": [
    {"name": "add", "kind": "function", "arity": 2, "required": 2,
     "params": [{"name": "a", "required": true}, {"name": "b", "required": true}]},
    {"name": "greet", "kind": "function", "arity": 1, "required": 0,
     "params": [{"name": "name", "required": false, "default": "\"world\""}],
     "return_type": "string"}
  ],
  "classes": [{"name": "Dog", "methods": [{"name": "bark", "arity": 0, "params": []}]}],
  "enums":   [{"name": "Color", "members": ["Red", "Green"]}],
  "constants": [{"name": "PI", "value": "3.14", "const": true}]
}
```

- `kind` is `"function"` or `"generator"` (generators are eager: calling
  one returns its collected values as a list).
- `default` is the NvS source of the default expression; `type` /
  `return_type` are rendered annotations, both omitted when absent.
- Classes expose methods (name/arity/params); enums expose member names;
  constants expose the initializer source and whether it was `const`.

`nvs bindgen --to=python calc.ns -o calc.py` consumes this surface and
emits a Python module: each function becomes a method, the NvS source is
embedded (base64) and evaluated once over the bridge, NvS errors raise
`NvSError`. See the generated module's docstring for the value contract.

---

## 5. NvS grammar summary (for client implementers)

You do **not** need to parse NvS to write a bridge client — send source as
strings. This summary exists so third parties can build tooling (syntax
highlighters, the `exports` surface, transpilers) with a shared
understanding. The reference implementation is `internal/lexer` +
`internal/parser`; where this summary and the parser disagree, the parser
wins.

### Lexical

- Line comments `// …`; no block comments.
- Identifiers: `[A-Za-z_][A-Za-z0-9_]*`. Keywords: `fn let const if else
  for while in break continue return class new this extends interface
  implements enum record try catch finally throw match case default defer
  yield import from as and or not true false null self`.
- Numbers: decimal ints (`123`), floats (`1.5`, `1e3`). Strings:
  double-quoted with escapes, plus `"…{expr}…"` interpolation.
- Statements are separated by newlines and/or `;`. There is **no**
  automatic semicolon insertion — a statement continues onto the next line
  while the expression is incomplete.

### Core syntax (EBNF-ish)

```
program      := statement*
statement    := letStmt | constStmt | fnDecl | classDecl | enumDecl
              | recordDecl | interfaceDecl | importStmt | returnStmt
              | breakStmt | continueStmt | throwStmt | tryStmt
              | deferStmt | exprStmt | block
letStmt      := "let" IDENT ("=" expr)? (";"?)
constStmt    := "const" IDENT "=" expr
fnDecl       := "fn" IDENT "(" params ")" (":" type)? block
params       := (IDENT (":" type)? ("=" expr)? ("," …)*)?
classDecl    := "class" IDENT ("extends" IDENT)? "{" method* "}"
method       := "fn" IDENT "(" params ")" (":" type)? block
enumDecl     := "enum" IDENT "{" IDENT ("," IDENT)* "}"
recordDecl   := "record" IDENT "{" IDENT ("," IDENT)* "}"
expr         := ternary | assignment | pipeline | range | "match" …
literal      := NUMBER | STRING | "true" | "false" | "null"
              | "[" expr* "]" | "{" (expr ":" expr)* "}"
              | "(" expr ("," expr)+ ")"        // tuple
```

### Expressions of note

- **Pipelines:** `x |> f |> g(y)` desugars left to right.
- **Ranges:** `a..b` (inclusive int range).
- **Optional chaining:** `a?.b?.c`, `a?.(args)`.
- **Destructuring:** `let {x, y} = point; let [a, b] = pair;`
- **Spread:** `f(...args)`, `[...a, ...b]`, `{...m, k: v}`.
- **Named args:** `move(x: 1, y: 2)`; partial application / currying via
  builtins.
- **Match:** `match (v) { case <pattern> [if guard]: body; … default: body }`
  with literal, binding, array/hash, and type patterns.
- **Loops:** `while`, C-style `for`, `for (x in iterable)` over
  array/tuple/record-values/string-chars/hash-keys; `break`/`continue`
  with optional labels; `else` on loop exhaustion.
- **Errors:** `throw expr`, `try { } catch (e) { } finally { }`.
- **Types (runtime-checked annotations):** unions `int | string`,
  interfaces, `implements()`, generics-ish `Array<int>` in annotations.
- **Concurrency:** `spawn`, channels, `pmap` — cooperative tasks, not OS
  threads.
- **Quantum:** `Quantum(n)` state-vector simulator (`h`, `x`, `cnot`, …,
  `measure`, `probabilities`, `circuit`), plus the low-level
  `qubit`/`qgate`/`qmeasure` API.

### Evaluation model (what a client observes)

- Dynamic typing; integers are 64-bit, floats 64-bit.
- Truthiness: only `false` and `null` are falsy — `0`, `""`, `[]`, `{}` are
  all truthy.
- `and`/`or` bind looser than `==` (fixed during reconciliation).
- Integer `/` is truncating division; `%` follows it.
- Assignment to an undeclared name creates/updates a binding in the
  current scope (REPL/script-friendly); `const` bindings are immutable.
- Errors are values that unwind to the nearest `catch`; uncaught, they
  become the `{"ok":false,…}` envelope (bridge) or a non-zero exit
  (CLI) — never a crash dump.

---

## 6. Tested integration matrix

| Client | Path | Proof |
|--------|------|-------|
| Python | ctypes → C ABI | `examples/wave10_ctypes.py` (all assertions pass) |
| Python | generated module → bridge | `nvs bindgen` + live round-trip (ints, strings+defaults, lists, maps, `NvSError` on `throw`) |
| Python | raw bridge client | multi-call session: persistence, id echo, honest type errors |
| Node.js | bridge client | `examples/wave11_node_bridge.mjs` (all assertions pass) |
| JS/Python | transpiler output | `internal/transpile` tests run emitted code under node/python3 |
| Any | `nvs bridge --once` | one-shot request/response verified |

Untested sketches (in this tree, labeled as such): Node ffi-napi, Ruby
fiddle, Rust `extern "C"`, C# P/Invoke, Java JNA. They follow the same
`nvs_eval`/`nvs_call`/`nvs_free` contract in §3; contributions with live
tests welcome.
