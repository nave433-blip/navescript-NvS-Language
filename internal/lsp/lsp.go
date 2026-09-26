// Package lsp implements a Language Server Protocol (LSP) server for NvS.
//
// The server speaks the LSP base protocol (Content-Length framed JSON-RPC
// 2.0 messages) over stdio and is exposed via Serve(r, w) so the `nvs lsp`
// command can wire it to os.Stdin/os.Stdout. It is hand-rolled with zero
// third-party dependencies.
//
// Implemented methods:
//   - initialize / initialized / shutdown / exit
//   - textDocument/didOpen, textDocument/didChange (full sync)
//   - textDocument/publishDiagnostics (parser + checker errors, never panics)
//   - textDocument/hover (signatures + /// doc comments, builtins)
//   - textDocument/completion (builtins + keywords + user names)
//   - textDocument/definition (same-file, then follows imports)
//   - textDocument/documentSymbol (top-level symbols)
//   - textDocument/references (name-based across open files)
//   - workspace/symbol (substring search across open files)
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/navescript/nvs/internal/checker"
)

// ---------------------------------------------------------------------------
// JSON-RPC / LSP wire types
// ---------------------------------------------------------------------------

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type textDocumentID struct {
	URI string `json:"uri"`
}

type positionParams struct {
	TextDocument textDocumentID `json:"textDocument"`
	Position     position       `json:"position"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
	Range    *lspRange     `json:"range,omitempty"`
}

type completionItem struct {
	Label  string `json:"label"`
	Kind   int    `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []completionItem `json:"items"`
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"` // 1 = Error
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type documentSymbol struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
}

// LSP SymbolKind values we emit.
const (
	symClass     = 5
	symEnum      = 10
	symInterface = 11
	symFunction  = 12
	symVariable  = 13
	symConstant  = 14
	symStruct    = 23
)

// LSP CompletionItemKind values we emit.
const (
	cmpFunction = 3
	cmpVariable = 6
	cmpClass    = 7
	cmpEnum     = 10
	cmpKeyword  = 14
)

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type server struct {
	mu       sync.Mutex // guards out and docs
	out      io.Writer
	docs     map[string]string
	shutdown bool
}

// Serve runs the LSP server: it reads Content-Length framed JSON-RPC
// messages from r, dispatches them, and writes framed responses to w.
// It returns when the client sends the `exit` notification.
func Serve(r io.Reader, w io.Writer) {
	s := &server{out: w, docs: map[string]string{}}
	br := bufio.NewReader(r)
	for {
		raw, err := readMessage(br)
		if err != nil {
			// EOF or a framing error: nothing more we can do.
			return
		}
		exit := false
		func() {
			defer func() {
				// Never let a bad document kill the server: a broken
				// input produces diagnostics, not a panic.
				_ = recover()
			}()
			exit = s.handle(raw)
		}()
		if exit {
			return
		}
	}
}

// readMessage reads one LSP base-protocol message: headers terminated by a
// blank line, then exactly Content-Length bytes of body. The Content-Type
// header is optional and ignored when absent.
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
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
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

func (s *server) writeFrame(body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body))
	_, _ = s.out.Write(body)
}

func (s *server) reply(id json.RawMessage, result any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(mustJSON(id)),
		"result":  result,
	})
	s.writeFrame(body)
}

func (s *server) replyError(id json.RawMessage, code int, message string) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(mustJSON(id)),
		"error":   rpcError{Code: code, Message: message},
	})
	s.writeFrame(body)
}

func (s *server) notify(method string, params any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	s.writeFrame(body)
}

func mustJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	return []byte(raw)
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func (s *server) handle(raw []byte) (exit bool) {
	var msg rpcMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return false
	}
	isRequest := len(msg.ID) > 0

	switch msg.Method {
	case "initialize":
		s.reply(msg.ID, map[string]any{
			"capabilities": map[string]any{
				"hoverProvider":          true,
				"completionProvider":     map[string]any{},
				"definitionProvider":     true,
				"documentSymbolProvider": true,
				"workspaceSymbolProvider": true,
				"textDocumentSync":       1,
			},
			"serverInfo": map[string]any{"name": "nvs-lsp", "version": "0.1.0"},
		})

	case "initialized":
		// no-op

	case "shutdown":
		s.shutdown = true
		s.reply(msg.ID, nil)

	case "exit":
		return true

	case "$/cancelRequest":
		// no-op: requests are answered synchronously

	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &p) == nil && p.TextDocument.URI != "" {
			s.setDoc(p.TextDocument.URI, p.TextDocument.Text)
			s.publishDiagnostics(p.TextDocument.URI)
		}

	case "textDocument/didChange":
		var p struct {
			TextDocument   textDocumentID `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(msg.Params, &p) == nil && p.TextDocument.URI != "" && len(p.ContentChanges) > 0 {
			// Full sync: the last change replaces the whole document.
			last := p.ContentChanges[len(p.ContentChanges)-1]
			s.setDoc(p.TextDocument.URI, last.Text)
			s.publishDiagnostics(p.TextDocument.URI)
		}

	case "textDocument/didClose":
		var p struct {
			TextDocument textDocumentID `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			s.mu.Lock()
			delete(s.docs, p.TextDocument.URI)
			s.mu.Unlock()
			s.notify("textDocument/publishDiagnostics", map[string]any{
				"uri":         p.TextDocument.URI,
				"diagnostics": []diagnostic{},
			})
		}

	case "textDocument/hover":
		var p positionParams
		if json.Unmarshal(msg.Params, &p) == nil {
			s.reply(msg.ID, s.hover(p.TextDocument.URI, p.Position))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid hover params")
		}

	case "textDocument/completion":
		var p struct {
			TextDocument textDocumentID `json:"textDocument"`
			Position     position       `json:"position"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			s.reply(msg.ID, s.completion(p.TextDocument.URI))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid completion params")
		}

	case "textDocument/definition":
		var p positionParams
		if json.Unmarshal(msg.Params, &p) == nil {
			s.reply(msg.ID, s.definition(p.TextDocument.URI, p.Position))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid definition params")
		}

	case "workspace/symbol":
		var pws struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(msg.Params, &pws) == nil {
			s.reply(msg.ID, s.workspaceSymbols(pws.Query))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid workspace/symbol params")
		}

	case "textDocument/references":
		var pr positionParams
		if json.Unmarshal(msg.Params, &pr) == nil {
			s.reply(msg.ID, s.references(pr.TextDocument.URI, pr.Position))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid references params")
		}

	case "textDocument/documentSymbol":
		var p struct {
			TextDocument textDocumentID `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			s.reply(msg.ID, s.documentSymbols(p.TextDocument.URI))
		} else if isRequest {
			s.replyError(msg.ID, -32602, "invalid documentSymbol params")
		}

	default:
		if isRequest {
			s.replyError(msg.ID, -32601, "method not found: "+msg.Method)
		}
		// Unknown notifications are ignored.
	}
	return false
}

func (s *server) setDoc(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = text
}

func (s *server) getDoc(uri string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.docs[uri]
	return t, ok
}

// ---------------------------------------------------------------------------
// Diagnostics: parser errors + checker.CheckSource, merged.
// ---------------------------------------------------------------------------

func (s *server) publishDiagnostics(uri string) {
	src, ok := s.getDoc(uri)
	if !ok {
		return
	}
	s.notify("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diagnosticsFor(uri, src),
	})
}

func diagnosticsFor(uri, src string) []diagnostic {
	errs := checker.CheckSource(uri, src)
	diags := make([]diagnostic, 0, len(errs))
	for _, e := range errs {
		line := e.Line - 1 // CheckError.Line is 1-based; parse errors use 0
		if line < 0 {
			line = 0
		}
		diags = append(diags, diagnostic{
			Range: lspRange{
				Start: position{Line: line, Character: 0},
				End:   position{Line: line, Character: 1},
			},
			Severity: 1,
			Source:   "nvs",
			Message:  e.Msg,
		})
	}
	return diags
}
