package eval

// Wave 22 tests: sequence tools, statistics, math, encoding, string
// shaping, and resilient scripting builtins.

import (
	"testing"
)

func TestWave22Scan(t *testing.T) {
	expectInspect(t, `scan([1,2,3,4], fn(a,b){ a+b })`, `[1, 3, 6, 10]`)
	expectInspect(t, `scan([1,2,3], 10, fn(a,b){ a+b })`, `[11, 13, 16]`)
	expectInspect(t, `scan([], fn(a,b){ a+b })`, `[]`)
	expectInspect(t, `scan([], 5, fn(a,b){ a+b })`, `[5]`)
	expectErrorContains(t, `scan(1, fn(a,b){a+b})`, "want array")
}

func TestWave22Partition(t *testing.T) {
	expectInspect(t, `partition([1,2,3,4], fn(x){ x % 2 == 0 })`, `[[2, 4], [1, 3]]`)
	expectInspect(t, `partition([], fn(x){ true })`, `[[], []]`)
}

func TestWave22TakeDropWhile(t *testing.T) {
	expectInspect(t, `take_while([1,2,3,1], fn(x){ x < 3 })`, `[1, 2]`)
	expectInspect(t, `drop_while([1,2,3,1], fn(x){ x < 3 })`, `[3, 1]`)
	expectInspect(t, `take_while([5,6], fn(x){ x < 3 })`, `[]`)
	expectInspect(t, `drop_while([5,6], fn(x){ x < 3 })`, `[5, 6]`)
}

func TestWave22FlatMap(t *testing.T) {
	expectInspect(t, `flat_map([1,2,3], fn(x){ [x, x*10] })`, `[1, 10, 2, 20, 3, 30]`)
	expectInspect(t, `flat_map([], fn(x){ [x] })`, `[]`)
	expectErrorContains(t, `flat_map([1], fn(x){ x })`, "must return array")
}

func TestWave22Windows(t *testing.T) {
	expectInspect(t, `windows([1,2,3,4], 2)`, `[[1, 2], [2, 3], [3, 4]]`)
	expectInspect(t, `windows([1,2], 3)`, `[]`)
	expectErrorContains(t, `windows([1,2], 0)`, "positive integer")
}

func TestWave22Stats(t *testing.T) {
	expectInspect(t, `median([3,1,2])`, `2`)
	expectInspect(t, `median([1,2,3,4])`, `2.5`)
	expectInspect(t, `median([1.5, 2.5])`, `2`)
	expectInspect(t, `variance([2,4,4,4,5,5,7,9])`, `4`)
	expectInspect(t, `stddev([2,4,4,4,5,5,7,9])`, `2`)
	expectInspect(t, `mode([1,2,2,3,3,3])`, `3`)
	expectInspect(t, `mode(["a","b","a"])`, `a`)
	expectErrorContains(t, `median([])`, "empty array")
	expectErrorContains(t, `median([1,"x"])`, "want array of numbers")
}

func TestWave22Math(t *testing.T) {
	for _, p := range []string{"2", "3", "17", "7919", "2305843009213693951"} {
		expectInspect(t, `is_prime(`+p+`)`, `true`)
	}
	for _, c := range []string{"1", "4", "9", "100", "0", "-7"} {
		expectInspect(t, `is_prime(`+c+`)`, `false`)
	}
	expectInspect(t, `lerp(0, 10, 0.5)`, `5`)
	expectInspect(t, `lerp(10, 20, 0.25)`, `12.5`)
	expectErrorContains(t, `is_prime("x")`, "want integer")
}

func TestWave22Encoding(t *testing.T) {
	expectInspect(t, `hex_encode("hello")`, `68656c6c6f`)
	expectInspect(t, `hex_decode(hex_encode("NvS-2026"))`, `NvS-2026`)
	expectErrorContains(t, `hex_decode("zz")`, "hex_decode")
	// gzip round-trips through the aes_gcm base64-string convention.
	expectInspect(t, `gzip_decompress(gzip_compress("the quick brown fox"))`, `the quick brown fox`)
	expectInspect(t, `gzip_decompress(gzip_compress(""))`, ``)
}

func TestWave22Template(t *testing.T) {
	expectInspect(t, `render_template("Hello {{name}}, you have {{n}} msgs", {"name": "Nave", "n": 3})`, `Hello Nave, you have 3 msgs`)
	expectInspect(t, `render_template("Hi {{missing}}!", {})`, `Hi !`)
	expectInspect(t, `render_template("{{ a }}-{{b}}", {"a": 1, "b": 2})`, `1-2`)
}

func TestWave22CaseShaping(t *testing.T) {
	expectInspect(t, `slugify("Hello, World! 2026")`, `hello-world-2026`)
	expectInspect(t, `slugify("  --a__b--  ")`, `a-b`)
	expectInspect(t, `snake_case("helloWorld")`, `hello_world`)
	expectInspect(t, `snake_case("HTTPRequest")`, `http_request`)
	expectInspect(t, `snake_case("hello-world_2")`, `hello_world_2`)
	expectInspect(t, `camel_case("hello_world")`, `helloWorld`)
	expectInspect(t, `camel_case("hello-world")`, `helloWorld`)
	expectInspect(t, `kebab_case("helloWorld")`, `hello-world`)
	expectInspect(t, `kebab_case("hello_world")`, `hello-world`)
}

func TestWave22Levenshtein(t *testing.T) {
	expectInspect(t, `levenshtein("kitten", "sitting")`, `3`)
	expectInspect(t, `levenshtein("", "")`, `0`)
	expectInspect(t, `levenshtein("abc", "abc")`, `0`)
	expectInspect(t, `levenshtein("abc", "")`, `3`)
}

func TestWave22Semver(t *testing.T) {
	expectInspect(t, `semver_compare("1.2.3", "1.2.4")`, `-1`)
	expectInspect(t, `semver_compare("2.0.0", "1.9.9")`, `1`)
	expectInspect(t, `semver_compare("1.0.0", "1.0.0")`, `0`)
	expectInspect(t, `semver_compare("v1.2.3", "1.2.3")`, `0`)
	expectInspect(t, `semver_compare("1.0.0-alpha", "1.0.0")`, `-1`)
	expectInspect(t, `semver_compare("1.0.0-alpha", "1.0.0-beta")`, `-1`)
	expectInspect(t, `semver_compare("1.0.0-alpha.1", "1.0.0-alpha.2")`, `-1`)
}

func TestWave22CSVStringify(t *testing.T) {
	expectInspect(t, `csv_stringify([["a","b"],["c","d"]])`, "a,b\nc,d\n")
	expectInspect(t, `csv_stringify([["a,b","c\"d"]])`, "\"a,b\",\"c\"\"d\"\n")
	expectInspect(t, `csv_stringify([[1,2.5,true]])`, "1,2.5,true\n")
	// simple values round-trip through the naive csv_parse reader
	expectInspect(t, `csv_parse(csv_stringify([["x","y"],["1","2"]]))`, `[[x, y], [1, 2]]`)
}

func TestWave22Retry(t *testing.T) {
	// Succeeds on the second attempt.
	expectInspect(t, `
let n = 0
let f = fn() { n = n + 1; if (n < 2) { throw "boom" }; return "ok" }
retry(f, 3, 0)
`, `ok`)
	// All attempts fail: the last error surfaces.
	expectErrorContains(t, `retry(fn(){ throw "always" }, 3, 0)`, "always")
	expectErrorContains(t, `retry(fn(){ 1 }, 0, 0)`, "positive integer")
	// Zero-delay retry still works.
	expectInspect(t, `retry(fn(){ 42 }, 1)`, `42`)
}
