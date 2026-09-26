# NvS Packages — `nvs pkg`

Honest premise: **there is no central NvS package registry.** GitHub is the
registry. `nvs pkg` installs versioned source trees from git repositories (or
local paths) into a local cache, records them in a lockfile, and teaches
`import "name"` to resolve through that cache.

## Quick start

```bash
nvs pkg init mylib              # scaffold nvs.json + main.nvs
nvs pkg install nave433/mylib   # from GitHub (default branch)
nvs pkg install nave433/mylib@v1.2.0   # pinned tag (must match manifest version)
nvs pkg install ./mylib         # local path (dev loop)
nvs get nave433/mylib           # alias for install
nvs pkg list
nvs pkg remove mylib
nvs pkg publish                 # validate + print manual release steps
```

Then in NvS code:

```nvs
import "mylib"            // → <cache>/mylib@<ver>/<main from nvs.json>
import "mylib/extra.nvs"  // → <cache>/mylib@<ver>/extra.nvs
```

## Manifest (`nvs.json`)

```json
{
  "name": "mylib",
  "version": "1.2.0",
  "description": "does useful things",
  "main": "main.nvs",
  "deps": { "otherlib": "nave433/otherlib@0.9.0" }
}
```

- `name`: lowercase letters, digits, `-`, `_`; required.
- `version`: semver `MAJOR.MINOR.PATCH`; required.
- `main`: entry file for `import "name"` (default `main.nvs`).
- `deps`: map of package name → install spec; installed transitively with
  cycle detection (a dependency cycle is a loud error, not a hang).

## Cache, lockfile, resolution

- Cache root: `~/.nvs/packages` (override with `$NVS_PKG_CACHE`).
- Every install appends to `./nvs.lock`: `{name: {version, source, commit}}`.
  `nvs pkg install` with no args replays the lockfile (`InstallFromLock`).
- `import "name"` resolution order: current directory's `nvs.lock` pin →
  single installed version → highest semver. Unresolvable names are a normal
  import error, never silent.

## Publishing

`nvs pkg publish` validates the manifest and the entry file, then prints the
manual steps: commit, tag the version (`git tag 1.2.0`), push with tags.
Keep the manifest version and the git tag in sync — the installer enforces it.

## Limitations (stated plainly)

- Needs `git` and network access for GitHub specs; failures surface git's
  stderr verbatim.
- No concurrent-install locking — fine for single-user CLI use.
- Lockfile pinning is cwd-scoped (run `nvs` from the project directory).
- Windows cache paths are untested (no Windows hardware in CI yet); the code
  is path-separator agnostic.
