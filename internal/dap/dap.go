// Package dap implements a Debug Adapter Protocol (DAP) server for NvS,
// exposed as `nvs dap`.
//
// It speaks DAP's Content-Length framed JSON over stdio and drives the
// shared stepping engine in internal/debug (the same Controller behind
// the terminal debugger) — stepping is reused, not reimplemented. The
// debuggee runs on its own goroutine; pauses block that goroutine in
// Controller.OnPause until the DAP client sends continue/next/stepIn/
// stepOut/disconnect.
//
// Implemented requests: initialize, launch, setBreakpoints,
// configurationDone, threads, stackTrace, scopes, variables, evaluate,
// continue, next, stepIn, stepOut, disconnect. Events: initialized,
// stopped, output, terminated.
//
// Honest scope:
//   - Like the terminal debugger, this drives the tree-walking
//     evaluator only; the experimental bytecode VM cannot be debugged.
//   - Breakpoints match the launched file only (an engine rule shared
//     with the terminal debugger); breakpoints set in other files are
//     reported unverified.
//   - Variables/scopes are served for the innermost frame only; outer
//     frames report names and call-site lines but no variables.
//   - disconnect while the program is running ends the adapter
//     process; the debuggee cannot be asynchronously interrupted.
package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/navescript/nvs/internal/debug"
	"github.com/navescript/nvs/internal/object"
)

// ---------------------------------------------------------------------------
// DAP wire types (only what we speak).
// ---------------------------------------------------------------------------

type wireMsg struct {
	Seq         int             `json:"seq"`
	Type        string          `json:"type"` // "request"
	Command     string          `json:"command"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

type responseMsg struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"` // "response"
	RequestSeq int    `json:"request_seq"`
	Success    bool   `json:"success"`
	Command    string `json:"command"`
	Message    string `json:"message,omitempty"`
	Body       any    `json:"body,omitempty"`
}

type eventMsg struct {
	Seq   int    `json:"seq"`
	Type  string `json:"type"` // "event"
	Event string `json:"event"`
	Body  any    `json:"body,omitempty"`
}

// ---------------------------------------------------------------------------
// Adapter
// ---------------------------------------------------------------------------

// resumeAction is how the protocol loop tells the paused evaluator to
// proceed.
type resumeAction int

const (
	resumeContinue resumeAction = iota
	resumeStep
	resumeNext
	resumeStepOut
	resumeTerminate
)

// Adapter is one DAP session.
type Adapter struct {
	ctl *debug.Controller

	mu     sync.Mutex // guards out and seq
	out    io.Writer
	seq    int
	closed atomic.Bool

	// Pause coordination with the evaluating goroutine.
	resumeCh chan resumeAction
	paused   atomic.Bool

	// Breakpoints requested per absolute file path (from setBreakpoints).
	bpMu    sync.Mutex
	pending map[string][]int // applied at next pause / at start

	programPath string
	started     bool
	progDone    chan struct{}

	// variablesReference -> object for container expansion.
	varRefs map[int]object.Object
	nextRef int
}

func newAdapter(out io.Writer) *Adapter {
	a := &Adapter{
		ctl:      debug.NewController(),
		out:      out,
		resumeCh: make(chan resumeAction),
		pending:  map[string][]int{},
		progDone: make(chan struct{}),
		varRefs:  map[int]object.Object{},
		nextRef:  2000,
	}
	a.ctl.OnPause = a.onPause
	return a
}

// Serve runs the DAP session: reads framed requests from r, writes
// framed responses/events to w. It returns when the client disconnects
// or input ends.
func Serve(r io.Reader, w io.Writer) {
	a := newAdapter(w)
	br := bufio.NewReader(r)
	for {
		raw, err := readMessage(br)
		if err != nil {
			return // EOF or framing error
		}
		var msg wireMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Type != "request" {
			continue
		}
		if !a.handle(msg) {
			return
		}
	}
}

func readMessage(br *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok &&
			strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("bad Content-Length: %q", value)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (a *Adapter) nextSeq() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	return a.seq
}

