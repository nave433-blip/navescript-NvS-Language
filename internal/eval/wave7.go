package eval

// Wave 7 — standard library robbery.
//
// Fills genuine gaps in the NvS builtin set, stdlib-only Go packages,
// no new mandatory external dependencies. Everything here is additive:
// no existing builtin changes shape or meaning (the only edit is
// `env` gaining an optional default argument).
//
// New builtins:
//
//	Datetime:    now_iso, unixtime_ms, date_format, parse_date, date_add
//	             (now() = unix seconds and date([ts[, layout]]) already existed)
//	HTTP:        http_request(method, url, opts?) -> {status, headers, body}
//	             (http_get/http_post keep their body-string return shape)
//	Crypto:      sha1, hmac_sha256 (md5/sha256 already existed;
//	             md5/sha1 are fingerprinting hashes, NOT security primitives)
//	Base64:      base64url_encode, base64url_decode
//	             (base64_encode/base64_decode already existed; uuid() already existed)
//	Subprocess:  exec(cmd, args...), sh(cmd) -> {code, stdout, stderr}
//	             (system() already existed but swallows the exit code and
//	             merges stderr into stdout)
//	Env:         env(name, default?) — set_env / args already existed
//	Path:        extname, abs_path
//	             (basename/dirname/join_path/exists already existed)
//	TOML:        toml_parse (documented subset — see tomlParseDoc)
//	Compression: gzip_compress, gzip_decompress (Go strings are byte-safe,
//	             so binary gzip data round-trips through NvS strings)
//
// Deliberately NOT added (documented in LANGUAGE.md):
//   - YAML: a correct YAML parser is genuinely hard (anchors, aliases,
//     multi-doc, the Norway problem). We will not ship a lying subset.
//   - uuid4(): uuid() already exists and is a v4 UUID.

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/object"
)

