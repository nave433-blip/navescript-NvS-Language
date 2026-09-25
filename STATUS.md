# Status — NvS 2.7.0

## Self-host stage 5
- [x] Go emit: **while** loops → `for cond { }`
- [x] Go emit: **fn** → Go closures + calls
- [x] mini_eval strings (`"hi"`)
- [x] `examples/selfhost5.ns` → SELFHOST5 OK (while=10, fn=42)

```bash
nvs run examples/selfhost5.ns
```

## Ladder
| Stage | Capability |
|-------|------------|
| 1–4 | mini_eval core + import |
| **5** | **richer Go emit (while/fn)** |
| Next | for-in emit; maps in Go emit; bytecode VM sketch |

## 2.8.0 Bytecode
- [x] Investigated legacy compiler/vm (stubs / non-building)
- [x] New `internal/bytecode` compiler + stack VM
- [x] CLI `nvs bytecode` / `nvs bc` with `--disasm`
- [x] docs/BYTECODE.md
