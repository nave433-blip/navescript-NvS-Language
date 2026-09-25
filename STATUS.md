# Status — NvS 2.6.0

## Self-host stage 4
- [x] `import "path"` inside mini_eval (shared env/funcs)
- [x] Function bodies stored as token snapshots (calls work after import)
- [x] Line comments `//` + safe unicode skip in tokenizer
- [x] `stdlib/selfhost/math_mini.ns` import module
- [x] `examples/selfhost4.ns` → SELFHOST4 OK

## Ladder
| Stage | Features |
|-------|----------|
| 1 | arith, let, print, if |
| 2 | while, arrays, fn |
| 3 | maps, for-in, Go emit |
| 4 | **import**, robust fn bodies, comments |

```bash
nvs run examples/selfhost4.ns
```
