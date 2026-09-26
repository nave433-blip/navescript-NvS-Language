package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer tokenizes Navescript source code.
type Lexer struct {
	input        string
	position     int  // current position (byte index)
	readPosition int  // next read position
	ch           rune // current character
	line         int
	column       int
}

// New creates a new Lexer for the given input.
func New(input string) *Lexer {
	// Wave 12: strip a Unix shebang line (#!/usr/bin/env nvs) so scripts
	// can be chmod +x'd and executed directly. The lexer never sees it.
	if strings.HasPrefix(input, "#!") {
		if i := strings.IndexByte(input, '\n'); i >= 0 {
			input = input[i+1:]
		} else {
			input = ""
		}
	}
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
		l.position = l.readPosition
	} else {
		r, size := utf8.DecodeRuneInString(l.input[l.readPosition:])
		l.ch = r
		l.position = l.readPosition
		l.readPosition += size
	}
	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition:])
	return r
}

// peekChar2 returns the character after peekChar (two runes ahead of current).
func (l *Lexer) peekChar2() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	_, size := utf8.DecodeRuneInString(l.input[l.readPosition:])
	if l.readPosition+size >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition+size:])
	return r
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	if l.ch == '/' && l.peekChar() == '/' {
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	}
	if l.ch == '/' && l.peekChar() == '*' {
		l.readChar()
		l.readChar()
		for {
			if l.ch == 0 {
				break
			}
			if l.ch == '*' && l.peekChar() == '/' {
				l.readChar()
				l.readChar()
				break
			}
			l.readChar()
		}
	}
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()
	for l.ch == '/' && (l.peekChar() == '/' || l.peekChar() == '*') {
		l.skipComment()
		l.skipWhitespace()
	}

	tok := Token{Line: l.line, Column: l.column}

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = EQ
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = ASSIGN
			tok.Literal = string(l.ch)
		}
	case '+':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = PLUS_ASSIGN
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = PLUS
			tok.Literal = string(l.ch)
		}
	case '-':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = MINUS_ASSIGN
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = MINUS
			tok.Literal = string(l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = NOT_EQ
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = BANG
			tok.Literal = string(l.ch)
		}
	case '*':
		if l.peekChar() == '*' {
			ch := l.ch
			l.readChar()
			tok.Type = POWER
			tok.Literal = string(ch) + string(l.ch)
		} else if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = STAR_ASSIGN
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = ASTERISK
			tok.Literal = string(l.ch)
		}
	case '/':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = SLASH_ASSIGN
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = SLASH
			tok.Literal = string(l.ch)
		}
	case '%':
		tok.Type = MOD
		tok.Literal = string(l.ch)
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = LTE
			tok.Literal = string(ch) + string(l.ch)
		} else if l.peekChar() == '<' {
			ch := l.ch
			l.readChar()
			tok.Type = SHL
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = LT
			tok.Literal = string(l.ch)
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = GTE
			tok.Literal = string(ch) + string(l.ch)
		} else if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			tok.Type = SHR
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = GT
			tok.Literal = string(l.ch)
		}
	case '&':
		if l.peekChar() == '&' {
			ch := l.ch
			l.readChar()
			tok.Type = AND
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = BIT_AND
			tok.Literal = string(l.ch)
		}
	case '|':
		if l.peekChar() == '|' {
			ch := l.ch
			l.readChar()
			tok.Type = OR
			tok.Literal = string(ch) + string(l.ch)
		} else if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			tok.Type = PIPE
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = BIT_OR
			tok.Literal = string(l.ch)
		}
	case '^':
		tok.Type = BIT_XOR
		tok.Literal = string(l.ch)
	case ',':
		tok.Type = COMMA
		tok.Literal = string(l.ch)
	case ';':
		tok.Type = SEMICOLON
		tok.Literal = string(l.ch)
	case ':':
		tok.Type = COLON
		tok.Literal = string(l.ch)
	case '?':
		if l.peekChar() == '?' {
			ch := l.ch
			l.readChar()
			tok.Type = NULL_COAL
			tok.Literal = string(ch) + string(l.ch)
		} else if l.peekChar() == '.' && !isDigit(l.peekChar2()) {
			// `?.` optional chaining. The digit guard keeps `a?.5:b`
			// lexing as `?` `.` `5` exactly like before.
			l.readChar()
			tok.Type = OPTIONAL_CHAIN
			tok.Literal = "?."
		} else {
			tok.Type = QUESTION
			tok.Literal = string(l.ch)
		}
	case '(':
		tok.Type = LPAREN
		tok.Literal = string(l.ch)
	case ')':
		tok.Type = RPAREN
		tok.Literal = string(l.ch)
	case '{':
		tok.Type = LBRACE
		tok.Literal = string(l.ch)
	case '}':
		tok.Type = RBRACE
		tok.Literal = string(l.ch)
	case '[':
		tok.Type = LBRACKET
		tok.Literal = string(l.ch)
	case ']':
		tok.Type = RBRACKET
		tok.Literal = string(l.ch)
	case '.':
		if l.peekChar() == '.' && l.peekChar2() == '.' {
			l.readChar()
			l.readChar()
			tok.Type = ELLIPSIS
			tok.Literal = "..."
		} else {
			// `..` lexes as two DOT tokens (ranges like 1..10); the parser
			// distinguishes member access from ranges. Only the first dot
			// is consumed here; the second lexes on the next call.
			tok.Type = DOT
			tok.Literal = "."
		}
	case '@':
		tok.Type = AT
		tok.Literal = string(l.ch)
	case '"', '\'':
		tok.Type = STRING
		tok.Literal = l.readString(l.ch)
		return tok
	case 0:
		tok.Type = EOF
		tok.Literal = ""
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			return l.readNumber()
		} else {
			tok.Type = ILLEGAL
			tok.Literal = string(l.ch)
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) readIdentifier() string {
	start := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.position]
}

