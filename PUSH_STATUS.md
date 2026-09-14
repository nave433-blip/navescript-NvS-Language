# Push status

Working interpreter is complete locally (1.3.0-working).

## Already on GitHub (via API)
- README, STATUS, go.mod, go.sum, CI, .gitignore
- cmd/nvs/main.go
- internal/lexer/* (working)
- internal/object/object.go
- internal/sqlite/sqlite.go
- examples (full_regression, classes, ...)

## Still required for a full remote `go build`
- internal/ast/ast.go
- internal/parser/parser.go
- internal/eval/eval.go

Local commit with full tree: `38b1190`

## Local verify
```bash
go run ./cmd/nvs/ version
go run ./cmd/nvs/ run examples/full_regression.ns
```