func registerWave7Builtins() {
	// ---------------- datetime ----------------

	builtins["now_iso"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return newError("now_iso: want 0 arguments")
			}
			return &object.String{Value: time.Now().UTC().Format(time.RFC3339)}
		},
	}
	builtins["unixtime_ms"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return newError("unixtime_ms: want 0 arguments")
			}
			return &object.Integer{Value: time.Now().UnixMilli()}
		},
	}
	builtins["date_format"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("date_format: want unix_seconds, layout")
			}
			ts, ok := args[0].(*object.Integer)
			if !ok {
				return newError("date_format: unix_seconds must be integer")
			}
			layout, ok := args[1].(*object.String)
			if !ok {
				return newError("date_format: layout must be string")
			}
			return &object.String{Value: time.Unix(ts.Value, 0).Format(layout.Value)}
		},
	}
	builtins["parse_date"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("parse_date: want 1 argument (date string)")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("parse_date: argument must be string")
			}
			t, err := wave7ParseDate(s.Value)
			if err != nil {
				return newError("parse_date: %s", err.Error())
			}
			return &object.Integer{Value: t.Unix()}
		},
	}
	builtins["date_add"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("date_add: want unix_seconds, amount, unit")
			}
			ts, ok := args[0].(*object.Integer)
			if !ok {
				return newError("date_add: unix_seconds must be integer")
			}
			amt, ok := args[1].(*object.Integer)
			if !ok {
				return newError("date_add: amount must be integer")
			}
			unit, ok := args[2].(*object.String)
			if !ok {
				return newError("date_add: unit must be string")
			}
			mult, ok := wave7DateUnits[strings.ToLower(strings.TrimSpace(unit.Value))]
			if !ok {
				return newError("date_add: unknown unit %q (want s/m/h/d/w or the long name)", unit.Value)
			}
			return &object.Integer{Value: ts.Value + amt.Value*mult}
		},
	}

	// ---------------- fuller HTTP ----------------

	builtins["http_request"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 || len(args) > 3 {
				return newError("http_request: want method, url [, opts]")
			}
			method, ok := args[0].(*object.String)
			if !ok {
				return newError("http_request: method must be string")
			}
			url, ok := args[1].(*object.String)
			if !ok {
				return newError("http_request: url must be string")
			}
			var bodyReader io.Reader
			reqHeaders := http.Header{}
			timeout := 15 * time.Second
			if len(args) == 3 {
				opts, ok := args[2].(*object.Hash)
				if !ok {
					return newError("http_request: opts must be a hash")
				}
				for _, pair := range opts.Pairs {
					ks, ok := pair.Key.(*object.String)
					if !ok {
						return newError("http_request: opts keys must be strings")
					}
					switch ks.Value {
					case "headers":
						hh, ok := pair.Value.(*object.Hash)
						if !ok {
							return newError("http_request: opts[\"headers\"] must be a hash")
						}
						for _, hp := range hh.Pairs {
							hn, ok := hp.Key.(*object.String)
							if !ok {
								return newError("http_request: header names must be strings")
							}
							switch hv := hp.Value.(type) {
							case *object.String:
								reqHeaders.Add(hn.Value, hv.Value)
							case *object.Array:
								for _, e := range hv.Elements {
									es, ok := e.(*object.String)
									if !ok {
										return newError("http_request: header values must be strings")
									}
									reqHeaders.Add(hn.Value, es.Value)
								}
							default:
								return newError("http_request: header values must be strings or arrays of strings")
							}
						}
					case "body":
						b := pair.Value.Inspect()
						if bs, ok := pair.Value.(*object.String); ok {
							b = bs.Value
						}
						bodyReader = strings.NewReader(b)
					case "timeout":
						switch tv := pair.Value.(type) {
						case *object.Integer:
							timeout = time.Duration(tv.Value) * time.Second
						case *object.Float:
							timeout = time.Duration(tv.Value * float64(time.Second))
						default:
							return newError("http_request: opts[\"timeout\"] must be a number (seconds)")
						}
					default:
						return newError("http_request: unknown opt %q (want headers, body, timeout)", ks.Value)
					}
				}
			}
			req, err := http.NewRequest(strings.ToUpper(strings.TrimSpace(method.Value)), url.Value, bodyReader)
			if err != nil {
				return newError("http_request: %s", err.Error())
			}
			for k, vs := range reqHeaders {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			client := &http.Client{Timeout: timeout}
			resp, err := client.Do(req)
			if err != nil {
				return newError("http_request: %s", err.Error())
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return newError("http_request: read body: %s", err.Error())
			}
			// Response: {status, headers, body}. headers maps name -> array
			// of values (lossless: HTTP allows repeated header fields).
			hpairs := map[object.HashKey]object.HashPair{}
			for k, vs := range resp.Header {
				els := make([]object.Object, len(vs))
				for i, v := range vs {
					els[i] = &object.String{Value: v}
				}
				hk := &object.String{Value: k}
				hpairs[hk.HashKey()] = object.HashPair{Key: hk, Value: &object.Array{Elements: els}}
			}
			pairs := map[object.HashKey]object.HashPair{}
			wave7SetStrKey(pairs, "status", &object.Integer{Value: int64(resp.StatusCode)})
			wave7SetStrKey(pairs, "headers", &object.Hash{Pairs: hpairs})
			wave7SetStrKey(pairs, "body", &object.String{Value: string(data)})
			return &object.Hash{Pairs: pairs}
		},
	}

	// ---------------- crypto: the missing hashes ----------------

	builtins["sha1"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sha1: want 1 argument")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("sha1: argument must be string")
			}
			sum := sha1.Sum([]byte(s.Value))
			return &object.String{Value: hex.EncodeToString(sum[:])}
		},
	}
	builtins["hmac_sha256"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("hmac_sha256: want key, message")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return newError("hmac_sha256: key must be string")
			}
			msg, ok := args[1].(*object.String)
			if !ok {
				return newError("hmac_sha256: message must be string")
			}
			mac := hmac.New(sha256.New, []byte(key.Value))
			mac.Write([]byte(msg.Value))
			return &object.String{Value: hex.EncodeToString(mac.Sum(nil))}
		},
	}

	// ---------------- base64: url-safe variants ----------------

	builtins["base64url_encode"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("base64url_encode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("base64url_encode: want string")
			}
			return &object.String{Value: base64.RawURLEncoding.EncodeToString([]byte(s.Value))}
		},
	}
	builtins["base64url_decode"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("base64url_decode: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("base64url_decode: want string")
			}
			b, err := base64.RawURLEncoding.DecodeString(s.Value)
			if err != nil {
				return newError("base64url_decode: %s", err.Error())
			}
			return &object.String{Value: string(b)}
		},
	}

	// ---------------- subprocess: honest exit codes ----------------

	builtins["exec"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("exec: want command [, args...]")
			}
			cmdStr, ok := args[0].(*object.String)
			if !ok {
				return newError("exec: command must be string")
			}
			argv := make([]string, 0, len(args)-1)
			for _, a := range args[1:] {
				s, ok := a.(*object.String)
				if !ok {
					return newError("exec: arguments must be strings (no shell quoting is done — use sh() for shell syntax)")
				}
				argv = append(argv, s.Value)
			}
			return wave7RunProcess(exec.Command(cmdStr.Value, argv...))
		},
	}
	builtins["sh"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("sh: want 1 argument (shell command string)")
			}
			cmdStr, ok := args[0].(*object.String)
			if !ok {
				return newError("sh: command must be string")
			}
			return wave7RunProcess(shellCommand(cmdStr.Value))
		},
	}

	// ---------------- path utilities: the missing pieces ----------------

	builtins["extname"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("extname: want path")
			}
			p, ok := args[0].(*object.String)
			if !ok {
				return newError("extname: path must be string")
			}
			return &object.String{Value: filepath.Ext(p.Value)}
		},
	}
	builtins["abs_path"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("abs_path: want path")
			}
			p, ok := args[0].(*object.String)
			if !ok {
				return newError("abs_path: path must be string")
			}
			abs, err := filepath.Abs(p.Value)
			if err != nil {
				return newError("abs_path: %s", err.Error())
			}
			return &object.String{Value: abs}
		},
	}

	// ---------------- TOML: documented subset ----------------

	builtins["toml_parse"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("toml_parse: want 1 argument (toml string)")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("toml_parse: argument must be string")
			}
			obj, err := tomlParse(s.Value)
			if err != nil {
				return newError("toml_parse: %s", err.Error())
			}
			return obj
		},
	}

	// ---------------- compression ----------------

	builtins["gzip_compress"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("gzip_compress: want string")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("gzip_compress: want string")
			}
			var buf bytes.Buffer
			w := gzip.NewWriter(&buf)
			if _, err := w.Write([]byte(s.Value)); err != nil {
				return newError("gzip_compress: %s", err.Error())
			}
			if err := w.Close(); err != nil {
				return newError("gzip_compress: %s", err.Error())
			}
			// Go strings are byte-safe, so the raw gzip bytes round-trip
			// through NvS strings untouched.
			return &object.String{Value: buf.String()}
		},
	}
	builtins["gzip_decompress"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("gzip_decompress: want string (gzip bytes)")
			}
			s, ok := args[0].(*object.String)
			if !ok {
				return newError("gzip_decompress: want string")
			}
			r, err := gzip.NewReader(strings.NewReader(s.Value))
			if err != nil {
				return newError("gzip_decompress: %s", err.Error())
			}
			defer r.Close()
			data, err := io.ReadAll(r)
			if err != nil {
				return newError("gzip_decompress: %s", err.Error())
			}
			return &object.String{Value: string(data)}
		},
	}
}

