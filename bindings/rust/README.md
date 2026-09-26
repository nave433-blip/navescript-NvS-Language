# nvs — Rust bindings for NvS

Safe Rust bindings over the NvS C ABI (`examples/c_embed/nvs.h`).

```rust
let v = nvs::eval("6 * 7").unwrap();
assert_eq!(v, serde_json::json!(42));

nvs::eval("fn add(a, b) { a + b }").unwrap();
let v = nvs::call("add", &serde_json::json!([20, 22])).unwrap();
assert_eq!(v, serde_json::json!(42));
```

## How it works

`build.rs` compiles `libnvs.so` from the Go sources in `../../cbridge`
with `go build -buildmode=c-shared` into Cargo's `$OUT_DIR`, links it,
and bakes an rpath — so `cargo test` needs no manual setup. Go is found
via `$GO_BIN`, `~/go-dist/go/bin/go`, or `PATH`.

The bridge has one global interpreter per process (serialized on the Go
side); the crate adds its own `Mutex` so concurrent Rust callers can't
interleave requests. Every returned C string is freed with `nvs_free`
exactly once (RAII via `take_envelope`).

Errors: NvS parse/runtime errors come back as `NvsError::Nvs(String)`;
bridge-level failures as `NvsError::Bridge(String)`.
