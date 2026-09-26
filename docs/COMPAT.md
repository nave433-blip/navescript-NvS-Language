# COMPAT.md — NvS compatibility policy (Wave 15)

NvS takes backwards compatibility seriously: code you write today should
run tomorrow. This document states exactly what is guaranteed stable,
what is explicitly unstable, and how we prove it.

## Stable guarantees

These will not break without a major-version bump and a migration path:

1. **Language syntax** — every program that parses under 2.x keeps parsing
   under 2.x. New syntax may be added; existing syntax is not removed or
   redefined.
2. **Stdlib module paths and signatures** — `stdlib/*.nvs` module paths
   and every documented function signature keep working. New modules and
   functions may be added.
3. **C ABI** — the `libnvs` exported functions and their signatures
   (`nvs.h`) are stable. See `docs/POLYGLOT.md`.
4. **Bridge protocol** — the `nvs bridge` JSON stdio protocol
   (`{"eval":...}` / `{"exec":...}` / `{"call":...}` → `{"ok":...}`) is
   stable.
5. **CLI flags** — existing `nvs` subcommands and flags keep their meaning.
   New subcommands may be added.

## Explicitly unstable

Do not build on these across versions:

- **Bytecode opcodes** — the experimental `nvs bc` bytecode format and
  opcode numbering may change at any time.
- **Internal Go APIs** — packages under `internal/` are internal by
  construction; their function signatures may change without notice.
  (The C ABI and the bridge protocol are the supported integration
  surfaces, not Go imports.)

## The frozen corpus

`compat/*.nvs` is a set of small programs exercising 2.1.0-era features:
pipelines, ranges, match, classes, string interpolation, destructuring,
generators, quantum simulator basics, higher-order functions, try/catch,
and loops.

The fixtures are **frozen**: they are never edited to accommodate a
language change. If a fixture fails, the language regressed — the change
gets reverted or given a migration path, never the fixture.

`TestCompatCorpus` (`compat/compat_test.go`) evaluates every fixture on
every `go test ./...` run and fails on any parse or eval error. It guards
"it still runs", not exact output formatting.

## Adding to the corpus

When a new language feature stabilizes, add a small frozen fixture that
exercises it the way a 2.1.0-era user would. Keep fixtures:

- small (one feature area each),
- deterministic (no randomness, no network, no wall-clock dependence —
  the quantum fixture asserts only deterministic properties),
- dependency-free (no imports outside the language core).
