// Package bridge hosts the persistent-interpreter sessions shared by the
// C ABI bridge (cbridge/) and the JSON stdio bridge (`nvs bridge`).
//
// A Session holds ONE NvS interpreter environment for its whole lifetime:
// functions and bindings defined by one Eval call are visible to later
// Eval/Call calls. All calls are serialized with a mutex — concurrent
// hosts share a single global interpreter, never two.
package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
	"github.com/navescript/nvs/internal/polyglot"
)

// Session is a persistent NvS interpreter: one environment, prelude loaded
// once, serialized access.
type Session struct {
	mu  sync.Mutex
	env *object.Environment
}

// NewSession creates a session with a fresh environment and the prelude loaded.
func NewSession() *Session {
	env := object.NewEnvironment()
	eval.LoadPrelude(env)
	return &Session{env: env}
}

// Eval parses and evaluates src in the session's persistent environment.
// Anything src defines (functions, bindings) stays visible afterwards.
// A non-terminating program blocks the calling thread, exactly like
// `nvs run` would.
func (s *Session) Eval(src string) (object.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l := lexer.New(src)
	p := parser.New(l)
	program := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("parse error: %s", strings.Join(errs, "; "))
	}
	eval.CurrentFile = ""
	result := eval.Eval(program, s.env)
	// Wave 6: same as `nvs run` — wait for spawned tasks so no task output
	// or state is lost to an early return.
	eval.DrainSpawnedTasks()
	if result == nil {
		return &object.Null{}, nil
	}
	if errObj, ok := result.(*object.Error); ok {
		return nil, fmt.Errorf("%s", errObj.Message)
	}
	return result, nil
}

// Call applies a named function from the session environment to args.
// args are JSON-decoded values (use polyglot.DecodeJSONArgs / a decoder
// with UseNumber). The name must resolve to an NvS function or builtin;
// anything else is an honest error. Arguments are positional only.
func (s *Session) Call(name string, args []any) (object.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fn, ok := s.env.Get(name)
	if !ok {
		// Mirror evalIdentifier: fall back to the builtin registry so
		// nvs_call can reach builtins (len, str, ...) too.
		if b, ok := eval.LookupBuiltin(name); ok {
			fn = b
		} else {
			return nil, fmt.Errorf("unknown function: %q", name)
		}
	}
	switch fn.(type) {
	case *object.Function, *object.Builtin:
	default:
		return nil, fmt.Errorf("%q is not callable (NvS %s)", name, fn.Type())
	}
	conv := make([]object.Object, len(args))
	for i, a := range args {
		o, err := polyglot.JSONValueToNvS(a)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %s", i, err)
		}
		conv[i] = o
	}
	result := eval.ApplyFunction(fn, conv)
	eval.DrainSpawnedTasks()
	if errObj, ok := result.(*object.Error); ok {
		return nil, fmt.Errorf("%s", errObj.Message)
	}
	if result == nil {
		return &object.Null{}, nil
	}
	return result, nil
}

// Envelope renders the shared {"ok":...} JSON envelope for an Eval/Call outcome.
func (s *Session) Envelope(obj object.Object, err error) string {
	return polyglot.EnvelopeJSON(obj, err)
}

// HandleLine implements one step of the `nvs bridge` stdio protocol.
// It never panics: any malformed input yields an error envelope.
//
//	{"eval": "<nvs source>", "id": <any>}          -> {"ok":true,"result":<json>,"id":<id>}
//	{"call": "<name>", "args": [...], "id": <any>} -> same envelope (shares Eval's env)
//	anything else                                  -> {"ok":false,"error":"..."}
//
// The "id" field is optional; when present it is echoed verbatim in the
// response so clients can match replies to requests. The response always
// ends with a single \n; request lines must be complete JSON on one line.
func (s *Session) HandleLine(line string) string {
	resp := s.handleLineInner(line)
	// Echo the request id, if any, so clients can correlate responses.
	var raw map[string]any
	dec := json.NewDecoder(bytes.NewReader([]byte(line)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err == nil {
		if id, ok := raw["id"]; ok {
			var env map[string]any
			if err := json.Unmarshal([]byte(resp), &env); err == nil {
				env["id"] = id
				if data, err := json.Marshal(env); err == nil {
					return string(data)
				}
			}
		}
	}
	return resp
}

func (s *Session) handleLineInner(line string) string {
	dec := json.NewDecoder(bytes.NewReader([]byte(line)))
	dec.UseNumber()
	var req map[string]any
	if err := dec.Decode(&req); err != nil {
		return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: %s", err))
	}
	if dec.More() {
		return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: trailing data after JSON object"))
	}
	if src, ok := req["eval"]; ok {
		srcStr, ok := src.(string)
		if !ok {
			return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: \"eval\" must be a string"))
		}
		obj, err := s.Eval(srcStr)
		return s.Envelope(obj, err)
	}
	if name, ok := req["call"]; ok {
		nameStr, ok := name.(string)
		if !ok {
			return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: \"call\" must be a string"))
		}
		var args []any
		if rawArgs, present := req["args"]; present {
			arr, ok := rawArgs.([]any)
			if !ok {
				return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: \"args\" must be a JSON array"))
			}
			args = arr
		}
		obj, err := s.Call(nameStr, args)
		return s.Envelope(obj, err)
	}
	return polyglot.EnvelopeJSON(nil, fmt.Errorf("invalid request: want {\"eval\": \"...\"} or {\"call\": \"...\", \"args\": [...]}"))
}
