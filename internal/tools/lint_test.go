package tools

import (
	"strings"
	"testing"
)

func lintRules(src string) []string {
	findings, perr := LintSource("test.ns", src)
	if len(perr) > 0 {
		panic("parse errors: " + strings.Join(perr, "; "))
	}
	var rules []string
	for _, f := range findings {
		rules = append(rules, f.Rule)
	}
	return rules
}

func hasRule(rules []string, want string) bool {
	for _, r := range rules {
		if r == want {
			return true
		}
	}
	return false
}

func TestLintUnusedBinding(t *testing.T) {
	// Positive: declared, never referenced.
	rules := lintRules("let unused_var = 42\nprint(\"hi\")\n")
	if !hasRule(rules, RuleUnusedBinding) {
		t.Fatal("expected unused-binding finding")
	}
	// Negative: used.
	rules = lintRules("let used_var = 42\nprint(used_var)\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("false positive on used var: %v", rules)
	}
	// Negative: assignment counts as a mention (no false positive).
	rules = lintRules("let w = 1\nw = 2\nprint(\"x\")\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("false positive on assigned var: %v", rules)
	}
	// Negative: use inside interpolation.
	rules = lintRules("let name = \"nave\"\nprint(\"hi ${name}\")\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("false positive on interpolated var: %v", rules)
	}
	// Negative: named fn declarations are exempt (entry points / library APIs).
	rules = lintRules("fn helper(a) {\nreturn a\n}\nprint(\"hi\")\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("named fn should be exempt: %v", rules)
	}
	// Negative: function params are excluded.
	rules = lintRules("let f = fn(unused_param) {\nreturn 1\n}\nprint(f(2))\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("function params must be excluded: %v", rules)
	}
	// Negative: member property names are not variable uses, but the
	// object itself is used — no finding for the object.
	rules = lintRules("let obj = {x: 1}\nprint(obj.x)\n")
	if hasRule(rules, RuleUnusedBinding) {
		t.Fatalf("false positive on used object: %v", rules)
	}
	// Positive: destructured names are bindings too.
	rules = lintRules("let [a, b] = [1, 2]\nprint(a)\n")
	if !hasRule(rules, RuleUnusedBinding) {
		t.Fatal("expected unused-binding for destructured b")
	}
	// Positive: const.
	rules = lintRules("const c = 1\nprint(2)\n")
	if !hasRule(rules, RuleUnusedBinding) {
		t.Fatal("expected unused-binding for const")
	}
	// Positive: typed let.
	rules = lintRules("let t: int = 1\nprint(2)\n")
	if !hasRule(rules, RuleUnusedBinding) {
		t.Fatal("expected unused-binding for typed let")
	}
	// Line numbers point at the declaration.
	findings, _ := LintSource("f.ns", "print(1)\nlet lonely = 2\nprint(3)\n")
	found := false
	for _, fd := range findings {
		if fd.Rule == RuleUnusedBinding && fd.Line == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("unused-binding should be on line 2: %+v", findings)
	}
}

func TestLintShadowBuiltin(t *testing.T) {
	// Positive: let shadows builtin.
	rules := lintRules("let len = 5\nprint(len)\n")
	if !hasRule(rules, RuleShadowBuiltin) {
		t.Fatal("expected shadow-builtin for `let len`")
	}
	// Positive: plain assignment shadows builtin.
	rules = lintRules("len = 5\nprint(len)\n")
	if !hasRule(rules, RuleShadowBuiltin) {
		t.Fatal("expected shadow-builtin for `len = 5`")
	}
	// Negative: ordinary names are fine.
	rules = lintRules("let mylen = 5\nprint(mylen)\n")
	if hasRule(rules, RuleShadowBuiltin) {
		t.Fatalf("false positive: %v", rules)
	}
	// Negative: member assignment is not shadowing.
	rules = lintRules("let o = {}\no.len = 5\nprint(o.len)\n")
	if hasRule(rules, RuleShadowBuiltin) {
		t.Fatalf("false positive on member assign: %v", rules)
	}
	// Severity is warning.
	findings, _ := LintSource("f.ns", "let print = 1\n")
	for _, fd := range findings {
		if fd.Rule == RuleShadowBuiltin && fd.Severity != SeverityWarning {
			t.Fatalf("shadow-builtin must be warning, got %s", fd.Severity)
		}
	}
}

