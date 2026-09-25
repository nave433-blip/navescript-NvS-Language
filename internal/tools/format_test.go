package tools

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// runAndCapture runs NvS source through the interpreter and returns stdout.
func runAndCapture(t *testing.T, src string) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	func() {
		defer func() { os.Stdout = old }()
		env := object.NewEnvironment()
		eval.LoadPrelude(env)
		p := parser.New(lexer.New(src))
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Fatalf("parse errors: %v", p.Errors())
		}
		result := eval.Eval(prog, env)
		eval.DrainSpawnedTasks()
		if result != nil && result.Type() == object.ERROR_OBJ {
			t.Fatalf("eval error: %s", result.Inspect())
		}
		w.Close()
	}()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestFormatIdempotent(t *testing.T) {
	nasty := "fn  main( ){\n\tlet\t x=1\n\n\n\tif(x>0){\n\tprint(\"brace { in string } and ${x + \"}\"}\") // } comment {\n\t}\n\n\n}\n\t\tmain( )\n"
	once := Format(nasty)
	twice := Format(once)
	if once != twice {
		t.Fatalf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	if !strings.HasSuffix(once, "\n") || strings.HasSuffix(once, "\n\n") {
		t.Fatalf("must end with exactly one newline: %q", once)
	}
	for _, line := range strings.Split(once, "\n") {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			t.Fatalf("trailing whitespace survived: %q", line)
		}
		if strings.HasPrefix(line, "\t") {
			t.Fatalf("tab indent survived: %q", line)
		}
	}
	// The string content must be byte-identical.
	if !strings.Contains(once, `print("brace { in string } and ${x + "}"}")`) {
		t.Fatalf("string literal was corrupted:\n%s", once)
	}
}

func TestFormatPreservesStringsAndComments(t *testing.T) {
	src := `let s = "}{"
let t = "${ {a: 1} }"
/* block {
   comment } */
let x = 1 // trailing } {
`
	out := Format(src)
	if !strings.Contains(out, `let s = "}{"`) {
		t.Fatalf("string with braces corrupted:\n%s", out)
	}
	if !strings.Contains(out, `/* block {`) || !strings.Contains(out, `   comment } */`) {
		t.Fatalf("block comment corrupted:\n%s", out)
	}
	// Braces inside the string/comment must not change indentation of
	// following lines.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "let x") && strings.HasPrefix(l, " ") {
			t.Fatalf("line %d wrongly indented by braces in string/comment: %q", i, l)
		}
	}
}

func TestFormatMultilineStringVerbatim(t *testing.T) {
	src := "let s = \"a   \n   b\"\nprint(s)\n"
	out := Format(src)
	if out != src {
		t.Fatalf("multi-line string lines must be verbatim:\n%q\nvs\n%q", out, src)
	}
	if Format(out) != out {
		t.Fatal("multi-line string case not idempotent")
	}
}

func TestFormatBlankCollapseAndIndent(t *testing.T) {
	src := "\n\n\nlet x = 1\n\n\n\nif (x) {\nprint(x)\n}\n"
	want := "let x = 1\n\nif (x) {\n    print(x)\n}\n"
	if got := Format(src); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatParenContinuation(t *testing.T) {
	src := "print(\n1,\n2\n)\n"
	out := Format(src)
	// Deterministic and idempotent; exact style is our choice.
	if Format(out) != out {
		t.Fatalf("not idempotent:\n%s", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "print(" || lines[3] != ")" {
		t.Fatalf("unexpected shape:\n%s", out)
	}
}

func TestFormatBehaviorPreserved(t *testing.T) {
	programs := map[string]string{
		"arith":     "let x = 1\nprint(x + 2 * 3)\n",
		"control":   "for (let i = 0; i < 3; i = i + 1) {\nif (i % 2 == 0) {\nprint(\"even\")\n} else {\nprint(\"odd\")\n}\n}\n",
		"strings":   "let m = {\"a\": 1, \"b\": [1, 2]}\nlet k = m[\"b\"][1]\nprint(\"val ${k}!\")\nprint(m[\"b\"][1])\n",
		"fn":        "fn add(a, b) {\nreturn a + b\n}\nprint(add(2, 3))\n",
		"match":     "let v = 2\nlet r = match (v) {\ncase 1: \"one\"\ncase 2: \"two\"\ndefault: \"other\"\n}\nprint(r)\n",
		"class":     "class A {\ninit(x) {\nthis.x = x\n}\nget() {\nreturn this.x\n}\n}\nlet a = new A(7)\nprint(a.get())\n",
		"interp":    "let n = \"world\"\nprint(\"hello ${n}, ${1 + 2}\")\n",
		"try":       "try {\nthrow \"boom\"\n} catch (e) {\nprint(\"caught \" + e)\n} finally {\nprint(\"done\")\n}\n",
		"whileElse": "let i = 0\nwhile (i < 2) {\ni = i + 1\n} else {\nprint(\"no break\")\n}\n",
	}
	for name, src := range programs {
		before := runAndCapture(t, src)
		after := runAndCapture(t, Format(src))
		if before != after {
			t.Errorf("%s: behavior changed\nbefore: %q\nafter:  %q\nformatted:\n%s", name, before, after, Format(src))
		}
		if Format(Format(src)) != Format(src) {
			t.Errorf("%s: not idempotent", name)
		}
	}
}

func TestFormatBehaviorPreservedOnExamples(t *testing.T) {
	// A few real, deterministic example files (kept small and side-effect free).
	files := []string{
		"../../examples/wave9_fmt_bad.ns", // created below; skip if absent
		"../../examples/wave1_ranges.ns",
		"../../examples/wave1_pipeline.ns",
		"../../examples/wave4_expr.ns",
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			if strings.Contains(f, "wave9") {
				t.Skip("wave9 example not created yet")
			}
			t.Fatal(err)
		}
		src := string(data)
		before := runAndCapture(t, src)
		formatted := Format(src)
		if Format(formatted) != formatted {
			t.Errorf("%s: not idempotent", f)
		}
		after := runAndCapture(t, formatted)
		if before != after {
			t.Errorf("%s: behavior changed\nbefore: %q\nafter: %q", f, before, after)
		}
	}
}

func TestFormatEmpty(t *testing.T) {
	if Format("") != "" {
		t.Fatal("empty input should format to empty")
	}
	if Format("\n\n  \n") != "" {
		t.Fatal("whitespace-only input should format to empty")
	}
}
