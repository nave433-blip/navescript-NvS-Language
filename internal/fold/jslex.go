package fold

// JavaScript subset lexer for the Wave 15 importer.

import (
	"fmt"
	"strings"
)

type jsKind int

const (
	jsEOF jsKind = iota
	jsName
	jsNum
	jsStr // val = decoded value; template marks backtick literals (val = raw with ${})
	jsOp
)

type jsTok struct {
	kind     jsKind
	text     string // raw: name, number, operator
	val      string // decoded string for jsStr
	template bool   // backtick literal
	line     int
}

func (t jsTok) String() string {
	switch t.kind {
	case jsEOF:
		return "EOF"
	case jsStr:
		return fmt.Sprintf("STR(%q)", t.val)
	case jsName:
		return "NAME(" + t.text + ")"
	case jsNum:
		return "NUM(" + t.text + ")"
	default:
		return "OP(" + t.text + ")"
	}
}

var jsMultiOps = []string{
	">>>=", "<<=", ">>=", "**=", "...",
	"===", "!==", "=>", "?.", "??",
	"==", "!=", "<=", ">=", "++", "--", "**", "&&", "||",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
	"<<", ">>",
}

func isJsNameStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isJsNameChar(c byte) bool { return isJsNameStart(c) || (c >= '0' && c <= '9') }

func lexJS(src string) ([]jsTok, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var toks []jsTok
	line := 1
	i := 0
	fail := func(detail string) ([]jsTok, error) {
		return nil, &SyntaxError{Lang: "js", Line: line, Detail: detail}
	}
	emit := func(t jsTok) { toks = append(toks, t) }
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return fail("unterminated block comment")
			}
			line += strings.Count(src[i:i+2+j], "\n")
			i += 2 + j + 2
		case c == '\'' || c == '"':
			t, ni, err := lexJsString(src, i, line, &line)
			if err != nil {
				return nil, err
			}
			emit(t)
			i = ni
		case c == '`':
			t, ni, nl, err := lexJsTemplate(src, i, line)
			if err != nil {
				return nil, err
			}
			line = nl
			emit(t)
			i = ni
		case isJsNameStart(c):
			j := i + 1
			for j < len(src) && isJsNameChar(src[j]) {
				j++
			}
			emit(jsTok{kind: jsName, text: src[i:j], line: line})
			i = j
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9'):
			t, ni, err := lexJsNumber(src, i, line)
			if err != nil {
				return nil, err
			}
			emit(t)
			i = ni
		default:
			matched := false
			for _, op := range jsMultiOps {
				if strings.HasPrefix(src[i:], op) {
					// Distinguish "/" regex? No regex literals in the subset.
					emit(jsTok{kind: jsOp, text: op, line: line})
					i += len(op)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			switch c {
			case '(', ')', '[', ']', '{', '}', ',', ';', ':', '.', '?', '~',
				'+', '-', '*', '/', '%', '<', '>', '=', '!', '&', '|', '^':
				emit(jsTok{kind: jsOp, text: string(c), line: line})
				i++
			default:
				return fail(fmt.Sprintf("unexpected character %q", string(c)))
			}
		}
	}
	emit(jsTok{kind: jsEOF, line: line})
	return toks, nil
}

func lexJsString(src string, i, line int, linep *int) (jsTok, int, error) {
	q := src[i]
	j := i + 1
	var sb strings.Builder
	for j < len(src) {
		c := src[j]
		if c == q {
			return jsTok{kind: jsStr, val: sb.String(), line: line}, j + 1, nil
		}
		if c == '\\' {
			if j+1 >= len(src) {
				break
			}
			e := src[j+1]
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
			case 'u':
				if j+5 < len(src) {
					var r rune
					_, err := fmt.Sscanf(src[j+2:j+6], "%04x", &r)
					if err == nil {
						sb.WriteRune(r)
						j += 6
						continue
					}
				}
				return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: "unsupported \\u escape in string"}
			default:
				return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: fmt.Sprintf("unsupported escape %q in string", "\\"+string(e))}
			}
			j += 2
			continue
		}
		if c == '\n' {
			return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: "unterminated string literal"}
		}
		sb.WriteByte(c)
		j++
	}
	return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: "unterminated string literal"}
}

// lexJsTemplate lexes a backtick template literal; val holds the raw body
// with ${...} holes intact (escapes processed).
func lexJsTemplate(src string, i, line int) (jsTok, int, int, error) {
	j := i + 1
	var sb strings.Builder
	for j < len(src) {
		c := src[j]
		if c == '`' {
			return jsTok{kind: jsStr, val: sb.String(), template: true, line: line}, j + 1, line, nil
		}
		if c == '\\' {
			if j+1 >= len(src) {
				break
			}
			e := src[j+1]
			switch e {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '`':
				sb.WriteByte('`')
			case '$':
				sb.WriteByte('$')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(e)
			}
			j += 2
			continue
		}
		if c == '\n' {
			line++
		}
		sb.WriteByte(c)
		j++
	}
	return jsTok{}, 0, line, &SyntaxError{Lang: "js", Line: line, Detail: "unterminated template literal"}
}

func lexJsNumber(src string, i, line int) (jsTok, int, error) {
	j := i
	if src[j] == '0' && j+1 < len(src) && (src[j+1] == 'x' || src[j+1] == 'X' || src[j+1] == 'o' || src[j+1] == 'O' || src[j+1] == 'b' || src[j+1] == 'B') {
		return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: "non-decimal numeric literals are outside the importable subset"}
	}
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		j++
	}
	if j < len(src) && src[j] == '.' {
		j++
		for j < len(src) && src[j] >= '0' && src[j] <= '9' {
			j++
		}
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		j++
		if j < len(src) && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j >= len(src) || src[j] < '0' || src[j] > '9' {
			return jsTok{}, 0, &SyntaxError{Lang: "js", Line: line, Detail: "malformed numeric literal"}
		}
		for j < len(src) && src[j] >= '0' && src[j] <= '9' {
			j++
		}
	}
	return jsTok{kind: jsNum, text: src[i:j], line: line}, j, nil
}