func TestLintUnreachable(t *testing.T) {
	// Positive: after return.
	rules := lintRules("fn f() {\nreturn 1\nprint(\"dead\")\n}\nprint(f())\n")
	if !hasRule(rules, RuleUnreachable) {
		t.Fatal("expected unreachable-code after return")
	}
	// Positive: after break / continue.
	rules = lintRules("for (let i = 0; i < 3; i = i + 1) {\nbreak\nprint(i)\n}\n")
	if !hasRule(rules, RuleUnreachable) {
		t.Fatal("expected unreachable-code after break")
	}
	rules = lintRules("for (let i = 0; i < 3; i = i + 1) {\ncontinue\nprint(i)\n}\n")
	if !hasRule(rules, RuleUnreachable) {
		t.Fatal("expected unreachable-code after continue")
	}
	// Positive: after throw.
	rules = lintRules("fn g() {\nthrow \"x\"\nprint(\"dead\")\n}\n")
	if !hasRule(rules, RuleUnreachable) {
		t.Fatal("expected unreachable-code after throw")
	}
	// Negative: return inside a nested if does not terminate the outer block.
	rules = lintRules("fn h(x) {\nif (x) {\nreturn 1\n}\nprint(\"alive\")\nreturn 2\n}\nprint(h(false))\n")
	if hasRule(rules, RuleUnreachable) {
		t.Fatalf("false positive: nested return must not poison outer block: %v", rules)
	}
	// Negative: nothing after return.
	rules = lintRules("fn k() {\nprint(1)\nreturn 2\n}\nprint(k())\n")
	if hasRule(rules, RuleUnreachable) {
		t.Fatalf("false positive: %v", rules)
	}
}

func TestLintNullComparison(t *testing.T) {
	rules := lintRules("let x = null\nif (x == null) {\nprint(1)\n}\n")
	if !hasRule(rules, RuleNullCompare) {
		t.Fatal("expected null-comparison for ==")
	}
	rules = lintRules("let y = 1\nif (y != null) {\nprint(1)\n}\n")
	if !hasRule(rules, RuleNullCompare) {
		t.Fatal("expected null-comparison for !=")
	}
	// Negative: null on neither side.
	rules = lintRules("let z = 1\nif (z == 2) {\nprint(1)\n}\n")
	if hasRule(rules, RuleNullCompare) {
		t.Fatalf("false positive: %v", rules)
	}
	// Message suggests is_null().
	findings, _ := LintSource("f.ns", "let x = null\nprint(x == null)\n")
	for _, fd := range findings {
		if fd.Rule == RuleNullCompare {
			if fd.Severity != SeverityStyle {
				t.Fatalf("null-comparison must be style, got %s", fd.Severity)
			}
			if !strings.Contains(fd.Message, "is_null") {
				t.Fatalf("message should suggest is_null(): %s", fd.Message)
			}
		}
	}
}

func TestLintParseError(t *testing.T) {
	findings, perr := LintSource("bad.ns", "let = =\n")
	if len(perr) == 0 {
		t.Fatal("expected parser errors")
	}
	if !hasRule(func() []string {
		var r []string
		for _, f := range findings {
			r = append(r, f.Rule)
		}
		return r
	}(), RuleParseError) {
		t.Fatal("expected parse-error finding")
	}
}

func TestLintCleanFile(t *testing.T) {
	src := "let x = 1\nprint(x + 1)\n"
	findings, perr := LintSource("clean.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(findings) != 0 {
		t.Fatalf("clean file should have no findings: %+v", findings)
	}
}

func TestLintJSON(t *testing.T) {
	findings, _ := LintSource("f.ns", "let q = 1\n")
	js := FindingsJSON(findings)
	if !strings.Contains(js, `"rule": "unused-binding"`) {
		t.Fatalf("JSON should contain the rule: %s", js)
	}
	if FindingsJSON(nil) != "[]\n" {
		t.Fatal("nil findings should render as []")
	}
}

func TestFindingStringFormat(t *testing.T) {
	fd := Finding{File: "a.ns", Line: 3, Rule: RuleUnusedBinding, Severity: SeverityWarning, Message: "m"}
	if fd.String() != "a.ns:3: warning unused-binding: m" {
		t.Fatalf("bad format: %s", fd.String())
	}
}
