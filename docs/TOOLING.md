# NvS Tooling — LSP, debugger, test runner, watcher, profiler

## `nvs lsp` — language server (stdio)

A hand-written JSON-RPC Language Server Protocol server over stdin/stdout —
zero third-party dependencies. It parses each open document and provides:

- **Diagnostics** — parser errors plus the Wave-13 gradual type checker's
  findings, published live as you type.
- **Hover** — signatures and `///` doc comments for your functions; builtins
  are marked as such.
- **Completion** — builtins, keywords, and in-scope names.
- **Go to definition** — jumps to `let`/`fn`/`class` bindings (same file).
- **Document symbols** — top-level outline.

### VS Code

Install any generic LSP client (e.g. `llvm-vs-code-extensions.vscode-clangd`
won't do — use an extension that lets you configure the server command, such
as `trixnz.vscode-lsp` or a minimal `languageclient` config) and point it at:

```json
{
  "command": "nvs",
  "args": ["lsp"],
  "filetypes": ["nvs"]
}
```

Register `.nvs`/`.ns` as a language first. The server speaks standard LSP
base protocol (`Content-Length` headers), so any compliant client works.

### Neovim (built-in LSP)

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "nvs" },
  callback = function()
    vim.lsp.start({
      name = "nvs",
      cmd = { "nvs", "lsp" },
      root_dir = vim.fn.getcwd(),
    })
  end,
})
```

### Zed

Zed doesn't yet allow arbitrary custom LSP binaries per language via settings;
until it does, run `nvs lsp` behind a small shim or use the CLI tools below.
(This will be revisited — no fake "just works" claim here.)

### Honest limits

- Definitions are same-file only; no workspace-wide symbol search.
- The file is re-parsed per request (fine for normal file sizes).
- Offsets assume ASCII-ish source (LSP UTF-16 units aren't special-cased).

## `nvs debug` — terminal debugger

```
nvs debug script.nvs
```

Set breakpoints *before* the program runs at the pre-run prompt, then:

| Command | Effect |
|---|---|
| `break <line>` / `b` | add/toggle breakpoint; bare `break` lists them |
| `step` / `s` | stop at the next statement (steps into calls) |
| `next` / `n` | stop at the next statement, stepping *over* calls |
| `continue` / `c` | run to the next breakpoint |
| `print <expr>` / `p` | evaluate an expression in the paused scope |
| `backtrace` / `bt` | call stack, innermost first |
| `locals` | bindings in the current frame |
| `quit` / `q` | stop the program |

Empty line repeats the last command (gdb-style). Example session:

```
(nvsdb) break 4
(nvsdb) continue
stopped at line 4
(nvsdb) bt
#0 add
#1 <toplevel>
(nvsdb) p a * b
200
(nvsdb) continue
30
```

Scope: the tree-walking evaluator only. The experimental `nvs bc` bytecode VM
is a separate engine and can't be debugged this way. Breakpoints match the
main file; if `step` walks into an imported file, the pause reports it as
`stopped at <file>:<line>`. DAP (Debug Adapter
Protocol) is future work.

## `nvs test` — test runner

```bash
nvs test              # current dir, recursive
nvs test stdlib/      # specific dirs
nvs test -v           # show each file's output
nvs test --timeout=10s
```

Discovers `test_*.nvs`, `test_*.ns`, `*_test.nvs`, `*_test.ns`. Each file runs
in a **fresh interpreter subprocess** (clean global state), passes on exit 0.
Use `assert(cond, "message")` — a failed assertion exits nonzero. Summary and
nonzero exit code on any failure, for CI.

## `nvs run --watch` — file watcher

```bash
nvs run --watch main.nvs
```

Re-runs the program whenever any `.nvs`/`.ns` file in its directory changes
(500 ms mtime polling — no new dependencies, works on every OS). Ctrl+C stops.

## `nvs run --profile` — statement profiler

```bash
nvs run --profile main.nvs
```

Counts statement executions via the debugger hook and prints the hottest
lines, grouped per file. Statements are attributed to the file where they
were **defined** — calls into imported files (and the prelude, e.g. its
`assert` helper) appear under their own file section, never as phantom
lines in your program:

```
profile: 20301 statement executions across 2 file(s) (main.nvs)
--- main.nvs ---
LINE     HITS     SOURCE
3        10000    total = total + i
2        10001    for (i in range(10000)) {
--- stdlib/prelude.ns ---
LINE     HITS     SOURCE
...
```

Tree-walker only, like the debugger.
