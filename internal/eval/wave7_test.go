package eval

// Wave 7 tests: standard library robbery — datetime, fuller HTTP, crypto,
// base64url, subprocess, env, path, TOML subset, gzip. At least one
// assertion per new builtin; error paths covered too.
//
// NOTE: NvS programs below are written exactly as they would appear in a
// .ns file (Go backtick strings): quotes are plain `"`, NvS `\n` escapes
// are literal backslash-n, hash-literal keys are quoted (bare keys in a
// hash *value* evaluate as identifiers), and `and`/`==` need explicit
// parens (NvS gives `and` the same precedence as `==`).

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/navescript/nvs/internal/object"
)

// ---- datetime ----

func TestWave7NowISOFormat(t *testing.T) {
	got := testEval(t, `now_iso()`)
	s, ok := got.(*object.String)
	if !ok {
		t.Fatalf("now_iso() returned %s, want string", got.Type())
	}
	matched, err := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, s.Value)
	if err != nil || !matched {
		t.Fatalf("now_iso() = %q, want RFC3339 UTC like 2026-09-25T13:52:00Z", s.Value)
	}
}

func TestWave7UnixtimeMsBounds(t *testing.T) {
	expectInspect(t, `
let a = now()
let b = unixtime_ms()
let ok = (b >= a * 1000) and (b < (a + 10) * 1000)
ok`, `true`)
}

func TestWave7DateFormatFixed(t *testing.T) {
	want := time.Unix(946684800, 0).Format("2006-01-02 15:04")
	expectInspect(t, `date_format(946684800, "2006-01-02 15:04")`, want)
}

func TestWave7ParseDateRFC3339(t *testing.T) {
	want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	expectInspect(t, fmt.Sprintf(`parse_date("2026-01-02T00:00:00Z")`), fmt.Sprintf("%d", want))
}

func TestWave7ParseDateBareDate(t *testing.T) {
	want, _ := time.ParseInLocation("2006-01-02", "2026-03-04", time.Local)
	expectInspect(t, fmt.Sprintf(`parse_date("2026-03-04")`), fmt.Sprintf("%d", want.Unix()))
}

func TestWave7ParseDateGarbage(t *testing.T) {
	expectErrorContains(t, `parse_date("not a date at all")`, "unrecognized date format")
}

func TestWave7DateAdd(t *testing.T) {
	expectInspect(t, `date_add(0, 1, "days")`, `86400`)
	expectInspect(t, `date_add(100, 2, "weeks")`, fmt.Sprintf("%d", 100+2*604800))
	expectInspect(t, `date_add(0, -1, "h")`, `-3600`)
	expectInspect(t, `date_add(0, 90, "minutes")`, `5400`)
	expectErrorContains(t, `date_add(0, 1, "fortnights")`, "unknown unit")
}

// ---- fuller HTTP (httptest, no internet needed) ----

