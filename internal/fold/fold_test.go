// Tests for Wave 15 code folding: Python/JS -> NvS importers and nvs extract.
//
// The importers are honest-subset converters: every test either asserts exact
// emitted NvS for a supported construct or asserts a LOUD UnsupportedError
// naming the construct and line for an unsupported one. Silent acceptance of
// an unsupported construct is the failure mode these tests exist to catch.
package fold

import (
	"strings"
	"testing"
)

func mustImport(t *testing.T, src, lang string) string {
	t.Helper()
	out, err := ImportSource(src, lang)
	if err != nil {
		t.Fatalf("import %s failed: %v\nsrc:\n%s", lang, err, src)
	}
	return out
}

func mustUnsupported(t *testing.T, src, lang, wantConstruct string, wantLine int) {
	t.Helper()
	_, err := ImportSource(src, lang)
	if err == nil {
		t.Fatalf("import %s of unsupported construct succeeded, want loud error\nsrc:\n%s", lang, src)
	}
	ue, ok := err.(*UnsupportedError)
	if !ok {
		t.Fatalf("import %s error = %T (%v), want *UnsupportedError\nsrc:\n%s", lang, err, err, src)
	}
	if ue.Construct != wantConstruct {
		t.Errorf("construct = %q, want %q", ue.Construct, wantConstruct)
	}
	if ue.Line != wantLine {
		t.Errorf("line = %d, want %d", ue.Line, wantLine)
	}
	if !strings.Contains(ue.Error(), wantConstruct) || !strings.Contains(ue.Error(), "line") {
		t.Errorf("error message %q does not name construct and line", ue.Error())
	}
}

// ---------------------------------------------------------------- Python ---