func (a *Adapter) send(v any) {
	if a.closed.Load() {
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed.Load() {
		return
	}
	fmt.Fprintf(a.out, "Content-Length: %d\r\n\r\n", len(body))
	_, _ = a.out.Write(body)
}

func (a *Adapter) respond(reqSeq int, command string, success bool, message string, body any) {
	a.send(responseMsg{
		Seq: a.nextSeq(), Type: "response", RequestSeq: reqSeq,
		Success: success, Command: command, Message: message, Body: body,
	})
}

func (a *Adapter) ok(reqSeq int, command string, body any) {
	a.respond(reqSeq, command, true, "", body)
}

func (a *Adapter) fail(reqSeq int, command, message string) {
	a.respond(reqSeq, command, false, message, nil)
}

func (a *Adapter) event(name string, body any) {
	a.send(eventMsg{Seq: a.nextSeq(), Type: "event", Event: name, Body: body})
}

// onPause runs on the evaluating goroutine. It applies any breakpoints
// that arrived while running, reports the stop, then blocks until the
// client resumes execution.
func (a *Adapter) onPause(c *debug.Controller) {
	a.bpMu.Lock()
	if lines, ok := a.pending[c.PausedFile()]; ok {
		for _, l := range lines {
			c.AddBreakpoint(l)
		}
		delete(a.pending, c.PausedFile())
	}
	a.bpMu.Unlock()

	a.paused.Store(true)
	a.event("stopped", map[string]any{
		"reason":   c.PauseReason(),
		"threadId": 1,
	})
	act := <-a.resumeCh
	a.paused.Store(false)
	switch act {
	case resumeStep:
		c.Step()
	case resumeNext:
		c.Next()
	case resumeStepOut:
		c.StepOut()
	case resumeTerminate:
		c.Terminate()
	default:
		c.Continue()
	}
}

// ---------------------------------------------------------------------------
// Request handling. Returns false when the session should end.
// ---------------------------------------------------------------------------

func (a *Adapter) handle(msg wireMsg) bool {
	switch msg.Command {
	case "initialize":
		a.ok(msg.Seq, "initialize", map[string]any{
			"supportsConfigurationDoneRequest": true,
		})
	case "launch":
		var args struct {
			Program       string `json:"program"`
			Configuration struct {
				Program string `json:"program"`
			} `json:"configuration"`
		}
		_ = json.Unmarshal(msg.Arguments, &args)
		prog := args.Program
		if prog == "" {
			prog = args.Configuration.Program
		}
		if prog == "" {
			a.fail(msg.Seq, "launch", "launch requires a \"program\" path")
			return true
		}
		abs, err := filepath.Abs(prog)
		if err != nil {
			a.fail(msg.Seq, "launch", "bad program path: "+err.Error())
			return true
		}
		a.programPath = abs
		a.ok(msg.Seq, "launch", nil)
		a.event("initialized", nil)
	case "setBreakpoints":
		var args struct {
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
			Breakpoints []struct {
				Line int `json:"line"`
			} `json:"breakpoints"`
		}
		_ = json.Unmarshal(msg.Arguments, &args)
		path := args.Source.Path
		var lines []int
		for _, b := range args.Breakpoints {
			if b.Line > 0 {
				lines = append(lines, b.Line)
			}
		}
		main := path == a.programPath
		a.bpMu.Lock()
		if main {
			a.pending[path] = lines
			if a.started && a.paused.Load() {
				// Paused: the next onPause applies pending; but we are
				// already paused, so apply directly — the evaluator is
				// parked and cannot race us.
				for _, l := range lines {
					a.ctl.AddBreakpoint(l)
				}
				delete(a.pending, path)
			} else if a.started {
				// Running: applied at the next pause by onPause.
			}
		} else {
			// Engine rule: breakpoints only match the launched file.
			delete(a.pending, path)
		}
		a.bpMu.Unlock()
		resp := make([]map[string]any, 0, len(lines))
		for _, l := range lines {
			bp := map[string]any{"verified": main, "line": l}
			if !main {
				bp["message"] = "nvs breakpoints are only supported in the launched file"
			}
			resp = append(resp, bp)
		}
		a.ok(msg.Seq, "setBreakpoints", map[string]any{"breakpoints": resp})
	case "configurationDone":
		a.ok(msg.Seq, "configurationDone", nil)
		if !a.started {
			a.started = true
			go a.runProgram()
		}
	case "threads":
		a.ok(msg.Seq, "threads", map[string]any{
			"threads": []map[string]any{{"id": 1, "name": "main"}},
		})
	case "stackTrace":
		a.handleStackTrace(msg)
	case "scopes":
		a.handleScopes(msg)
	case "variables":
		a.handleVariables(msg)
	case "evaluate":
		a.handleEvaluate(msg)
	case "continue":
		if !a.paused.Load() {
			a.fail(msg.Seq, "continue", "not paused")
			return true
		}
		a.ok(msg.Seq, "continue", map[string]any{"allThreadsContinued": true})
		a.resumeCh <- resumeContinue
	case "next":
		if !a.paused.Load() {
			a.fail(msg.Seq, "next", "not paused")
			return true
		}
		a.ok(msg.Seq, "next", map[string]any{})
		a.resumeCh <- resumeNext
	case "stepIn":
		if !a.paused.Load() {
			a.fail(msg.Seq, "stepIn", "not paused")
			return true
		}
		a.ok(msg.Seq, "stepIn", map[string]any{})
		a.resumeCh <- resumeStep
	case "stepOut":
		if !a.paused.Load() {
			a.fail(msg.Seq, "stepOut", "not paused")
			return true
		}
		a.ok(msg.Seq, "stepOut", map[string]any{})
		a.resumeCh <- resumeStepOut
	case "disconnect":
		a.ok(msg.Seq, "disconnect", nil)
		if a.paused.Load() {
			a.resumeCh <- resumeTerminate
			<-a.progDone
		}
		a.closed.Store(true)
		return false
	default:
		a.fail(msg.Seq, msg.Command, "unsupported request: "+msg.Command)
	}
	return true
}

// ---------------------------------------------------------------------------
// Program execution
// ---------------------------------------------------------------------------

// outputWriter turns debuggee stdout into DAP `output` events,
// line-buffered.
type outputWriter struct {
	a   *Adapter
	buf []byte
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := -1
		for j, b := range w.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		line := string(w.buf[:i+1])
		w.buf = w.buf[i+1:]
		w.a.event("output", map[string]any{"category": "stdout", "output": line})
	}
	return len(p), nil
}

