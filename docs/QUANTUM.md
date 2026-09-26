# NvS Quantum — honest simulator guide

> **This is a LOCAL, classical state-vector simulation of quantum computing.
> It never touches quantum hardware.** Ideal and noise-free, capped at 24
> qubits by the backend. Anything here that claims otherwise is a bug in the
> docs — please report it.

## Low-level builtins (Wave 8)

Registers: `q = qalloc(n)` creates an n-qubit register in |0…0⟩.

| Builtin | Effect |
|---|---|
| `q_h/q_x/q_y/q_z/q_s/q_t(q, i)` | single-qubit gates |
| `q_rx/q_ry/q_rz(q, i, theta)` | parameterized rotations (radians) |
| `q_cnot/q_cz(q, c, t)` | two-qubit entangling gates |
| `q_swap(q, a, b)` | SWAP |
| `q_toffoli(q, c1, c2, t)` | Toffoli (CCNOT) — flips `t` iff both controls are 1 |
| `q_cphase(q, c, t, phi)` | controlled phase e^{iφ} on |11⟩ |
| `q_probs(q)` | probability array (does not collapse) |
| `q_measure(q, i)` / `q_measure_all(q)` | measure (collapses; crypto-rand) |
| `q_seed(s)` | reseed the **Wave-17** measurement RNG (reproducible demos) |
| `q_shots(q, n)` | `{outcome: count}` over n shots — does **not** collapse the register |

Example — Bell state with statistics:

```nvs
let q = qalloc(2)
q_h(q, 0)
q_cnot(q, 0, 1)
q_seed(42)
print q_shots(q, 1000)   // {0: ~500, 3: ~500}
```

## Circuit builder

```nvs
let cb = circuit(2)
cb["h"](0)["cnot"](0, 1)["draw"]()   // ASCII diagram; methods chain
q_seed(7)
print cb["run"](1000, 7)             // seeded shots → {outcome: count}
print cb["probs"]()                 // state probabilities, no collapse
```

(The builder is a hash of method values, so calls use the `cb["h"](0)`
index-call form — NvS method syntax only exists on class instances.)

## Worked examples

- `examples/quantum_grover.nvs` — Grover's search, 3 qubits, marked |111⟩:
  12.5% → **78.1%** → **94.5%** over two iterations (matches theory), with
  seeded, reproducible shot counts.
- `examples/quantum_qft.nvs` — Quantum Fourier Transform: uniform output on
  |000⟩, textbook phases on basis states, and QFT→inverse-QFT round-tripping
  |101⟩ back to itself with probability 1.0.

## Limits

- 24-qubit cap (2^24 complex amplitudes is already 256 MiB-scale).
- No noise model, no decoherence, no error correction — an ideal simulator.
- Only the Wave-17 paths (`q_seed`, `q_shots`, builder `measure`/`run`) are
  seedable; the older `q_measure` builtins use crypto randomness.
