# NvS Standard Library

Pure-NvS counterparts to the core functions of popular languages —
Python (`itertools`, `functools`, `collections`, `statistics`, `re`),
JavaScript/TypeScript, Java, Go, Rust (`Option`/`Result`), and Ruby —
written in native NvS so other languages can read the implementation
directly. Every function carries `///` doc comments; run `nvs doc` on a
module to render them.

## Importing

```nvs
import "stdlib/iter.nvs"
print iter_chain([1, 2], [3, 4])   // [1, 2, 3, 4]
```

Resolution rules:

1. An absolute path is used as-is.
2. A relative path first resolves against the **importing file's directory**
   (so `import "../iter.nvs"` works from `stdlib/tests/test_iter.nvs`
   no matter where you run `nvs` from).
3. Otherwise it falls back to the process working directory
   (so `import "stdlib/iter.nvs"` works when run from the repo root).
4. Each file is evaluated at most once per process, into the importer's
   environment — imported declarations become globals of the importing script.
5. Modules are self-contained: a stdlib module never imports another stdlib
   module, so any single import is enough.

## Module catalog

### `stdlib/iter.nvs` — itertools-style iteration

Fills gaps around the builtins (`map`, `filter`, `reduce`, `zip`, `chunk`,
`take`, `drop`, `find`, `flatten`, `unique`, `group_by`, `enumerate`).

| Function | One-liner |
|---|---|
| `iter_chain(a, b)` | `iter_chain([1,2],[3,4])` → `[1,2,3,4]` (shallow concat) |
| `iter_zip_longest(a, b, fill)` | `iter_zip_longest([1,2,3],["a"],0)` → `[[1,"a"],[2,0],[3,0]]` |
| `iter_take_while(arr, pred)` | `iter_take_while([1,2,3,1], fn(x){return x<3})` → `[1,2]` |
| `iter_drop_while(arr, pred)` | `iter_drop_while([1,2,3,1], fn(x){return x<3})` → `[3,1]` |
| `iter_pairwise(arr)` | `iter_pairwise([1,2,3,4])` → `[[1,2],[2,3],[3,4]]` |
| `iter_cycle(arr, n)` | `iter_cycle([1,2],3)` → `[1,2,1,2,1,2]` (finite, explicit count) |
| `iter_repeat_n(x, n)` | `iter_repeat_n("x",3)` → `["x","x","x"]` |
| `iter_accumulate(arr, f)` | `iter_accumulate([1,2,3,4], fn(a,b){return a+b})` → `[1,3,6,10]` |
| `iter_flatten1(arr)` | `iter_flatten1([[1,2],[3],4])` → `[1,2,3,4]` (one level only) |
| `iter_flat_map(arr, f)` | `iter_flat_map([1,2], fn(x){return [x,x*10]})` → `[1,10,2,20]` |
| `iter_find_index(arr, pred)` | `iter_find_index([10,20,30], fn(x){return x>15})` → `1` (`-1` if none) |
| `iter_partition(arr, pred)` | `iter_partition([1,2,3,4], fn(x){return x%2==0})` → `[[2,4],[1,3]]` |
| `iter_interleave(a, b)` | `iter_interleave([1,2,3],["a","b"])` → `[1,"a",2,"b",3]` |

### `stdlib/functools.nvs` — higher-order helpers

Around the builtins `compose`, `curry`, `partial`.

| Function | One-liner |
|---|---|
| `fn_memoize(f)` | caches a unary function by JSON-encoded argument |
| `fn_once(f)` | `let o = fn_once(fn(){ return 42 }); o()` → runs `f` once |
| `fn_complement(pred)` | `fn_complement(fn(x){return x>0})` → "not positive" |
| `fn_juxt(fns, x)` | `fn_juxt([fn(x){return x+1}, fn(x){return x*10}], 5)` → `[6,50]` |
| `fn_constant(v)` | `fn_constant(99)()` → `99` |
| `fn_tap(v, f)` | runs `f(v)` for side effects, returns `v` |
| `fn_flip(f)` | `fn_flip(fn(a,b){return a-b})(10,3)` → `-7` |