func (w *outputWriter) flush() {
	if len(w.buf) > 0 {
		w.a.event("output", map[string]any{"category": "stdout", "output": string(w.buf)})
		w.buf = nil
	}
}

func (a *Adapter) runProgram() {
	defer close(a.progDone)
	src, err := os.ReadFile(a.programPath)
	if err != nil {
		a.event("output", map[string]any{"category": "stderr", "output": "cannot read program: " + err.Error() + "\n"})
		a.event("terminated", nil)
		return
	}
	outW := &outputWriter{a: a}
	preRun := func(c *debug.Controller) {
		a.bpMu.Lock()
		for _, l := range a.pending[a.programPath] {
			c.AddBreakpoint(l)
		}
		delete(a.pending, a.programPath)
		a.bpMu.Unlock()
	}
	_ = debug.Run(a.ctl, string(src), a.programPath, outW, nil, preRun)
	outW.flush()
	a.event("terminated", nil)
}

// ---------------------------------------------------------------------------
// Stack, scopes, variables, evaluate
// ---------------------------------------------------------------------------

// dapFrames returns frames innermost-first for DAP. The innermost frame
// uses the exact pause position; outer frames use their recorded
// call-site line.
func (a *Adapter) dapFrames() []map[string]any {
	stack := a.ctl.Stack() // innermost last
	frames := make([]map[string]any, 0, len(stack)+1)
	// Innermost frame: exact pause position.
	frames = append(frames, map[string]any{
		"id":     0,
		"name":   frameName(stack),
		"source": dapSource(a.ctl.PausedFile()),
		"line":   a.ctl.PausedLine(),
		"column": 1,
	})
	// Outer frames, innermost-first.
	id := 1
	for i := len(stack) - 1; i >= 0; i-- {
		f := stack[i]
		line := f.Line
		if line < 1 {
			line = 1
		}
		frames = append(frames, map[string]any{
			"id":     id,
			"name":   f.Name,
			"source": dapSource(f.File),
			"line":   line,
			"column": 1,
		})
		id++
	}
	return frames
}

func frameName(stack []debug.Frame) string {
	if len(stack) == 0 {
		return "<toplevel>"
	}
	return stack[len(stack)-1].Name
}

func dapSource(path string) map[string]any {
	if path == "" {
		return map[string]any{"name": "<unknown>"}
	}
	return map[string]any{"name": filepath.Base(path), "path": path}
}

func (a *Adapter) handleStackTrace(msg wireMsg) {
	if !a.paused.Load() {
		a.fail(msg.Seq, "stackTrace", "not paused")
		return
	}
	var args struct {
		StartFrame int `json:"startFrame"`
		Levels     int `json:"levels"`
	}
	_ = json.Unmarshal(msg.Arguments, &args)
	frames := a.dapFrames()
	if args.StartFrame > 0 && args.StartFrame < len(frames) {
		frames = frames[args.StartFrame:]
	}
	if args.Levels > 0 && args.Levels < len(frames) {
		frames = frames[:args.Levels]
	}
	total := len(a.dapFrames())
	a.ok(msg.Seq, "stackTrace", map[string]any{
		"stackFrames": frames,
		"totalFrames": total,
	})
}

