#!/usr/bin/env python3
"""python_embed.py — embed NvS in Python via the C ABI (libnvs.so + ctypes).

This is the FAST path: in-process calls, no subprocess, no JSON stdio.
The other two Python paths:
  * `nvs bindgen --to=python file.nvs -o mod.py` — generates a typed
    Python module for one NvS file (goes through the JSON bridge).
  * `nvs bridge` — drive NvS as a subprocess from any language.

Setup (from the repo root):
  go build -buildmode=c-shared -o examples/python_embed/libnvs.so ./cbridge
  python3 examples/python_embed.py

Run with:  NVS_LIB=/path/to/libnvs.so python3 examples/python_embed.py
"""
import ctypes
import json
import os
import sys

LIB = os.environ.get(
    "NVS_LIB",
    os.path.join(os.path.dirname(os.path.abspath(__file__)), "libnvs.so"),
)

if not os.path.exists(LIB):
    sys.exit(
        f"libnvs.so not found at {LIB}\n"
        "build it first: go build -buildmode=c-shared "
        "-o examples/python_embed/libnvs.so ./cbridge"
    )

nvs = ctypes.CDLL(LIB)
# NB: restype must be c_void_p, NOT c_char_p. With c_char_p ctypes copies
# the C string into a Python bytes object and drops the original pointer,
# so nvs_free would receive a pointer to the copy's buffer -> abort.
# With c_void_p we keep the raw pointer, read via string_at, then free.
nvs.nvs_eval.restype = ctypes.c_void_p
nvs.nvs_eval.argtypes = [ctypes.c_char_p]
nvs.nvs_call.restype = ctypes.c_void_p
nvs.nvs_call.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
nvs.nvs_free.restype = None
nvs.nvs_free.argtypes = [ctypes.c_void_p]


class NvSError(Exception):
    """An error reported by NvS itself."""


def _envelope(raw: bytes):
    env = json.loads(raw.decode("utf-8"))
    if env.get("ok"):
        return env["result"]
    raise NvSError(env.get("error", "unknown nvs error"))


def nvs_eval(src: str):
    """Evaluate NvS source in the persistent interpreter."""
    ptr = nvs.nvs_eval(src.encode("utf-8"))
    try:
        return _envelope(ctypes.string_at(ptr))
    finally:
        nvs.nvs_free(ptr)


def nvs_call(name: str, args):
    """Call a named NvS function with a JSON-encodable arg list."""
    ptr = nvs.nvs_call(name.encode("utf-8"), json.dumps(args).encode("utf-8"))
    try:
        return _envelope(ctypes.string_at(ptr))
    finally:
        nvs.nvs_free(ptr)


def check(cond, label):
    print(("ok: " if cond else "FAIL: ") + label)
    if not cond:
        check.failed += 1


check.failed = 0

# 1. eval an expression
check(nvs_eval("6 * 7") == 42, "eval 6*7 -> 42")

# 2. define a function, then call it (persistent environment)
nvs_eval("fn pyadd(a, b) { a + b }")
check(nvs_call("pyadd", [20, 22]) == 42, "call pyadd(20,22) -> 42")

# 3. strings and nested values round-trip
check(nvs_eval('"hello" + " " + "python"') == "hello python",
      "eval string concat")
check(nvs_call("len", [[1, 2, 3]]) == 3, "call builtin len")
check(nvs_eval('{"a": 1, "b": [true, null]}') == {"a": 1, "b": [True, None]},
      "eval nested hash round-trip")

# 4. errors propagate as NvSError
try:
    nvs_call("no_such_fn_xyz", [])
    check(False, "call unknown fn raises")
except NvSError as e:
    check("no_such_fn_xyz" in str(e), "call unknown fn -> NvSError")
try:
    nvs_eval("1 + ")
    check(False, "eval bad syntax raises")
except NvSError:
    check(True, "eval bad syntax -> NvSError")

if check.failed:
    print(f"{check.failed} CHECK(S) FAILED")
    sys.exit(1)
print("ALL PYTHON EMBED CHECKS PASSED")
