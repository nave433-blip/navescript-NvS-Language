#!/usr/bin/env python3
"""Build NvS host binary from a polyglot bootstrap (Python driver)."""
import os, subprocess, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "bin", "nvs")

def main():
    os.makedirs(os.path.join(ROOT, "bin"), exist_ok=True)
    env = os.environ.copy()
    env.setdefault("GOTOOLCHAIN", "local")
    env.setdefault("GOPROXY", "off")
    env.setdefault("GOSUMDB", "off")
    r = subprocess.run(
        ["go", "build", "-mod=mod", "-o", OUT, "./cmd/nvs/"],
        cwd=ROOT, env=env, capture_output=True, text=True,
    )
    if r.returncode != 0:
        print(r.stderr or r.stdout, file=sys.stderr)
        sys.exit(r.returncode)
    print(f"built {OUT}")
    v = subprocess.run([OUT, "version"], capture_output=True, text=True)
    print(v.stdout.strip())
    # self-host smoke
    smoke = 'import "stdlib/selfhost/mini_eval.ns"\nprint mini_eval("let x = 6 * 7 print x")\n'
    open(os.path.join(ROOT, "/tmp/selfhost_smoke.ns"), "w")  # may fail
    sm = os.path.join(ROOT, "examples", "selfhost.ns")
    if os.path.isfile(sm):
        r2 = subprocess.run([OUT, "run", sm], cwd=ROOT, capture_output=True, text=True)
        print("selfhost:", "OK" if r2.returncode == 0 and "SELFHOST OK" in r2.stdout else r2.stdout[-200:])
        sys.exit(r2.returncode)
    sys.exit(0)

if __name__ == "__main__":
    main()
