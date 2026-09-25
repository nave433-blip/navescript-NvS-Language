# Status — NvS 2.3.0 (self-hosting bootstrap)

## Self-hosting
- [x] `stdlib/selfhost/mini_eval.ns` — pure NvS tokenizer + evaluator (subset)
- [x] `examples/selfhost.ns` — NvS interpreting NvS → SELFHOST OK
- [x] `self_eval(src)` builtin — full host re-entry interpreter
- [x] `nvs selfhost <code|file>` CLI
- [x] `nvs bootstrap` — build + selfhost + baseline
- [x] Polyglot build drivers: `bootstrap/build_nvs.{sh,py,js,rb}`

## Multi-language builders
| Driver | Path |
|--------|------|
| Shell | `bootstrap/build_nvs.sh` |
| Python | `bootstrap/build_nvs.py` |
| Node | `bootstrap/build_nvs.js` |
| Ruby | `bootstrap/build_nvs.rb` |
| NvS | `build_nvs("bin/nvs")` builtin |

## Prior (still green)
- Baseline, quantum, polyglot, highlight, fuzzy, corrections

```bash
nvs run examples/selfhost.ns
nvs selfhost 'let x = 6 * 7 print x'
bash bootstrap/build_nvs.sh
```