`fn_memoize` only memoizes **unary** functions whose arguments are
JSON-encodable (NvS functions are not variadic).

### `stdlib/str.nvs` — string utilities

Around the builtins (`trim`, `upper`, `lower`, `split`, `join`, `lines`,
`starts_with`, `ends_with`, `contains`, `replace`, `repeat`, `substr`,
`pad_left`, `pad_right`, `index_of`, `count`).

| Function | One-liner |
|---|---|
| `str_reverse(s)` | `str_reverse("hello")` → `"olleh"` |
| `str_to_snake(s)` | `str_to_snake("camelCase")` → `"camel_case"` |
| `str_to_kebab(s)` | `str_to_kebab("camelCase")` → `"camel-case"` |
| `str_to_camel(s)` | `str_to_camel("snake_case")` → `"snakeCase"` |
| `str_capitalize(s)` | `str_capitalize("hELLO")` → `"Hello"` |
| `str_truncate(s, n, suffix?)` | `str_truncate("hello world", 8)` → `"hello..."` |
| `str_words(s)` | `str_words("  foo   bar ")` → `["foo","bar"]` |
| `str_trim_start(s)` / `str_trim_end(s)` | trim one side only |
| `str_template(tpl, vars)` | `str_template("Hi {n}!", {"n":"Nave"})` → `"Hi Nave!"` |
| `str_is_digit(s)` / `str_is_alpha(s)` | ASCII checks; `""` → `false` |

### `stdlib/result.nvs` — Option / Result

Rust-style tagged hashes: `{"_tag": "Some", "value": v}`,
`{"_tag": "None"}`, `{"_tag": "Ok", "value": v}`,
`{"_tag": "Err", "error": e}`.

| Function | One-liner |
|---|---|
| `res_some(v)` / `res_none()` / `res_ok(v)` / `res_err(e)` | constructors |
| `res_is_some(o)` / `res_is_none(o)` / `res_is_ok(o)` / `res_is_err(o)` | predicates (non-hash input → `false`) |
| `res_map(o, f)` | `res_map(res_some(2), fn(x){return x*3})` → `Some(6)`; passes `None`/`Err` through |
| `res_and_then(o, f)` | chains functions returning Option/Result |
| `res_unwrap_or(o, dflt)` | `res_unwrap_or(res_none(), 0)` → `0` |
| `res_expect(o, msg)` | value or `throw msg` |
| `res_map_err(o, f)` | maps the `Err` payload only |
| `res_ok_or(o, err)` | `res_ok_or(res_none(), "missing")` → `Err("missing")` |
| `res_transpose(o)` | `Some(Ok(v))` → `Ok(Some(v))`; `Some(Err(e))` → `Err(e)` |

### `stdlib/collections.nvs` — Counter / defaultdict / deque

Python `collections` counterparts.

| Function | One-liner |
|---|---|
| `coll_counter(arr)` | `coll_counter(["a","b","a"])` → counts hash |
| `coll_count_get(c, k)` | missing key → `0` (never throws) |
| `coll_most_common(c, n)` | `coll_most_common(c, 2)` → `[["a",3],["b",2]]`; `n<=0` → all |
| `coll_defaultdict(factory)` | factory called with no args for missing keys |
| `coll_dd_get(dd, k)` / `coll_dd_set(dd, k, v)` / `coll_dd_has(dd, k)` | accessors |
| `coll_deque()` | two-stack amortized deque |
| `coll_dq_push(d, x)` / `coll_dq_push_front(d, x)` | push back / front |
| `coll_dq_pop(d)` / `coll_dq_shift(d)` | pop back / front (throw when empty) |
| `coll_dq_len(d)` | element count |
| `coll_dq_to_array(d)` | front-to-back snapshot as a plain array |

