#!/usr/bin/env node
const { spawnSync } = require("child_process");
const path = require("path");
const root = path.join(__dirname, "..");
const env = { ...process.env, GOTOOLCHAIN: "local", GOPROXY: "off", GOSUMDB: "off" };
const out = path.join(root, "bin", "nvs");
let r = spawnSync("go", ["build", "-mod=mod", "-o", out, "./cmd/nvs/"], { cwd: root, env, encoding: "utf8" });
if (r.status !== 0) { console.error(r.stderr || r.stdout); process.exit(r.status || 1); }
console.log("built", out);
r = spawnSync(out, ["version"], { encoding: "utf8" });
console.log((r.stdout || "").trim());
r = spawnSync(out, ["run", "examples/selfhost.ns"], { cwd: root, encoding: "utf8" });
console.log(r.status === 0 ? "selfhost OK" : r.stderr || r.stdout);
process.exit(r.status || 0);
