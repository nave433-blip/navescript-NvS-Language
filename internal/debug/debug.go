// Package debug implements nvsdb, the interactive terminal debugger for
// NvS ("nvs debug").
//
// The stepping engine is Controller (controller.go), shared with the DAP
// adapter ("nvs dap", internal/dap). This file is only the terminal
// front end: the prompt, the command loop, and the break/step/next/
// continue/print/backtrace/locals/quit commands.
//
// Honest scope: this drives the tree-walking evaluator only. The
// experimental bytecode VM (nvs bc) is a separate execution engine and
// does not fire these hooks, so it cannot be debugged with nvsdb.
package debug

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/object"
)

// Session is one interactive debugging session. It embeds the shared
// Controller (the stepping engine) and adds the terminal command loop.
type Session struct {
	*Controller
	in      *bufio.Reader
	out     io.Writer // mutex-guarded; all session writes go here
	rawOut  io.Writer // the underlying writer, for type checks
	lastCmd string
}

// New creates a debugging session reading commands from in and writing
// all debugger and debuggee output to out.
func New(in io.Reader, out io.Writer) *Session {
	s := &Session{
		in:     bufio.NewReader(in),
		out:    &syncWriter{w: out},
		rawOut: out,
	}
	s.Controller = NewController()
	s.Controller.OnPause = s.onPause
	return s
}

// onPause implements the Controller pause hook for the terminal: report
// where we stopped, then run the interactive command loop.
func (s *Session) onPause(c *Controller) {
	inMain := c.mainFile == "" || c.pausedFile == c.mainFile
	if inMain {
		fmt.Fprintf(s.out, "stopped at line %d\n", c.pausedLine)
	} else {
		fmt.Fprintf(s.out, "stopped at %s:%d\n", c.pausedFile, c.pausedLine)
	}
	s.commandLoop()
}

// prompt is the interactive prompt, gdb-style.
const prompt = "(nvsdb) "

// ---------------------------------------------------------------------------
// Running programs
// ---------------------------------------------------------------------------

// RunFile parses the file at path and debugs it. Parser errors are
// printed and returned. A runtime ERROR object is printed and stops the
// session.
func (s *Session) RunFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return s.RunCode(string(src), path)
}

// RunCode debugs a source string (name is used in error messages).
// Used by tests and by callers that already hold the source.
//
// gdb-style pre-run prompt: let the user set breakpoints before the
// program starts. `continue` (or EOF) begins execution; `step` / `next`
// pause at the first statement; `quit` exits.
func (s *Session) RunCode(code, name string) error {
	return Run(s.Controller, code, name, s.out, s.rawOut, func(*Controller) {
		s.commandLoop()
	})
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
		s.Step()
		return true
	case "next", "n":
		s.Next()
		return true
	case "continue", "c":
		s.Continue()
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
		s.Terminate()
		return false // unreachable
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
		lines := s.Breakpoints()
		if len(lines) == 0 {
			fmt.Fprintln(s.out, "no breakpoints")
			return
		}
		for _, l := range lines {
			fmt.Fprintf(s.out, "breakpoint at line %d\n", l)
		}
		return
	}
	n, err := parseLine(arg)
	if err != nil {
		fmt.Fprintf(s.out, "bad line number %q: want a positive integer\n", arg)
		return
	}
	if s.breakpoints[n] {
		s.RemoveBreakpoint(n)
		fmt.Fprintf(s.out, "breakpoint at line %d removed\n", n)
	} else {
		s.AddBreakpoint(n)
		fmt.Fprintf(s.out, "breakpoint at line %d added\n", n)
	}
}

func parseLine(arg string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.Fields(arg)[0], "%d", &n)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("bad line")
	}
	return n, nil
}

// cmdPrint evaluates expr in the paused environment and prints the
// result. Null results print nothing.
func (s *Session) cmdPrint(expr string) {
	if strings.TrimSpace(expr) == "" {
		fmt.Fprintln(s.out, "usage: print <expr>")
		return
	}
	if s.PausedEnv() == nil {
		fmt.Fprintln(s.out, "not stopped in a program")
		return
	}
	val, err := s.EvalInPausedEnv(expr)
	if err != nil {
		fmt.Fprintf(s.out, "%s\n", err.Error())
		return
	}
	if val == nil || val.Type() == object.NULL_OBJ {
		return
	}
	fmt.Fprintln(s.out, val.Inspect())
}

// cmdBacktrace prints the call stack, innermost frame first, gdb-style.
func (s *Session) cmdBacktrace() {
	stack := s.Stack()
	for i := len(stack) - 1; i >= 0; i-- {
		fmt.Fprintf(s.out, "#%d %s\n", len(stack)-1-i, stack[i].Name)
	}
	fmt.Fprintf(s.out, "#%d <toplevel>\n", len(stack))
}

// cmdLocals prints the current frame's bindings (env.Names is sorted,
// current frame only; Get walks outer scopes).
func (s *Session) cmdLocals() {
	env := s.PausedEnv()
	if env == nil {
		fmt.Fprintln(s.out, "no frame")
		return
	}
	names := env.Names()
	if len(names) == 0 {
		fmt.Fprintln(s.out, "(no locals)")
		return
	}
	for _, name := range names {
		val := "<undefined>"
		if obj, ok := env.Get(name); ok && obj != nil {
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