func wave7SetStrKey(pairs map[object.HashKey]object.HashPair, key string, val object.Object) {
	ks := &object.String{Value: key}
	pairs[ks.HashKey()] = object.HashPair{Key: ks, Value: val}
}

// wave7RunProcess runs cmd, capturing stdout/stderr separately, and returns
// {code, stdout, stderr}. A non-zero exit is data, not an NvS error.
func wave7RunProcess(cmd *exec.Cmd) object.Object {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			// Could not even start the process (e.g. command not found):
			// that is an honest error.
			return newError("exec: %s", err.Error())
		}
	}
	pairs := map[object.HashKey]object.HashPair{}
	wave7SetStrKey(pairs, "code", &object.Integer{Value: int64(code)})
	wave7SetStrKey(pairs, "stdout", &object.String{Value: stdout.String()})
	wave7SetStrKey(pairs, "stderr", &object.String{Value: stderr.String()})
	return &object.Hash{Pairs: pairs}
}

// ---------------- datetime helpers ----------------

var wave7DateUnits = map[string]int64{
	"s": 1, "sec": 1, "second": 1, "seconds": 1,
	"m": 60, "min": 60, "minute": 60, "minutes": 60,
	"h": 3600, "hour": 3600, "hours": 3600,
	"d": 86400, "day": 86400, "days": 86400,
	"w": 604800, "week": 604800, "weeks": 604800,
}

// Layouts tried by parse_date, in order. Layouts without an explicit zone
// are interpreted in the local timezone (same convention as date()).
var wave7DateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.Kitchen,
	"2006/01/02",
}

func wave7ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range wave7DateLayouts {
		var t time.Time
		var err error
		if strings.Contains(layout, "Z07") || strings.Contains(layout, "MST") {
			t, err = time.Parse(layout, s)
		} else {
			t, err = time.ParseInLocation(layout, s, time.Local)
		}
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format %q (tried RFC3339, 2006-01-02[ 15:04:05], RFC1123/822, Kitchen)", s)
}

