// Package debug implements nvsdb, the interactive terminal debugger for
// NvS ("nvs debug").
//
// It is built on the cooperative hooks in internal/eval/debug.go: the
// evaluator calls Session.BeforeStmt once per statement and brackets
// user-function calls with EnterCall/LeaveCall. All hooks run
// synchronously on the evaluating goroutine, so BeforeStmt can block on
// an interactive prompt and evaluation resumes when it returns.
//
// Honest scope: this drives the tree-walking evaluator only. The
// experimental bytecode VM (nvs bc) is a separate execution engine and
// does not fire these hooks, so it cannot be debugged with nvsdb.
// A DAP (Debug Adapter Protocol) server is future work; the hook
// interface here is deliberately small enough to back one later.
package debug

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// quitSignal is the private panic value used to unwind the entire
// evaluation when the user types `quit`. It is recovered ONLY in
// RunFile/RunCode (see the deferred recover there); any other panic
// value is re-panicked untouched, so this sentinel never leaks out of
// this package and never masks a real evaluator bug.
type quitSignal struct{}

// syncWriter serializes concurrent writes: the stdout-capture
// goroutine (program output) and the command loop (debugger output)
// both write to the session output.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Session is one interactive debugging session. It implements the
// eval.Debugger interface, so a Session can be installed as
// eval.ActiveDebugger while a program runs.
type Session struct {
	in     *bufio.Reader
	out    io.Writer // mutex-guarded; all session writes go here
	rawOut io.Writer // the underlying writer, for type checks

	breakpoints map[int]bool

	// Pause modes. stepMode is one-shot: pause at the very next
	// statement, then clear. nextMode pauses at the next statement
	// whose call depth is <= nextDepth (step over calls).
	stepMode  bool
	nextMode  bool
	nextDepth int

	// Call tracking, maintained from EnterCall/LeaveCall.
	depth int
	stack []string // innermost call last

	// State of the current pause.
	pausedEnv  *object.Environment
	pausedLine int
	pausedFile string
	// mainFile is the program being debugged (RunCode's name argument).
	// Breakpoints are matched against it; pauses inside other files
	// (imports) are reported with their file path.
	mainFile string
	lastCmd  string

	// suppress, when true, makes the hooks ignore events. It is set
	// while the `print` command evaluates an expression in the paused
	// environment, so that functions called by the expression neither
	// disturb the call stack/depth bookkeeping nor trigger pauses.
	suppress bool
}

// New creates a debugging session reading commands from in and writing
// all debugger and debuggee output to out.
func New(in io.Reader, out io.Writer) *Session {
	return &Session{
		in:          bufio.NewReader(in),
		out:         &syncWriter{w: out},
		rawOut:      out,
		breakpoints: map[int]bool{},
	}
}

// prompt is the interactive prompt, gdb-style.
const prompt = "(nvsdb) "

// ---------------------------------------------------------------------------
// eval.Debugger implementation
// ---------------------------------------------------------------------------

// BeforeStmt is called by the evaluator once per statement, before the
// statement executes. It pauses (prints "stopped at line N" and runs the
// command loop) when the line has a breakpoint (breakpoints match the main
// file only), when stepMode is set, or when nextMode is set and the current
// call depth is at or above the depth recorded by `next`. Pauses inside
// imported files report their file path.
func (s *Session) BeforeStmt(line int, file string, env *object.Environment) {
	if s.suppress {
		return
	}
	inMain := s.mainFile == "" || file == s.mainFile
	shouldPause := (inMain && s.breakpoints[line]) || s.stepMode ||
		(s.nextMode && s.depth <= s.nextDepth)
	if !shouldPause {
		return
	}
	// One-shot modes are consumed by the pause they cause.
	s.stepMode = false
	s.nextMode = false
	s.pausedEnv = env
	s.pausedLine = line
	s.pausedFile = file
	if inMain {
		fmt.Fprintf(s.out, "stopped at line %d\n", line)
	} else {
		fmt.Fprintf(s.out, "stopped at %s:%d\n", file, line)
	}
	s.commandLoop()
}

// EnterCall records entry into a user-defined function call.
func (s *Session) EnterCall(name string) {
	if s.suppress {
		return
	}
	s.stack = append(s.stack, name)
	s.depth++
}

