package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportSource(t *testing.T) {
	src := `fn add(a, b) { return a + b }
fn greet(name = "world") { return "hello" }
fn gen() { yield 1 }
const PI = 3.14
let counter = 0
class Dog { fn bark() { return "woof" } }
enum Color { Red, Green }
`
	exp, err := ExportSource(src)
	if err != nil {
		t.Fatalf("ExportSource: %v", err)
	}
	if len(exp.Functions) != 3 {
		t.Fatalf("want 3 functions, got %d", len(exp.Functions))
	}
	add := exp.Functions[0]
	if add.Name != "add" || add.Arity != 2 || add.Required != 2 || add.Kind != "function" {
		t.Fatalf("bad add export: %+v", add)
	}
	greet := exp.Functions[1]
	if greet.Required != 0 || greet.Params[0].Default == nil {
		t.Fatalf("bad greet export: %+v", greet)
	}
	if exp.Functions[2].Kind != "generator" {
		t.Fatalf("gen should be marked generator: %+v", exp.Functions[2])
	}
	if len(exp.Classes) != 1 || exp.Classes[0].Name != "Dog" || len(exp.Classes[0].Methods) != 1 {
		t.Fatalf("bad class export: %+v", exp.Classes)
	}
	if len(exp.Enums) != 1 || len(exp.Enums[0].Members) != 2 {
		t.Fatalf("bad enum export: %+v", exp.Enums)
	}
	if len(exp.Constants) != 2 || !exp.Constants[0].Const {
		t.Fatalf("bad constants export: %+v", exp.Constants)
	}
	// JSON shape check
	out, err := ExportSourceJSON(src)
	if err != nil {
		t.Fatalf("ExportSourceJSON: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("exports JSON invalid: %v", err)
	}
	for _, k := range []string{"functions", "classes", "enums", "constants"} {
		if _, ok := doc[k]; !ok {
			t.Fatalf("exports JSON missing key %q", k)
		}
	}
}

func TestExportSourceParseError(t *testing.T) {
	if _, err := ExportSource("fn broken( {"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestBindgenPython(t *testing.T) {
	src := "fn add(a, b) { return a + b }\nfn greet(name = \"world\") { return name }\n"
	gen, err := Bindgen("python", "calc.ns", src)
	if err != nil {
		t.Fatalf("Bindgen: %v", err)
	}
	for _, want := range []string{
		"class Calc:", "def add(self, a, b):", "def greet(self, name=\"world\"):",
		"class NvSError", "nvs bridge", "_NVS_SOURCE",
	} {
		if !strings.Contains(gen, want) {
			t.Fatalf("generated python missing %q", want)
		}
	}
	if _, err := Bindgen("ruby", "calc.ns", src); err == nil {
		t.Fatal("expected error for unsupported target")
	}
}
