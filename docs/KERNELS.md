# NvS Kernel Compatibility (NT / Unix)

NvS targets both kernel families: **NT** (Windows) and **Unix** (Linux, macOS,
*BSD). This document records what that means in practice, what was verified
and how, and what remains honestly unverified.

## Support matrix

| OS | Kernel | Build | Runtime tests | Notes |
|---|---|---|---|---|
| Linux (x86_64) | unix | ✅ `go build ./...` | ✅ full `go test ./...` + examples + live CLI checks | Primary dev/CI platform |
| macOS (arm64/x86_64) | unix | ✅ `GOOS=darwin go build ./...` | ⚠️ compile only | No macOS hardware in this VM; Unix behavior shared with Linux |
| Windows (x86_64) | nt | ✅ `GOOS=windows go build ./...` | ⚠️ compile only | No Windows hardware in this VM; see "What differs on NT" |

`GOOS=windows`/`GOOS=darwin` builds are part of the Wave 16 verification and
should stay green — a commit that breaks them is a bug.

## What differs on NT (and how NvS handles it)

| Area | Unix | Windows (NT) | NvS behavior |
|---|---|---|---|
| Shell for `sh()` / `system()` | `sh -c` | `cmd /c` | Build-tagged `shellCommand` (`internal/eval/shell_unix.go`, `shell_windows.go`); same NvS code runs on both. Exposed via `os_shell()` |
| Python invocation | `python3` | `python` (stock installs) | `polyglot.PythonBinary()` tries `python3`, falls back to `python` |
| Executable suffix | none | `.exe` | Polyglot Rust plugin appends `.exe`; `os_exe_suffix()` in `stdlib/os.nvs` |
| Path separator | `/` | `\` | `filepath`-based builtins (`join_path`, `basename`, `dirname`, `abs_path`, `extname`) are platform-correct; `os_sep()` exposes it; `stdlib/os.nvs` normalizes foreign-format paths |
| Line endings | `\n` | `\r\n` | `os_eol()` reports it; `lines()` normalizes `\r\n`→`\n`; the lexer skips `\r`; `@nvs` extract markers match after `TrimSpace` — CRLF sources work everywhere |
| File modes | `0644` honored | largely ignored by Go | `write_file` passes `0644`; no error, just no effect on NT — documented, not hidden |
| File I/O translation | none | none | Go does no CRLF translation: `read_file`/`write_file` preserve bytes exactly on both kernels |

## The `os` module

Runtime facts come from Go builtins; the adaptation layer is pure NvS in
`stdlib/os.nvs` (import with `import "stdlib/os.nvs"`):

| Builtin | Returns |
|---|---|
| `os_name()` | `runtime.GOOS`: `"linux"`, `"windows"`, `"darwin"`, … |
| `os_kernel()` | `"nt"` on Windows, `"unix"` everywhere else |
| `os_sep()` | `"/"` or `"\\"` |
| `os_eol()` | `"\n"` or `"\r\n"` |
| `os_shell()` | `"sh"` or `"cmd"` — the shell `sh()`/`system()` use |

| `stdlib/os.nvs` function | Purpose |
|---|---|
| `os_is_windows()` / `os_is_unix()` | kernel predicates |
| `os_exe_suffix()` | `".exe"` on NT, `""` on Unix |
| `os_path_norm(p)` / `os_path_norm_with(sep, p)` | collapse separators, resolve `.`/`..`, convert the foreign separator; understands drive letters (`C:\a\..\b` → `C:\b`) |
| `os_is_abs(p)` / `os_is_abs_with(sep, p)` | absolute-path test for either convention (`/x`, `C:\x`, `\\server\x`) |
| `os_path_join(parts)` / `os_path_join_with(sep, parts)` | join an array of parts; the `_with` forms take an explicit separator so Windows paths can be manipulated on Unix and vice versa (testable without Windows hardware) |

Already-existing builtins (not duplicated): `env()`, `set_env()`, `args()`,
`join_path()`, `basename()`, `dirname()`, `exists()`, `abs_path()`, `extname()`.

```nvs
import "stdlib/os.nvs"

if (os_is_windows()) {
  print "running on NT: " + os_name()
}
let bin = os_path_join(["tools", "nvs" + os_exe_suffix()])
```

## Verification (Wave 16)

- `go build ./...`, `go vet ./...` clean on Linux.
- `GOOS=windows go build ./...` and `GOOS=darwin go build ./...` clean.
- `go test ./...`: all pass (new: `TestShellCommandRuns`,
  `TestShellNameMatchesRuntime`, `TestOsBuiltins`, `stdlib/tests/test_os.nvs`
  via the existing stdlib-suite harness — including Windows-separator cases
  injected through the `_with(sep, …)` forms, which run on Linux).
- `examples/*.ns` suite: no regressions vs the 70/73 baseline.
- Live Linux checks: every `os_*` builtin and every `stdlib/os.nvs` function.

## Honestly unverified

- **No Windows or macOS hardware exists in this VM.** The Windows code paths
  (`shell_windows.go` → `cmd /c`, `.exe` suffix, `python` fallback,
  `\r\n` EOL) are compile-verified and unit-tested where the logic is
  platform-independent, but no NvS program has executed on a real NT kernel
  yet. That is the next verification step when hardware (or CI runners) is
  available.
- Third-party host tools (`python`, `node`, `ruby`, `rustc`, `go`, `gcc`)
  are located via `PATH` on every platform; NvS reports a clean error when
  one is missing rather than assuming Unix install locations.

## Porting notes (for future kernels)

- Kernel-specific behavior belongs in `//go:build` tagged files
  (`*_unix.go` / `*_windows.go`), never in `if runtime.GOOS` sprinkled
  through shared code — except for pure data queries like `os_kernel()`.
- New builtins that touch the OS must answer: does it compile for
  windows+darwin? does it behave, or fail loudly, on NT?
- This groundwork also serves HybridOS userspace later: NvS programs that
  key off `os_kernel()` / `os_sep()` / `os_eol()` instead of hardcoding
  Unix assumptions will port to new kernels without rewrites. (No
  HybridOS backend exists yet — this is preparation, not a claim.)
