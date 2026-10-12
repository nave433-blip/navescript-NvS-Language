package eval

// Wave 20 — stdlib revival: real TCP sockets, full environment listing,
// and AES-GCM symmetric encryption.
//
// Everything here is backed by the Go standard library only
// (net, os, crypto/aes, crypto/cipher, crypto/rand, encoding/base64):
// no new external dependencies, no license questions, no cgo.
//
// These builtins exist so the pure-NvS stdlib (std/net/tcp, std/os/env,
// std/crypto) can be honest, working code instead of ffi::call stubs.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/navescript/nvs/internal/object"
)

// ---- TCP handle registry (same string-id pattern as internal/sqlite) ----

var (
	tcpMu      sync.Mutex
	tcpConns   = map[string]net.Conn{}
	tcpListens = map[string]net.Listener{}
	tcpNext    int64
)

func tcpConnID() string {
	tcpMu.Lock()
	defer tcpMu.Unlock()
	tcpNext++
	return fmt.Sprintf("tcpc-%d", tcpNext)
}

func tcpListenID() string {
	tcpMu.Lock()
	defer tcpMu.Unlock()
	tcpNext++
	return fmt.Sprintf("tcpl-%d", tcpNext)
}

func tcpGetConn(id string) (net.Conn, bool) {
	tcpMu.Lock()
	defer tcpMu.Unlock()
	c, ok := tcpConns[id]
	return c, ok
}

func tcpGetListener(id string) (net.Listener, bool) {
	tcpMu.Lock()
	defer tcpMu.Unlock()
	l, ok := tcpListens[id]
	return l, ok
}

func tcpPortOf(arg object.Object) (string, *object.Error) {
	switch v := arg.(type) {
	case *object.Integer:
		return fmt.Sprintf("%d", v.Value), nil
	case *object.String:
		return v.Value, nil
	default:
		return "", newError("port must be integer or string, got %s", arg.Type())
	}
}

