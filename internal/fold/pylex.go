package fold

// Python subset lexer for the Wave 15 importer.
//
// Produces INDENT/DEDENT/NEWLINE structure like CPython's tokenizer, so the
// parser sees block structure explicitly. Only the documented subset needs
// to lex; everything else either lexes generically (and fails in the parser
// with a precise UnsupportedError) or fails here with a SyntaxError.

import (
	"fmt"
	"strings"
)

type pyKind int

const (
	pyEOF pyKind = iota
	pyNewline
	pyIndent
	pyDedent
	pyName
	pyInt
	pyFloat
	pyStr // decoded value in val; fstr marks f-strings (val holds raw text with braces)
	pyOp
)

type pyTok struct {
	kind pyKind
	text string // raw text: name, number literal, operator
	val  string // decoded string for pyStr
	fstr bool   // pyStr originated from an f-string
	line int
}

func (t pyTok) String() string {
	switch t.kind {
	case pyEOF:
		return "EOF"
	case pyNewline:
		return "NEWLINE"
	case pyIndent:
		return "INDENT"
	case pyDedent:
		return "DEDENT"
	case pyStr:
		return fmt.Sprintf("STR(%q)", t.val)
	default:
		return fmt.Sprintf("%s(%s)", kindName(t.kind), t.text)
	}
}

func kindName(k pyKind) string {
	switch k {
	case pyName:
		return "NAME"
	case pyInt:
		return "INT"
	case pyFloat:
		return "FLOAT"
	case pyOp:
		return "OP"
	}
	return "?"
}

var pyMultiOps = []string{
	"**=", "//=", "+=", "-=", "*=", "/=", "%=", // lexed; parser rejects with a clear error where unsupported
	"==", "!=", "<=", ">=", "**", "//", "->", ":=",
}

func isPyNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isPyNameChar(c byte) bool { return isPyNameStart(c) || (c >= '0' && c <= '9') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lexPython tokenizes the documented Python subset.
func lexPython(src string) ([]pyTok, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")

	var toks []pyTok
	indents := []int{0}
	depth := 0 // bracket depth for implicit line joining
	emit := func(t pyTok) { toks = append(toks, t) }

	i := 0
	for i < len(lines) {
		lineno := i + 1
		line := lines[i]
		// Explicit line joining with backslash (outside strings).
		for strings.HasSuffix(line, "\\") && !strings.HasSuffix(line, "\\\\") && i+1 < len(lines) {
			line = line[:len(line)-1] + lines[i+1]
			i++
		}
		stripped := strings.TrimSpace(line)
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			i++
			continue // blank/comment lines never affect structure
		}
		// Indentation.
		j := 0
		spaces, tabs := 0, 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			if line[j] == ' ' {
				spaces++
			} else {
				tabs++
			}
			j++
		}
		if tabs > 0 && spaces > 0 {
			return nil, &SyntaxError{Lang: "python", Line: lineno, Detail: "mixed tabs and spaces in indentation"}
		}
		indent := spaces + tabs*8
		content := line[j:]
		if depth == 0 {
			top := indents[len(indents)-1]
			switch {
			case indent > top:
				indents = append(indents, indent)
				emit(pyTok{kind: pyIndent, line: lineno})
			case indent < top:
				for len(indents) > 1 && indents[len(indents)-1] > indent {
					indents = indents[:len(indents)-1]
					emit(pyTok{kind: pyDedent, line: lineno})
				}
				if indents[len(indents)-1] != indent {
					return nil, &SyntaxError{Lang: "python", Line: lineno, Detail: "unindent does not match any outer indentation level"}
				}
			}
		}
		lt, dd, err := lexPyLine(content, lineno)
		if err != nil {
			return nil, err
		}
		depth += dd
		if depth < 0 {
			return nil, &SyntaxError{Lang: "python", Line: lineno, Detail: "unbalanced closing bracket"}
		}
		toks = append(toks, lt...)
		if depth == 0 {
			emit(pyTok{kind: pyNewline, line: lineno})
		}
		i++
	}
	if depth != 0 {
		return nil, &SyntaxError{Lang: "python", Line: len(lines), Detail: "unbalanced brackets at end of file"}
	}
	for len(indents) > 1 {
		indents = indents[:len(indents)-1]
		emit(pyTok{kind: pyDedent, line: len(lines)})
	}
	emit(pyTok{kind: pyEOF, line: len(lines)})
	return toks, nil
}

