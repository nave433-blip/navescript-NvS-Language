# PLACEHOLDERS.md

When work needs manual action from Evan (his credentials, logins, installs,
paid memberships, or a decision only he can make), we don't stall: we put a
clearly-marked placeholder here and build around it. Everything keeps
compiling and tests stay green.

## Active placeholders

### 1. `ffi` native-binding layer (needs Evan's design call)
- **What:** `std/*.ns` files call `ffi::call("nasi.vector.new", [...])`.
  Wave 19 made `a::b` parse (as member access), so those files read — but
  `ffi` is not defined in the runtime. Calling one fails honestly with
  `identifier not found: ffi`.
- **Until he decides:** the parser, checker, and interpreter are complete
  around it; std files parse but can't execute their native calls.
- **Options for Evan:** (a) spec a real `ffi` bridge to the nasi-core native
  layer; (b) bless pure-NvS fallbacks (arrays/hashes) as the default stdlib
  with `ffi` as an optional accelerator; (c) defer.

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

## Resolved placeholders
(none yet)
