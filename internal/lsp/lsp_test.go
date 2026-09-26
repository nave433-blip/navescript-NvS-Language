package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClient speaks LSP to a Serve instance over in-memory pipes.
type testClient struct {
	t       *testing.T
	w       io.Writer
	mu      sync.Mutex
	raw     chan []byte
	pending [][]byte
	id      int
}

func newTestClient(t *testing.T) (*testClient, func()) {
	t.Helper()
	sr, cw := io.Pipe() // client -> server
	cr, sw := io.Pipe() // server -> client
	c := &testClient{t: t, w: cw, raw: make(chan []byte, 64)}
	go Serve(sr, sw)
	go func() {
		br := bufio.NewReader(cr)
		for {
			raw, err := readMessage(br)
			if err != nil {
				return
			}
			c.raw <- raw
		}
	}()
	return c, func() {}
}

func (c *testClient) send(method string, params any, wantReply bool) int {
	c.t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if wantReply {
		msg["id"] = c.id
	}
	if params != nil {
		msg["params"] = params
	}
	body, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatal(err)
	}
	frame := append([]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))), body...)
	if _, err := c.w.Write(frame); err != nil {
		c.t.Fatal(err)
	}
	return c.id
}

func (c *testClient) notify(method string, params any) {
	c.t.Helper()
	c.send(method, params, false)
}

func (c *testClient) request(method string, params any) int {
	c.t.Helper()
	return c.send(method, params, true)
}

// nextReply waits for the response with the given id, returning raw body.
func (c *testClient) nextReply(id int) []byte {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		// Check stashed messages first.
		for i, raw := range c.pending {
			var m rpcMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			var got int
			if len(m.ID) > 0 && json.Unmarshal(m.ID, &got) == nil && got == id {
				c.pending = append(c.pending[:i], c.pending[i+1:]...)
				return raw
			}
		}
		select {
		case raw := <-c.raw:
			var m rpcMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			var got int
			if len(m.ID) > 0 && json.Unmarshal(m.ID, &got) == nil && got == id {
				return raw
			}
			c.pending = append(c.pending, raw) // a notification; stash it
		case <-timeout:
			c.t.Fatalf("timed out waiting for reply to request %d", id)
		}
	}
}

// nextNotification waits for a notification with the given method,
// returning the raw body.
func (c *testClient) nextNotification(method string) []byte {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		for i, raw := range c.pending {
			var m rpcMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if len(m.ID) == 0 && m.Method == method {
				c.pending = append(c.pending[:i], c.pending[i+1:]...)
				return raw
			}
		}
		select {
		case raw := <-c.raw:
			var m rpcMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if len(m.ID) == 0 && m.Method == method {
				return raw
			}
			c.pending = append(c.pending, raw)
		case <-timeout:
			c.t.Fatalf("timed out waiting for notification %q", method)
		}
	}
}

func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("cannot decode message: %v", err)
	}
	return v
}

func resultOf[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v struct {
		Result T `json:"result"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("cannot decode result: %v", err)
	}
	return v.Result
}

func initialize(t *testing.T, c *testClient) {
	t.Helper()
	id := c.request("initialize", map[string]any{
		"processId":    nil,
		"rootUri":      "file:///tmp",
		"capabilities": map[string]any{},
	})
	raw := c.nextReply(id)
	var v struct {
		Result struct {
			Capabilities map[string]any `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("cannot decode initialize result: %v", err)
	}
	for _, cap := range []string{"hoverProvider", "completionProvider", "definitionProvider", "documentSymbolProvider", "textDocumentSync"} {
		if _, ok := v.Result.Capabilities[cap]; !ok {
			t.Fatalf("initialize result missing capability %q", cap)
		}
	}
	if sync, _ := v.Result.Capabilities["textDocumentSync"].(float64); sync != 1 {
		t.Fatalf("expected textDocumentSync=1, got %v", v.Result.Capabilities["textDocumentSync"])
	}
	c.notify("initialized", map[string]any{})
}

func didOpen(t *testing.T, c *testClient, uri, text string) {
	t.Helper()
	c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": "nvs",
			"version":    1,
			"text":       text,
		},
	})
}

