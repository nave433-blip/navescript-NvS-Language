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
	"testing"
	"time"
)

// testClient speaks DAP to a server over pipes.
type testClient struct {
	t      *testing.T
	w      io.Writer // to server
	rd     *bufio.Reader
	seq    int
	mu     sync.Mutex
	events chan map[string]any
}

func newTestClient(t *testing.T) (*testClient, *io.PipeWriter, *io.PipeReader) {
	t.Helper()
	serverIn, clientOut := io.Pipe() // client writes, server reads
	clientIn, serverOut := io.Pipe() // server writes, client reads
	go Serve(serverIn, serverOut)
	c := &testClient{
		t:      t,
		w:      clientOut,
		rd:     bufio.NewReader(clientIn),
		events: make(chan map[string]any, 64),
	}
	go c.readLoop()
	return c, clientOut, clientIn
}

func (c *testClient) readLoop() {
	for {
		body, err := readMessage(c.rd)
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		if msg["type"] == "event" {
			c.events <- msg
		}
	}
}

func (c *testClient) send(command string, args any) {
	c.t.Helper()
	c.seq++
	body, _ := json.Marshal(map[string]any{
		"seq": c.seq, "type": "request", "command": command, "arguments": args,
	})
	fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body))
	if _, err := c.w.Write(body); err != nil {
		c.t.Fatalf("client write failed: %v", err)
	}
}

// waitEvent waits for an event with the given name.
func (c *testClient) waitEvent(name string) map[string]any {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-c.events:
			if ev["event"] == name {
				return ev
			}
		case <-timeout:
			c.t.Fatalf("timed out waiting for event %q", name)
		}
	}
}

func writeTempProg(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "prog.nvs")
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDAPBreakpointSession runs a full scripted session: launch,
// breakpoint, stop, stack, variables, step, continue to termination.
func TestDAPBreakpointSession(t *testing.T) {
	prog := writeTempProg(t, `let x = 1
let y = x + 41
print y
`)
	c, clientOut, clientIn := newTestClient(t)
	defer clientOut.Close()
	defer clientIn.Close()

	c.send("initialize", map[string]any{})
	// Give the server a moment to answer (responses are not tracked by
	// this minimal client; events are what we assert on).
	time.Sleep(100 * time.Millisecond)

	c.send("launch", map[string]any{"program": prog})
	stopped := make(chan map[string]any, 1)
	go func() { stopped <- c.waitEvent("stopped") }()

	c.send("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": prog},
		"breakpoints": []map[string]any{{"line": 2}},
	})
	c.send("configurationDone", map[string]any{})

	ev := <-stopped
	body := ev["body"].(map[string]any)
	if body["reason"] != "breakpoint" {
		t.Fatalf("want stopped reason breakpoint, got %v", body["reason"])
	}

	// stackTrace / scopes / variables are asserted via a second,
	// response-tracking client below; here we just continue on.
	c.send("continue", map[string]any{})
	term := c.waitEvent("terminated")
	if term["event"] != "terminated" {
		t.Fatalf("want terminated event, got %v", term["event"])
	}
	c.send("disconnect", map[string]any{})
}

// trackingClient extends testClient with response matching.
type trackingClient struct {
	*testClient
	resps   map[int]chan map[string]any
	respsMu sync.Mutex
}

func newTrackingClient(t *testing.T) (*trackingClient, *io.PipeWriter, *io.PipeReader) {
	t.Helper()
	serverIn, clientOut := io.Pipe() // client writes, server reads
	clientIn, serverOut := io.Pipe() // server writes, client reads
	go Serve(serverIn, serverOut)
	c := &trackingClient{
		testClient: &testClient{
			t:      t,
			w:      clientOut,
			rd:     bufio.NewReader(clientIn),
			events: make(chan map[string]any, 64),
		},
		resps: map[int]chan map[string]any{},
	}
	go c.routeLoop()
	return c, clientOut, clientIn
}

func (c *trackingClient) routeLoop() {
	for {
		body, err := readMessage(c.rd)
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "event":
			c.events <- msg
		case "response":
			rs := int(msg["request_seq"].(float64))
			c.respsMu.Lock()
			ch := c.resps[rs]
			c.respsMu.Unlock()
			if ch != nil {
				ch <- msg
			}
		}
	}
}

func (c *trackingClient) request(command string, args any) map[string]any {
	c.t.Helper()
	c.seq++
	ch := make(chan map[string]any, 1)
	c.respsMu.Lock()
	c.resps[c.seq] = ch
	c.respsMu.Unlock()
	body, _ := json.Marshal(map[string]any{
		"seq": c.seq, "type": "request", "command": command, "arguments": args,
	})
	fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body))
	if _, err := c.w.Write(body); err != nil {
		c.t.Fatalf("client write failed: %v", err)
	}
	select {
	case r := <-ch:
		return r
	case <-time.After(10 * time.Second):
		c.t.Fatalf("timed out waiting for response to %q", command)
		return nil
	}
}

