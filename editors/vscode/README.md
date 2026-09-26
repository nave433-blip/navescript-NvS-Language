# NvS for VS Code

Language support for **NvS (Navescript)** — `.nvs`, `.ns`, and `.nave` files:

- Syntax highlighting (TextMate grammar generated from the real NvS lexer
  keyword list, so it can't drift from the language).
- IntelliSense powered by `nvs lsp`: hover docs, go-to-definition
  (including across files via imports), references, workspace symbol
  search, completions, and live diagnostics.
- Debugging via `nvs dap` (Debug Adapter Protocol): breakpoints,
  step in / over / out, locals, and expression evaluation.

This is a **scaffold**, not a published extension. It is not on the VS Code
Marketplace, and this README does not claim otherwise. Everything runs
locally from your own `nvs` binary — no network, no telemetry.

## Prerequisites

1. An `nvs` binary on your `PATH` (or set the `nvs.path` setting to its
   location). Build it from this repo:
   ```sh
   go build -o ~/.local/bin/nvs ./cmd/nvs
   ```
2. Node.js (only needed to package the `.vsix`; the extension itself has
   zero npm dependencies — it talks to `nvs lsp` / `nvs dap` over stdio).

## Build and install the .vsix

```sh
cd editors/vscode
npm install -g @vscode/vsce     # one-time: the VS Code packaging tool
vsce package                     # produces nvs-vscode-0.1.0.vsix
code --install-extension nvs-vscode-0.1.0.vsix
```

Or install without packaging: in VS Code run **Developer: Install Extension
from Location...** and point it at this `editors/vscode` folder.

## Use

- Open any `.nvs` file. The extension starts `nvs lsp` automatically and
  diagnostics appear as you type.
- **Run → Start Debugging** (or F5) with an NvS file open: the
  "Debug current NvS file" configuration launches it under `nvs dap`.
  Set breakpoints in the gutter, step with F10/F11, inspect locals in
  the Variables pane, and evaluate expressions in the Debug Console.
- If `nvs` isn't on your `PATH`, set it in Settings → Extensions → NvS →
  `nvs.path`.

## Layout

| File | What it is |
|---|---|
| `package.json` | Extension manifest: language registration, grammar, debug adapter wiring |
| `extension.js` | Activation: minimal LSP client + DAP descriptor factory (no deps) |
| `syntaxes/nvs.tmLanguage.json` | TextMate grammar (35 real lexer keywords) |
| `language-configuration.json` | Brackets, auto-closing pairs, `//` comments |

## Honest limits

- The LSP client is hand-written and minimal; it covers exactly what
  `nvs lsp` implements. Unknown requests degrade silently.
- The debugger is tree-walker based (see `internal/dap`): breakpoints
  only in the launched file, locals only for the innermost frame.
- The grammar is regex-based highlighting, not a real parser.
