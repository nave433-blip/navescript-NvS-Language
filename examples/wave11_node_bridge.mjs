// Wave 11 — drive NvS from Node.js through the JSON stdio bridge.
//
// Usage:
//   node examples/wave11_node_bridge.mjs [/path/to/nvs]
//
// This is the tested Node integration path: spawn `nvs bridge` once and
// exchange line-delimited JSON. (The ffi-napi C-ABI route is documented in
// docs/POLYGLOT.md but requires an npm build step; this file is the path
// with live test coverage.)

import { spawn } from "node:child_process";
import { createInterface } from "node:readline";
import assert from "node:assert";

const NVS = process.argv[2] || "nvs";

class NvsBridge {
  constructor(nvsBin) {
    this.nextId = 0;
    this.pending = new Map();
    this.proc = spawn(nvsBin, ["bridge"], { stdio: ["pipe", "pipe", "inherit"] });
    const rl = createInterface({ input: this.proc.stdout, crlfDelay: Infinity });
    rl.on("line", (line) => {
      let resp;
      try {
        resp = JSON.parse(line);
      } catch (e) {
        for (const [, p] of this.pending) p.reject(new Error("bad JSON from nvs: " + line));
        this.pending.clear();
        return;
      }
      const p = this.pending.get(resp.id);
      if (p) {
        this.pending.delete(resp.id);
        if (resp.ok) p.resolve(resp.result);
        else p.reject(new Error("NvS: " + resp.error));
      }
    });
    this.proc.on("exit", (code) => {
      for (const [, p] of this.pending) p.reject(new Error(`nvs exited (${code})`));
      this.pending.clear();
    });
  }

  _send(payload) {
    const id = ++this.nextId;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.proc.stdin.write(JSON.stringify({ ...payload, id }) + "\n", (err) => {
        if (err) {
          this.pending.delete(id);
          reject(err);
        }
      });
    });
  }

  eval(src) {
    return this._send({ eval: src });
  }

  call(name, args = []) {
    return this._send({ call: name, args });
  }

  async close() {
    this.proc.stdin.end();
    await new Promise((res) => this.proc.on("close", res));
  }
}

// ---- live self-test ----
const bridge = new NvsBridge(NVS);
try {
  await bridge.eval(`fn add(a, b) { return a + b }
fn shout(s) { return s + "!" }
fn pack() { return {"nums": [1, 2], "ok": true} }`);

  assert.strictEqual(await bridge.call("add", [20, 22]), 42);
  assert.strictEqual(await bridge.call("shout", ["hello"]), "hello!");
  assert.deepStrictEqual(await bridge.call("pack"), { nums: [1, 2], ok: true });

  // errors propagate as JS exceptions
  await assert.rejects(bridge.call("nope"), /unknown function/);
  await assert.rejects(bridge.eval("1 +"), /parse error/);

  console.log("NODE BRIDGE: ALL OK");
} finally {
  await bridge.close();
}
