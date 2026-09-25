# Jarvis / Nave workflows

## What “Jarvis” is in this repo

`.nave` files are **NJSON workflow documents** (JSON), not NvS source. They describe
step graphs: `log`, `set`, `polyglot_eval`, `http_get`, `input`, `if`, etc.

Files under `examples/jarvis_*.nave` are successive platform demos (0.3 → 1.0).

## Rename / evolution (hint)

| Name | Role |
|------|------|
| **Jarvis** | Original branding on workflow demos |
| **system_health.nave** | Same style pipeline; operational monitor (practical “successor” demo) |
| **NASI** | Navescript Application System Interface (`nasi_test.nave`, `nasi:core` imports in v0.4+) |
| **NASM** | Assembly-like op `nasm_exec` (still stubbed on host) |

There is no single binary rename; the **workflow format** is shared. CLI accepts both:

```bash
nvs nave examples/jarvis_interface.nave
nvs jarvis examples/system_health.nave
```

## Runner (`internal/nave`)

Supported ops: `log`, `set`, `answer`, `input` (non-interactive default),
`polyglot_eval` (python/js), `http_get`, `file_read`/`file_write`, `if`/`try` (basic),
`assert_eq`. Stubs: `nasm_exec`, `component_call`.
