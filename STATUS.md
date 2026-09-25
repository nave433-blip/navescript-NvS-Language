# Status — NvS 2.6.0

## Self-host stage 4
- [x] `import "path.ns"` inside mini_eval (shared env/funcs)
- [x] `stdlib/selfhost/math_mini.ns` sample module
- [x] `examples/selfhost4.ns` → SELFHOST4 OK
- [x] Go emit + `go run` of emitted program
- [x] Python `nvs_mini.py`: maps, for-in, import

```bash
nvs run examples/selfhost4.ns
python3 bootstrap/nvs_mini.py 'import "stdlib/selfhost/math_mini.ns" print square(6)'
```