// ---------------- TOML subset parser ----------------

const tomlParseDoc = `
toml_parse implements a documented SUBSET of TOML, not the full spec.

Supported:
  - [table] and [table.sub] headers
  - key = value pairs; dotted keys (a.b.c = 1 creates nested tables)
  - bare keys ([A-Za-z0-9_-]+) and quoted keys ("k", 'k')
  - values: basic strings ("..." with \b \t \n \f \r \" \\ \uXXXX \UXXXXXXXX
    escapes), literal strings ('...' — no escapes), integers (decimal,
    0x/0o/0b, underscores), floats, booleans, single-line arrays
    (nested arrays ok, trailing comma ok)
  - # comments and blank lines

NOT supported (parse error): multi-line strings, multi-line arrays,
inline tables {k = v}, datetimes (quote them as strings), [[array-of-tables]],
inf/nan. Duplicate keys and redefined tables are errors.
`

func tomlParse(src string) (object.Object, error) {
	root := map[object.HashKey]object.HashPair{}
	defined := map[string]bool{"": true}
	cur := root
	lines := strings.Split(src, "\n")
	for ln, raw := range lines {
		lineNo := ln + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if strings.HasPrefix(line, "[[") {
				return nil, fmt.Errorf("line %d: [[array-of-tables]] is not supported in this TOML subset", lineNo)
			}
			end := tomlHeaderEnd(line)
			if end < 0 {
				return nil, fmt.Errorf("line %d: bad table header (missing ])", lineNo)
			}
			if rest := strings.TrimSpace(line[end+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
				return nil, fmt.Errorf("line %d: trailing content after table header", lineNo)
			}
			segs, err := tomlParseKey(line[1:end])
			if err != nil {
				return nil, fmt.Errorf("line %d: %s", lineNo, err.Error())
			}
			path := strings.Join(segs, ".")
			if defined[path] {
				return nil, fmt.Errorf("line %d: table [%s] defined twice", lineNo, path)
			}
			tbl, err := tomlEnsureTable(root, segs)
			if err != nil {
				return nil, fmt.Errorf("line %d: %s", lineNo, err.Error())
			}
			defined[path] = true
			cur = tbl
			continue
		}
		eq := tomlFindEq(line)
		if eq < 0 {
			return nil, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		segs, err := tomlParseKey(strings.TrimSpace(line[:eq]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %s", lineNo, err.Error())
		}
		val, err := tomlParseValue(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %s", lineNo, err.Error())
		}
		target := cur
		for _, seg := range segs[:len(segs)-1] {
			target, err = tomlChildTable(target, seg)
			if err != nil {
				return nil, fmt.Errorf("line %d: %s", lineNo, err.Error())
			}
		}
		last := segs[len(segs)-1]
		k := &object.String{Value: last}
		if _, exists := target[k.HashKey()]; exists {
			return nil, fmt.Errorf("line %d: duplicate key %q", lineNo, last)
		}
		target[k.HashKey()] = object.HashPair{Key: k, Value: val}
	}
	return &object.Hash{Pairs: root}, nil
}

// tomlEnsureTable walks/creates nested tables from root for segs.
func tomlEnsureTable(root map[object.HashKey]object.HashPair, segs []string) (map[object.HashKey]object.HashPair, error) {
	cur := root
	for _, seg := range segs {
		k := &object.String{Value: seg}
		if p, ok := cur[k.HashKey()]; ok {
			h, ok := p.Value.(*object.Hash)
			if !ok {
				return nil, fmt.Errorf("table [%s]: %q is already a value, not a table", strings.Join(segs, "."), seg)
			}
			cur = h.Pairs
			continue
		}
		h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
		cur[k.HashKey()] = object.HashPair{Key: k, Value: h}
		cur = h.Pairs
	}
	return cur, nil
}

// tomlChildTable returns the child table named seg, creating it if missing.
func tomlChildTable(pairs map[object.HashKey]object.HashPair, seg string) (map[object.HashKey]object.HashPair, error) {
	k := &object.String{Value: seg}
	if p, ok := pairs[k.HashKey()]; ok {
		h, ok := p.Value.(*object.Hash)
		if !ok {
			return nil, fmt.Errorf("key %q is already a value, not a table", seg)
		}
		return h.Pairs, nil
	}
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	pairs[k.HashKey()] = object.HashPair{Key: k, Value: h}
	return h.Pairs, nil
}

// tomlHeaderEnd finds the closing ] of a [table] header, respecting quotes.
func tomlHeaderEnd(line string) int {
	inStr := byte(0)
	esc := false
	for i := 1; i < len(line); i++ {
		c := line[i]
		if esc {
			esc = false
			continue
		}
		if inStr != 0 {
			if c == '\\' && inStr == '"' {
				esc = true
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			continue
		}
		if c == ']' {
			return i
		}
	}
	return -1
}

// tomlFindEq finds the first = outside of strings.
func tomlFindEq(line string) int {
	inStr := byte(0)
	esc := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if esc {
			esc = false
			continue
		}
		if inStr != 0 {
			if c == '\\' && inStr == '"' {
				esc = true
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			continue
		}
		if c == '=' {
			return i
		}
	}
	return -1
}

func tomlIsBareKeyChar(c byte) bool {
	return c == '_' || c == '-' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// tomlParseKey parses a (possibly dotted) key into segments.
func tomlParseKey(s string) ([]string, error) {
	var segs []string
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		var seg string
		switch s[i] {
		case '"':
			v, n, err := tomlBasicString(s[i:])
			if err != nil {
				return nil, err
			}
			seg, i = v, i+n
		case '\'':
			v, n, err := tomlLiteralString(s[i:])
			if err != nil {
				return nil, err
			}
			seg, i = v, i+n
		default:
			j := i
			for j < len(s) && tomlIsBareKeyChar(s[j]) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("bad key %q", s)
			}
			seg, i = s[i:j], j
		}
		segs = append(segs, seg)
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] != '.' {
			return nil, fmt.Errorf("bad key %q (expected . or end)", s)
		}
		i++
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty key")
	}
	return segs, nil
}

// tomlBasicString parses a "..." string starting at s[0], returning the
// value and the number of bytes consumed (including quotes).
func tomlBasicString(s string) (string, int, error) {
	var out strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return out.String(), i + 1, nil
		}
		if c == '\\' {
			i++
			if i >= len(s) {
				return "", 0, fmt.Errorf("unterminated escape in string")
			}
			e := s[i]
			switch e {
			case 'b':
				out.WriteByte('\b')
			case 't':
				out.WriteByte('\t')
			case 'n':
				out.WriteByte('\n')
			case 'f':
				out.WriteByte('\f')
			case 'r':
				out.WriteByte('\r')
			case '"':
				out.WriteByte('"')
			case '\\':
				out.WriteByte('\\')
			case 'u', 'U':
				n := 4
				if e == 'U' {
					n = 8
				}
				if i+n >= len(s) {
					return "", 0, fmt.Errorf("bad unicode escape in string")
				}
				cp, err := strconv.ParseUint(s[i+1:i+1+n], 16, 32)
				if err != nil {
					return "", 0, fmt.Errorf("bad unicode escape in string")
				}
				out.WriteRune(rune(cp))
				i += n
			default:
				return "", 0, fmt.Errorf("unknown escape \\%c in string", e)
			}
			i++
			continue
		}
		out.WriteByte(c)
		i++
	}
	return "", 0, fmt.Errorf("unterminated string")
}