func TestPyFunctionDefaults(t *testing.T) {
	out := mustImport(t, "def greet(name, punct=\"!\"):\n    return \"hi \" + name + punct\n", "python")
	want := "fn greet(name, punct=\"!\") {\n  return ((\"hi \" + name) + punct)\n}"
	if strings.TrimSpace(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestPyIfElifElse(t *testing.T) {
	src := "if a:\n    x = 1\nelif b:\n    x = 2\nelse:\n    x = 3\n"
	out := mustImport(t, src, "python")
	for _, want := range []string{"if (a) {", "} else if (b) {", "} else {", "let x = 1", "x = 2", "x = 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPyForRangeAndWhile(t *testing.T) {
	src := "for i in range(3):\n    print(i)\nwhile n > 0:\n    n = n - 1\n"
	out := mustImport(t, src, "python")
	if !strings.Contains(out, "for (i in range(3))") {
		t.Errorf("for-range wrong:\n%s", out)
	}
	if !strings.Contains(out, "while ((n > 0))") {
		t.Errorf("while wrong:\n%s", out)
	}
}

func TestPyListConcatAndStrRepeat(t *testing.T) {
	out := mustImport(t, "a = xs + [1, 2]\nb = [0] + ys\nc = \"ab\" * 3\n", "python")
	for _, want := range []string{
		"let a = [...xs, 1, 2]",
		"let b = [0, ...ys]",
		`let c = repeat("ab", 3)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPyInOperator(t *testing.T) {
	out := mustImport(t, "print(1 in [1, 2])\nprint(\"b\" in \"abc\")\nprint(\"k\" in {\"k\": 1})\nprint(9 not in [1])\n", "python")
	for _, want := range []string{
		"(index_of([1, 2], 1) != -1)",
		`(index_of("abc", "b") != -1)`,
		`(index_of(keys({"k": 1}), "k") != -1)`,
		"(!((index_of([1], 9) != -1)))",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPyFString(t *testing.T) {
	out := mustImport(t, "print(f\"n={n}\")\n", "python")
	if !strings.Contains(out, `print("n=${n}")`) {
		t.Errorf("f-string wrong:\n%s", out)
	}
}

func TestPyTupleSwap(t *testing.T) {
	out := mustImport(t, "a, b = b, a\n", "python")
	for _, want := range []string{"let __py1", "let __py2", "let a = __py1", "let b = __py2"} {
		if !strings.Contains(out, want) {
			t.Errorf("swap wrong, missing %q:\n%s", want, out)
		}
	}
}

func TestPyLoudRejections(t *testing.T) {
	cases := []struct {
		src       string
		construct string
		line      int
	}{
		{"x = 1\nclass A:\n    pass\n", "class definition", 2},
		{"import os\n", "import statement", 1},
		{"from x import y\n", "import statement", 1},
		{"xs = [x * 2 for x in ys]\n", "list comprehension", 1},
		{"d = {k: v for k, v in items}\n", "dict comprehension", 1},
		{"def f():\n    global x\n", "global declaration", 2},
		{"with open(\"f\") as fh:\n    pass\n", "with statement", 1},
		{"try:\n    pass\nexcept:\n    pass\n", "try/except", 1},
		{"@deco\ndef f():\n    pass\n", "decorator", 1},
		{"def g():\n    yield 1\n", "yield", 2},
		{"x = [1] * 3\n", "list repetition", 1},
		{"print(a < b < c)\n", "chained comparison", 1},
	}
	for _, c := range cases {
		mustUnsupported(t, c.src, "python", c.construct, c.line)
	}
}

// --------------------------------------------------------------------- JS ---

func TestJsBasics(t *testing.T) {
	src := "function add(a, b = 1) {\n  return a + b;\n}\n" +
		"const sq = (n) => n * n;\n" +
		"let total = 0;\n" +
		"for (let i = 1; i <= 3; i++) {\n  total += add(i);\n}\n" +
		"console.log(sq(total));\n"
	out := mustImport(t, src, "js")
	for _, want := range []string{
		"fn add(a, b=1) {",
		"const sq = fn(n) { return (n * n) }",
		"for (let i = 1; (i <= 3); i = i + 1) {",
		"total = (total + (add(i)))",
		"print(sq(total))",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJsTemplateAndMethods(t *testing.T) {
	out := mustImport(t, "console.log(`hi ${name}!`);\nlet s = words.join(\"-\");\nlet t = s.toUpperCase();\n", "js")
	for _, want := range []string{
		`print("hi ${name}!")`,
		`let s = join(words, "-")`,
		`let t = upper(s)`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJsStringConcatCoercion(t *testing.T) {
	out := mustImport(t, "console.log(1 + 2 + \"x\");\nconsole.log(\"x\" + 1 + 2);\n", "js")
	for _, want := range []string{
		`print((str((1 + 2)) + "x"))`,
		`print((("x" + str(1)) + str(2)))`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestJsLoudRejections(t *testing.T) {
	cases := []struct {
		src       string
		construct string
		line      int
	}{
		{"class A {}\n", "class", 1},
		{"import x from \"y\";\n", "import statement", 1},
		{"export const x = 1;\n", "export statement", 1},
		{"try {\n} catch (e) {\n}\n", "try/catch", 1},
		{"switch (x) {\n}\n", "switch", 1},
		{"throw \"boom\";\n", "throw statement", 1},
		{"let o = new Foo();\n", "new", 1},
		{"let g = function* () {};\n", "generator function", 1},
		{"async function f() {}\n", "async function", 1},
		{"let r = /ab+c/;\n", "regex literal", 1},
		{"x?.y\n", "optional chaining", 1},
		{"console.warn(\"x\");\n", "method call .warn()", 1},
	}
	for _, c := range cases {
		mustUnsupported(t, c.src, "js", c.construct, c.line)
	}
}

// ---------------------------------------------------------------- extract ---

func TestExtractBlocks(t *testing.T) {
	src := "# @nvs\nlet a = 1\n# @endnvs\nx = 2\n// @nvs\nlet b = 2\n// @endnvs\n"
	blocks, err := ExtractBlocks(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Code != "let a = 1\n" || blocks[0].StartLine != 2 || blocks[0].EndLine != 2 {
		t.Errorf("block 1 wrong: %+v", blocks[0])
	}
	if blocks[1].Code != "let b = 2\n" || blocks[1].StartLine != 6 {
		t.Errorf("block 2 wrong: %+v", blocks[1])
	}
}

func TestExtractUnbalanced(t *testing.T) {
	for _, src := range []string{
		"# @nvs\nlet a = 1\n",              // never closed
		"# @endnvs\n",                      // closer without opener
		"# @nvs\n# @nvs\n# @endnvs\n",       // nested
		"# @nvs\nlet a = 1\n# @endnvs\n# @endnvs\n", // extra closer
	} {
		if _, err := ExtractBlocks(src); err == nil {
			t.Errorf("ExtractBlocks(%q) succeeded, want loud unbalanced-marker error", src)
		}
	}
}
