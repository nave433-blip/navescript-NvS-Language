// Controller is the stepping/breakpoint state machine shared by the
// terminal debugger (nvs debug) and the DAP adapter (nvs dap). It
// implements eval.Debugger, so it installs as eval.ActiveDebugger while
// a program runs.
//
// The stepping logic lives here exactly once: breakpoints, step, next,
// and step-out are modes on the Controller. Front ends differ only in
// what happens at a pause — the terminal prints and prompts, the DAP
// adapter signals over a channel — via the OnPause hook.
package debug

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// quitSignal is the private panic value used to unwind the entire
// evaluation when the user quits (terminal `quit`) or the DAP client
// disconnects. It is recovered ONLY in Run (see below); any other panic
// value is re-panicked untouched, so this sentinel never leaks out of
// this package and never masks a real evaluator bug.
type quitSignal struct{}

// frame is one call-stack entry. line/file is the call-site: the
// statement that was executing when the call was entered.
type frame struct {
	name string
	file string
	line int
}

// Frame is the exported view of a call-stack entry.
type Frame struct {
	Name string
	File string
	Line int
}

// resumeKind tells a paused controller how to continue.
type resumeKind int

const (
	resumeContinue resumeKind = iota
	resumeStep
	resumeNext
	resumeStepOut
)

// Controller holds debugger stepping state. All hooks run synchronously
// on the evaluating goroutine; OnPause may block and evaluation resumes
// when it returns.
type Controller struct {
	breakpoints map[int]bool

	// Pause modes. stepMode is one-shot: pause at the very next
	// statement, then clear. nextMode pauses at the next statement
	// whose call depth is <= nextDepth (step over calls). outMode
	// pauses when the depth drops below outDepth (step out).
	stepMode  bool
	nextMode  bool
	nextDepth int
	outMode   bool
	outDepth  int

	// Call tracking, maintained from EnterCall/LeaveCall.
	depth int
	stack []frame

	// mainFile scopes breakpoints: they only match the program being
	// debugged, not imported files.
	mainFile string

	// suppress, when true, makes the hooks ignore events. It is set
	// while a paused expression is evaluated (terminal `print`, DAP
	// `evaluate`), so functions called by the expression neither
	// disturb the call stack/depth bookkeeping nor trigger pauses.
	suppress bool

	// State of the current pause.
	pausedEnv    *object.Environment
	pausedLine   int
	pausedFile   string
	pauseReason  string // "breakpoint", "step", or "" when not paused
	lastStmtLine int
	lastStmtFile string

	// OnPause is invoked on the evaluating goroutine each time
	// execution pauses. It MUST resume execution before returning by
	// calling one of Continue/Step/Next/StepOut (which arm the next
	// pause) or Terminate (which unwinds the run). A nil OnPause means
	// "never pause" is impossible — pauses still arm modes; front ends
	// must set OnPause.
	OnPause func(c *Controller)
}

// NewController creates a Controller with no breakpoints and no pause
// handler. Set OnPause before running.
func NewController() *Controller {
	return &Controller{breakpoints: map[int]bool{}}
}

// ---------------------------------------------------------------------------
// Control API used by front ends.
// ---------------------------------------------------------------------------

// AddBreakpoint arms a breakpoint at the 1-based line. Breakpoints only
// match statements in the main file, not imports.
func (c *Controller) AddBreakpoint(line int) { c.breakpoints[line] = true }

// RemoveBreakpoint disarms the breakpoint at line.
func (c *Controller) RemoveBreakpoint(line int) { delete(c.breakpoints, line) }

// Breakpoints returns the sorted armed breakpoint lines.
func (c *Controller) Breakpoints() []int {
	lines := make([]int, 0, len(c.breakpoints))
	for l := range c.breakpoints {
		lines = append(lines, l)
	}
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j] < lines[j-1]; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
	return lines
}

// Continue resumes until the next breakpoint.
func (c *Controller) Continue() {}

// Step pauses at the very next statement (steps into calls).
func (c *Controller) Step() { c.stepMode = true }

// Next pauses at the next statement at the current call depth or above
// (steps over calls).
func (c *Controller) Next() {
	c.nextMode = true
	c.nextDepth = c.depth
}

// StepOut resumes until the current call returns.
func (c *Controller) StepOut() {
	c.outMode = true
	c.outDepth = c.depth
}

// Terminate unwinds the whole evaluation. It must be called on the
// evaluating goroutine (i.e. from inside OnPause); Run recovers it.
func (c *Controller) Terminate() { panic(quitSignal{}) }

// PausedEnv is the environment of the paused frame (nil when running).
func (c *Controller) PausedEnv() *object.Environment { return c.pausedEnv }

// PausedLine is the 1-based line where execution is paused.
func (c *Controller) PausedLine() int { return c.pausedLine }

// PausedFile is the file where execution is paused.
func (c *Controller) PausedFile() string { return c.pausedFile }

// PauseReason is "breakpoint" or "step" for the current pause.
func (c *Controller) PauseReason() string { return c.pauseReason }

// Stack returns the call stack, innermost frame last.
func (c *Controller) Stack() []Frame {
	out := make([]Frame, len(c.stack))
	for i, f := range c.stack {
		out[i] = Frame{Name: f.name, File: f.file, Line: f.line}
	}
	return out
}

// Depth is the current call depth.
func (c *Controller) Depth() int { return c.depth }