Counters key values by JSON encoding; integers and floats with the same
encoding (e.g. `1` and `1.0`) share a bucket and report the first-seen
value. (This matches the `json` builtin's output.)

### `stdlib/mathx.nvs` — extra math

Around the builtins (`avg`, `sum`, `min`, `max`, `abs`, `floor`, `ceil`,
`round`, `sqrt`, `pow`, …) and `stdlib/math.ns` (`clamp`, `lerp`).

| Function | One-liner |
|---|---|
| `mx_median(arr)` | `mx_median([1,2,3,4])` → `2.5`; does not mutate input |
| `mx_mode(arr)` | most frequent value; ties → first to reach the max |
| `mx_variance(arr)` | population variance |
| `mx_stdev(arr)` | population standard deviation |
| `mx_gcd(a, b)` | `mx_gcd(48,18)` → `6` |
| `mx_lcm(a, b)` | `mx_lcm(4,6)` → `12` |
| `mx_is_prime(n)` | trial division; non-integers and `n<2` → `false` |
| `mx_factorial(n)` | `mx_factorial(5)` → `120` |
| `mx_normalize(x, a, b)` | maps `[a,b]` into `[0.0,1.0]`, clamping outside values |

Note: NvS integer division truncates (`7/2 == 3`); these use float
arithmetic where a fractional result is expected.

### `stdlib/re.nvs` — regex helpers

The builtins `regex_find` / `regex_match` / `regex_replace` take
`(pattern, string)`; this module offers subject-first order plus extras.

| Function | One-liner |
|---|---|
| `re_test(s, pat)` | `re_test("abc123", "[0-9]+")` → `true` |
| `re_find_all(s, pat)` | `re_find_all("a1b22", "[0-9]+")` → `["1","22"]` |
| `re_replace_all(s, pat, repl)` | `re_replace_all("a1b22", "[0-9]+", "#")` → `"a#b#"` |
| `re_escape(s)` | `re_escape("a.b+c")` → `"a\\.b\\+c"` |
| `re_split(s, pat)` | `re_split("foo bar  baz", " +")` → `["foo","bar","baz"]` |

`re_split` works by substituting matches with a sentinel that cannot occur
in the input, then splitting on it; a pattern that matches the sentinel
itself is a degenerate case.

## Testing

Each module has `stdlib/tests/test_<module>.nvs`, runnable from the repo root:

```sh
nvs stdlib/tests/test_iter.nvs   # prints PASS: test_iter
```

The Go suite `TestWave14StdlibSuites` (in `internal/eval/wave14_test.go`)
globs and evaluates every suite, failing on any parse/runtime error
(suites print `FAIL:` lines and `throw` on a failed check).

## Known quirks

- `sort_by` compares keys **lexicographically** (numbers sort as strings:
  `12` before `3`). The stdlib sorts numbers with `sort()` or in pure NvS
  where numeric order matters.
- `sort()` and `reverse()` mutate the array in place; the stdlib copies
  first when the caller's array must be preserved.
- `push()` mutates in place and also returns the array.
- Regex builtins take `(pattern, string)`; `regex_find` returns matched
  strings only (no capture groups / match objects).

## Not yet expressible

Honest gaps — these popular features cannot be built on today's builtins:

- **Infinite iterators / generators as values**: `iter_cycle` takes an
  explicit repetition count; true lazy infinite sequences are not possible
  (there is a `Generator` object for `for-in`, but stdlib functions cannot
  construct or compose them in pure NvS).
- **Capture groups**: `regex_find` returns whole matches only, so helpers
  like "find all groups" or named-group extraction cannot be written.
- **Multi-argument memoization**: NvS functions are not variadic, so
  `fn_memoize` is unary-only.
- **Unicode-aware case mapping**: `upper`/`lower` are used as-is; full
  Unicode case folding is not available.
- **Deep equality on cyclic structures**: `deep_equal` is assumed acyclic.