func TestLSPDiagnosticsTypeError(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/typeerr.ns"
	didOpen(t, c, uri, "let x: int = \"hello\";\n")

	raw := c.nextNotification("textDocument/publishDiagnostics")
	var p struct {
		Params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Range    lspRange `json:"range"`
				Severity int      `json:"severity"`
				Source   string   `json:"source"`
				Message  string   `json:"message"`
			} `json:"diagnostics"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.Params.URI != uri {
		t.Fatalf("diagnostics for wrong uri: %q", p.Params.URI)
	}
	found := false
	for _, d := range p.Params.Diagnostics {
		t.Logf("diag line=%d msg=%q", d.Range.Start.Line, d.Message)
		if d.Range.Start.Line == 0 && d.Severity == 1 && d.Source == "nvs" &&
			strings.Contains(d.Message, "cannot assign string") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a type-error diagnostic on line 0, got %+v", p.Params.Diagnostics)
	}

	// didChange with fixed code must clear the diagnostics.
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{"text": "let x: int = 5;\n"}},
	})
	raw = c.nextNotification("textDocument/publishDiagnostics")
	p = struct {
		Params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Range    lspRange `json:"range"`
				Severity int      `json:"severity"`
				Source   string   `json:"source"`
				Message  string   `json:"message"`
			} `json:"diagnostics"`
		} `json:"params"`
	}{}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Params.Diagnostics) != 0 {
		t.Fatalf("expected diagnostics to clear after fix, got %+v", p.Params.Diagnostics)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPDiagnosticsBrokenInput(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/broken.ns"
	didOpen(t, c, uri, "fn (((\nlet = =\n")
	raw := c.nextNotification("textDocument/publishDiagnostics")
	var p struct {
		Params struct {
			Diagnostics []struct {
				Message string `json:"message"`
			} `json:"diagnostics"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Params.Diagnostics) == 0 {
		t.Fatal("expected parse-error diagnostics for broken input, got none")
	}
	t.Logf("broken input diagnostics: %d", len(p.Params.Diagnostics))

	// Server must still be alive: hover over garbage must not hang/crash.
	id := c.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 0, "character": 1},
	})
	raw = c.nextReply(id)
	res := resultOf[map[string]any](t, raw)
	t.Logf("hover on broken input: %v", res)

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPHoverDocumentedFunction(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/hover.ns"
	src := "/// Adds two numbers.\n" +
		"/// Returns their sum.\n" +
		"fn add(a: int, b: int): int {\n" +
		"    return a + b;\n" +
		"}\n" +
		"\n" +
		"let r = add(1, 2);\n"
	didOpen(t, c, uri, src)
	c.nextNotification("textDocument/publishDiagnostics") // drain

	// 0-based: line 6, "let r = " is 8 chars so `add` starts at char 8.
	id := c.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 6, "character": 9},
	})
	raw := c.nextReply(id)
	res := resultOf[hoverResult](t, raw)
	t.Logf("hover contents: %q", res.Contents.Value)
	if !strings.Contains(res.Contents.Value, "Adds two numbers.") {
		t.Fatalf("hover missing doc comment, got %q", res.Contents.Value)
	}
	if !strings.Contains(res.Contents.Value, "fn add(a: int, b: int): int") {
		t.Fatalf("hover missing signature, got %q", res.Contents.Value)
	}

	// Hover over a builtin.
	didOpen(t, c, "file:///tmp/builtin.ns", "let n = len([1, 2]);\n")
	c.nextNotification("textDocument/publishDiagnostics") // drain
	id = c.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": "file:///tmp/builtin.ns"},
		"position":     map[string]any{"line": 0, "character": 9},
	})
	raw = c.nextReply(id)
	res = resultOf[hoverResult](t, raw)
	if !strings.Contains(res.Contents.Value, "`len` — builtin") {
		t.Fatalf("expected builtin hover, got %q", res.Contents.Value)
	}

	// Hover over an unknown name.
	didOpen(t, c, "file:///tmp/unknown.ns", "print(undefined_name_xyz);\n")
	c.nextNotification("textDocument/publishDiagnostics") // drain
	id = c.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": "file:///tmp/unknown.ns"},
		"position":     map[string]any{"line": 0, "character": 8},
	})
	raw = c.nextReply(id)
	res = resultOf[hoverResult](t, raw)
	if res.Contents.Value != "no documentation" {
		t.Fatalf("expected 'no documentation', got %q", res.Contents.Value)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPCompletion(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/compl.ns"
	didOpen(t, c, uri, "fn myhelper() {\n    return 1;\n}\n\nlet myvar = 2;\n")
	c.nextNotification("textDocument/publishDiagnostics") // drain

	id := c.request("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 4, "character": 0},
	})
	raw := c.nextReply(id)
	res := resultOf[completionList](t, raw)
	labels := map[string]bool{}
	for _, it := range res.Items {
		labels[it.Label] = true
	}
	for _, want := range []string{"len", "myhelper", "myvar", "fn", "let", "if"} {
		if !labels[want] {
			t.Fatalf("completion missing %q (have %d items)", want, len(res.Items))
		}
	}
	if res.IsIncomplete {
		t.Fatal("expected isIncomplete=false")
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPDefinition(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/def.ns"
	src := "let myvar = 42;\nprint(myvar);\n"
	didOpen(t, c, uri, src)
	c.nextNotification("textDocument/publishDiagnostics") // drain

	// `myvar` usage on 0-based line 1; "print(" is 6 chars.
	id := c.request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 1, "character": 7},
	})
	raw := c.nextReply(id)
	locs := resultOf[[]location](t, raw)
	if len(locs) == 0 {
		t.Fatal("expected a definition location, got none")
	}
	if locs[0].URI != uri {
		t.Fatalf("wrong uri: %q", locs[0].URI)
	}
	if locs[0].Range.Start.Line != 0 {
		t.Fatalf("expected definition on line 0, got %+v", locs[0].Range)
	}
	t.Logf("definition range: %+v", locs[0].Range)

	// Definition of a function parameter inside the function body.
	uri2 := "file:///tmp/defparam.ns"
	didOpen(t, c, uri2, "fn greet(name) {\n    print(name);\n}\n")
	c.nextNotification("textDocument/publishDiagnostics") // drain
	id = c.request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri2},
		"position":     map[string]any{"line": 1, "character": 11},
	})
	raw = c.nextReply(id)
	locs = resultOf[[]location](t, raw)
	if len(locs) == 0 || locs[0].Range.Start.Line != 0 {
		t.Fatalf("expected param definition on line 0, got %+v", locs)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPDocumentSymbol(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	uri := "file:///tmp/syms.ns"
	src := "/// Doc for f.\nfn f() {\n}\n\nlet v = 1;\n\nclass C {\n}\n\nenum E {\n}\n"
	didOpen(t, c, uri, src)
	c.nextNotification("textDocument/publishDiagnostics") // drain

	id := c.request("textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	raw := c.nextReply(id)
	syms := resultOf[[]documentSymbol](t, raw)
	byName := map[string]documentSymbol{}
	for _, s := range syms {
		byName[s.Name] = s
		t.Logf("symbol %q kind=%d range=%+v", s.Name, s.Kind, s.Range)
	}
	want := map[string]int{"f": symFunction, "v": symVariable, "C": symClass, "E": symEnum}
	for name, kind := range want {
		s, ok := byName[name]
		if !ok {
			t.Fatalf("missing symbol %q", name)
		}
		if s.Kind != kind {
			t.Fatalf("symbol %q: expected kind %d, got %d", name, kind, s.Kind)
		}
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPUnknownMethod(t *testing.T) {
	c, _ := newTestClient(t)
	initialize(t, c)

	id := c.request("workspace/definitelyNotAMethod", map[string]any{"query": "x"})
	raw := c.nextReply(id)
	var v struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if v.Error == nil || v.Error.Code != -32601 {
		t.Fatalf("expected -32601 method-not-found error, got %+v", v.Error)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}
