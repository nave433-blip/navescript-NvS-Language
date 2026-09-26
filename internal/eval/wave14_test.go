package eval

// Wave 14 tests:
//  1. for-in loops whose final iteration hits `continue` must not leak the
//     Continue signal object as the loop's value (the while/C-for fix from
//     the reconcile did not cover the six for-in cases).
//  2. Relative imports resolve against the importing file's directory,
//     falling back to the working directory (and nested imports resolve
//     against their own file).
//  3. Every stdlib/tests/test_*.nvs suite evaluates cleanly.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func wave14Eval(t *testing.T, src string) object.Object {
	t.Helper()
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors for %q: %v", src, p.Errors())
	}
	got := Eval(prog, env)
	if isError(got) {
		t.Fatalf("eval error for %q: %s", src, got.Inspect())
	}
	return got
}

func TestWave14ForInContinueNoLeak(t *testing.T) {
	// Each loop's last iteration hits `continue`; the loop value must be
	// null, never the string "continue" or a signal object.
	cases := []struct{ src, want string }{
		{"for (x in [1, 2]) {\n if (x > 0) { continue }\n}", "null"},
		{"for (x in (1, 2)) {\n if (x > 0) { continue }\n}", "null"},
		{"for (c in \"ab\") {\n if (c != \"\") { continue }\n}", "null"},
		{"for (k in {\"a\": 1}) {\n if (k != \"\") { continue }\n}", "null"},
		{"fn f() {\n let out = []\n for (x in [1, 2, 3]) {\n  if (x < 9) { continue }\n  out = push(out, x)\n }\n return out\n}\nf()", "[]"},
		// A function whose for-in ends on continue must return its own value.
		{"fn f() {\n for (x in [1]) { continue }\n return 42\n}\nf()", "42"},
	}
	for _, c := range cases {
		got := wave14Eval(t, c.src)
		if got.Inspect() != c.want {
			t.Errorf("loop value = %s (%s), want %s\nsrc:\n%s", got.Inspect(), got.Type(), c.want, c.src)
		}
	}
}

func TestWave14ImportRelativeToImporter(t *testing.T) {
	dir := t.TempDir()
	// sub/helper.nvs imports ./nested/deep.nvs (nested resolution).
	mustWrite := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "sub", "nested", "deep.nvs"),
		"fn deep_value() { return 7 }\n")
	mustWrite(filepath.Join(dir, "sub", "helper.nvs"),
		"import \"./nested/deep.nvs\"\nfn helper_value() { return deep_value() * 2 }\n")
	mustWrite(filepath.Join(dir, "main.nvs"),
		"import \"./sub/helper.nvs\"\nhelper_value()\n")

	old := CurrentFile
	CurrentFile = filepath.Join(dir, "main.nvs")
	defer func() { CurrentFile = old }()

	src, err := os.ReadFile(filepath.Join(dir, "main.nvs"))
	if err != nil {
		t.Fatal(err)
	}
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(string(src))
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	got := Eval(prog, env)
	if isError(got) {
		t.Fatalf("eval error: %s", got.Inspect())
	}
	if got.Inspect() != "14" {
		t.Errorf("nested relative import = %s, want 14", got.Inspect())
	}
}

func TestWave14StdlibSuites(t *testing.T) {
	files, err := filepath.Glob("../../stdlib/tests/test_*.nvs")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no stdlib test files found")
	}
	old := CurrentFile
	defer func() { CurrentFile = old }()
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			abs, err := filepath.Abs(f)
			if err != nil {
				t.Fatal(err)
			}
			CurrentFile = abs // so ../<module>.nvs resolves against the suite
			env := object.NewEnvironment()
			LoadPrelude(env)
			l := lexer.New(string(src))
			p := parser.New(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parse errors: %v", p.Errors())
			}
			got := Eval(prog, env)
			if isError(got) {
				t.Fatalf("eval error: %s", got.Inspect())
			}
			// Suites print FAIL: lines and throw on any failed check, so
			// reaching here without an error means every check passed.
		})
	}
}