// LeaveCall records return from a user-defined function call.
func (s *Session) LeaveCall() {
	if s.suppress {
		return
	}
	if len(s.stack) > 0 {
		s.stack = s.stack[:len(s.stack)-1]
	}
	if s.depth > 0 {
		s.depth--
	}
}

// ---------------------------------------------------------------------------
// Running programs
// ---------------------------------------------------------------------------

// RunFile parses the file at path and debugs it. Parser errors are
// printed and returned. A runtime ERROR object is printed and stops the
// session. eval.ActiveDebugger is set to the session for the duration of
// the run and restored afterwards.
func (s *Session) RunFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return s.RunCode(string(src), path)
}

// RunCode debugs a source string (name is used in error messages).
// Used by tests and by callers that already hold the source.
func (s *Session) RunCode(code, name string) error {
	l := lexer.New(code)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(s.out, "parse error: %s\n", e)
		}
		return fmt.Errorf("parse errors in %s", name)
	}

	prev := eval.ActiveDebugger
	eval.ActiveDebugger = s
	defer func() { eval.ActiveDebugger = prev }()

	// The main file identifies breakpoint scope (see BeforeStmt).
	s.mainFile = name

	// The debuggee's print/printf builtins write to os.Stdout, so
	// capture it into the session output while the program runs. This
	// keeps program output and debugger output in one transcript.
	restoreStdout := s.captureStdout()
	defer restoreStdout()

	// `quit` unwinds the whole evaluation via a private panic value;
	// recover it here and let every other panic propagate.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(quitSignal); !ok {
				panic(r)
			}
			fmt.Fprintln(s.out, "quit")
		}
	}()

	env := object.NewEnvironment()
	eval.LoadPrelude(env)

	// The hook reports eval.CurrentFile, so point it at the debugged
	// program for the duration of the run (restored afterwards).
	prevFile := eval.CurrentFile
	eval.CurrentFile = name
	defer func() { eval.CurrentFile = prevFile }()

	// gdb-style pre-run prompt: let the user set breakpoints before
	// the program starts. `continue` (or EOF) begins execution; `step`
	// / `next` pause at the first statement; `quit` exits.
	s.commandLoop()

	result := eval.Eval(program, env)
	if errObj, ok := result.(*object.Error); ok && errObj != nil {
		fmt.Fprintf(s.out, "runtime error: %s\n", errObj.Inspect())
	}
	return nil
}

// captureStdout redirects os.Stdout into s.out for the duration of the
// run and returns a restore function. The copy runs on a goroutine;
// restore closes the pipe and waits for the copy to drain so no program
// output is lost. Ordering caveat: debugger writes go directly to s.out
// while program output travels through the pipe, so under a transcript
// the two streams can interleave slightly out of order; content is never
// lost.
func (s *Session) captureStdout() func() {
	if f, ok := s.rawOut.(*os.File); ok && f == os.Stdout {
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
		io.Copy(s.out, r) //nolint:errcheck
	}()
	return func() {
		w.Close()
		os.Stdout = old
		wg.Wait()
		r.Close()
	}
}

// ---------------------------------------------------------------------------
// Command loop
// ---------------------------------------------------------------------------

// commandLoop reads commands until one of them resumes execution
// (continue/step/next), the user quits, or input hits EOF. On EOF the
// loop ends and the program continues to its end, so a scripted session
// that forgets `quit` still terminates.
func (s *Session) commandLoop() {
	for {
		fmt.Fprint(s.out, prompt)
		raw, err := s.in.ReadString('\n')
		if err != nil {
			// EOF or read error: resume to end of program.
			fmt.Fprintln(s.out)
			return
		}
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			// gdb-style: empty line repeats the last command.
			cmd = s.lastCmd
			if cmd == "" {
				s.printHelp()
				continue
			}
		} else {
			s.lastCmd = cmd
		}
		if s.execCommand(cmd) {
			return
		}
	}
}