// lexPyLine tokenizes one logical line's content; returns tokens and the
// net bracket-depth delta.
func lexPyLine(s string, lineno int) ([]pyTok, int, error) {
	var toks []pyTok
	depth := 0
	i := 0
	fail := func(detail string) ([]pyTok, int, error) {
		return nil, 0, &SyntaxError{Lang: "python", Line: lineno, Detail: detail}
	}
	for i < len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\f' {
			i++
			continue
		}
		if c == '#' {
			break // comment: rest of line ignored
		}
		// String prefixes f/F (only f supported); r/b/u rejected loudly.
		if (c == 'f' || c == 'F' || c == 'r' || c == 'R' || c == 'b' || c == 'B' || c == 'u' || c == 'U') &&
			i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '"') {
			if c != 'f' && c != 'F' {
				return fail(fmt.Sprintf("unsupported string prefix %q", string(c)))
			}
			t, ni, err := lexPyString(s, i+1, lineno, true)
			if err != nil {
				return nil, 0, err
			}
			toks = append(toks, t)
			i = ni
			continue
		}
		if c == '\'' || c == '"' {
			t, ni, err := lexPyString(s, i, lineno, false)
			if err != nil {
				return nil, 0, err
			}
			toks = append(toks, t)
			i = ni
			continue
		}
		if isPyNameStart(c) {
			j := i + 1
			for j < len(s) && isPyNameChar(s[j]) {
				j++
			}
			toks = append(toks, pyTok{kind: pyName, text: s[i:j], line: lineno})
			i = j
			continue
		}
		if isDigit(c) || (c == '.' && i+1 < len(s) && isDigit(s[i+1])) {
			t, ni, err := lexPyNumber(s, i, lineno)
			if err != nil {
				return nil, 0, err
			}
			toks = append(toks, t)
			i = ni
			continue
		}
		// Multi-char operators first.
		matched := false
		for _, op := range pyMultiOps {
			if strings.HasPrefix(s[i:], op) {
				toks = append(toks, pyTok{kind: pyOp, text: op, line: lineno})
				i += len(op)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
			toks = append(toks, pyTok{kind: pyOp, text: string(c), line: lineno})
		case ')', ']', '}':
			depth--
			toks = append(toks, pyTok{kind: pyOp, text: string(c), line: lineno})
		case '+', '-', '*', '/', '%', '<', '>', '=', '!', '&', '|', '^', '~', ',', ':', ';', '.', '@':
			toks = append(toks, pyTok{kind: pyOp, text: string(c), line: lineno})
		case '?':
			return fail("unexpected character '?'")
		default:
			return fail(fmt.Sprintf("unexpected character %q", string(c)))
		}
		i++
	}
	return toks, depth, nil
}

// lexPyString lexes a string starting at s[i] (the quote). Returns the
// token and the index just past the closing quote.
func lexPyString(s string, i, lineno int, fstr bool) (pyTok, int, error) {
	q := s[i]
	triple := i+2 < len(s) && s[i+1] == q && s[i+2] == q
	quote := string(q)
	if triple {
		quote = string([]byte{q, q, q})
	}
	j := i + len(quote)
	var sb strings.Builder
	for j < len(s) {
		if strings.HasPrefix(s[j:], quote) {
			// For f-strings keep the raw text (braces included); escapes
			// still processed so \\n etc. behave.
			return pyTok{kind: pyStr, val: sb.String(), fstr: fstr, line: lineno}, j + len(quote), nil
		}
		c := s[j]
		if c == '\\' {
			if j+1 >= len(s) {
				break
			}
			e := s[j+1]
			switch e {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '\'':
				sb.WriteByte('\'')
			case '"':
				sb.WriteByte('"')
			case '0':
				sb.WriteByte(0)
			default:
				return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno,
					Detail: fmt.Sprintf("unsupported escape sequence %q in string", "\\"+string(e))}
			}
			j += 2
			continue
		}
		if c == '\n' && !triple {
			return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno, Detail: "unterminated string literal"}
		}
		sb.WriteByte(c)
		j++
	}
	return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno, Detail: "unterminated string literal"}
}

func lexPyNumber(s string, i, lineno int) (pyTok, int, error) {
	j := i
	if s[j] == '0' && j+1 < len(s) && (s[j+1] == 'x' || s[j+1] == 'X' || s[j+1] == 'o' || s[j+1] == 'O' || s[j+1] == 'b' || s[j+1] == 'B') {
		return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno,
			Detail: "non-decimal integer literals are outside the importable subset (use decimal)"}
	}
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	isFloat := false
	if j < len(s) && s[j] == '.' {
		isFloat = true
		j++
		for j < len(s) && isDigit(s[j]) {
			j++
		}
	}
	if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
		isFloat = true
		j++
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j >= len(s) || !isDigit(s[j]) {
			return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno, Detail: "malformed numeric literal"}
		}
		for j < len(s) && isDigit(s[j]) {
			j++
		}
	}
	text := s[i:j]
	// Underscores in literals are not supported by the subset lexer.
	if strings.Contains(text, "_") {
		return pyTok{}, 0, &SyntaxError{Lang: "python", Line: lineno, Detail: "underscores in numeric literals are outside the importable subset"}
	}
	kind := pyInt
	if isFloat {
		kind = pyFloat
	}
	return pyTok{kind: kind, text: text, line: lineno}, j, nil
}