// registerWave20Builtins adds the wave-20 stdlib-revival builtins.
// Called once from initBuiltins().
func registerWave20Builtins() {
	// ---- TCP sockets ----
	builtins["tcp_connect"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("tcp_connect: want host, port")
			}
			host, ok := args[0].(*object.String)
			if !ok {
				return newError("tcp_connect: host must be string, got %s", args[0].Type())
			}
			port, errObj := tcpPortOf(args[1])
			if errObj != nil {
				return errObj
			}
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host.Value, port), 10*time.Second)
			if err != nil {
				return newError("tcp_connect: %s", err.Error())
			}
			id := tcpConnID()
			tcpMu.Lock()
			tcpConns[id] = conn
			tcpMu.Unlock()
			return &object.String{Value: id}
		},
	}
	builtins["tcp_listen"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("tcp_listen: want port")
			}
			port, errObj := tcpPortOf(args[0])
			if errObj != nil {
				return errObj
			}
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
			if err != nil {
				return newError("tcp_listen: %s", err.Error())
			}
			id := tcpListenID()
			tcpMu.Lock()
			tcpListens[id] = ln
			tcpMu.Unlock()
			return &object.String{Value: id}
		},
	}
	builtins["tcp_accept"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("tcp_accept: want listener handle")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("tcp_accept: handle must be string, got %s", args[0].Type())
			}
			ln, found := tcpGetListener(id.Value)
			if !found {
				return newError("tcp_accept: unknown listener handle")
			}
			conn, err := ln.Accept()
			if err != nil {
				return newError("tcp_accept: %s", err.Error())
			}
			cid := tcpConnID()
			tcpMu.Lock()
			tcpConns[cid] = conn
			tcpMu.Unlock()
			return &object.String{Value: cid}
		},
	}
	builtins["tcp_write"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("tcp_write: want handle, data")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("tcp_write: handle must be string, got %s", args[0].Type())
			}
			data, ok := args[1].(*object.String)
			if !ok {
				return newError("tcp_write: data must be string, got %s", args[1].Type())
			}
			conn, found := tcpGetConn(id.Value)
			if !found {
				return newError("tcp_write: unknown connection handle")
			}
			n, err := conn.Write([]byte(data.Value))
			if err != nil {
				return newError("tcp_write: %s", err.Error())
			}
			return newInteger(int64(n))
		},
	}
	builtins["tcp_read"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("tcp_read: want handle, max_bytes")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("tcp_read: handle must be string, got %s", args[0].Type())
			}
			n, ok := args[1].(*object.Integer)
			if !ok || n.Value <= 0 {
				return newError("tcp_read: max_bytes must be a positive integer")
			}
			conn, found := tcpGetConn(id.Value)
			if !found {
				return newError("tcp_read: unknown connection handle")
			}
			buf := make([]byte, n.Value)
			// One Read call: returns what's available (may be fewer bytes).
			m, err := conn.Read(buf)
			if err != nil {
				return newError("tcp_read: %s", err.Error())
			}
			return &object.String{Value: string(buf[:m])}
		},
	}
	builtins["tcp_close"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("tcp_close: want handle")
			}
			id, ok := args[0].(*object.String)
			if !ok {
				return newError("tcp_close: handle must be string, got %s", args[0].Type())
			}
			tcpMu.Lock()
			defer tcpMu.Unlock()
			if c, found := tcpConns[id.Value]; found {
				delete(tcpConns, id.Value)
				if err := c.Close(); err != nil {
					return newError("tcp_close: %s", err.Error())
				}
				return NULL
			}
			if l, found := tcpListens[id.Value]; found {
				delete(tcpListens, id.Value)
				if err := l.Close(); err != nil {
					return newError("tcp_close: %s", err.Error())
				}
				return NULL
			}
			return newError("tcp_close: unknown handle")
		},
	}

	// ---- environment ----
	builtins["env_all"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return newError("env_all: want 0 arguments")
			}
			pairs := map[object.HashKey]object.HashPair{}
			for _, kv := range os.Environ() {
				k, v, _ := strings.Cut(kv, "=")
				ks := &object.String{Value: k}
				vs := &object.String{Value: v}
				pairs[ks.HashKey()] = object.HashPair{Key: ks, Value: vs}
			}
			return &object.Hash{Pairs: pairs}
		},
	}

	// ---- AES-GCM symmetric encryption ----
	// Keys must be 16, 24, or 32 bytes (AES-128/192/256) — same contract
	// as every AES API. Output is base64(nonce || ciphertext) with a fresh
	// random 96-bit nonce per encryption.
	builtins["aes_gcm_encrypt"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("aes_gcm_encrypt: want key, plaintext")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return newError("aes_gcm_encrypt: key must be string, got %s", args[0].Type())
			}
			pt, ok := args[1].(*object.String)
			if !ok {
				return newError("aes_gcm_encrypt: plaintext must be string, got %s", args[1].Type())
			}
			block, err := aes.NewCipher([]byte(key.Value))
			if err != nil {
				return newError("aes_gcm_encrypt: %s (key must be 16, 24, or 32 bytes)", err.Error())
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return newError("aes_gcm_encrypt: %s", err.Error())
			}
			nonce := make([]byte, gcm.NonceSize())
			if _, err := rand.Read(nonce); err != nil {
				return newError("aes_gcm_encrypt: %s", err.Error())
			}
			out := gcm.Seal(nonce, nonce, []byte(pt.Value), nil)
			return &object.String{Value: base64.StdEncoding.EncodeToString(out)}
		},
	}
	builtins["aes_gcm_decrypt"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("aes_gcm_decrypt: want key, ciphertext")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return newError("aes_gcm_decrypt: key must be string, got %s", args[0].Type())
			}
			ct, ok := args[1].(*object.String)
			if !ok {
				return newError("aes_gcm_decrypt: ciphertext must be string, got %s", args[1].Type())
			}
			block, err := aes.NewCipher([]byte(key.Value))
			if err != nil {
				return newError("aes_gcm_decrypt: %s (key must be 16, 24, or 32 bytes)", err.Error())
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return newError("aes_gcm_decrypt: %s", err.Error())
			}
			raw, err := base64.StdEncoding.DecodeString(ct.Value)
			if err != nil {
				return newError("aes_gcm_decrypt: not valid base64")
			}
			if len(raw) < gcm.NonceSize() {
				return newError("aes_gcm_decrypt: ciphertext too short")
			}
			nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
			pt, err := gcm.Open(nil, nonce, sealed, nil)
			if err != nil {
				return newError("aes_gcm_decrypt: authentication failed (wrong key or corrupted data)")
			}
			return &object.String{Value: string(pt)}
		},
	}
	// ---- Ed25519 signatures ----
	// Keys and signatures are base64 of the raw bytes — the same wire
	// format as every other Ed25519 API.
	builtins["ed25519_generate"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return newError("ed25519_generate: want 0 arguments")
			}
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return newError("ed25519_generate: %s", err.Error())
			}
			ks := &object.String{Value: "public_key"}
			vs := &object.String{Value: base64.StdEncoding.EncodeToString(pub)}
			kp := &object.String{Value: "private_key"}
			vp := &object.String{Value: base64.StdEncoding.EncodeToString(priv)}
			pairs := map[object.HashKey]object.HashPair{
				ks.HashKey(): {Key: ks, Value: vs},
				kp.HashKey(): {Key: kp, Value: vp},
			}
			return &object.Hash{Pairs: pairs}
		},
	}
	builtins["ed25519_sign"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("ed25519_sign: want private_key, data")
			}
			privB64, ok := args[0].(*object.String)
			if !ok {
				return newError("ed25519_sign: private_key must be string, got %s", args[0].Type())
			}
			data, ok := args[1].(*object.String)
			if !ok {
				return newError("ed25519_sign: data must be string, got %s", args[1].Type())
			}
			raw, err := base64.StdEncoding.DecodeString(privB64.Value)
			if err != nil || len(raw) != ed25519.PrivateKeySize {
				return newError("ed25519_sign: invalid private key")
			}
			sig := ed25519.Sign(ed25519.PrivateKey(raw), []byte(data.Value))
			return &object.String{Value: base64.StdEncoding.EncodeToString(sig)}
		},
	}
	builtins["ed25519_verify"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("ed25519_verify: want public_key, data, signature")
			}
			pubB64, ok := args[0].(*object.String)
			if !ok {
				return newError("ed25519_verify: public_key must be string, got %s", args[0].Type())
			}
			data, ok := args[1].(*object.String)
			if !ok {
				return newError("ed25519_verify: data must be string, got %s", args[1].Type())
			}
			sigB64, ok := args[2].(*object.String)
			if !ok {
				return newError("ed25519_verify: signature must be string, got %s", args[2].Type())
			}
			pubRaw, err := base64.StdEncoding.DecodeString(pubB64.Value)
			if err != nil || len(pubRaw) != ed25519.PublicKeySize {
				return newError("ed25519_verify: invalid public key")
			}
			sigRaw, err := base64.StdEncoding.DecodeString(sigB64.Value)
			if err != nil || len(sigRaw) != ed25519.SignatureSize {
				return &object.Boolean{Value: false}
			}
			return &object.Boolean{Value: ed25519.Verify(ed25519.PublicKey(pubRaw), []byte(data.Value), sigRaw)}
		},
	}
}

