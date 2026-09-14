# Navescript (NvS) 1.3.0

A practical scripting language implemented in Go. Original ambitious claims are implemented **where feasible**, with honest stubs where not.

## Quick start

```bash
go run ./cmd/nvs/ run examples/claimed.ns
go run ./cmd/nvs/ run examples/hof.ns
go run ./cmd/nvs/ version
```

## Language features

| Area | Status |
|------|--------|
| Core syntax | `let`, `const`, `null`, operators, `??`, `&&`/`||` short-circuit |
| Types | Optional `let x: int = 1` runtime checks |
| Collections | Arrays, maps, `in` membership |
| Control | `if/else if/else`, `while`, `for`, `for-in`, `break`/`continue` |
| Match | `match/case/default` |
| Errors | `try/catch/throw` |
| Functions | Closures, generators (`yield`/`next`), decorators (`@name`) |
| OOP | `class`, `new`, `this`, `extends` |
| Modules | `import "file.ns"` |
| Polyglot | `python()`, `js()`, `ruby()`, `system()` |
| Higher-order | `map_fn`, `filter`, `reduce`, `any`, `all`, `find`, `group_by`, `sort_by` |
| HTTP | `http_get`, `http_post`, `http_serve` |
| WebSocket | `ws_connect(url, msg)` |
| JSON / CSV | parse/stringify, `read_json`/`write_json`, `csv_parse` |
| Database | Mini in-memory SQL-ish store |
| FS | read/write, mkdir, remove, glob, cwd/cd, exists, listdir |
| Crypto | md5, sha256, base64 |
| Metrics | `metric_inc/gauge`, `metrics_text` (Prometheus-ish), `trace` |
| Claimed stubs | `hardware_info`, `wasi_info`, `grpc_info`, `otel_info`, `ws_info` |

## Not fully implemented (original brochure)

- Full Hindley-Milner inference
- Real USB/SPI/I2C/Serial/HID drivers
- Embedded WASI + Component Model runtime
- Production gRPC server/client stack
- Full OpenTelemetry SDK

## Project layout

```
cmd/nvs/                 # CLI (run, eval, version)
internal/lexer|parser|ast|object|eval|sqlite
examples/
```

Legacy packages from the original incomplete repo may exist under `internal/` but are **not** part of the working pipeline.
