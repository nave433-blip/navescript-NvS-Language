package debug

import (
	"bytes"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/eval"
)

// runScripted runs code under a scripted debugger session and returns
// the full transcript (debugger output + captured program output).
func runScripted(t *testing.T, code, commands string) string {
	t.Helper()
	in := strings.NewReader(commands)
	var out bytes.Buffer
	sess := New(in, &out)
	if err := sess.RunCode(code, "test"); err != nil {
		t.Fatalf("RunCode: %v", err)
	}
	// The debugger must always uninstall itself.
	if eval.ActiveDebugger != nil {
		t.Fatalf("ActiveDebugger not restored after RunCode")
	}
	return out.String()
}

func mustContain(t *testing.T, transcript, want string) {
	t.Helper()
	if !strings.Contains(transcript, want) {
		t.Fatalf("transcript missing %q\n--- transcript ---\n%s", want, transcript)
	}
}

func mustNotContain(t *testing.T, transcript, want string) {
	t.Helper()
	if strings.Contains(transcript, want) {
		t.Fatalf("transcript unexpectedly contains %q\n--- transcript ---\n%s", want, transcript)
	}
}

// Breakpoint, locals, and continue: pause at line 3, inspect locals,
// resume to end of program.
func TestBreakpointLocalsContinue(t *testing.T) {
	code := "let x = 1\nlet y = 2\nlet z = x + y\nprint(z)\n"
	transcript := runScripted(t, code, "break 3\ncontinue\nlocals\ncontinue\n")

	mustContain(t, transcript, "breakpoint at line 3 added")
	mustContain(t, transcript, "stopped at line 3")
	// Paused BEFORE line 3 executes, so x and y are bound but z is not.
	mustContain(t, transcript, "x = 1")
	mustContain(t, transcript, "y = 2")
	mustNotContain(t, transcript, "z = 3")
	// Program ran to completion after the last continue: its own
	// print output is captured into the transcript.
	mustContain(t, transcript, "(nvsdb) 3\n")
}

// Step stops at consecutive statements.
func TestStep(t *testing.T) {
	code := "let a = 10\nlet b = 20\nlet c = a + b\nprint(c)\n"
	transcript := runScripted(t, code, "break 1\ncontinue\nstep\nstep\ncontinue\n")

	mustContain(t, transcript, "stopped at line 1")
	mustContain(t, transcript, "stopped at line 2")
	mustContain(t, transcript, "stopped at line 3")
}

// print evaluates an expression in the paused environment.
func TestPrintExpr(t *testing.T) {
	code := "let x = 41\nlet y = x + 1\nprint(y)\n"
	transcript := runScripted(t, code, "break 2\ncontinue\np x + 1\ncontinue\n")

	mustContain(t, transcript, "stopped at line 2")
	// p x + 1 -> 42 from the paused env, and the program's own
	// print(y) -> 42 after resuming: 42 appears at least twice.
	if got := strings.Count(transcript, "(nvsdb) 42"); got < 2 {
		t.Fatalf("expected >= 2 occurrences of 42 (print-cmd + program), got %d\n%s", got, transcript)
	}
}

// Backtrace inside nested calls shows innermost frame first.
func TestBacktrace(t *testing.T) {
	code := "fn inner() {\n  let q = 1\n  print(q)\n}\nfn outer() {\n  inner()\n}\nouter()\n"
	transcript := runScripted(t, code, "break 2\ncontinue\nbt\ncontinue\n")

	mustContain(t, transcript, "stopped at line 2")
	mustContain(t, transcript, "#0 inner")
	mustContain(t, transcript, "#1 outer")
	mustContain(t, transcript, "<toplevel>")
}

// next steps over a call instead of into it.
func TestNextStepsOverCall(t *testing.T) {
	code := "fn f() {\n  let v = 99\n  print(v)\n}\nlet a = 1\nf()\nprint(a)\n"
	// line 6 is the f() call; `next` there must land on line 7,
	// never stopping inside f (lines 2-3).
	transcript := runScripted(t, code, "break 6\ncontinue\nnext\ncontinue\n")

	mustContain(t, transcript, "stopped at line 6")
	mustContain(t, transcript, "stopped at line 7")
	mustNotContain(t, transcript, "stopped at line 2")
	mustNotContain(t, transcript, "stopped at line 3")
}

// quit stops the program immediately: later prints never happen.
func TestQuit(t *testing.T) {
	code := "print(\"one\")\nprint(\"two\")\nprint(\"three\")\n"
	transcript := runScripted(t, code, "break 2\ncontinue\nquit\n")

	mustContain(t, transcript, "one")
	mustContain(t, transcript, "quit")
	mustNotContain(t, transcript, "two")
	mustNotContain(t, transcript, "three")
}

// A bare `break` lists breakpoints; repeating it toggles one off.
func TestBreakListAndToggle(t *testing.T) {
	code := "let x = 1\nprint(x)\n"
	transcript := runScripted(t, code, "break 2\nbreak\nbreak 2\nbreak\nquit\n")

	mustContain(t, transcript, "breakpoint at line 2 added")
	mustContain(t, transcript, "breakpoint at line 2\n")
	mustContain(t, transcript, "breakpoint at line 2 removed")
	mustContain(t, transcript, "no breakpoints")
}

// Unknown commands print help; empty line repeats the last command.
func TestUnknownAndRepeat(t *testing.T) {
	code := "let x = 1\nprint(x)\n"
	// frobnicate -> help; empty line repeats frobnicate -> help again.
	transcript := runScripted(t, code, "break 1\ncontinue\nfrobnicate\n\nquit\n")

	if got := strings.Count(transcript, "unknown command"); got != 2 {
		t.Fatalf("expected 2 unknown-command notices (repeat on empty line), got %d\n%s", got, transcript)
	}
}

// A runtime error is reported and stops the session.
func TestRuntimeError(t *testing.T) {
	code := "let x = 1\nprint(undefined_var_xyz)\nprint(\"never\")\n"
	transcript := runScripted(t, code, "continue\n")

	mustContain(t, transcript, "runtime error:")
	mustNotContain(t, transcript, "never")
}

// Parser errors are printed and returned, without running anything.
func TestParseError(t *testing.T) {
	in := strings.NewReader("")
	var out bytes.Buffer
	sess := New(in, &out)
	err := sess.RunCode("let = =\n", "bad")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	mustContain(t, out.String(), "parse error")
}
