// Package fold implements NvS Wave 15: folding code between languages.
//
// Part A — foreign -> NvS importers ("nvs import"):
// hand-written subset parsers for Python and JavaScript that emit NvS
// source. Anything outside the documented subset fails LOUDLY with an
// *UnsupportedError naming the construct and line number — the package
// never emits silently-wrong NvS.
//
// Part B — "nvs extract": runs @nvs blocks embedded in foreign source
// files inside one shared interpreter session.
//
// Honest scope: these are documented-subset converters, not full language
// frontends. See docs/FOLDING.md for the exact supported subsets.
package fold

import (
	"fmt"
	"os"
	"strings"
)

// UnsupportedError is returned when the input uses a construct outside the
// documented importable subset. It always names the construct and its line
// number so the user knows exactly what to rewrite.
type UnsupportedError struct {
	Lang      string // "python" or "js"
	Line      int
	Construct string
	Detail    string
}

func (e *UnsupportedError) Error() string {
	where := ""
	if e.Line > 0 {
		where = fmt.Sprintf(" (line %d)", e.Line)
	}
	return fmt.Sprintf("import error: unsupported %s construct: %s%s — %s",
		e.Lang, e.Construct, where, e.Detail)
}

// SyntaxError is a malformed-input error (bad indentation, unterminated
// string, ...). Unlike UnsupportedError it means the input itself is broken.
type SyntaxError struct {
	Lang   string
	Line   int
	Detail string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("import error: invalid %s syntax (line %d): %s", e.Lang, e.Line, e.Detail)
}

// ImportSource converts foreign source text to NvS source.
// lang is "python" (aliases: "py") or "js" (aliases: "javascript").
func ImportSource(src, lang string) (string, error) {
	switch strings.ToLower(lang) {
	case "python", "py":
		return importPython(src)
	case "js", "javascript":
		return importJS(src)
	default:
		return "", fmt.Errorf("import: unknown --from=%q (want python or js)", lang)
	}
}

// ImportFile reads path and converts it to NvS source.
func ImportFile(path, lang string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("import: cannot read %s: %v", path, err)
	}
	return ImportSource(string(data), lang)
}

// emitter accumulates NvS output with indentation.
type emitter struct {
	sb  strings.Builder
	ind int
}

func (e *emitter) line(s string) {
	for i := 0; i < e.ind; i++ {
		e.sb.WriteString("  ")
	}
	e.sb.WriteString(s)
	e.sb.WriteString("\n")
}

func (e *emitter) open(s string) {
	e.line(s + " {")
	e.ind++
}

func (e *emitter) close() {
	e.ind--
	e.line("}")
}

func (e *emitter) closeElse(s string) {
	// "} else {" / "} else if (c) {" — dedent, emit, re-indent.
	e.ind--
	e.line("} " + s + " {")
	e.ind++
}

func (e *emitter) String() string { return e.sb.String() }
