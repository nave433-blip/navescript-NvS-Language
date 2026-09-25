package bridge

import (
	"encoding/json"
	"strings"
	"testing"
)

// decode parses a response envelope into a generic map.
func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("response is not valid JSON %q: %v", line, err)
	}
	return m
}

func mustOK(t *testing.T, line string) any {
	t.Helper()
	m := decode(t, line)
	if m["ok"] != true {
		t.Fatalf("want ok:true, got %s", line)
	}
	return m["result"]
}

func mustErr(t *testing.T, line string, substr string) {
	t.Helper()
	m := decode(t, line)
	if m["ok"] != false {
		t.Fatalf("want ok:false, got %s", line)
	}
	msg, _ := m["error"].(string)
	if !strings.Contains(msg, substr) {
		t.Fatalf("error %q does not contain %q (full line: %s)", msg, substr, line)
	}
}

func TestEvalArithmetic(t *testing.T) {
	s := NewSession()
	if got := mustOK(t, s.HandleLine(`{"eval": "1 + 2 * 3"}`)); got != float64(7) {
		t.Errorf("result = %v", got)
	}
}

func TestDefinitionsPersist(t *testing.T) {
	s := NewSession()
	mustOK(t, s.HandleLine(`{"eval": "fn add(a, b) { a + b }"}`))
	if got := mustOK(t, s.HandleLine(`{"call": "add", "args": [20, 22]}`)); got != float64(42) {
		t.Errorf("result = %v", got)
	}
	// Bindings persist too.
	mustOK(t, s.HandleLine(`{"eval": "let x = 7"}`))
	if got := mustOK(t, s.HandleLine(`{"eval": "x * 6"}`)); got != float64(42) {
		t.Errorf("result = %v", got)
	}
}

func TestCallBuiltin(t *testing.T) {
	s := NewSession()
	if got := mustOK(t, s.HandleLine(`{"call": "len", "args": [[1, 2, 3]]}`)); got != float64(3) {
		t.Errorf("result = %v", got)
	}
}

func TestCallErrors(t *testing.T) {
	s := NewSession()
	mustErr(t, s.HandleLine(`{"call": "nope", "args": []}`), "unknown function")
	mustOK(t, s.HandleLine(`{"eval": "let notfn = 5"}`))
	mustErr(t, s.HandleLine(`{"call": "notfn", "args": []}`), "not callable")
	mustErr(t, s.HandleLine(`{"call": "len"}`), "argument") // len needs 1 arg
}

func TestEvalErrors(t *testing.T) {
	s := NewSession()
	mustErr(t, s.HandleLine(`{"eval": "1 + "}`), "parse error")
	mustErr(t, s.HandleLine(`{"eval": "undefined_name_xyz"}`), "undefined_name_xyz")
	mustErr(t, s.HandleLine(`{"eval": "1 / 0"}`), "division by zero")
}

func TestUnconvertibleResultIsHonest(t *testing.T) {
	s := NewSession()
	mustOK(t, s.HandleLine(`{"eval": "fn f() { 1 }"}`))
	// Evaluating the bare identifier yields the function object itself.
	mustErr(t, s.HandleLine(`{"eval": "f"}`), "FUNCTION")
}

func TestMalformedLines(t *testing.T) {
	s := NewSession()
	for _, line := range []string{
		``,
		`not json`,
		`[1, 2]`,
		`{"evaluate": "1"}`,
		`{"eval": 42}`,
		`{"call": 42}`,
		`{"call": "len", "args": "nope"}`,
		`{"eval": "1"} trailing`,
		`{"eval": "1"} {"eval": "2"}`,
	} {
		mustErr(t, s.HandleLine(line), "")
	}
	// The session still works after malformed input.
	if got := mustOK(t, s.HandleLine(`{"eval": "40 + 2"}`)); got != float64(42) {
		t.Errorf("result = %v", got)
	}
}

