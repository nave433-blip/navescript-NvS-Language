package eval

// Wave 20 tests: TCP sockets, env_all, AES-GCM. The TCP test uses a fixed
// high port with channel-ordered handshake — deterministic, no sleeps.

import "testing"

func TestWave20TCPEcho(t *testing.T) {
	expectInspect(t, `
let port = 18443;
let ready = chan(1);
let done = chan(1);
spawn(fn() {
    let ln = tcp_listen(port);
    send(ready, true);
    let c = tcp_accept(ln);
    let msg = tcp_read(c, 1024);
    tcp_write(c, "echo:" + msg);
    tcp_close(c);
    tcp_close(ln);
    send(done, true);
});
recv(ready);
let c = tcp_connect("127.0.0.1", port);
tcp_write(c, "hello");
let resp = tcp_read(c, 1024);
tcp_close(c);
recv(done);
resp`, `echo:hello`)
}

func TestWave20TCPWriteReturnsBytes(t *testing.T) {
	expectInspect(t, `
let port = 18444;
let ready = chan(1);
let done = chan(1);
spawn(fn() {
    let ln = tcp_listen(port);
    send(ready, true);
    let c = tcp_accept(ln);
    tcp_read(c, 1024);
    tcp_close(c);
    tcp_close(ln);
    send(done, true);
});
recv(ready);
let c = tcp_connect("127.0.0.1", port);
let n = tcp_write(c, "12345");
tcp_close(c);
recv(done);
n`, `5`)
}

func TestWave20TCPConnectRefused(t *testing.T) {
	expectErrorContains(t, `tcp_connect("127.0.0.1", 18445)`, "tcp_connect")
}

func TestWave20TCPBadHandle(t *testing.T) {
	expectErrorContains(t, `tcp_write("tcpc-999999", "x")`, "unknown connection handle")
	expectErrorContains(t, `tcp_read("nope", 10)`, "unknown connection handle")
	expectErrorContains(t, `tcp_close("nope")`, "unknown handle")
	expectErrorContains(t, `tcp_accept("nope")`, "unknown listener handle")
}

func TestWave20TCPBadPort(t *testing.T) {
	expectErrorContains(t, `tcp_listen("notaport")`, "tcp_listen")
}

func TestWave20EnvAll(t *testing.T) {
	expectInspect(t, `
set_env("NVS_WAVE20_TEST", "yes-indeed");
env_all()["NVS_WAVE20_TEST"]`, `yes-indeed`)
	expectInspect(t, `typeof(env_all())`, `HASH`)
}

func TestWave20AESRoundTrip(t *testing.T) {
	expectInspect(t, `
let key = "0123456789abcdef";
let ct = aes_gcm_encrypt(key, "secret message");
aes_gcm_decrypt(key, ct)`, `secret message`)
}

func TestWave20AESKeySizes(t *testing.T) {
	// 16, 24, 32 bytes all valid; anything else rejected.
	expectInspect(t, `
let k16 = "0123456789abcdef";
let k24 = "0123456789abcdef01234567";
let k32 = "0123456789abcdef0123456789abcdef";
[aes_gcm_decrypt(k16, aes_gcm_encrypt(k16, "a")),
 aes_gcm_decrypt(k24, aes_gcm_encrypt(k24, "b")),
 aes_gcm_decrypt(k32, aes_gcm_encrypt(k32, "c"))]`, `[a, b, c]`)
	expectErrorContains(t, `aes_gcm_encrypt("short", "x")`, "16, 24, or 32 bytes")
}

func TestWave20AESWrongKey(t *testing.T) {
	expectErrorContains(t, `
let ct = aes_gcm_encrypt("0123456789abcdef", "data");
aes_gcm_decrypt("fedcba9876543210", ct)`, "authentication failed")
}

func TestWave20AESTampered(t *testing.T) {
	expectErrorContains(t, `
let ct = aes_gcm_encrypt("0123456789abcdef", "data");
// flip the last base64 char to a different valid one
let bad = ct[0:len(ct)-1] + (ct[len(ct)-1] == "A" ? "B" : "A");
aes_gcm_decrypt("0123456789abcdef", bad)`, "authentication failed")
}

func TestWave20AESArgShapes(t *testing.T) {
	expectErrorContains(t, `aes_gcm_encrypt("0123456789abcdef")`, "want key, plaintext")
	expectErrorContains(t, `aes_gcm_decrypt("0123456789abcdef", "!!!notbase64!!!")`, "valid base64")
	expectErrorContains(t, `tcp_write("x")`, "want handle, data")
	expectErrorContains(t, `env_all("x")`, "want 0 arguments")
}

func TestWave20Ed25519RoundTrip(t *testing.T) {
	expectInspect(t, `
let keys = ed25519_generate();
let sig = ed25519_sign(keys["private_key"], "hello world");
ed25519_verify(keys["public_key"], "hello world", sig)`, `true`)
}

func TestWave20Ed25519Tampered(t *testing.T) {
	expectInspect(t, `
let keys = ed25519_generate();
let sig = ed25519_sign(keys["private_key"], "hello");
ed25519_verify(keys["public_key"], "goodbye", sig)`, `false`)
	expectErrorContains(t, `ed25519_sign("bogus", "x")`, "invalid private key")
	expectErrorContains(t, `ed25519_verify("bogus", "x", "y")`, "invalid public key")
	expectErrorContains(t, `ed25519_generate("x")`, "want 0 arguments")
}
