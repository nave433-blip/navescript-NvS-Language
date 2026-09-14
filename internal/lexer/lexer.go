package lexer

import (
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
		if l.peekChar() == '=' {
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
		tok.Type = DOT
		tok.Literal = string(l.ch)
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
	for l.ch != quote && l.ch != 0 {
		if l.ch == '\\' {
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
		} else {
			b = append(b, l.ch)
			l.readChar()
		}
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
