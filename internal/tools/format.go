// Package tools implements the nvs developer-tooling subcommands:
//
//	fmt  — lexical code formatter (gofmt/rustfmt idea)
//	lint — a small set of sound static checks (clippy idea)
//	doc  — minimal doc-comment extractor (cargo-doc idea, honestly a stub)
//
// All three are deliberately small and honest about their limits; see
// LANGUAGE.md "Wave 9 — tooling" for the documented scope.
package tools

import (
	"os"
	"strings"
)

// ---------------------------------------------------------------------------
// fmt — lexical formatter
// ---------------------------------------------------------------------------
//
// Design decision: NvS AST String() methods are lossy (try prints as
// "try { ... }", match as "match (...) { ... }", defer as "defer ..."), so an
// AST-based pretty-printer cannot round-trip. The formatter is therefore
// LEXICAL: it never parses, it only re-indents and tidies whitespace.
//
// What it normalizes:
//   - indentation: 4 spaces per nesting level of { } ( ) [ ]
//   - tabs → 4 spaces (outside string literals only)
//   - trailing whitespace removed (outside multi-line strings/comments)
//   - consecutive blank lines collapsed to at most one; leading blank lines dropped
//   - exactly one trailing newline
//
// What it deliberately leaves alone:
//   - everything inside string literals (including ${...} interpolation —
//     braces there do NOT affect indentation)
//   - // line comments and /* */ block comments (content preserved verbatim;
//     lines inside a multi-line block comment or string are not re-indented)
//   - spacing inside a line (no opinion on `f (x)` vs `f(x)`, operator spacing, …)
//
// Guarantees: idempotent (Format(Format(x)) == Format(x)) and behavior
// preserving (it only touches whitespace outside strings/comments).

const fmtIndent = "    " // 4 spaces per level

// Format returns the canonical formatting of src.
func Format(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")

	st := &fmtScanner{}
	depth := 0
	var out []string
	prevBlank := true // drop leading blank lines

	for _, raw := range lines {
		formatted, isBlank := st.formatLine(raw, &depth)
		if isBlank {
			if prevBlank {
				continue
			}
			prevBlank = true
			out = append(out, "")
			continue
		}
		prevBlank = false
		out = append(out, formatted)
	}

	// Drop trailing blank lines; exactly one trailing newline below.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// fmtScanner carries the multi-line lexical state (string / block comment)
// across lines.
type fmtScanner struct {
	inString       bool
	quote          rune
	interpDepth    int // >0 while inside ${...} within a string
	inBlockComment bool
}

// formatLine formats one line. It returns the formatted line and whether the
// line is blank. depth is the current nesting depth and is updated in place
// for the next line.
func (st *fmtScanner) formatLine(raw string, depth *int) (string, bool) {
	// A line that begins inside a multi-line string or block comment is
	// preserved VERBATIM (not even trailing-whitespace stripping — trailing
	// spaces inside a string are significant). We still scan it to update
	// the lexical state.
	if st.inString || st.inBlockComment {
		st.scanLine(raw)
		return raw, false
	}

	// Expand tabs to 4 spaces outside strings. Trailing whitespace is stripped
	// here ONLY if the line does not end inside a string literal — trailing
	// spaces on a line where a string continues are significant.
	line, endsInString := st.expandTabs(raw)
	if !endsInString {
		line = strings.TrimRight(line, " \t")
	}

	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		st.scanLine(line) // no-op state-wise, keeps accounting simple
		return "", true
	}

	// Leading closers dedent this line: `}`, `)`, `]`.
	leadingClosers := 0
	for _, r := range trimmed {
		if r == '}' || r == ')' || r == ']' {
			leadingClosers++
		} else {
			break
		}
	}
	eff := *depth - leadingClosers
	if eff < 0 {
		eff = 0
	}

	delta := st.scanLine(line) // net depth change from this line's brackets
	*depth += delta
	if *depth < 0 {
		*depth = 0 // unbalanced closers (would be a parse error anyway)
	}

	return strings.Repeat(fmtIndent, eff) + trimmed, false
}

