# Start here: what NvS is and how to use it

**NvS (Navescript)** is a programming language built from scratch — think
of it as a workshop where one tool (`nvs`) writes, runs, checks, fixes,
documents, packages, and even debugs code. The goal: "the new C for the
new era" — a language that plays well with other languages instead of
competing with them.

You don't need to know any of the history below to use it. This page
tells you what exists and which command to reach for.

## The 60-second example

Save this as `hello.nvs`:

```nvs
print "hello, nave"
```

Run it:

```sh
nvs run hello.nvs
```

That's the whole language in miniature: you write plain-English-ish
instructions, `nvs` runs them. A slightly bigger taste:

```nvs
fn greet(name) {
  return "hey, " + name
}

let friends = ["nave", "world"]
for f in friends {
  print greet(f)
}
```

`fn` makes a reusable chunk of code (a function), `let` stores a value,
`for ... in` loops over a list. If you've seen any programming before,
most of NvS will look familiar on purpose — it borrows the best ideas
from everywhere.

## What can it actually do?

- **Everyday scripting**: files, text, math, lists, hashes (dictionaries).
- **Real programs**: functions, classes, error handling (`try`/`catch`),
  pattern matching (`match`), concurrency (`go`/`chan`-style tasks).
- **Types when you want them**: write plain code and it just runs, or add
  type annotations and `nvs check` will catch mistakes before you run.
- **Talk to other languages**: call NvS from C, Rust, and Python; convert
  Python and JavaScript into NvS; export NvS functions so C programs can
  use them; compile to WebAssembly.
- **Quantum experiments**: a built-in honest simulator for playing with
  qubits and quantum circuits (a simulator, not a real quantum computer).
- **Packages**: share and reuse other people's NvS code from GitHub.

## Every command in one sentence

| Command | What it does |
|---|---|
| `nvs run <file>` | Runs your program. Add `--watch` to re-run when you save, `--profile` to see which lines are slowest. |
| `nvs eval '<code>'` | Runs a one-liner without making a file. |
| `nvs repl` | Opens an interactive prompt: type code, see results immediately. |
| `nvs fmt` | Auto-formats your code so it looks tidy. |
| `nvs lint` | Points out suspicious code (unused variables, etc.). |
| `nvs check` | Type-checks your code and reports mistakes with file and line number. |
| `nvs doc` | Turns your `///` comments into pretty Markdown documentation. |
| `nvs test` | Finds and runs your test files, reports pass/fail. |
| `nvs debug <file>` | Step-through debugger in your terminal: breakpoints, inspect variables. |
| `nvs dap` | Same debugger, but speaking the Debug Adapter Protocol so VS Code (and other editors) can drive it. |
| `nvs lsp` | Language server for editors: hover docs, go-to-definition, autocomplete, error squiggles. |
| `nvs pkg` | Package manager: start a project (`init`), add code from GitHub (`install`/`get`), list what's installed. |
| `nvs init` | Scaffolds a new NvS project folder. |
| `nvs import --from=python` | Converts a Python file into NvS (and `--from=js` for JavaScript). |
| `nvs extract` | Pulls out NvS code blocks embedded inside other files and runs them. |
| `nvs transpile` | Translates NvS into JavaScript or Python. |
| `nvs bridge` | Talks to other programs over JSON messages (how editors and tools plug in). |
| `nvs bindgen` / `nvs exports` | Generates glue code so C, Rust, and Python can call your NvS functions. |
| `nvs bc` | Compiles NvS to bytecode (with `--disasm` to peek at the raw instructions). |
| `nvs nave` | Runs `.nave` workflow files (JSON-described task pipelines). |
| `nvs info` | Tells you about the language and what this build can do. |
| `nvs version` | Prints the version number. |

## The usual workflow

```sh
nvs init myproject      # make a project
cd myproject
# ... write code in main.nvs ...
nvs run main.nvs        # run it
nvs check main.nvs      # type-check it
nvs test                # run its tests
nvs fmt main.nvs        # tidy it up
```

## Editor setup

- **VS Code**: see `editors/vscode/README.md` — syntax highlighting,
  autocomplete, and F5 debugging via `nvs lsp` + `nvs dap`.
- **Any editor with LSP/DAP support**: point it at `nvs lsp` and `nvs dap`.

## Where things live

- `docs/` — guides for each subsystem (`INSTALL.md`, `STDLIB.md`, `POLYGLOT.md`, …)
- `stdlib/` — ready-made helper libraries written in pure NvS
- `examples/` — small programs you can run and steal from
- `editors/vscode/` — the VS Code extension scaffold

## Honest limits (read this once)

- The debugger, profiler, and watch mode run the **tree-walking**
  interpreter, not the bytecode VM — fine for development, not a
  production profiler.
- The quantum module is an **ideal simulator** (no noise, max 24 qubits),
  not a quantum computer.
- `nvs import --from=python/js` handles a documented **subset** of those
  languages and fails loudly on the rest — it never silently mistranslates.
- The installer builds from source today because **no release binaries
  are published yet** (see `docs/INSTALL.md`).

**Next:** if you're brand new, run the 60-second example above. If you
want depth, `docs/` has a guide per topic, and `nvs help` lists every
command with its flags.
