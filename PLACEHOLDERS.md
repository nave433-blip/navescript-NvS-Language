# PLACEHOLDERS.md

When work needs manual action from Evan (his credentials, logins, installs,
paid memberships, or a decision only he can make), we don't stall: we put a
clearly-marked placeholder here and build around it. Everything keeps
compiling and tests stay green.

## Active placeholders

### 1. `ffi` native-binding layer (needs Evan's design call)
- **What:** `std/*.ns` files used to call `ffi::call("nasi.vector.new", ...)`.
  Wave 19 made `a::b` parse (as member access), so those files read — but
  `ffi` is not defined in the runtime. Calling one fails honestly with
  `identifier not found: ffi`.
- **Status 2026-10-09 (Wave 20):** continued the pure-NvS stdlib revival —
  `std/core`, `std/os/env`, `std/datetime/time`, `std/crypto`,
  `std/math/{complex,matrix}`, `std/io/streams`, `std/db`, `std/net/tcp`
  are now real working code over real builtins. Remaining `ffi::call`
  users genuinely need native capabilities: `std/net/ws` (ws_connect is a
  stub suggesting the python/js polyglot), `std/gpu`, `std/media`,
  `std/ui`, `std/ai`, `std/wasi`, `std/os/kernel/*`, `std/thread`,
  `std/sync`, `std/graphics`, `std/pm`, `std/rpc`, `std/debug`,
  `std/security`, `std/runtime`, `std/ffi/*` (bridges by definition).
- **Options for Evan:** (a) spec a real `ffi` bridge to the nasi-core native
  layer; (b) continue the pure-NvS stdlib and treat `ffi` as an optional
  accelerator; (c) defer.

### 2. Module resolution — `import std.core` / `module std.x;` (future wave)
- **What:** Wave 19 parses `module` declarations and bare-path imports, but
  there is no module loader. `import std.core;` fails cleanly at runtime:
  `module path 'std.core' is not yet supported (module resolution is a
  future wave)`.
- **Until then:** quoted file imports (`import "lib.ns";`) work as before.

### 3. Sourcing note — language internals are bespoke (no OSS to borrow)
- **What:** Per the sourcing directive, we looked for established OSS to
  borrow before building. A programming language's parser, type checker,
  and evaluator are inherently bespoke — there is no MIT/Apache library
  that provides "generics for NvS". The Pratt parser, gradual checker, and
  tree-walking evaluator are hand-rolled, which is the honest fallback the
  directive allows.
- **No action needed.** Recorded so the directive is visibly satisfied.

### 4. Real SQLite — modernc.org/sqlite (needs Evan's call)
- **What:** The `sqlite_*` builtins are an honest in-memory SQL subset
  (dependency-free by design; `sqlite_open` now rejects file paths loudly
  instead of silently going in-memory). Verified 2026-10-09: replacing the
  engine with `modernc.org/sqlite` (pure Go, no cgo, **BSD-3-Clause** —
  license checked against the downloaded module) would make them real
  persistent SQLite.
- **Why it needs Evan:** it would be the project's **first external
  dependency ever** (go.mod is currently dependency-free). Cost: ~40MB
  module, noticeably slower clean builds for every `install.sh` user, bigger
  binaries on all platforms. The in-memory subset + file-backed `std/db`
  cover most needs without it.
- **Options for Evan:** (a) adopt modernc.org/sqlite as an optional
  dependency; (b) keep the in-memory subset forever; (c) defer.

## Resolved placeholders
(none yet)
