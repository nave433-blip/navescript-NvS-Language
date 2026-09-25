// Package highlight provides ANSI terminal syntax highlighting for NvS and related languages.
package highlight

import (
	"strings"

	"github.com/navescript/nvs/internal/lexer"
)

// ANSI styles
const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	white   = "\x1b[37m"
	gray    = "\x1b[90m"
)

// NvS highlights Navescript source using the real lexer.
func NvS(code string) string {
	l := lexer.New(code)
	var b strings.Builder
	for {
		tok := l.NextToken()
		if tok.Type == lexer.EOF {
			break
		}
		style := styleFor(tok.Type)
		// Reconstruct approximate spacing from literal only (lexer drops pure whitespace)
		// Emit token with color; insert space heuristics between tokens
		b.WriteString(style)
		b.WriteString(tok.Literal)
		b.WriteString(reset)
		if needsSpaceAfter(tok.Type) {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func styleFor(t lexer.TokenType) string {
	switch t {
	case lexer.FUNCTION, lexer.LET, lexer.CONST, lexer.IF, lexer.ELSE, lexer.RETURN,
		lexer.WHILE, lexer.FOR, lexer.IMPORT, lexer.PRINT, lexer.BREAK, lexer.CONTINUE,
		lexer.IN, lexer.CLASS, lexer.NEW, lexer.THIS, lexer.EXTENDS, lexer.TRY, lexer.CATCH,
		lexer.THROW, lexer.MATCH, lexer.CASE, lexer.DEFAULT, lexer.YIELD:
		return bold + magenta
	case lexer.TRUE, lexer.FALSE, lexer.NULL:
		return bold + yellow
	case lexer.INT, lexer.FLOAT:
		return cyan
	case lexer.STRING:
		return green
	case lexer.IDENT:
		return white
	case lexer.ASSIGN, lexer.PLUS_ASSIGN, lexer.MINUS_ASSIGN, lexer.STAR_ASSIGN, lexer.SLASH_ASSIGN,
		lexer.PLUS, lexer.MINUS, lexer.BANG, lexer.ASTERISK, lexer.SLASH, lexer.MOD,
		lexer.LT, lexer.GT, lexer.EQ, lexer.NOT_EQ, lexer.LTE, lexer.GTE,
		lexer.AND, lexer.OR, lexer.NULL_COAL, lexer.QUESTION,
		lexer.OPTIONAL_CHAIN, lexer.PIPE, lexer.ELLIPSIS:
		return yellow
	case lexer.ILLEGAL:
		return bold + red
	default:
		// delimiters
		if t == lexer.LPAREN || t == lexer.RPAREN || t == lexer.LBRACE || t == lexer.RBRACE ||
			t == lexer.LBRACKET || t == lexer.RBRACKET || t == lexer.COMMA || t == lexer.SEMICOLON ||
			t == lexer.COLON || t == lexer.DOT || t == lexer.AT {
			return dim + white
		}
		return reset
	}
}

func needsSpaceAfter(t lexer.TokenType) bool {
	switch t {
	case lexer.LPAREN, lexer.LBRACKET, lexer.LBRACE, lexer.DOT, lexer.AT:
		return false
	default:
		return true
	}
}

// Generic applies simple regex-free line highlighting for other languages.
func Generic(lang, code string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch lang {
	case "", "nvs", "navescript":
		// Prefer layout-preserving keyword highlighter; lexer-based drops whitespace
		return highlightKeywords(code, nvsKeywords)
	case "python", "py":
		return highlightKeywords(code, pythonKeywords)
	case "js", "javascript":
		return highlightKeywords(code, jsKeywords)
	case "go", "golang":
		return highlightKeywords(code, goKeywords)
	case "rust":
		return highlightKeywords(code, rustKeywords)
	case "ruby":
		return highlightKeywords(code, rubyKeywords)
	case "java", "c", "cpp", "c++":
		return highlightKeywords(code, cFamilyKeywords)
	default:
		return NvS(code)
	}
}

var (
	nvsKeywords     = []string{"fn", "let", "const", "if", "else", "return", "while", "for", "import", "print", "null", "true", "false", "break", "continue", "in", "class", "new", "this", "extends", "try", "catch", "throw", "match", "case", "default", "yield", "switch", "and", "or", "record", "interface"}
	pythonKeywords  = []string{"def", "return", "if", "elif", "else", "for", "while", "import", "from", "class", "True", "False", "None", "and", "or", "not", "in", "print", "with", "as", "try", "except", "raise", "yield", "lambda"}
	jsKeywords      = []string{"function", "return", "if", "else", "for", "while", "const", "let", "var", "class", "true", "false", "null", "undefined", "new", "this", "import", "export", "async", "await", "try", "catch", "throw"}
	goKeywords      = []string{"func", "return", "if", "else", "for", "range", "package", "import", "var", "const", "type", "struct", "interface", "true", "false", "nil", "go", "defer", "select", "case", "default", "switch", "map"}
	rustKeywords    = []string{"fn", "let", "mut", "return", "if", "else", "for", "while", "loop", "match", "struct", "enum", "impl", "trait", "true", "false", "pub", "use", "mod", "async", "await"}
	rubyKeywords    = []string{"def", "end", "return", "if", "elsif", "else", "for", "while", "class", "module", "true", "false", "nil", "do", "yield", "begin", "rescue", "raise"}
	cFamilyKeywords = []string{"int", "return", "if", "else", "for", "while", "class", "public", "private", "static", "void", "true", "false", "null", "new", "this", "include", "define", "struct", "typedef"}
)

func highlightKeywords(code string, keywords []string) string {
	// Tokenize roughly by scanning
	var b strings.Builder
	i := 0
	for i < len(code) {
		ch := code[i]
		// whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			b.WriteByte(ch)
			i++
			continue
		}
		// line comment // or #
		if ch == '#' || (ch == '/' && i+1 < len(code) && code[i+1] == '/') {
			j := i
			for j < len(code) && code[j] != '\n' {
				j++
			}
			b.WriteString(dim + gray)
			b.WriteString(code[i:j])
			b.WriteString(reset)
			i = j
			continue
		}
		// string
		if ch == '"' || ch == '\'' {
			quote := ch
			j := i + 1
			for j < len(code) {
				if code[j] == '\\' && j+1 < len(code) {
					j += 2
					continue
				}
				if code[j] == quote {
					j++
					break
				}
				j++
			}
			b.WriteString(green)
			b.WriteString(code[i:j])
			b.WriteString(reset)
			i = j
			continue
		}
		// number
		if ch >= '0' && ch <= '9' {
			j := i
			for j < len(code) && ((code[j] >= '0' && code[j] <= '9') || code[j] == '.') {
				j++
			}
			b.WriteString(cyan)
			b.WriteString(code[i:j])
			b.WriteString(reset)
			i = j
			continue
		}
		// ident / keyword
		if isIdentStart(ch) {
			j := i
			for j < len(code) && isIdentPart(code[j]) {
				j++
			}
			word := code[i:j]
			if contains(keywords, word) {
				b.WriteString(bold + magenta)
				b.WriteString(word)
				b.WriteString(reset)
			} else {
				b.WriteString(word)
			}
			i = j
			continue
		}
		// operator-ish
		if strings.ContainsRune("=+-*/<>!&|%^?:", rune(ch)) {
			b.WriteString(yellow)
			b.WriteByte(ch)
			b.WriteString(reset)
			i++
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}
func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}
func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Strip removes ANSI escape sequences.
func Strip(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