// tomlLiteralString parses a '...' string starting at s[0].
func tomlLiteralString(s string) (string, int, error) {
	end := strings.IndexByte(s[1:], '\'')
	if end < 0 {
		return "", 0, fmt.Errorf("unterminated string")
	}
	return s[1 : 1+end], 1 + end + 1, nil
}

// tomlParseValue parses a single-line TOML value.
func tomlParseValue(s string) (object.Object, error) {
	if s == "" {
		return nil, fmt.Errorf("missing value")
	}
	switch s[0] {
	case '"':
		v, n, err := tomlBasicString(s)
		if err != nil {
			return nil, err
		}
		if err := tomlExpectRest(s[n:], "string"); err != nil {
			return nil, err
		}
		return &object.String{Value: v}, nil
	case '\'':
		v, n, err := tomlLiteralString(s)
		if err != nil {
			return nil, err
		}
		if err := tomlExpectRest(s[n:], "string"); err != nil {
			return nil, err
		}
		return &object.String{Value: v}, nil
	case '[':
		return tomlParseArray(s)
	case 't', 'f':
		var v bool
		var word string
		if strings.HasPrefix(s, "true") {
			v, word = true, "true"
		} else if strings.HasPrefix(s, "false") {
			v, word = false, "false"
		} else {
			return nil, fmt.Errorf("bad value %q", s)
		}
		if err := tomlExpectRest(s[len(word):], word); err != nil {
			return nil, err
		}
		return &object.Boolean{Value: v}, nil
	default:
		if (s[0] >= '0' && s[0] <= '9') || s[0] == '+' || s[0] == '-' {
			return tomlParseNumber(s)
		}
		return nil, fmt.Errorf("bad value %q", s)
	}
}

