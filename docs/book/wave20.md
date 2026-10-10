# Wave 20: stdlib revival — sockets, static methods, honest crypto

This chapter documents what Wave 20 made real. The theme: replacing
`ffi::call` stubs (for a native layer that never existed) with working
code over real builtins.

## Static methods

`static fn` in a class body is called on the class itself:

```ns
class Sha256 {
    pub static fn hash(data: string) -> string {
        return sha256(data);
    }
}

print(Sha256.hash("abc"));
// ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
```

Rules:
- `this` is **not** bound inside a static method; using it is a runtime error.
- Calling a non-static method on the class is a clean error:
  `Math2.inst is not static; call it on an instance of Math2`.
- `static` is a **contextual** keyword: it only acts as a modifier directly
  before `fn`/`pub`/a method name in a class body. It stays a legal
  identifier everywhere else, and a method can even be named `static`.
- `pub static fn` works; static methods are inherited like any method.

## New builtins

All backed by the Go standard library — no new dependencies.

### TCP sockets

```ns
import "./std/net/tcp/tcp.ns";

let ln = TcpListener.bind(8080);
let c = TcpStream.connect("127.0.0.1", 8080);
c.write("hello");
print(c.read(1024));
c.close();
```

Raw builtins: `tcp_connect(host, port)`, `tcp_listen(port)` (binds
127.0.0.1), `tcp_accept(h)`, `tcp_read(h, n)` (returns up to n bytes —
one Read call, may return fewer), `tcp_write(h, data)` → bytes written,
`tcp_close(h)`. Handles are strings; unknown handles are errors, not
crashes.

### Environment

`env_all()` returns the whole environment as a hash:
`env_all()["HOME"]`.

### AES-GCM symmetric encryption

```ns
let key = "0123456789abcdef";  // 16, 24, or 32 bytes
let ct = aes_gcm_encrypt(key, "secret");
print(aes_gcm_decrypt(key, ct));  // "secret"
```

Wire format is `base64(nonce || ciphertext)` with a fresh random 96-bit
nonce per encryption. A wrong key or corrupted data is an *authentication
error*, never silent garbage.

### Ed25519 signatures

```ns
let keys = ed25519_generate();  // {public_key, private_key}, base64
let sig = ed25519_sign(keys["private_key"], "message");
print(ed25519_verify(keys["public_key"], "message", sig));  // true
```

## Revived stdlib modules

| Module | What it is now |
|--------|----------------|
| `std/core` | `println`, `panic` over real builtins |
| `std/os/env` | `get`/`get_or`/`set`/`all` over `env`/`set_env`/`env_all` |
| `std/datetime/time` | `Time` class over `unixtime_ms`/`date_format` |
| `std/crypto` | `Sha256.hash`, `Aes.encrypt`/`decrypt` (AES-GCM), `Ed25519.generate`/`sign`/`verify` |
| `std/math/complex` | `Complex` with `+`/`-`/`*`/`/`, `abs`, `conj` |
| `std/math/matrix` | `Matrix` with `get`/`set`/`add`/`mul` |
| `std/io/streams` | Buffered `FileStream`: reads load at open, writes flush on `close()` |
| `std/db` | **File-backed JSON document store** — `put`/`get`/`delete`/`query` with real persistence |
| `std/net/tcp` | `TcpStream`/`TcpListener` over real sockets |

Import with a relative path: `import "./std/crypto/mod.ns";`

## Honest boundaries

- `sqlite_*` builtins are an **in-memory SQL subset** (CREATE TABLE /
  INSERT / SELECT), not SQLite-the-database. `sqlite_open` accepts only
  `":memory:"` — a file path is a loud error, never silent data loss.
  (Real SQLite via modernc.org/sqlite is tracked in PLACEHOLDERS.md.)
- `FileStream` is buffered; true unbuffered I/O needs the future ffi layer.
- `ws_connect` remains a stub (suggests the python/js polyglot); raw UDP,
  `std/gpu`, `std/media`, `std/ui`, `std/ai` still need native support.
- A parse error is never a panic (regression-tested in
  `internal/parser/wave20_test.go`).
