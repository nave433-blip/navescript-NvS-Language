# Status — 1.3.0-working

## Fixed
- [x] `examples/complete_test.ns` rewritten to modern syntax
- [x] Regression pass: classes, hof, claimed, more3, complete_test
- [x] Short-circuit `&&` `||` `??`
- [x] Membership `in`

## Added
- [x] `log`, `printf`
- [x] `metrics_text` (Prometheus-like exposition)
- [x] `ws_connect` WebSocket client
- [x] Claim status APIs: `ws_info`, `grpc_info`, `otel_info`

## Verify
```bash
go run ./cmd/nvs/ run examples/complete_test.ns
go run ./cmd/nvs/ run examples/more3.ns
go run ./cmd/nvs/ run examples/classes.ns
go run ./cmd/nvs/ version
```