func TestIntegersStayIntegers(t *testing.T) {
	s := NewSession()
	mustOK(t, s.HandleLine(`{"eval": "fn id(x) { x }"}`))
	// 9007199254740993 loses precision as float64; the bridge must pass
	// it through as an integer.
	line := s.HandleLine(`{"call": "id", "args": [9007199254740993]}`)
	if got := mustOK(t, line); got != float64(9007199254740993) {
		t.Errorf("big int round trip = %v", got)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatal(err)
	}
	// The raw JSON must carry the integer syntax, not 9.007199254740992e+15.
	if !strings.Contains(line, "9007199254740993") {
		t.Errorf("integer syntax not preserved in %s", line)
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	a := NewSession()
	b := NewSession()
	mustOK(t, a.HandleLine(`{"eval": "let iso = 1"}`))
	mustErr(t, b.HandleLine(`{"eval": "iso"}`), "iso")
}

func TestResultTypes(t *testing.T) {
	s := NewSession()
	if got := mustOK(t, s.HandleLine(`{"eval": "[1, \"a\", true, null]"}`)); true {
		arr, ok := got.([]any)
		if !ok || len(arr) != 4 {
			t.Fatalf("array result = %v", got)
		}
	}
	m := decode(t, s.HandleLine(`{"eval": "{\"k\": [1, 2]}"}`))
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("hash result = %v", m["result"])
	}
	inner, ok := res["k"].([]any)
	if !ok || len(inner) != 2 || inner[0] != float64(1) {
		t.Errorf("nested = %v", res)
	}
}

func TestRequestIDEchoed(t *testing.T) {
	s := NewSession()
	m := decode(t, s.HandleLine(`{"eval": "1+2", "id": 42}`))
	if m["ok"] != true || m["result"] != float64(3) {
		t.Fatalf("bad eval response: %v", m)
	}
	if m["id"] != float64(42) {
		t.Fatalf("id not echoed: %v", m)
	}
	// string ids work too, and errors echo the id as well
	m = decode(t, s.HandleLine(`{"call": "nope", "id": "req-7"}`))
	if m["ok"] != false || m["id"] != "req-7" {
		t.Fatalf("id not echoed on error: %v", m)
	}
	// no id -> no id key
	m = decode(t, s.HandleLine(`{"eval": "1"}`))
	if _, ok := m["id"]; ok {
		t.Fatalf("id key present without request id: %v", m)
	}
}

func TestSessionPersistenceAcrossCalls(t *testing.T) {
	s := NewSession()
	mustOK(t, s.HandleLine(`{"eval": "fn double(x) { return x * 2 }"}`))
	if got := mustOK(t, s.HandleLine(`{"call": "double", "args": [21]}`)); got != float64(42) {
		t.Fatalf("want 42, got %v", got)
	}
}

func TestNoSilentNulls(t *testing.T) {
	s := NewSession()
	// A function VALUE is not JSON-convertible: honest error, not null.
	mustErr(t, s.HandleLine(`{"eval": "fn f() { } f"}`), "cannot convert NvS FUNCTION to JSON")
}

func TestExecDiscardsNonJSONValue(t *testing.T) {
	s := NewSession()
	// A source ending in a class declaration: eval would fail converting
	// the CLASS value to JSON, but exec must succeed and return null.
	src := `fn add(a, b) { a + b }
class Dog { fn bark() { return "woof" } }`
	if got := mustOK(t, s.HandleLine(`{"exec": `+quoteJSON(src)+`}`)); got != nil {
		t.Errorf("exec result = %v, want null", got)
	}
	// Definitions from exec are visible to later calls.
	if got := mustOK(t, s.HandleLine(`{"call": "add", "args": [20, 22]}`)); got != float64(42) {
		t.Errorf("result = %v, want 42", got)
	}
	// exec propagates runtime errors honestly.
	mustErr(t, s.HandleLine(`{"exec": "1 + "}`), "error")
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
