# NvS Changelog

## Unreleased — Wave 22 (draft release notes)

Language additions (all backward compatible; no existing behavior changed):

**New builtins (20)** — `internal/eval/wave22.go`
- Sequences (Python itertools / Ruby / JS / Rust): `scan` (cumulative fold,
  `itertools.accumulate`), `partition`, `take_while`, `drop_while`,
  `flat_map`, `windows` (sliding windows)
- Statistics (Python statistics): `median`, `variance` (population),
  `stddev`, `mode`
- Math: `is_prime` (deterministic Miller-Rabin, exact for all int64),
  `lerp`
- Encoding: `hex_encode` / `hex_decode`, `gzip_compress` / `gzip_decompress`
  (base64-string transport, same convention as `aes_gcm_encrypt`)
- Strings: `render_template` (`{{key}}` Mustache-lite), `slugify`,
  `snake_case`, `camel_case`, `kebab_case` (lodash-style word splitting),
  `levenshtein`, `semver_compare` (npm ordering, -1/0/1)
- Data: `csv_stringify` (RFC 4180 quoting; writer half of `csv_parse`),
  `retry(fn, attempts[, delay_ms])` (tenacity-lite)

**New stdlib module** — `stdlib/argparse.nvs` (pure NvS, Python argparse
counterpart): `ap_new` / `ap_add_arg` / `ap_add_flag` / `ap_parse` /
`ap_help`. Long (`--name value`, `--name=value`), short (`-n value`,
`-n=value`, `-n5`), combined shorts (`-ab`), `--` terminator, positionals,
`int`/`bool`/`string`/`flag` types, `choices`, `required`, defaults,
`-h/--help`. Tests: `stdlib/tests/test_argparse.nvs` (run with
`nvs stdlib/tests/test_argparse.nvs`).

**Installer / build**
- Build-time version stamping: `go build -ldflags "-X main.version=…"` for
  the CLI and `-X github.com/navescript/nvs/internal/eval.versionStamp=…`
  for the language runtime (`nvs_version()` / `nvs_info()`). Plain
  `go build` keeps the `2.1.0` default.
- New `make release` target: stamps both from `git describe --tags --always`.
- Fixed `Makefile` `build` target: it compiled `./cmd/nvs/main.go` as a lone
  file (missing package files); now builds the `./cmd/nvs` package.
- `make release` skips `nvm` with a note when `./cmd/nvm` has no Go sources
  (pre-existing upstream gap, unchanged).

**Examples**: `examples/wave22_seq.ns` showcases the new builtins.

**Tests**: 14 new Go test functions (`internal/eval/wave22_test.go`);
full suite green (525 PASS, 0 FAIL across `./internal/...`);
`examples/language_features.ns` → LANG_FEATURES OK;
`examples/full_regression.ns` → REGRESSION OK;
`go vet` clean on linux and windows; `GOOS=windows go build` clean.

**Notes**
- No external dependencies added (Go standard library only).
- The SQLite-as-first-external-dependency question remains Evan's call —
  untouched.
- Gemini CLI was unavailable this wave (free-tier quota exhausted); the
  loop ran as solo senior review. Re-run the review prompts once quota
  resets for the adversarial second-opinion pass.