func (l *Lexer) readNumber() Token {
	start := l.position
	line, col := l.line, l.column
	isFloat := false

	// 0x hex / 0b binary
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X' || l.peekChar() == 'b' || l.peekChar() == 'B') {
		l.readChar() // 0
		l.readChar() // x or b
		for isDigit(l.ch) || (l.ch >= 'a' && l.ch <= 'f') || (l.ch >= 'A' && l.ch <= 'F') {
			l.readChar()
		}
		lit := l.input[start:l.position]
		return Token{Type: INT, Literal: lit, Line: line, Column: col}
	}

	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peekChar()) {
		isFloat = true
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}

	lit := l.input[start:l.position]
	if isFloat {
		return Token{Type: FLOAT, Literal: lit, Line: line, Column: col}
	}
	return Token{Type: INT, Literal: lit, Line: line, Column: col}
}

func (l *Lexer) readString(quote rune) string {
	l.readChar() // consume opening quote
	var b []rune
	interpDepth := 0 // >0 while inside a ${...} interpolation
	for l.ch != 0 {
		if interpDepth == 0 && l.ch == quote {
			break
		}
		// Escapes are processed at the top level only; inside ${...} the
		// text is kept raw so the interpolation sub-parser sees faithful source.
		if l.ch == '\\' && interpDepth == 0 {
			l.readChar()
			switch l.ch {
			case 'n':
				b = append(b, '\n')
			case 't':
				b = append(b, '\t')
			case 'r':
				b = append(b, '\r')
			case '\\':
				b = append(b, '\\')
			case '"':
				b = append(b, '"')
			case '\'':
				b = append(b, '\'')
			default:
				b = append(b, l.ch)
			}
			l.readChar()
			continue
		}
		// `${` suspends the closing quote until braces balance.
		if l.ch == '$' && l.peekChar() == '{' {
			interpDepth++
			b = append(b, '$', '{')
			l.readChar()
			l.readChar()
			continue
		}
		if interpDepth > 0 {
			switch l.ch {
			case '{':
				interpDepth++
			case '}':
				interpDepth--
			case '"', '\'':
				// Nested string inside interpolation: consume it raw
				// (escapes kept) so its quotes/braces don't confuse
				// depth tracking and the sub-parser sees real source.
				q := l.ch
				b = append(b, q)
				l.readChar()
				for l.ch != 0 && l.ch != q {
					if l.ch == '\\' {
						b = append(b, l.ch)
						l.readChar()
						if l.ch != 0 {
							b = append(b, l.ch)
							l.readChar()
						}
						continue
					}
					b = append(b, l.ch)
					l.readChar()
				}
				if l.ch == q {
					b = append(b, q)
					l.readChar()
				}
				continue
			}
		}
		b = append(b, l.ch)
		l.readChar()
	}
	if l.ch == quote {
		l.readChar() // consume closing quote
	}
	return string(b)
}

func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}