// expandTabs replaces tabs with 4 spaces outside string literals and
// reports whether the line ends inside a string (i.e. the string continues
// on the next line). Tabs inside strings are significant and kept.
func (st *fmtScanner) expandTabs(line string) (string, bool) {
	var b strings.Builder
	b.Grow(len(line) + 8)
	runes := []rune(line)
	i := 0
	inStr := false
	var q rune
	for i < len(runes) {
		r := runes[i]
		if inStr {
			b.WriteRune(r)
			if r == '\\' && i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
			} else if r == q {
				inStr = false
			} else if r == '$' && i+1 < len(runes) && runes[i+1] == '{' {
				// Copy the interpolation raw; quotes inside can't end the
				// outer string, and we don't expand tabs in there either —
				// safest is to copy through the balanced ${...}.
				b.WriteRune(runes[i+1])
				i += 2
				id := 1
				for i < len(runes) && id > 0 {
					c := runes[i]
					b.WriteRune(c)
					if c == '\\' && i+1 < len(runes) {
						i++
						b.WriteRune(runes[i])
					} else if c == '"' || c == '\'' {
						qq := c
						i++
						for i < len(runes) && runes[i] != qq {
							if runes[i] == '\\' && i+1 < len(runes) {
								b.WriteRune(runes[i])
								i++
							}
							b.WriteRune(runes[i])
							i++
						}
						if i < len(runes) {
							b.WriteRune(runes[i])
						} else {
							i--
						}
					} else if c == '{' {
						id++
					} else if c == '}' {
						id--
					}
					i++
				}
				continue
			}
			i++
			continue
		}
		switch r {
		case '\t':
			b.WriteString("    ")
		case '"', '\'':
			inStr = true
			q = r
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
		i++
	}
	return b.String(), inStr
}

// scanLine walks a line tracking strings/comments/interpolation and returns
// the net bracket-depth delta from code outside strings and comments.
// It updates the scanner's multi-line state (inString, inBlockComment).
func (st *fmtScanner) scanLine(line string) int {
	delta := 0
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case st.inBlockComment:
			if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				st.inBlockComment = false
				i += 2
			} else {
				i++
			}
		case st.inString:
			switch {
			case r == '\\':
				i += 2 // escape: skip next rune
			case st.interpDepth == 0 && r == st.quote:
				st.inString = false
				i++
			case r == '$' && i+1 < len(runes) && runes[i+1] == '{':
				st.interpDepth++
				i += 2
			case st.interpDepth > 0 && r == '{':
				st.interpDepth++
				i++
			case st.interpDepth > 0 && r == '}':
				st.interpDepth--
				i++
			case st.interpDepth > 0 && (r == '"' || r == '\''):
				// Nested string inside interpolation: consume raw so its
				// quotes/braces don't confuse the outer string tracking
				// (mirrors the lexer's readString).
				q := r
				i++
				for i < len(runes) && runes[i] != q {
					if runes[i] == '\\' {
						i++
					}
					i++
				}
				if i < len(runes) {
					i++ // closing quote
				}
			default:
				i++
			}
		default: // code
			switch {
			case r == '/' && i+1 < len(runes) && runes[i+1] == '/':
				return delta // rest of line is a comment
			case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
				st.inBlockComment = true
				i += 2
			case r == '"' || r == '\'':
				st.inString = true
				st.quote = r
				st.interpDepth = 0
				i++
			case r == '{' || r == '(' || r == '[':
				delta++
				i++
			case r == '}' || r == ')' || r == ']':
				delta--
				i++
			default:
				i++
			}
		}
	}
	return delta
}

// FormatFile formats the file at path in place. It reports whether the file
// changed. The original file mode is preserved.
func FormatFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted := Format(string(data))
	if formatted == string(data) {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(formatted), info.Mode()); err != nil {
		return false, err
	}
	return true, nil
}
