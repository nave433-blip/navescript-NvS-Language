# FOLDING.md — folding foreign code into NvS (Wave 15)

NvS can *fold* code from other languages into itself: convert foreign source
to NvS (`nvs import`), and run NvS blocks embedded in foreign files
(`nvs extract`). The guiding rule is **honest subsets**: each importer
converts exactly what it can convert *correctly*, and fails loudly —
naming the construct and the line number — for everything else. Silent
miscompilation is treated as a bug.

## nvs import

```
nvs import --from=python <file.py>     # Python -> NvS (stdout)
nvs import --from=js <file.js>         # JavaScript -> NvS (stdout)
```

Both importers are hand-written (no third-party parser dependencies) and
emit plain NvS source you can read, edit, and run with `nvs`.

Errors look like this:

```
import error: unsupported python construct: class definition (line 12) — Python classes are outside the importable subset
import error: invalid js syntax (line 4): expected ";", found OP(})
```

### Python subset (exact)

**Supported:**

- Assignments: `x = 1`, chained `a = b = 0`, tuple assignment and swaps
  (`a, b = b, a` via temporaries), augmented assignment (`+=`, `-=`,
  `*=`, `/=`, `%=`, `//=`, `**=`, `&=`, `|=`, `^=`, `<<=`, `>>=`).
- Arithmetic: `+ - * / // % **` (`//` maps to `/` — see divergences),
  comparisons, `and`/`or`/`not`, `in`/`not in` (lists, strings, and dict
  *literals* via `keys()`), parentheses, unary minus/not.
- Strings: concatenation with `+`, repetition with `*` (`"ab" * 3` →
  `repeat("ab", 3)`), simple f-strings (`f"n={n}"` → `"n=${n}"` — only
  `{name}` placeholders, no format specs or nested quotes).
- Lists: literals, indexing, basic slices (`a[1:3]`, `a[1:]`, `a[:2]`,
  `a[:]`, negative indices), `+` concatenation with a list literal on
  either side (`xs + [1]` → `[...xs, 1]`, `[0] + ys` → `[0, ...ys]`),
  `append`, `len()`, iteration.
- Dicts: literals, indexing, `keys()`/`values()`/`items()`, `len()`,
  `in` on dict literals.
- Functions: `def` with positional and default args, `return` (bare
  `return` → `return null`), calls, lambdas (`lambda x: x*2` → `fn(x) ...`).
- Control flow: `if`/`elif`/`else`, `while` (+ `else`), `for x in ...`
  (lists, strings, dicts, `range()`, `enumerate()`), `break`/`continue`.
- `print(...)`, `len(...)`, `range(...)` (1–3 args; NvS `range` matches
  Python semantics).
- `assert cond`, `raise <expr>` (→ `throw(...)`), `del x[i]`, `pass`.

**Loudly rejected** (each names the construct and line):

- `class` definitions, `import`/`from` statements, decorators,
  `global`/`nonlocal`, `with` blocks, `try`/`except`/`finally`,
  `yield` (generators), comprehensions (list/dict/set),
  list repetition (`[1] * 3`), chained comparisons (`a < b < c`),
  `**kwargs`/`*args` beyond simple collection, keyword arguments in calls.

### JavaScript subset (exact)

**Supported:**

- `var`/`let`/`const` declarations (with and without initializers),
  assignment and augmented assignment (`+=`, `-=`, `*=`, `/=`, `%=`,
  `**=`, `&&=`, `||=`, `??=`), `++`/`--` (prefix and postfix).
- `function` declarations (defaults, rest params), arrow functions
  (expression and block bodies), function expressions, calls.
- `if`/`else`, `while`, C-style `for`, `for...of`, `break`/`continue`.
- Arrays: literals, spread, indexing, `.push(x)`, `.pop()`, `.join()`,
  `.split()` (on strings), `.slice()`, `.indexOf()`, `.includes()`,
  `.map()`, `.filter()`, `.length` → `len(...)`.
- Objects: literals (incl. shorthand and spread), `o.a` / `o["a"]` access.
- Strings: `.length`, `.toUpperCase()`, `.toLowerCase()`, `.trim()`,
  template literals with `${...}` interpolation.
- Operators: arithmetic, comparisons, `===`/`!==` (→ `==`/`!=`),
  `&&`/`||`/`!`, `??` (NvS supports `??`), ternary `?:`.
- `console.log(...)` → `print(...)`.

**Loudly rejected** (each names the construct and line):

- `class` definitions/expressions, `import`/`export`, `try`/`catch`,
  `switch`, `throw`, `new`, generator functions (`function*`),
  `async`/`await`, regex literals, optional chaining (`?.`),
  labeled break/continue, `do-while`, and any method call outside the
  supported list above.

### Known divergences (documented, not bugs)

- **Array printing:** Node prints long arrays multi-line; NvS prints
  single-line. Values are identical. Python's list repr also quotes
  contained strings (`['a']`) where NvS prints them bare (`[a]`).
- **`//` floor division:** NvS `/` truncates toward zero; Python `//`
  floors. Identical for non-negative operands; differs for negatives.
- **Truthiness:** NvS truthiness rules apply to converted conditions
  (e.g. `if []:` / `if "":`); check edge cases with empty values.
- **`+` on two non-literal arrays** (Python): emits `+`, which NvS
  rejects at *runtime*. Prefer the supported literal forms, which fold
  to spread syntax.
- **JS `+` with arrays/objects:** when a string is present, the
  other side is wrapped in `str()` to match JS coercion. Scalars and
  strings match exactly; arrays/objects stringify differently
  (`[1,2] + ""` → `"1,2"` in JS, `"[1, 2]"` in NvS).
- **JS `+` on arrays** is string coercion in JS; NvS rejects it at
  runtime. Write `.join()` or spread instead.

### Verifying a conversion

```sh
python3 app.py > orig.out
nvs import --from=python app.py > app.nvs
nvs app.nvs > nvs.out
diff orig.out nvs.out
```

The Wave 15 test suite (`internal/fold/fold_test.go`) asserts exact
emitted NvS for supported constructs and asserts loud `UnsupportedError`s
for the rejected ones.

## nvs extract

Run NvS blocks embedded in any foreign source file:

```
nvs extract <file>
```

Block markers are whole-line comments:

| Host language | Open | Close |
|---|---|---|
| Python | `# @nvs` | `# @endnvs` |
| JS / TS / C / Rust / Go | `// @nvs` | `// @endnvs` |
| (also accepted) | `/* @nvs */` | `/* @endnvs */` |

- **All blocks share one interpreter session**: bindings defined in block 1
  are visible in block 2, etc.
- Each block prints a header (`# --- block N (file:a-b) ---`) and a JSON
  result envelope (`{"ok":true,"result":...}`), the same shape `nvs bridge`
  uses.
- **Unbalanced markers are a loud error** naming the line: a closer without
  an opener, a nested opener, or an opener never closed.

Example (`app.py`):

```python
# @nvs
let rate = 1.5
fn total(n) { return n * rate }
# @endnvs
print("python side")
# @nvs
print(total(10))
# @endnvs
```

`nvs extract app.py` runs both blocks in one session, so the second block
sees `total`.
