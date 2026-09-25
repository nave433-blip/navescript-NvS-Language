# Status — NvS 2.4.0

## Self-hosting stage 2
- [x] `mini_eval` supports: while, arrays, index, fn/return/calls, if/else, &&/||
- [x] `examples/selfhost2.ns` → SELFHOST2 OK
- [x] Python twin interpreter: `bootstrap/nvs_mini.py`
- [x] `examples/polyglot_bootstrap.ns` → multi-path OK
- [x] Host: `self_eval`, `build_nvs`, `nvs selfhost`, `nvs bootstrap`

## Build from other languages
```bash
bash bootstrap/build_nvs.sh
python3 bootstrap/build_nvs.py
python3 bootstrap/nvs_mini.py 'let x = 6*7 print x'
node bootstrap/build_nvs.js
ruby bootstrap/build_nvs.rb
```

## Next
- Expand mini_eval: maps, for-in, import
- Emit real Go from NvS subset AST