// tomlExpectRest allows trailing whitespace or a # comment after a value.
func tomlExpectRest(rest, what string) error {
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, "#") {
		return nil
	}
	return fmt.Errorf("trailing content after %s", what)
}

func tomlParseNumber(s string) (object.Object, error) {
	j := 0
	for j < len(s) && (tomlIsBareKeyChar(s[j]) || s[j] == '+' || s[j] == '.') {
		j++
	}
	tok := s[:j]
	if err := tomlExpectRest(s[j:], "number"); err != nil {
		return nil, err
	}
	clean := strings.ReplaceAll(tok, "_", "")
	if strings.ContainsAny(tok, ".eE") {
		f, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", tok)
		}
		return &object.Float{Value: f}, nil
	}
	n, err := strconv.ParseInt(clean, 0, 64)
	if err != nil {
		return nil, fmt.Errorf("bad number %q", tok)
	}
	return &object.Integer{Value: n}, nil
}

// tomlParseArray parses a single-line [...] array (nested arrays allowed).
func tomlParseArray(s string) (object.Object, error) {
	var els []object.Object
	i := 1 // past [
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil, fmt.Errorf("unterminated array (multi-line arrays are not supported in this TOML subset)")
		}
		if s[i] == ']' {
			if err := tomlExpectRest(s[i+1:], "array"); err != nil {
				return nil, err
			}
			return &object.Array{Elements: els}, nil
		}
		if s[i] == '#' {
			return nil, fmt.Errorf("comments inside arrays are not supported in this TOML subset")
		}
		el, n, err := tomlParseValueAt(s[i:])
		if err != nil {
			return nil, err
		}
		els = append(els, el)
		i += n
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil, fmt.Errorf("unterminated array (multi-line arrays are not supported in this TOML subset)")
		}
		if s[i] == ',' {
			i++
			continue
		}
		if s[i] == ']' {
			continue // loop top handles it
		}
		return nil, fmt.Errorf("expected , or ] in array")
	}
}

// tomlParseValueAt parses a value at the start of s, returning the value
// and bytes consumed (used inside arrays).
func tomlParseValueAt(s string) (object.Object, int, error) {
	switch s[0] {
	case '"':
		v, n, err := tomlBasicString(s)
		if err != nil {
			return nil, 0, err
		}
		return &object.String{Value: v}, n, nil
	case '\'':
		v, n, err := tomlLiteralString(s)
		if err != nil {
			return nil, 0, err
		}
		return &object.String{Value: v}, n, nil
	case '[':
		depth := 0
		inStr := byte(0)
		esc := false
		for i := 0; i < len(s); i++ {
			c := s[i]
			if esc {
				esc = false
				continue
			}
			if inStr != 0 {
				if c == '\\' && inStr == '"' {
					esc = true
				} else if c == inStr {
					inStr = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				inStr = c
				continue
			}
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
				if depth == 0 {
					arr, err := tomlParseArray(s[:i+1])
					return arr, i + 1, err
				}
			} else if c == '\n' {
				return nil, 0, fmt.Errorf("multi-line arrays are not supported in this TOML subset")
			}
		}
		return nil, 0, fmt.Errorf("unterminated array")
	case 't', 'f':
		if strings.HasPrefix(s, "true") {
			return &object.Boolean{Value: true}, 4, nil
		}
		if strings.HasPrefix(s, "false") {
			return &object.Boolean{Value: false}, 5, nil
		}
		return nil, 0, fmt.Errorf("bad value")
	default:
		if (s[0] >= '0' && s[0] <= '9') || s[0] == '+' || s[0] == '-' {
			j := 0
			for j < len(s) && (tomlIsBareKeyChar(s[j]) || s[j] == '+' || s[j] == '.') {
				j++
			}
			v, err := tomlParseNumber(strings.TrimSpace(s[:j]))
			if err != nil {
				return nil, 0, err
			}
			return v, j, nil
		}
		return nil, 0, fmt.Errorf("bad value %q", s)
	}
}

// Silence: tomlParseDoc documents the supported subset for LANGUAGE.md.
var _ = tomlParseDoc