func wave7TestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Echo", string(body))
		w.Header().Add("X-Multi", "one")
		w.Header().Add("X-Multi", "two")
		if r.URL.Path == "/created" {
			w.WriteHeader(201)
		}
		fmt.Fprintf(w, "%s:%s", r.Method, string(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWave7HTTPRequestFull(t *testing.T) {
	srv := wave7TestServer(t)
	code := fmt.Sprintf(`
let r = http_request("PUT", "%s/created", {"headers": {"X-Test": "yes"}, "body": "ping", "timeout": 5})
let ok = (r["status"] == 201) and (r["body"] == "PUT:ping") and (r["headers"]["X-Echo"][0] == "ping")
ok`, srv.URL)
	expectInspect(t, code, `true`)
}

func TestWave7HTTPRequestMultiHeaders(t *testing.T) {
	srv := wave7TestServer(t)
	code := fmt.Sprintf(`
let r = http_request("GET", "%s")
let ok = (r["status"] == 200) and (len(r["headers"]["X-Multi"]) == 2) and (r["headers"]["X-Multi"][0] == "one") and (r["headers"]["X-Multi"][1] == "two")
ok`, srv.URL)
	expectInspect(t, code, `true`)
}

func TestWave7HTTPRequestUnknownOpt(t *testing.T) {
	expectErrorContains(t, `http_request("GET", "http://127.0.0.1/", {"bogus": 1})`, "unknown opt")
}

func TestWave7HTTPRequestConnRefused(t *testing.T) {
	// Nothing listens here: an honest runtime error, not a panic.
	expectErrorContains(t, `http_request("GET", "http://127.0.0.1:1/", {"timeout": 1})`, "http_request:")
}

// ---- crypto: the missing hashes ----

func TestWave7SHA1KnownVector(t *testing.T) {
	expectInspect(t, `sha1("abc") == "a9993e364706816aba3e25717850c26c9cd0d89d"`, `true`)
}

func TestWave7HMACSHA256KnownVector(t *testing.T) {
	// Wikipedia HMAC-SHA256 test vector.
	expectInspect(t,
		`hmac_sha256("key", "The quick brown fox jumps over the lazy dog") == "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"`,
		`true`)
}

func TestWave7SHA1TypeError(t *testing.T) {
	expectErrorContains(t, `sha1(42)`, "must be string")
}

// ---- base64 url-safe variants ----

func TestWave7Base64URLKnown(t *testing.T) {
	want := base64.RawURLEncoding.EncodeToString([]byte("hello?+/="))
	expectInspect(t, `base64url_encode("hello?+/=")`, want)
}

func TestWave7Base64URLRoundTrip(t *testing.T) {
	expectInspect(t, `base64url_decode(base64url_encode("NvS rocks?!")) == "NvS rocks?!"`, `true`)
}

func TestWave7Base64URLBadInput(t *testing.T) {
	expectErrorContains(t, `base64url_decode("***not-base64***")`, "base64url_decode:")
}

// ---- subprocess ----

func TestWave7ExecEcho(t *testing.T) {
	expectInspect(t, `
let r = exec("echo", "hi")
let ok = (r["code"] == 0) and (r["stdout"] == "hi\n") and (r["stderr"] == "")
ok`, `true`)
}

func TestWave7ExecNoShell(t *testing.T) {
	// No shell expansion: the glob stays literal.
	expectInspect(t, `
let r = exec("echo", "*.ns")
let ok = (r["code"] == 0) and (r["stdout"] == "*.ns\n")
ok`, `true`)
}

func TestWave7ExecNonZeroExit(t *testing.T) {
	expectInspect(t, `exec("false")["code"]`, `1`)
	expectInspect(t, `exec("true")["code"]`, `0`)
}

func TestWave7ExecStderrSeparated(t *testing.T) {
	expectInspect(t, `
let r = sh("echo oops >&2")
let ok = (r["code"] == 0) and (r["stdout"] == "") and (r["stderr"] == "oops\n")
ok`, `true`)
}

func TestWave7ShPipeline(t *testing.T) {
	expectInspect(t, `sh("printf hello | tr a-z A-Z")["stdout"]`, `HELLO`)
}

func TestWave7ShExitCode(t *testing.T) {
	expectInspect(t, `sh("exit 3")["code"]`, `3`)
}

func TestWave7ExecNotFound(t *testing.T) {
	expectErrorContains(t, `exec("nvs-no-such-command-xyz")`, "exec:")
}

func TestWave7ExecArgsMustBeStrings(t *testing.T) {
	expectErrorContains(t, `exec("echo", 42)`, "must be strings")
}

// ---- env ----

func TestWave7EnvDefault(t *testing.T) {
	expectInspect(t, `env("NVS_WAVE7_DEFINITELY_UNSET", "fallback")`, `fallback`)
}

func TestWave7EnvSetWinsOverDefault(t *testing.T) {
	t.Setenv("NVS_WAVE7_SET_ME", "v7-value")
	expectInspect(t, `env("NVS_WAVE7_SET_ME", "fallback")`, `v7-value`)
}

func TestWave7EnvOneArgUnchanged(t *testing.T) {
	t.Setenv("NVS_WAVE7_SET_ME", "v7-value")
	expectInspect(t, `env("NVS_WAVE7_SET_ME")`, `v7-value`)
	expectInspect(t, `env("NVS_WAVE7_DEFINITELY_UNSET")`, ``)
}

func TestWave7EnvDefaultMustBeString(t *testing.T) {
	expectErrorContains(t, `env("NVS_WAVE7_DEFINITELY_UNSET", 5)`, "default must be string")
}

// ---- path ----

func TestWave7Extname(t *testing.T) {
	expectInspect(t, `extname("dir/file.ns")`, `.ns`)
	expectInspect(t, `extname("archive.tar.gz")`, `.gz`)
	expectInspect(t, `extname("Makefile")`, ``)
}

func TestWave7AbsPath(t *testing.T) {
	want, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	expectInspect(t, `abs_path(".")`, want)
}

// ---- TOML subset ----

func TestWave7TomlBasic(t *testing.T) {
	prog := `
let toml = "title = \"hello\"\n" +
  "# a comment\n" +
  "[server]\n" +
  "host = \"localhost\"\n" +
  "port = 8080\n" +
  "tls = false\n" +
  "ratio = 1.5\n" +
  "[server.limits]\n" +
  "max = 100\n" +
  "[db]\n" +
  "ports = [5432, 5433,]\n" +
  "name = 'literal'\n" +
  "dotted.key = true\n"
let c = toml_parse(toml)
let ok = (c["title"] == "hello") and
(c["server"]["host"] == "localhost") and
(c["server"]["port"] == 8080) and
(c["server"]["tls"] == false) and
(c["server"]["ratio"] == 1.5) and
(c["server"]["limits"]["max"] == 100) and
(c["db"]["ports"][0] == 5432) and (c["db"]["ports"][1] == 5433) and (len(c["db"]["ports"]) == 2) and
(c["db"]["name"] == "literal") and
(c["db"]["dotted"]["key"] == true)
ok`
	expectInspect(t, prog, `true`)
}

func TestWave7TomlEscapesAndNumbers(t *testing.T) {
	// The NvS string below produces TOML source containing a real \n escape
	// (backslash-n) inside the quoted string, plus a real newline after it.
	prog := `
let c = toml_parse("s = \"a\\nb\"\nhex = 0xFF\noct = 0o17\nbig = 1_000\nf = 2.5e3\n")
let ok = (c["s"] == "a\nb") and (c["hex"] == 255) and (c["oct"] == 15) and (c["big"] == 1000) and (c["f"] == 2500.0)
ok`
	expectInspect(t, prog, `true`)
}

func TestWave7TomlDuplicateKey(t *testing.T) {
	expectErrorContains(t, `toml_parse("a = 1\na = 2\n")`, "duplicate key")
}

func TestWave7TomlRedefinedTable(t *testing.T) {
	expectErrorContains(t, `toml_parse("[a]\n[a]\n")`, "defined twice")
}

func TestWave7TomlBadValue(t *testing.T) {
	expectErrorContains(t, `toml_parse("a = {b = 1}\n")`, "bad value")
}

func TestWave7TomlArrayOfTablesRejected(t *testing.T) {
	expectErrorContains(t, `toml_parse("[[x]]\n")`, "not supported")
}

func TestWave7TomlMultilineArrayRejected(t *testing.T) {
	expectErrorContains(t, `toml_parse("a = [1,\n2]\n")`, "not supported")
}

func TestWave7TomlEmpty(t *testing.T) {
	expectInspect(t, `len(toml_parse("# nothing here\n"))`, `0`)
}

// ---- compression ----

func TestWave7GzipRoundTrip(t *testing.T) {
	expectInspect(t, `gzip_decompress(gzip_compress("hello gzip world")) == "hello gzip world"`, `true`)
}

func TestWave7GzipRoundTripEmpty(t *testing.T) {
	expectInspect(t, `gzip_decompress(gzip_compress("")) == ""`, `true`)
}

func TestWave7GzipActuallyCompresses(t *testing.T) {
	prog := `
let s = ""
for (let i = 0; i < 500; i = i + 1) { s = s + "ab" }
len(gzip_compress(s)) < len(s)`
	expectInspect(t, prog, `true`)
}

func TestWave7GzipGarbageErrors(t *testing.T) {
	expectErrorContains(t, `gzip_decompress("definitely not gzip data")`, "gzip_decompress:")
}

func TestWave7GzipTypeError(t *testing.T) {
	expectErrorContains(t, `gzip_compress(42)`, "want string")
}

// Keep os imported for the env tests on all platforms.
var _ = os.Getenv
