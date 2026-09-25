# Status — NvS 2.5.0

## Self-host stage 3
- [x] Maps `{a: 1, b: 2}` in mini_eval
- [x] `for (x in arr) { ... }`
- [x] fn + for-in composition
- [x] `emit_go_program` — emit runnable Go from let/print/arith subset
- [x] `examples/selfhost3.ns` → SELFHOST3 OK

## Ladder
| Stage | Capability |
|-------|------------|
| 1 | arith, let, print, if |
| 2 | while, arrays, fn/return |
| 3 | maps, for-in, Go emit |
| Host | full Go tree-walker + polyglot builders |

```bash
nvs run examples/selfhost3.ns
python3 bootstrap/nvs_mini.py 'let s = 0 for (x in [1,2,3]) { s = s + x } print s'
```
