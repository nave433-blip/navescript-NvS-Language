# Status — NvS 2.2.0 (baseline green)

## Systematic baseline
- [x] All core examples pass (`examples/*.ns` except experimental `security_research.ns`)
- [x] `examples/baseline_all.ns` → **BASELINE OK**
- [x] Fixed `&&` / `||` precedence (ANDOR < EQUALS) so `a == 1 && b == 2` works
- [x] Added `**` power operator (lexer + parser + eval)

## Feature checklist
| Area | Status |
|------|--------|
| let / const / assign | OK |
| null / ?? | OK |
| and/or / &&/|| | OK (precedence fixed) |
| hex 0x / binary 0b | OK |
| ** power | OK |
| strings + slices | OK |
| arrays + slices + push | OK |
| maps / sets | OK |
| if / while / for / for-in | OK |
| functions / defaults / closures | OK |
| match/switch | OK |
| try/catch/finally | OK |
| defer | OK |
| class / new / this | OK |
| enum | OK |
| typeof / deep_equal | OK |
| HOF map_fn etc | OK |
| generators | OK (basic) |
| polyglot / interop / corrections | OK |
| highlight / fuzzy | OK |
| json / sqlite / stdlib | OK |

```bash
go build -o bin/nvs ./cmd/nvs/
./bin/nvs run examples/baseline_all.ns
```

## 2.2.0 — Quantum + Greek / physics symbols
- [x] Greek & physics constants: π φ ℏ α τ, hbar, c_light, G_grav, k_B, e_charge, m_e, m_p, N_A
- [x] `physics_const(name)` / full map
- [x] Quantum simulation (state vectors): `qubit`, `qzero`, `qgate`, `qmeasure`, `qprob`, `qtensor`, `qnormalize`, `qinner`
- [x] Gates: H, X, Y, Z, S, T, RX/RY/RZ, CNOT
- [x] `examples/quantum.ns` → QUANTUM OK

Honest scope: **classical simulation** of small circuits (≤8 qubits), not physical QPU hardware.