// TestDAPInspectSession exercises stackTrace, scopes, variables,
// evaluate, and stepIn against a real paused program.
func TestDAPInspectSession(t *testing.T) {
	prog := writeTempProg(t, `fn add(a, b) {
  let s = a + b
  return s
}
let r = add(20, 22)
print r
`)
	c, clientOut, clientIn := newTrackingClient(t)
	defer clientOut.Close()
	defer clientIn.Close()

	r := c.request("initialize", map[string]any{})
	if r["success"] != true {
		t.Fatalf("initialize failed: %v", r)
	}
	r = c.request("launch", map[string]any{"program": prog})
	if r["success"] != true {
		t.Fatalf("launch failed: %v", r)
	}
	// Break inside add(), at the `let s` line.
	r = c.request("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": prog},
		"breakpoints": []map[string]any{{"line": 2}},
	})
	bps := r["body"].(map[string]any)["breakpoints"].([]any)
	if len(bps) != 1 || bps[0].(map[string]any)["verified"] != true {
		t.Fatalf("breakpoint not verified: %v", r["body"])
	}
	c.request("configurationDone", map[string]any{})

	ev := c.waitEvent("stopped")
	if ev["body"].(map[string]any)["reason"] != "breakpoint" {
		t.Fatalf("want breakpoint stop, got %v", ev["body"])
	}

	// stackTrace: innermost frame is add() at line 2.
	r = c.request("stackTrace", map[string]any{"threadId": 1})
	frames := r["body"].(map[string]any)["stackFrames"].([]any)
	if len(frames) != 2 {
		t.Fatalf("want 2 frames (add + toplevel), got %d", len(frames))
	}
	f0 := frames[0].(map[string]any)
	if f0["name"] != "add" {
		t.Fatalf("want innermost frame add, got %v", f0["name"])
	}
	if int(f0["line"].(float64)) != 2 {
		t.Fatalf("want innermost frame at line 2, got %v", f0["line"])
	}
	src := f0["source"].(map[string]any)
	if !strings.HasSuffix(src["path"].(string), "prog.nvs") {
		t.Fatalf("bad frame source: %v", src)
	}

	// scopes + variables: frame 0 has Locals with a=20, b=22.
	r = c.request("scopes", map[string]any{"frameId": 0})
	scopes := r["body"].(map[string]any)["scopes"].([]any)
	if len(scopes) != 1 {
		t.Fatalf("want 1 scope, got %d", len(scopes))
	}
	ref := int(scopes[0].(map[string]any)["variablesReference"].(float64))
	if ref == 0 {
		t.Fatal("want nonzero variablesReference for frame 0")
	}
	r = c.request("variables", map[string]any{"variablesReference": ref})
	vars := r["body"].(map[string]any)["variables"].([]any)
	vals := map[string]string{}
	for _, v := range vars {
		vm := v.(map[string]any)
		vals[vm["name"].(string)] = vm["value"].(string)
	}
	if vals["a"] != "20" || vals["b"] != "22" {
		t.Fatalf("want a=20 b=22, got %v", vals)
	}

	// evaluate in the paused frame.
	r = c.request("evaluate", map[string]any{"expression": "a * b", "frameId": 0, "context": "repl"})
	if r["success"] != true {
		t.Fatalf("evaluate failed: %v", r)
	}
	if r["body"].(map[string]any)["result"] != "440" {
		t.Fatalf("want evaluate a*b = 440, got %v", r["body"])
	}

	// Outer frame has no variables (documented limitation).
	r = c.request("scopes", map[string]any{"frameId": 1})
	scopes = r["body"].(map[string]any)["scopes"].([]any)
	if int(scopes[0].(map[string]any)["variablesReference"].(float64)) != 0 {
		t.Fatal("want variablesReference 0 for outer frame")
	}

	// stepIn then continue to the end.
	c.request("stepIn", map[string]any{})
	ev = c.waitEvent("stopped")
	if ev["body"].(map[string]any)["reason"] != "step" {
		t.Fatalf("want step stop, got %v", ev["body"])
	}
	c.request("continue", map[string]any{})
	c.waitEvent("terminated")
	c.request("disconnect", map[string]any{})
}

// TestDAPForeignBreakpointUnverified checks the honest rule that
// breakpoints only verify in the launched file.
func TestDAPForeignBreakpointUnverified(t *testing.T) {
	prog := writeTempProg(t, "print 1\n")
	c, clientOut, clientIn := newTrackingClient(t)
	defer clientOut.Close()
	defer clientIn.Close()

	c.request("initialize", map[string]any{})
	c.request("launch", map[string]any{"program": prog})
	r := c.request("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": filepath.Join(os.TempDir(), "other.nvs")},
		"breakpoints": []map[string]any{{"line": 1}},
	})
	bps := r["body"].(map[string]any)["breakpoints"].([]any)
	if bps[0].(map[string]any)["verified"] != false {
		t.Fatalf("foreign breakpoint should be unverified: %v", bps)
	}
	c.request("disconnect", map[string]any{})
}

// TestFramingRoundTrip guards the Content-Length codec.
func TestFramingRoundTrip(t *testing.T) {
	var sb strings.Builder
	payload := `{"seq":1,"type":"request","command":"initialize"}`
	fmt.Fprintf(&sb, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
	br := bufio.NewReader(strings.NewReader(sb.String()))
	raw, err := readMessage(br)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != payload {
		t.Fatalf("framing round trip mismatch: %q", raw)
	}
	if strconv.Itoa(len(payload)) == "" {
		t.Fatal("unreachable")
	}
}
