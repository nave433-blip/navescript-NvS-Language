package eval

import (
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// Wave 17: the debugger/profiler hook must attribute each statement to
// the file where it was DEFINED, not the file of the caller. Regression
// test for the phantom-line bug: a function defined in "a.nvs" but called
// from "b.nvs" must report a.nvs for its body statements.

type hookCall struct {
	line int
	file string
}

type recordingDebugger struct{ calls []hookCall }

func (d *recordingDebugger) BeforeStmt(line int, file string, env *object.Environment) {
	d.calls = append(d.calls, hookCall{line, file})
}
func (d *recordingDebugger) EnterCall(name string) {}
func (d *recordingDebugger) LeaveCall()            {}

func evalSource(t *testing.T, src, file string, env *object.Environment) object.Object {
	t.Helper()
	prev := CurrentFile
	CurrentFile = file
	defer func() { CurrentFile = prev }()
	program := parser.New(lexer.New(src)).ParseProgram()
	return Eval(program, env)
}

func TestHookAttributesFunctionBodyToDefinitionFile(t *testing.T) {
	env := object.NewEnvironment()
	rec := &recordingDebugger{}
	prev := ActiveDebugger
	ActiveDebugger = rec
	defer func() { ActiveDebugger = prev }()

	// Define the function "in a.nvs" (3 lines: fn line, body line, closing).
	evalSource(t, "fn f(x) {\n  return x + 1\n}\n", "a.nvs", env)
	// Call it "from b.nvs".
	evalSource(t, "f(41)\n", "b.nvs", env)

	var bodyFile string
	var bodyLine int
	var callFile string
	for _, c := range rec.calls {
		// The `return x + 1` statement is line 2 of a.nvs.
		if c.line == 2 {
			bodyFile, bodyLine = c.file, c.line
		}
		// The `f(41)` call statement is line 1 of b.nvs.
		if c.line == 1 && c.file == "b.nvs" {
			callFile = c.file
		}
	}
	if callFile != "b.nvs" {
		t.Errorf("call statement not attributed to b.nvs (calls: %+v)", rec.calls)
	}
	if bodyFile != "a.nvs" || bodyLine != 2 {
		t.Errorf("function body misattributed: got %s:%d, want a.nvs:2 (calls: %+v)",
			bodyFile, bodyLine, rec.calls)
	}
}
