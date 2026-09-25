# NvS Bytecode Investigation

## What existed (original repo)

| Path | State |
|------|-------|
| `internal/compiler/compiler.go` | Stub (~110 lines): few opcodes, incomplete `Compile` switch |
| `internal/vm/vm.go` | **Does not compile** (syntax error / broken strings) |
| `internal/vm/frame` | Minimal frame struct |
| `internal/codegen/x86_64` | Placeholder `.ns` notes only |

The original “bytecode VM” claim was **not functional** and was **not wired** to the working tree-walker (`internal/eval`).

## What works now (2.8.0)

New package: **`internal/bytecode`**

### Pipeline
```
source → lexer → parser → AST → bytecode.Compiler → Bytecode → bytecode.VM
```

### Opcodes
`Constant`, `Add/Sub/Mul/Div/Mod`, comparisons, `Minus/Bang`,
`True/False/Null`, `Pop`, `Jump` / `JumpNotTruthy`,
`SetGlobal` / `GetGlobal`, `Print`, `Halt`

### Supported (stage 1)
- integers / floats / strings / bools / null
- arithmetic & comparisons
- `let` / `const` / assignment
- `print` statement and `print(...)`
- `if / else` (jumps)

### Not yet
- while / for loops
- functions / closures
- arrays / maps / classes
- locals (only globals)
- integration with selfhost mini_eval emit

### CLI
```bash
nvs bytecode 'print 1+2*3' --disasm
nvs bytecode program.ns
nvs bc 'let x=1 print x' -d
```

### Example disassembly
```
0000 Constant 0 (1)
0003 Constant 1 (2)
0006 Constant 2 (3)
0009 Mul
0010 Add
0011 Print
0012 Halt
```