// execCommand runs one command. It reports true when execution should
// resume (the command loop exits), false to keep prompting. `quit`
// does not return at all: it panics with quitSignal.
func (s *Session) execCommand(cmd string) (resume bool) {
	fields := strings.Fields(cmd)
	verb := fields[0]
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, verb))

	switch verb {
	case "break", "b":
		s.cmdBreak(arg)
		return false
	case "step", "s":
		s.stepMode = true
		return true
	case "next", "n":
		s.nextMode = true
		s.nextDepth = s.depth
		return true
	case "continue", "c":
		return true
	case "print", "p":
		s.cmdPrint(arg)
		return false
	case "backtrace", "bt":
		s.cmdBacktrace()
		return false
	case "locals":
		s.cmdLocals()
		return false
	case "quit", "q":
		panic(quitSignal{})
	case "help", "h", "?":
		s.printHelp()
		return false
	default:
		fmt.Fprintf(s.out, "unknown command %q\n", verb)
		s.printHelp()
		return false
	}
}

func (s *Session) printHelp() {
	fmt.Fprintln(s.out, "commands: break|b <line> | step|s | next|n | continue|c | print|p <expr> | backtrace|bt | locals | quit|q | help")
}

// cmdBreak with no argument lists breakpoints; with a line number it
// toggles the breakpoint (adds it, or removes it if already set).
func (s *Session) cmdBreak(arg string) {
	if arg == "" {
		if len(s.breakpoints) == 0 {
			fmt.Fprintln(s.out, "no breakpoints")
			return
		}
		lines := make([]int, 0, len(s.breakpoints))
		for l := range s.breakpoints {
			lines = append(lines, l)
		}
		sort.Ints(lines)
		for _, l := range lines {
			fmt.Fprintf(s.out, "breakpoint at line %d\n", l)
		}
		return
	}
	n, err := strconv.Atoi(strings.Fields(arg)[0])
	if err != nil || n < 1 {
		fmt.Fprintf(s.out, "bad line number %q: want a positive integer\n", arg)
		return
	}
	if s.breakpoints[n] {
		delete(s.breakpoints, n)
		fmt.Fprintf(s.out, "breakpoint at line %d removed\n", n)
	} else {
		s.breakpoints[n] = true
		fmt.Fprintf(s.out, "breakpoint at line %d added\n", n)
	}
}

// cmdPrint parses expr and evaluates it in the paused environment. The
// debuggee's control flow is not disturbed: hooks are suppressed during
// the evaluation so any functions the expression calls neither pause
// nor corrupt the call-stack bookkeeping. Null results print nothing.
func (s *Session) cmdPrint(expr string) {
	if strings.TrimSpace(expr) == "" {
		fmt.Fprintln(s.out, "usage: print <expr>")
		return
	}
	if s.pausedEnv == nil {
		fmt.Fprintln(s.out, "not stopped in a program")
		return
	}
	p := parser.New(lexer.New(expr))
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		fmt.Fprintf(s.out, "parse error: %s\n", strings.Join(errs, "; "))
		return
	}
	s.suppress = true
	defer func() { s.suppress = false }()
	val := eval.Eval(prog, s.pausedEnv)
	if errObj, ok := val.(*object.Error); ok && errObj != nil {
		fmt.Fprintf(s.out, "error: %s\n", errObj.Inspect())
		return
	}
	if val == nil || val.Type() == object.NULL_OBJ {
		return
	}
	fmt.Fprintln(s.out, val.Inspect())
}

// cmdBacktrace prints the call stack, innermost frame first, gdb-style.
func (s *Session) cmdBacktrace() {
	for i := len(s.stack) - 1; i >= 0; i-- {
		fmt.Fprintf(s.out, "#%d %s\n", len(s.stack)-1-i, s.stack[i])
	}
	fmt.Fprintf(s.out, "#%d <toplevel>\n", len(s.stack))
}

// cmdLocals prints the current frame's bindings (env.Names is sorted,
// current frame only; Get walks outer scopes).
func (s *Session) cmdLocals() {
	if s.pausedEnv == nil {
		fmt.Fprintln(s.out, "no frame")
		return
	}
	names := s.pausedEnv.Names()
	if len(names) == 0 {
		fmt.Fprintln(s.out, "(no locals)")
		return
	}
	for _, name := range names {
		val := "<undefined>"
		if obj, ok := s.pausedEnv.Get(name); ok && obj != nil {
			val = truncate(obj.Inspect(), 120)
		}
		fmt.Fprintf(s.out, "%s = %s\n", name, val)
	}
}

func truncate(str string, max int) string {
	if len(str) <= max {
		return str
	}
	return str[:max] + "..."
}