func (a *Adapter) handleScopes(msg wireMsg) {
	var args struct {
		FrameId int `json:"frameId"`
	}
	_ = json.Unmarshal(msg.Arguments, &args)
	if !a.paused.Load() {
		a.fail(msg.Seq, "scopes", "not paused")
		return
	}
	// Variables are served for the innermost frame only (frame 0); the
	// engine retains the paused environment for the current frame.
	ref := 0
	if args.FrameId == 0 {
		ref = 1000
	}
	a.ok(msg.Seq, "scopes", map[string]any{
		"scopes": []map[string]any{
			{"name": "Locals", "variablesReference": ref, "expensive": false},
		},
	})
}

// allocRef registers obj for later expansion and returns its reference.
func (a *Adapter) allocRef(obj object.Object) int {
	a.nextRef++
	a.varRefs[a.nextRef] = obj
	return a.nextRef
}

func varFor(name string, obj object.Object, a *Adapter) map[string]any {
	v := map[string]any{
		"name":               name,
		"value":              obj.Inspect(),
		"type":               string(obj.Type()),
		"variablesReference": 0,
	}
	switch o := obj.(type) {
	case *object.Array:
		v["indexedVariables"] = len(o.Elements)
		if len(o.Elements) > 0 {
			v["variablesReference"] = a.allocRef(obj)
		}
	case *object.Hash:
		v["namedVariables"] = len(o.Pairs)
		if len(o.Pairs) > 0 {
			v["variablesReference"] = a.allocRef(obj)
		}
	}
	return v
}

func (a *Adapter) handleVariables(msg wireMsg) {
	var args struct {
		VariablesReference int `json:"variablesReference"`
	}
	_ = json.Unmarshal(msg.Arguments, &args)
	if !a.paused.Load() {
		a.fail(msg.Seq, "variables", "not paused")
		return
	}
	var vars []map[string]any
	switch {
	case args.VariablesReference == 1000:
		env := a.ctl.PausedEnv()
		if env == nil {
			a.fail(msg.Seq, "variables", "no paused frame")
			return
		}
		for _, name := range env.Names() {
			obj, ok := env.Get(name)
			if !ok || obj == nil {
				continue
			}
			vars = append(vars, varFor(name, obj, a))
		}
	default:
		obj, ok := a.varRefs[args.VariablesReference]
		if !ok {
			a.fail(msg.Seq, "variables", "unknown variables reference")
			return
		}
		vars = expandObject(obj, a)
	}
	if vars == nil {
		vars = []map[string]any{}
	}
	a.ok(msg.Seq, "variables", map[string]any{"variables": vars})
}

// expandObject lists one level of children for arrays and hashes.
func expandObject(obj object.Object, a *Adapter) []map[string]any {
	var vars []map[string]any
	switch o := obj.(type) {
	case *object.Array:
		for i, el := range o.Elements {
			if i >= 100 {
				break
			}
			vars = append(vars, varFor(fmt.Sprintf("[%d]", i), el, a))
		}
	case *object.Hash:
		n := 0
		for _, p := range o.Pairs {
			if n >= 100 {
				break
			}
			vars = append(vars, varFor(p.Key.Inspect(), p.Value, a))
			n++
		}
	}
	return vars
}

func (a *Adapter) handleEvaluate(msg wireMsg) {
	var args struct {
		Expression string `json:"expression"`
		FrameId    int    `json:"frameId"`
		Context    string `json:"context"`
	}
	_ = json.Unmarshal(msg.Arguments, &args)
	if !a.paused.Load() {
		a.fail(msg.Seq, "evaluate", "not paused")
		return
	}
	if args.FrameId != 0 {
		a.fail(msg.Seq, "evaluate", "evaluation is only supported in the innermost frame")
		return
	}
	val, err := a.ctl.EvalInPausedEnv(args.Expression)
	if err != nil {
		a.fail(msg.Seq, "evaluate", err.Error())
		return
	}
	if val == nil {
		val = &object.Null{}
	}
	a.ok(msg.Seq, "evaluate", map[string]any{
		"result":             val.Inspect(),
		"type":               string(val.Type()),
		"variablesReference": 0,
	})
}
