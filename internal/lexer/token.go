package lexer

// TokenType identifies the type of a lexical token.
type TokenType string

const (
	// Special
	ILLEGAL TokenType = "ILLEGAL"
	EOF     TokenType = "EOF"

	// Identifiers & literals
	IDENT  TokenType = "IDENT"
	INT    TokenType = "INT"
	FLOAT  TokenType = "FLOAT"
	STRING TokenType = "STRING"

	// Operators
	ASSIGN       TokenType = "="
	PLUS_ASSIGN  TokenType = "+="
	MINUS_ASSIGN TokenType = "-="
	STAR_ASSIGN  TokenType = "*="
	SLASH_ASSIGN TokenType = "/="
	PLUS         TokenType = "+"
	MINUS        TokenType = "-"
	BANG         TokenType = "!"
	ASTERISK     TokenType = "*"
	SLASH        TokenType = "/"
	MOD          TokenType = "%"
	LT           TokenType = "<"
	GT           TokenType = ">"
	EQ           TokenType = "=="
	NOT_EQ       TokenType = "!="
	LTE          TokenType = "<="
	GTE          TokenType = ">="
	AND          TokenType = "&&"
	OR           TokenType = "||"
	BIT_AND      TokenType = "&"
	BIT_OR       TokenType = "|"
	BIT_XOR      TokenType = "^"
	SHL          TokenType = "<<"
	SHR          TokenType = ">>"

	// Delimiters
	COMMA          TokenType = ","
	SEMICOLON      TokenType = ";"
	COLON          TokenType = ":"
	QUESTION       TokenType = "?"
	NULL_COAL      TokenType = "??"
	OPTIONAL_CHAIN TokenType = "?."
	PIPE           TokenType = "|>"
	LPAREN         TokenType = "("
	RPAREN         TokenType = ")"
	LBRACE         TokenType = "{"
	RBRACE         TokenType = "}"
	LBRACKET       TokenType = "["
	RBRACKET       TokenType = "]"
	DOT            TokenType = "."
	AT             TokenType = "@"

	// Keywords
	FUNCTION TokenType = "fn"
	LET      TokenType = "let"
	CONST    TokenType = "const"
	TRUE     TokenType = "true"
	FALSE    TokenType = "false"
	IF       TokenType = "if"
	ELSE     TokenType = "else"
	RETURN   TokenType = "return"
	WHILE    TokenType = "while"
	FOR      TokenType = "for"
	IMPORT   TokenType = "import"
	PRINT    TokenType = "print"
	NULL     TokenType = "null"
	BREAK    TokenType = "break"
	CONTINUE TokenType = "continue"
	IN       TokenType = "in"
	CLASS    TokenType = "class"
	NEW      TokenType = "new"
	THIS     TokenType = "this"
	EXTENDS  TokenType = "extends"
	TRY      TokenType = "try"
	CATCH    TokenType = "catch"
	THROW    TokenType = "throw"
	MATCH    TokenType = "match"
	CASE     TokenType = "case"
	DEFAULT  TokenType = "default"
	YIELD    TokenType = "yield"
	ENUM     TokenType = "enum"
	RECORD   TokenType = "record"
	DEFER    TokenType = "defer"
	FINALLY  TokenType = "finally"
	ELLIPSIS TokenType = "..."
	// Wave 5: `interface Name { method(): type }` declarations.
	INTERFACE TokenType = "interface"
)

// Token represents a single lexical token.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

var keywords = map[string]TokenType{
	"fn":        FUNCTION,
	"let":       LET,
	"const":     CONST,
	"true":      TRUE,
	"false":     FALSE,
	"if":        IF,
	"else":      ELSE,
	"return":    RETURN,
	"while":     WHILE,
	"for":       FOR,
	"import":    IMPORT,
	"print":     PRINT,
	"null":      NULL,
	"break":     BREAK,
	"continue":  CONTINUE,
	"in":        IN,
	"class":     CLASS,
	"new":       NEW,
	"this":      THIS,
	"extends":   EXTENDS,
	"try":       TRY,
	"catch":     CATCH,
	"throw":     THROW,
	"match":     MATCH,
	"case":      CASE,
	"default":   DEFAULT,
	"yield":     YIELD,
	"enum":      ENUM,
	"record":    RECORD,
	"defer":     DEFER,
	"finally":   FINALLY,
	"interface": INTERFACE,
	"switch":    MATCH, // alias for match
	"and":       AND,
	"or":        OR,
}

// LookupIdent returns the token type for an identifier (keyword or IDENT).
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
