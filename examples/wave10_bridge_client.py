#!/usr/bin/env python3
"""wave10_bridge_client.py — Wave 10 polyglot interop.

Drives `nvs bridge` (the JSON stdio bridge) from Python: one persistent
NvS interpreter on the other end of a pipe, functions defined by one
request visible to later requests.

Usage:
    go build -o /tmp/nvs ./cmd/nvs/
    python3 examples/wave10_bridge_client.py /tmp/nvs
"""
import json
import subprocess
import sys


class NvSBridge:
    def __init__(self, nvs_bin):
        self.proc = subprocess.Popen(
            [nvs_bin, "bridge"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            text=True,
            bufsize=1,
        )

    def _request(self, req):
        self.proc.stdin.write(json.dumps(req) + "\n")
        line = self.proc.stdout.readline()
        if not line:
            raise RuntimeError("bridge closed the pipe")
        resp = json.loads(line)
        if not resp.get("ok"):
            raise RuntimeError("nvs error: " + resp.get("error", "?"))
        return resp["result"]

    def eval(self, src):
        return self._request({"eval": src})

    def call(self, name, args):
        return self._request({"call": name, "args": args})

    def close(self):
        self.proc.stdin.close()
        self.proc.wait()


def main():
    if len(sys.argv) != 2:
        print("usage: wave10_bridge_client.py <nvs-binary>", file=sys.stderr)
        sys.exit(1)
    b = NvSBridge(sys.argv[1])
    try:
        print("eval 1+2*3 ->", b.eval("1 + 2 * 3"))
        # Definitions persist across requests in the one session.
        b.eval("fn add(a, b) { a + b }")
        print("call add(20, 22) ->", b.call("add", [20, 22]))
        b.eval("let nums = [1, 2, 3]")
        print("call len(nums) ->", b.call("len", [b.eval("nums")]))
        print("nested ->", b.eval('{"a": [1, true, null]}'))
        try:
            b.eval("1 +")
        except RuntimeError as e:
            print("parse error (honest) ->", e)
        try:
            b.call("nope", [])
        except RuntimeError as e:
            print("unknown fn (honest) ->", e)
    finally:
        b.close()


if __name__ == "__main__":
    main()
