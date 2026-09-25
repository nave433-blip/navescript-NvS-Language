#!/usr/bin/env python3
"""Wave 10 proof: drive NvS from Python through the C ABI (libnvs.so).

Builds the shared library if needed, then uses ctypes to call nvs_eval /
nvs_call / nvs_free and asserts real results — including persistence
(functions defined by one nvs_eval call are visible to later calls),
the JSON error envelope, and nvs_free discipline.

Usage:
    python3 examples/wave10_ctypes.py [path/to/libnvs.so]

Build the library first (done automatically if missing):
    go build -buildmode=c-shared -o libnvs.so ./cbridge
"""
import ctypes
import json
import os
import subprocess
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LIB = sys.argv[1] if len(sys.argv) > 1 else os.path.join(REPO, "libnvs.so")

if not os.path.exists(LIB):
    print(f"building {LIB} ...")
    subprocess.run(
        ["go", "build", "-buildmode=c-shared", "-o", LIB, "./cbridge"],
        cwd=REPO, check=True,
        env={**os.environ, "PATH": os.environ.get("PATH", "") + ":" + os.path.expanduser("~/go-dist/go/bin")},
    )

lib = ctypes.CDLL(LIB)

# char* nvs_eval(const char* src)
lib.nvs_eval.argtypes = [ctypes.c_char_p]
lib.nvs_eval.restype = ctypes.c_void_p
# char* nvs_call(const char* func_name, const char* args_json)
lib.nvs_call.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
lib.nvs_call.restype = ctypes.c_void_p
# void nvs_free(char* s)
lib.nvs_free.argtypes = [ctypes.c_void_p]
lib.nvs_free.restype = None


def _take(ptr):
    """Take ownership of a char* returned by the bridge: decode it, then free it."""
    if not ptr:
        raise RuntimeError("bridge returned NULL")
    try:
        return ctypes.cast(ptr, ctypes.c_char_p).value.decode("utf-8")
    finally:
        lib.nvs_free(ptr)


def nvs_eval(src):
    return json.loads(_take(lib.nvs_eval(src.encode("utf-8"))))


def nvs_call(name, args):
    return json.loads(_take(lib.nvs_call(name.encode("utf-8"), json.dumps(args).encode("utf-8"))))


def check(label, got, want):
    status = "ok" if got == want else "MISMATCH"
    print(f"[{status}] {label}: got={got!r} want={want!r}")
    assert got == want, label


# 1. basic eval
r = nvs_eval("1 + 2 * 3")
check("eval arithmetic envelope", r, {"ok": True, "result": 7})

# 2. persistence: define a function, see it from a later call
r = nvs_eval("fn add(a, b) { return a + b }")
check("define fn envelope", r["ok"], True)
r = nvs_eval("add(40, 2)")
check("persistent fn visible to later nvs_eval", r, {"ok": True, "result": 42})

# 3. nvs_call with JSON args
r = nvs_call("add", [20, 22])
check("nvs_call add", r, {"ok": True, "result": 42})

# 4. nvs_call on a builtin (len) with nested values
r = nvs_call("len", [[1, 2, 3]])
check("nvs_call builtin len", r, {"ok": True, "result": 3})
r = nvs_eval('{"name": "nvs", "tags": ["a", "b"], "meta": {"n": 1, "ok": true, "nothing": null}}')
check("hash/array/bool/null conversion", r,
      {"ok": True, "result": {"name": "nvs", "tags": ["a", "b"],
                              "meta": {"n": 1, "ok": True, "nothing": None}}})

# 5. error envelope: runtime error
r = nvs_eval("nosuchvar + 1")
check("error envelope ok flag", r["ok"], False)
assert "nosuchvar" in r["error"] or "identifier not found" in r["error"], r
print(f"[ok] runtime error message: {r['error']!r}")

# 6. error envelope: parse error
r = nvs_eval("let = = =")
check("parse error envelope ok flag", r["ok"], False)
print(f"[ok] parse error message: {r['error']!r}")

# 7. error envelope: calling something unknown / not callable
r = nvs_call("nope", [])
check("unknown function ok flag", r["ok"], False)
r = nvs_eval("let x = 5")
r = nvs_call("x", [])
check("not-callable ok flag", r["ok"], False)
print(f"[ok] not-callable message: {r['error']!r}")

# 8. honest conversion error: functions are not JSON values
r = nvs_eval("fn f() { return 1 } f")
check("function value envelope ok flag", r["ok"], False)
assert "FUNCTION" in r["error"], r
print(f"[ok] non-convertible type message: {r['error']!r}")

print("\nAll ctypes bridge assertions passed.")