// EvalInPausedEnv parses expr and evaluates it in the paused
// environment, with hooks suppressed so the evaluation cannot pause or
// corrupt call-stack bookkeeping. It returns the resulting object.
func (c *Controller) EvalInPausedEnv(expr string) (object.Object, error) {
	if c.pausedEnv == nil {
		return nil, fmt.Errorf("not paused in a program")
	}
	p := parser.New(lexer.New(expr))
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("parse error: %s", errs[0])
	}
	c.suppress = true
	defer func() { c.suppress = false }()
	val := eval.Eval(prog, c.pausedEnv)
	if errObj, ok := val.(*object.Error); ok && errObj != nil {
		return nil, fmt.Errorf("%s", errObj.Inspect())
	}
	return val, nil
}

// ---------------------------------------------------------------------------
// eval.Debugger implementation
// ---------------------------------------------------------------------------

// BeforeStmt is called by the evaluator once per statement, before the
// statement executes. It pauses when the line has a breakpoint
// (breakpoints match the main file only), when stepMode is set, when
// nextMode applies, or when outMode applies (depth dropped below the
// recorded depth).
func (c *Controller) BeforeStmt(line int, file string, env *object.Environment) {
	if c.suppress {
		return
	}
	// Remember the latest statement so EnterCall can record an honest
	// call-site line for the new frame.
	c.lastStmtLine = line
	c.lastStmtFile = file
	inMain := c.mainFile == "" || file == c.mainFile
	hitBp := inMain && c.breakpoints[line]
	shouldPause := hitBp || c.stepMode ||
		(c.nextMode && c.depth <= c.nextDepth) ||
		(c.outMode && c.depth < c.outDepth)
	if !shouldPause {
		return
	}
	switch {
	case hitBp:
		c.pauseReason = "breakpoint"
	default:
		c.pauseReason = "step"
	}
	// One-shot modes are consumed by the pause they cause.
	c.stepMode = false
	c.nextMode = false
	c.outMode = false
	c.pausedEnv = env
	c.pausedLine = line
	c.pausedFile = file
	if c.OnPause != nil {
		c.OnPause(c)
	}
}

// EnterCall records entry into a user-defined function call, capturing
// the call-site line from the statement that was executing.
func (c *Controller) EnterCall(name string) {
	if c.suppress {
		return
	}
	c.stack = append(c.stack, frame{name: name, file: c.lastStmtFile, line: c.lastStmtLine})
	c.depth++
}

// LeaveCall records return from a user-defined function call.
func (c *Controller) LeaveCall() {
	if c.suppress {
		return
	}
	if len(c.stack) > 0 {
		c.stack = c.stack[:len(c.stack)-1]
	}
	if c.depth > 0 {
		c.depth--
	}
}

// ---------------------------------------------------------------------------
// Running programs
// ---------------------------------------------------------------------------

// syncWriter serializes concurrent writes: the stdout-capture
// goroutine (program output) and the front end (debugger output) may
// write concurrently.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Run debugs src (name is used in error messages and breakpoint
// scoping). preRun runs after the debugger is installed but before
// evaluation — the terminal uses it for the pre-run breakpoint prompt,
// the DAP adapter for applying initial breakpoints. Program output
// (the debuggee's print/printf write to os.Stdout) is captured into
// out, unless the underlying writer IS os.Stdout. A runtime ERROR
// object is printed and stops the session; parse errors are printed
// and returned.
func Run(c *Controller, src, name string, out, rawOut io.Writer, preRun func(*Controller)) error {
	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(out, "parse error: %s\n", e)
		}
		return fmt.Errorf("parse errors in %s", name)
	}

	prev := eval.ActiveDebugger
	eval.ActiveDebugger = c
	defer func() { eval.ActiveDebugger = prev }()

	// The main file identifies breakpoint scope (see BeforeStmt).
	c.mainFile = name

	// Capture the debuggee's stdout into out while the program runs.
	restoreStdout := captureStdout(out, rawOut)
	defer restoreStdout()

	// `quit`/`Terminate` unwinds the whole evaluation via a private
	// panic value; recover it here and let every other panic propagate.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(quitSignal); !ok {
				panic(r)
			}
			fmt.Fprintln(out, "quit")
		}
	}()

	env := object.NewEnvironment()
	eval.LoadPrelude(env)

	// The hook reports eval.CurrentFile, so point it at the debugged
	// program for the duration of the run (restored afterwards).
	prevFile := eval.CurrentFile
	eval.CurrentFile = name
	defer func() { eval.CurrentFile = prevFile }()

	if preRun != nil {
		preRun(c)
	}

	result := eval.Eval(program, env)
	if errObj, ok := result.(*object.Error); ok && errObj != nil {
		fmt.Fprintf(out, "runtime error: %s\n", errObj.Inspect())
	}
	return nil
}

// captureStdout redirects os.Stdout into out for the duration of the
// run and returns a restore function. rawOut is the unwrapped writer
// (when out wraps it); when it IS os.Stdout no capture is needed. The
// copy runs on a goroutine; restore closes the pipe and waits for the
// copy to drain so no program output is lost. Ordering caveat: front-end
// writes go directly to out while program output travels through the
// pipe, so the two streams can interleave slightly out of order;
// content is never lost.
func captureStdout(out, rawOut io.Writer) func() {
	check := out
	if rawOut != nil {
		check = rawOut
	}
	if f, ok := check.(*os.File); ok && f == os.Stdout {
		return func() {}
	}
	r, w, err := os.Pipe()
	if err != nil {
		return func() {}
	}
	old := os.Stdout
	os.Stdout = w
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(out, r) //nolint:errcheck
	}()
	return func() {
		w.Close()
		os.Stdout = old
		wg.Wait()
		r.Close()
	}
}
